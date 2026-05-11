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

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/grafana/cisco-aichat-app/pkg/cache"
	"github.com/grafana/cisco-aichat-app/pkg/metrics"
	"github.com/grafana/cisco-aichat-app/pkg/storage"
	"github.com/grafana/cisco-aichat-app/pkg/tokens"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
	"golang.org/x/time/rate"
)

// Make sure App implements required interfaces. This is important to do
// since otherwise we will only get a not implemented error response from plugin in
// runtime. Plugin should not implement all these interfaces - only those which are
// required for a particular task.
var (
	_ backend.CallResourceHandler   = (*App)(nil)
	_ instancemgmt.InstanceDisposer = (*App)(nil)
	_ backend.CheckHealthHandler    = (*App)(nil)
)

// App is the AI Chat Assistant backend plugin which handles chat history persistence.
type App struct {
	backend.CallResourceHandler
	appSettings  *backend.AppInstanceSettings
	storage      storage.Storage
	tokenService *storage.TokenService
	// Auto-compaction threshold percentage (100 = compact when context window is full)
	autoCompactThreshold float64
	// Number of source tokens targeted per compaction pass
	compactionBatchTokens int
	cleanup               *storage.CleanupScheduler
	// Phase 15: Message caching for performance (PERF-01, PERF-04)
	messageCache *cache.MessageCache
	// Phase 15: Session pre-loading for cache warming (PERF-03)
	preloader *cache.SessionPreloader
	// Security: Rate limiter to prevent DoS attacks
	rateLimiters map[string]*rate.Limiter
	limiterMu    sync.RWMutex
}

