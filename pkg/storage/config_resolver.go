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
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// Environment variable constants for database configuration
const (
	// Plugin-specific override (highest priority)
	AIChatDatabaseURL = "AICHAT_DATABASE_URL"
	// Opt-in memory fallback for runtime DB errors (disabled by default for consistency)
	AIChatEnableRuntimeMemoryFallback = "AICHAT_ENABLE_RUNTIME_MEMORY_FALLBACK"

	// Grafana database environment variables
	GFDatabaseURL      = "GF_DATABASE_URL"
	GFDatabaseType     = "GF_DATABASE_TYPE"
	GFDatabaseHost     = "GF_DATABASE_HOST"
	GFDatabaseName     = "GF_DATABASE_NAME"
	GFDatabaseUser     = "GF_DATABASE_USER"
	GFDatabasePassword = "GF_DATABASE_PASSWORD"
	GFDatabaseSSLMode  = "GF_DATABASE_SSL_MODE"
	GFDatabasePath     = "GF_DATABASE_PATH"
	GFPathsConfig      = "GF_PATHS_CONFIG"
	GFPathsData        = "GF_PATHS_DATA"
)

// ResolvedConfig contains the resolved storage configuration
type ResolvedConfig struct {
	// Source indicates where the configuration came from:
	// "explicit" - AICHAT_DATABASE_URL was set
	// "grafana" - GF_DATABASE_* environment variables
	// "file" - File storage with writable data directory
	// "memory" - In-memory storage (fallback)
	Source string

	// DBURL is the database connection URL (empty for file/memory)
	DBURL string

	// DataDir is the data directory for file storage
	DataDir string

	// Writable indicates whether the data directory is writable
	Writable bool

	// EnableRuntimeFallback controls whether DB mode uses in-memory runtime fallback.
	// Disabled by default to avoid split-brain behavior in multi-instance deployments.
	EnableRuntimeFallback bool
}

// ConfigResolver detects and resolves storage configuration from environment
type ConfigResolver struct {
	logger log.Logger
}

// NewConfigResolver creates a new ConfigResolver instance
func NewConfigResolver(logger log.Logger) *ConfigResolver {
	return &ConfigResolver{
		logger: logger,
	}
}

// Resolve determines the appropriate storage configuration based on environment
// detection and filesystem checks.
//
// Priority order:
// 1. AICHAT_DATABASE_URL (explicit plugin override)
// 2. GF_DATABASE_URL (Grafana's unified database URL)
// 3. GF_DATABASE_* components (type, host, name, user, password)
// 4. Grafana config file database settings (grafana.ini)
// 5. File storage (if dataDir is writable)
// 6. Memory storage (fallback for read-only filesystems)
func (r *ConfigResolver) Resolve(dataDir string) ResolvedConfig {
	enableRuntimeFallback := parseBoolEnv(AIChatEnableRuntimeMemoryFallback, false)

	// Priority 1: Check explicit plugin configuration
	if dbURL := os.Getenv(AIChatDatabaseURL); dbURL != "" {
		r.logger.Info("Database configured via AICHAT_DATABASE_URL")
		return ResolvedConfig{
			Source:                "explicit",
			DBURL:                 dbURL,
			DataDir:               dataDir,
			EnableRuntimeFallback: enableRuntimeFallback,
		}
	}

	// Priority 2: Check Grafana's unified database URL
	if dbURL := os.Getenv(GFDatabaseURL); dbURL != "" {
		r.logger.Info("Using Grafana database from GF_DATABASE_URL")
		return ResolvedConfig{
			Source:                "grafana",
			DBURL:                 dbURL,
			DataDir:               dataDir,
			EnableRuntimeFallback: enableRuntimeFallback,
		}
	}

	// Priority 3: Check Grafana database components and build URL
	if dbType := os.Getenv(GFDatabaseType); dbType != "" {
		dbURL := r.buildURLFromComponents(dbType)
		if dbURL != "" {
			r.logger.Info("Using Grafana database from GF_DATABASE_* variables", "type", dbType)
			return ResolvedConfig{
				Source:                "grafana",
				DBURL:                 dbURL,
				DataDir:               dataDir,
				EnableRuntimeFallback: enableRuntimeFallback,
			}
		}
	}

	// Priority 4: Check Grafana config file ([database] in grafana.ini)
	if dbURL, configPath, ok := r.resolveFromGrafanaConfigFile(); ok {
		r.logger.Info("Using Grafana database from config file", "path", configPath)
		return ResolvedConfig{
			Source:                "grafana",
			DBURL:                 dbURL,
			DataDir:               dataDir,
			EnableRuntimeFallback: enableRuntimeFallback,
		}
	}

	// Priority 5: Check if dataDir is writable for file storage
	writable := isWritable(dataDir)
	if writable {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			r.logger.Warn("Using local file storage in Kubernetes may cause inconsistent session state across replicas; configure Grafana database settings for shared storage")
		}
		r.logger.Info("Using file storage", "dataDir", dataDir)
		return ResolvedConfig{
			Source:                "file",
			DataDir:               dataDir,
			Writable:              true,
			EnableRuntimeFallback: enableRuntimeFallback,
		}
	}

	// Priority 6: Memory storage fallback
	r.logger.Warn("No persistent storage available, using in-memory storage (sessions will not persist)")
	return ResolvedConfig{
		Source:                "memory",
		DataDir:               dataDir,
		Writable:              false,
		EnableRuntimeFallback: enableRuntimeFallback,
	}
}

