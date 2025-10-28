// Copyright 2025 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"path/filepath"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// NewStorage creates a Storage instance based on environment configuration
// Priority:
// 1. AICHAT_DATABASE_URL environment variable (explicit override)
// 2. GF_DATABASE_URL (Grafana's unified database URL)
// 3. GF_DATABASE_* components (type, host, name, user, password)
// 4. grafana.ini [database] settings
// 5. File storage if dataDir is writable
// 6. Memory storage fallback (Kubernetes read-only filesystem)
func NewStorage(dataDir string, logger log.Logger) (Storage, error) {
	resolver := NewConfigResolver(logger)
	config := resolver.Resolve(dataDir)

	switch config.Source {
	case "explicit", "grafana":
		// Database storage (explicit AICHAT_DATABASE_URL or Grafana's database)
		return newDatabaseStorage(config.DBURL, dataDir, logger, config.EnableRuntimeFallback)
	case "file":
		// File storage (dataDir is writable)
		logger.Info("Using file storage", "dataDir", dataDir)
		return NewFileStorage(filepath.Join(dataDir, "chat-history"))
	case "memory":
		// Memory storage fallback (read-only filesystem)
		logger.Warn("Using in-memory storage - sessions will not persist across restarts")
		return NewMemoryStorage(), nil
	default:
		// Should not reach here, but handle gracefully
		logger.Warn("Unknown config source, falling back to memory storage", "source", config.Source)
		return NewMemoryStorage(), nil
	}
}

// NewStorageFromURL creates a Storage instance from an explicit database URL
// This is useful for testing or when the URL is known programmatically
func NewStorageFromURL(dbURL string, dataDir string, logger log.Logger) (Storage, error) {
	enableFallback := parseBoolEnv(AIChatEnableRuntimeMemoryFallback, false)
	return newDatabaseStorage(dbURL, dataDir, logger, enableFallback)
}

// newDatabaseStorage creates a database-backed storage with resilient fallback
func newDatabaseStorage(dbURL, dataDir string, logger log.Logger, enableRuntimeFallback bool) (Storage, error) {
	// Create primary database storage
	primary, err := NewDBStorage(dbURL, logger)
	if err != nil {
		logger.Error("Database storage initialization failed",
			"error", err,
			"dbURL", maskSensitiveURL(dbURL))
		return nil, err
	}

	if !enableRuntimeFallback {
		logger.Info("Database storage initialized in strict consistency mode (runtime memory fallback disabled)")
		return primary, nil
	}

	// Create in-memory fallback for runtime resilience
	fallback := NewMemoryStorage()

	// Wrap in resilient storage for graceful degradation
	logger.Warn("Database storage initialized with resilient fallback; this mode may produce inconsistent sessions in multi-instance deployments")
	return NewResilientStorage(primary, fallback, logger), nil
}

// maskSensitiveURL masks password in database URL for logging
func maskSensitiveURL(dbURL string) string {
	// Simple masking - replace password portion in postgres:// URLs
	// For production, consider using a proper URL parser
	if len(dbURL) > 12 {
		// Return only the scheme and a masked indicator
		for i, c := range dbURL {
			if c == ':' && i > 0 {
				scheme := dbURL[:i]
				return scheme + "://***masked***"
			}
		}
	}
	return "***masked***"
}