// validateDataDir validates that the data directory is safe to use
// Security: Prevents path traversal attacks via configuration
func validateDataDir(dataDir string) error {
	// Whitelist of allowed base directories
	allowedPrefixes := []string{
		"/var/lib/grafana/plugins/cisco-aichat-app",
		"/var/lib/grafana/grafana-aichat-data",
		"/tmp/cisco-aichat-app",
		"/data/grafana/plugins/cisco-aichat-app",
	}

	// Clean the path to resolve any .. or symlinks
	cleanPath, err := filepath.Abs(filepath.Clean(dataDir))
	if err != nil {
		return fmt.Errorf("invalid data directory path: %w", err)
	}

	// Check if path starts with any allowed prefix
	allowed := false
	for _, prefix := range allowedPrefixes {
		absPrefix, err := filepath.Abs(prefix)
		if err != nil {
			continue
		}
		if filepath.HasPrefix(cleanPath, absPrefix) {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("data directory must be within allowed paths: %v", allowedPrefixes)
	}

	return nil
}

// getRateLimiter gets or creates a rate limiter for a user
// Security: Per-user rate limiting to prevent DoS attacks
func (a *App) getRateLimiter(userID string) *rate.Limiter {
	a.limiterMu.Lock()
	defer a.limiterMu.Unlock()

	if limiter, exists := a.rateLimiters[userID]; exists {
		return limiter
	}

	// Create new limiter: 50 requests per second, burst of 100
	// Note: Increased from 10/20 to support normal frontend usage patterns
	// (page loads, session switches, message sends require multiple rapid requests)
	limiter := rate.NewLimiter(50, 100)
	a.rateLimiters[userID] = limiter
	return limiter
}

// checkRateLimit checks if a user has exceeded their rate limit
// Security: Returns true if request should be allowed
func (a *App) checkRateLimit(userID string) bool {
	limiter := a.getRateLimiter(userID)
	return limiter.Allow()
}

// NewApp creates a new App instance.
func NewApp(ctx context.Context, settings backend.AppInstanceSettings) (instancemgmt.Instance, error) {
	var app App
	app.appSettings = &settings
	app.rateLimiters = make(map[string]*rate.Limiter)

	// Initialize metrics early so they are available for error tracking
	metrics.Initialize("aichat")

	// Initialize storage
	// Parse JSONData to get custom data directory if provided
	var jsonData map[string]interface{}
	// Use Grafana's persistent data directory instead of plugin directory
	dataDir := "/var/lib/grafana/grafana-aichat-data"
	if len(settings.JSONData) > 0 {
		if err := json.Unmarshal(settings.JSONData, &jsonData); err == nil {
			if customDir, ok := jsonData["dataDir"].(string); ok && customDir != "" {
				// Security: Validate custom data directory
				if err := validateDataDir(customDir); err != nil {
					log.DefaultLogger.Error("Invalid data directory configuration", "error", err)
					return nil, fmt.Errorf("invalid data directory: %w", err)
				}
				dataDir = customDir
			}
		}
	}

	// Security: Validate default data directory too
	if err := validateDataDir(dataDir); err != nil {
		log.DefaultLogger.Error("Invalid default data directory", "error", err)
		return nil, fmt.Errorf("invalid default data directory: %w", err)
	}

	// Create storage using factory (supports database or file-based storage)
	store, err := storage.NewStorage(dataDir, log.DefaultLogger)
	if err != nil {
		log.DefaultLogger.Error("Failed to initialize storage", "error", err)
		return nil, err
	}
	app.storage = store

	// Start cleanup scheduler for session retention
	retentionDays := storage.DefaultRetentionDays
	if customRetention, ok := jsonData["retentionDays"].(float64); ok && customRetention > 0 {
		retentionDays = int(customRetention)
	}
	app.cleanup = storage.NewCleanupScheduler(store, retentionDays, log.DefaultLogger)
	app.cleanup.Start()

	// Initialize token counting infrastructure
	openaiCounter, err := tokens.NewOpenAICounter()
	if err != nil {
		// OpenAI counter initialization failed - log and continue with estimation only
		log.DefaultLogger.Warn("Failed to initialize OpenAI token counter, using estimation fallback", "error", err)
		openaiCounter = nil
	}

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	anthropicCounter := tokens.NewAnthropicCounter(anthropicKey)
	estimator := tokens.NewEstimationCounter()

	// Build counter list (excluding nil counters)
	var counters []tokens.TokenCounter
	if openaiCounter != nil {
		counters = append(counters, openaiCounter)
	}
	counters = append(counters, anthropicCounter)

	registry := tokens.NewRegistry(counters, estimator)

	app.tokenService = storage.NewTokenService(
		store,
		registry,
		storage.WithLogger(log.DefaultLogger),
	)

	// Compaction defaults
	app.autoCompactThreshold = 100
	app.compactionBatchTokens = 10000

	// Phase 15: Initialize message cache (PERF-01, PERF-04)
	// 64MB max memory, 15 minute TTL
	messageCache, err := cache.NewMessageCacheWithSize(64*1024*1024, 15*time.Minute)
	if err != nil {
		// Cache creation failed - log warning but continue without cache (graceful degradation)
		log.DefaultLogger.Warn("Failed to initialize message cache, running without caching", "error", err)
	} else {
		app.messageCache = messageCache
		log.DefaultLogger.Info("Message cache initialized", "maxMemory", "64MB", "ttl", "15m")
	}

	// Phase 15: Session pre-loading for cache warming (PERF-03)
	if app.messageCache != nil {
		app.preloader = cache.NewSessionPreloader(
			app.storage,
			app.messageCache,
			cache.WithMaxSessions(10),
			cache.WithRefreshInterval(5*time.Minute),
			cache.WithPreloaderLogger(log.DefaultLogger),
		)
		app.preloader.Start()
		log.DefaultLogger.Info("Session preloader started", "maxSessions", 10, "refreshInterval", "5m")
	}

	log.DefaultLogger.Info("Chat storage initialized", "dataDir", dataDir, "retentionDays", retentionDays)

	// Use a httpadapter (provided by the SDK) for resource calls. This allows us
	// to use a *http.ServeMux for resource calls, so we can map multiple routes
	// to CallResource without having to implement extra logic.
	mux := http.NewServeMux()
	app.registerRoutes(mux)
	app.CallResourceHandler = httpadapter.New(mux)

	return &app, nil
}

// Dispose here tells plugin SDK that plugin wants to clean up resources when a new instance
// created.
func (a *App) Dispose() {
	if a.cleanup != nil {
		a.cleanup.Stop()
	}
	// Stop token service background worker before closing storage
	if a.tokenService != nil {
		a.tokenService.Close()
	}
	// Stop session preloader before closing cache
	if a.preloader != nil {
		a.preloader.Stop()
	}
	// Close message cache
	if a.messageCache != nil {
		a.messageCache.Close()
	}
	if a.storage != nil {
		if err := a.storage.Close(); err != nil {
			log.DefaultLogger.Error("Failed to close storage", "error", err)
		}
	}
}

// CheckHealth handles health checks sent from Grafana to the plugin.
func (a *App) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	// Check degraded mode first: resilient storage can mask Ping errors while degraded.
	if rs, ok := a.storage.(*storage.ResilientStorage); ok && rs.IsDegraded() {
		metrics.SetPluginUp(false)
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "AI Chat running in degraded mode (history may not persist)",
		}, nil
	}

	// Check storage health using Ping
	if err := a.storage.Ping(ctx); err != nil {
		metrics.SetPluginUp(false)
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Storage check failed: " + err.Error(),
		}, nil
	}

	metrics.SetPluginUp(true)
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "AI Chat Assistant backend is healthy",
	}, nil
}
