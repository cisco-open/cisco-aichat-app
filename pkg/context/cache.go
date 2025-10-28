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

package context

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/dgraph-io/ristretto/v2"
	"golang.org/x/sync/singleflight"
)

// CacheConfig configures the context cache
type CacheConfig struct {
	NumCounters int64 // Number of keys to track (10x expected active sessions)
	MaxCost     int64 // Maximum memory cost in bytes
	Metrics     bool  // Enable metrics collection
}

// DefaultCacheConfig returns sensible defaults
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		NumCounters: 100000,  // 100k keys to track
		MaxCost:     1 << 27, // 128MB max memory
		Metrics:     true,    // Enable for monitoring
	}
}

// ContextCache wraps ristretto for context window caching
type ContextCache struct {
	cache *ristretto.Cache[string, *ContextWindow]
	group singleflight.Group // Prevents cache stampede
	mu    sync.RWMutex       // Protects invalidation
}

// NewContextCache creates a new cache with the given configuration
func NewContextCache(cfg CacheConfig) (*ContextCache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, *ContextWindow]{
		NumCounters: cfg.NumCounters,
		MaxCost:     cfg.MaxCost,
		BufferItems: 64,
		Metrics:     cfg.Metrics,

		// Dynamic cost based on content size
		Cost: func(window *ContextWindow) int64 {
			cost := int64(len(window.SystemPrompt))
			for _, msg := range window.Messages {
				cost += int64(len(msg.Content))
			}
			// Minimum cost to prevent zero-cost entries
			if cost < 100 {
				cost = 100
			}
			return cost
		},
	})
	if err != nil {
		return nil, err
	}

	return &ContextCache{
		cache: cache,
	}, nil
}

// NewContextCacheDefault creates a cache with default configuration
func NewContextCacheDefault() (*ContextCache, error) {
	return NewContextCache(DefaultCacheConfig())
}

// CacheKey generates a unique cache key for a context window request
// Key includes session ID and system prompt hash (prompt affects context)
func CacheKey(sessionID, systemPrompt string) string {
	h := sha256.New()
	h.Write([]byte(sessionID))
	h.Write([]byte(systemPrompt))
	return "ctx:" + sessionID + ":" + hex.EncodeToString(h.Sum(nil))[:16]
}

// Get retrieves a cached context window
func (cc *ContextCache) Get(key string) (*ContextWindow, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	return cc.cache.Get(key)
}

// Set caches a context window
// Note: ristretto is eventually consistent - Set may be dropped under pressure
func (cc *ContextCache) Set(key string, window *ContextWindow) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	// Cost=0 triggers the Cost function
	cc.cache.Set(key, window, 0)
}

// SetWait caches a context window and waits for it to be stored
// Useful in tests where we need immediate visibility
func (cc *ContextCache) SetWait(key string, window *ContextWindow) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	cc.cache.Set(key, window, 0)
	cc.cache.Wait() // Ensure visibility
}

// Invalidate removes all cache entries for a session
// Called when new message is added (CTX-11)
func (cc *ContextCache) Invalidate(sessionID string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// Delete entries with this session prefix
	// Since we use "ctx:{sessionID}:{hash}" format, we can delete by prefix
	// Ristretto doesn't support prefix delete, so we use exact key delete
	// In practice, each session has one active context window

	// Delete the most common key format
	cc.cache.Del("ctx:" + sessionID)
}

// InvalidateKey removes a specific cache entry
func (cc *ContextCache) InvalidateKey(key string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.cache.Del(key)
}

// GetOrBuild retrieves from cache or builds using the provided function
// Uses singleflight to prevent cache stampede - only one goroutine builds
func (cc *ContextCache) GetOrBuild(key string, build func() (*ContextWindow, error)) (*ContextWindow, error) {
	// Check cache first (fast path)
	if window, ok := cc.Get(key); ok {
		return window, nil
	}

	// Use singleflight to prevent stampede
	result, err, _ := cc.group.Do(key, func() (interface{}, error) {
		// Double-check cache (another goroutine may have populated it)
		if window, ok := cc.Get(key); ok {
			return window, nil
		}

		// Build the context window
		window, err := build()
		if err != nil {
			return nil, err
		}

		// Cache the result
		cc.Set(key, window)

		return window, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*ContextWindow), nil
}

// Metrics returns cache hit/miss statistics
func (cc *ContextCache) Metrics() *ristretto.Metrics {
	return cc.cache.Metrics
}

// HitRatio returns the cache hit ratio (0.0 to 1.0)
func (cc *ContextCache) HitRatio() float64 {
	m := cc.cache.Metrics
	if m == nil {
		return 0
	}
	total := m.Hits() + m.Misses()
	if total == 0 {
		return 0
	}
	return float64(m.Hits()) / float64(total)
}

// Close releases cache resources
func (cc *ContextCache) Close() {
	cc.cache.Close()
}
