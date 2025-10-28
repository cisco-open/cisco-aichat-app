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

// Package cache provides in-memory caching for chat data.
package cache

import (
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/grafana/grafana-aichat-app/pkg/storage"
	"golang.org/x/sync/singleflight"
)

// MessageCacheConfig configures the message cache
type MessageCacheConfig struct {
	NumCounters int64         // Number of keys to track (10x expected active sessions)
	MaxCost     int64         // Maximum memory cost in bytes
	TTL         time.Duration // TTL for cache entries
	Metrics     bool          // Enable metrics collection
}

// DefaultMessageCacheConfig returns sensible defaults
func DefaultMessageCacheConfig() MessageCacheConfig {
	return MessageCacheConfig{
		NumCounters: 10000,             // 10k keys to track
		MaxCost:     64 * 1024 * 1024,  // 64MB max memory
		TTL:         15 * time.Minute,  // 15 minute TTL per PERF-01
		Metrics:     true,              // Enable for monitoring
	}
}

// MessageCache wraps ristretto for session message caching with TTL expiration.
// It caches session messages to reduce database load on repeated access (PERF-01, PERF-04).
type MessageCache struct {
	cache *ristretto.Cache[string, []storage.ChatMessage]
	group singleflight.Group // Prevents cache stampede
	mu    sync.RWMutex       // Protects invalidation
	ttl   time.Duration      // TTL for cache entries
}

// NewMessageCache creates a new message cache with the given configuration.
func NewMessageCache(cfg MessageCacheConfig) (*MessageCache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, []storage.ChatMessage]{
		NumCounters: cfg.NumCounters,
		MaxCost:     cfg.MaxCost,
		BufferItems: 64,
		Metrics:     cfg.Metrics,

		// Dynamic cost based on message content size
		Cost: func(messages []storage.ChatMessage) int64 {
			var cost int64
			for _, msg := range messages {
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

	return &MessageCache{
		cache: cache,
		ttl:   cfg.TTL,
	}, nil
}

// NewMessageCacheDefault creates a cache with default configuration.
func NewMessageCacheDefault() (*MessageCache, error) {
	return NewMessageCache(DefaultMessageCacheConfig())
}

// NewMessageCacheWithSize creates a cache with specified max memory and TTL.
func NewMessageCacheWithSize(maxCost int64, ttl time.Duration) (*MessageCache, error) {
	cfg := DefaultMessageCacheConfig()
	cfg.MaxCost = maxCost
	cfg.TTL = ttl
	return NewMessageCache(cfg)
}

// cacheKey generates a cache key for session messages.
// Format: "msgs:{sessionID}" - simpler than ContextCache since no prompt hash needed.
func cacheKey(sessionID string) string {
	return "msgs:" + sessionID
}

// Get retrieves cached session messages.
// Returns (messages, true) if found in cache, (nil, false) otherwise.
func (mc *MessageCache) Get(sessionID string) ([]storage.ChatMessage, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return mc.cache.Get(cacheKey(sessionID))
}

// Set caches session messages with TTL.
// Note: ristretto is eventually consistent - Set may be dropped under pressure.
func (mc *MessageCache) Set(sessionID string, messages []storage.ChatMessage) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Use SetWithTTL for automatic expiration
	mc.cache.SetWithTTL(cacheKey(sessionID), messages, 0, mc.ttl)
}

// SetWait caches session messages and waits for storage.
// Useful in tests where immediate visibility is needed.
func (mc *MessageCache) SetWait(sessionID string, messages []storage.ChatMessage) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	mc.cache.SetWithTTL(cacheKey(sessionID), messages, 0, mc.ttl)
	mc.cache.Wait() // Ensure visibility
}

// Invalidate removes the cache entry for a session.
// Called when messages are modified (add, edit, delete, pin).
func (mc *MessageCache) Invalidate(sessionID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.cache.Del(cacheKey(sessionID))
}

// GetOrFetch retrieves from cache or fetches using the provided function.
// Uses singleflight to prevent cache stampede - only one goroutine fetches.
func (mc *MessageCache) GetOrFetch(sessionID string, fetch func() ([]storage.ChatMessage, error)) ([]storage.ChatMessage, error) {
	// Check cache first (fast path)
	if messages, ok := mc.Get(sessionID); ok {
		return messages, nil
	}

	// Use singleflight to prevent stampede
	key := cacheKey(sessionID)
	result, err, _ := mc.group.Do(key, func() (interface{}, error) {
		// Double-check cache (another goroutine may have populated it)
		if messages, ok := mc.Get(sessionID); ok {
			return messages, nil
		}

		// Fetch from source
		messages, err := fetch()
		if err != nil {
			return nil, err
		}

		// Cache the result
		mc.Set(sessionID, messages)

		return messages, nil
	})

	if err != nil {
		return nil, err
	}

	return result.([]storage.ChatMessage), nil
}

// Metrics returns cache hit/miss statistics.
func (mc *MessageCache) Metrics() *ristretto.Metrics {
	return mc.cache.Metrics
}

// HitRatio returns the cache hit ratio (0.0 to 1.0).
// Used for monitoring PERF-04 target (>80% hit rate).
func (mc *MessageCache) HitRatio() float64 {
	m := mc.cache.Metrics
	if m == nil {
		return 0
	}
	total := m.Hits() + m.Misses()
	if total == 0 {
		return 0
	}
	return float64(m.Hits()) / float64(total)
}

// Close releases cache resources.
func (mc *MessageCache) Close() {
	mc.cache.Close()
}