// buildURLFromComponents constructs a database URL from individual environment variables
func (r *ConfigResolver) buildURLFromComponents(dbType string) string {
	host := os.Getenv(GFDatabaseHost)
	name := os.Getenv(GFDatabaseName)
	user := os.Getenv(GFDatabaseUser)
	password := os.Getenv(GFDatabasePassword)
	sslMode := os.Getenv(GFDatabaseSSLMode)
	dbPath := os.Getenv(GFDatabasePath)

	return r.buildURLFromValues(dbType, host, name, user, password, sslMode, dbPath)
}

func (r *ConfigResolver) buildURLFromValues(dbType, host, name, user, password, sslMode, dbPath string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return r.buildPostgresURL(host, name, user, password, sslMode)
	case "mysql":
		return r.buildMySQLURL(host, name, user, password)
	case "sqlite3":
		if dbPath == "" {
			r.logger.Warn("Database type is sqlite3 but path is not set")
			return ""
		}
		return "file:" + dbPath
	default:
		r.logger.Warn("Unknown or unsupported database type", "type", dbType)
		return ""
	}
}

// buildPostgresURL constructs a PostgreSQL connection URL
// Uses url.UserPassword for proper credential encoding
func (r *ConfigResolver) buildPostgresURL(host, name, user, password, sslMode string) string {
	if host == "" {
		r.logger.Warn("GF_DATABASE_HOST is required for PostgreSQL")
		return ""
	}

	// Default database name to "grafana" if not specified
	if name == "" {
		name = "grafana"
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   host,
		Path:   "/" + name,
	}

	// Set user credentials with proper URL encoding
	if user != "" {
		if password != "" {
			u.User = url.UserPassword(user, password)
		} else {
			u.User = url.User(user)
		}
	}

	// Add SSL mode if specified
	if sslMode != "" {
		q := u.Query()
		q.Set("sslmode", sslMode)
		u.RawQuery = q.Encode()
	}

	return u.String()
}

// buildMySQLURL constructs a MySQL connection URL in DSN format
func (r *ConfigResolver) buildMySQLURL(host, name, user, password string) string {
	if host == "" {
		r.logger.Warn("GF_DATABASE_HOST is required for MySQL")
		return ""
	}

	// Default database name to "grafana" if not specified
	if name == "" {
		name = "grafana"
	}

	// MySQL DSN format: user:password@tcp(host:port)/dbname
	if user != "" {
		if password != "" {
			return fmt.Sprintf("%s:%s@tcp(%s)/%s", user, password, host, name)
		}
		return fmt.Sprintf("%s@tcp(%s)/%s", user, host, name)
	}

	return fmt.Sprintf("tcp(%s)/%s", host, name)
}

// isWritable tests whether a directory is writable by creating and removing a test file
// This is more reliable than checking permissions, as it handles read-only mounts
func isWritable(dir string) bool {
	// Ensure the directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Try to create the directory
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false
		}
	}

	// Create a unique test file name
	testFile := filepath.Join(dir, ".write-test-"+strconv.FormatInt(time.Now().UnixNano(), 36))

	// Attempt to create the test file
	f, err := os.Create(testFile)
	if err != nil {
		return false
	}

	// Close and remove the test file
	f.Close()
	os.Remove(testFile)

	return true
}

func parseBoolEnv(name string, defaultValue bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func (r *ConfigResolver) resolveFromGrafanaConfigFile() (string, string, bool) {
	configPath := strings.TrimSpace(os.Getenv(GFPathsConfig))
	if configPath == "" {
		configPath = "/etc/grafana/grafana.ini"
	}

	sections, err := parseINISections(configPath)
	if err != nil {
		if os.Getenv(GFPathsConfig) != "" && !os.IsNotExist(err) {
			r.logger.Warn("Failed to read Grafana config file for database discovery", "path", configPath, "error", err)
		}
		return "", configPath, false
	}

	database, ok := sections["database"]
	if !ok {
		return "", configPath, false
	}

	if rawURL := strings.TrimSpace(database["url"]); rawURL != "" {
		return rawURL, configPath, true
	}

	dbType := strings.TrimSpace(database["type"])
	if dbType == "" {
		return "", configPath, false
	}

	host := strings.TrimSpace(database["host"])
	name := strings.TrimSpace(database["name"])
	user := strings.TrimSpace(database["user"])
	password := strings.TrimSpace(database["password"])
	sslMode := strings.TrimSpace(database["ssl_mode"])
	dbPath := strings.TrimSpace(database["path"])

	if strings.EqualFold(dbType, "sqlite3") && dbPath != "" && !filepath.IsAbs(dbPath) {
		dataPath := strings.TrimSpace(os.Getenv(GFPathsData))
		if dataPath == "" {
			if paths, ok := sections["paths"]; ok {
				dataPath = strings.TrimSpace(paths["data"])
			}
		}
		if dataPath == "" {
			dataPath = "/var/lib/grafana"
		}
		dbPath = filepath.Join(dataPath, dbPath)
	}

	dbURL := r.buildURLFromValues(dbType, host, name, user, password, sslMode, dbPath)
	if dbURL == "" {
		return "", configPath, false
	}

	return dbURL, configPath, true
}

func parseINISections(path string) (map[string]map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	sections := map[string]map[string]string{
		"default": {},
	}
	current := "default"

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if end := strings.Index(line, "]"); end > 1 {
				current = strings.ToLower(strings.TrimSpace(line[1:end]))
				if _, exists := sections[current]; !exists {
					sections[current] = map[string]string{}
				}
			}
			continue
		}

		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		if key == "" {
			continue
		}
		value := strings.TrimSpace(stripInlineComment(line[eq+1:]))
		value = strings.Trim(value, `"'`)
		value = strings.TrimSpace(os.ExpandEnv(value))

		sections[current][key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return sections, nil
}

func stripInlineComment(value string) string {
	inSingleQuote := false
	inDoubleQuote := false

	for i, r := range value {
		switch r {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case ';', '#':
			if inSingleQuote || inDoubleQuote {
				continue
			}
			if i == 0 {
				return ""
			}
			prev := value[i-1]
			if prev == ' ' || prev == '\t' {
				return strings.TrimSpace(value[:i])
			}
		}
	}

	return value
}
