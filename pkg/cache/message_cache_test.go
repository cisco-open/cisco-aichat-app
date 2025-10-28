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

package cache

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-aichat-app/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessageCache(t *testing.T) {
	t.Run("creates cache with default config", func(t *testing.T) {
		mc, err := NewMessageCacheDefault()
		require.NoError(t, err)
		require.NotNil(t, mc)
		defer mc.Close()
	})

	t.Run("creates cache with custom config", func(t *testing.T) {
		cfg := MessageCacheConfig{
			NumCounters: 1000,
			MaxCost:     1024 * 1024, // 1MB
			TTL:         5 * time.Minute,
			Metrics:     true,
		}
		mc, err := NewMessageCache(cfg)
		require.NoError(t, err)
		require.NotNil(t, mc)
		defer mc.Close()
	})

	t.Run("creates cache with size helper", func(t *testing.T) {
		mc, err := NewMessageCacheWithSize(64*1024*1024, 15*time.Minute)
		require.NoError(t, err)
		require.NotNil(t, mc)
		defer mc.Close()
	})
}

func TestMessageCache_GetSet(t *testing.T) {
	mc, err := NewMessageCache(MessageCacheConfig{
		NumCounters: 1000,
		MaxCost:     1024 * 1024,
		TTL:         5 * time.Minute,
		Metrics:     true,
	})
	require.NoError(t, err)
	defer mc.Close()

	t.Run("cache miss returns false", func(t *testing.T) {
		messages, ok := mc.Get("nonexistent")
		assert.False(t, ok)
		assert.Nil(t, messages)
	})

	t.Run("cache hit returns stored messages", func(t *testing.T) {
		sessionID := "test-session-1"
		messages := []storage.ChatMessage{
			{ID: "msg-1", Role: "user", Content: "Hello"},
			{ID: "msg-2", Role: "assistant", Content: "Hi there!"},
		}

		mc.SetWait(sessionID, messages)

		cached, ok := mc.Get(sessionID)
		assert.True(t, ok)
		assert.Equal(t, messages, cached)
	})

	t.Run("stores multiple sessions independently", func(t *testing.T) {
		session1 := "session-a"
		session2 := "session-b"

		msgs1 := []storage.ChatMessage{{ID: "1", Content: "Session 1"}}
		msgs2 := []storage.ChatMessage{{ID: "2", Content: "Session 2"}}

		mc.SetWait(session1, msgs1)
		mc.SetWait(session2, msgs2)

		cached1, ok1 := mc.Get(session1)
		cached2, ok2 := mc.Get(session2)

		assert.True(t, ok1)
		assert.True(t, ok2)
		assert.Equal(t, msgs1, cached1)
		assert.Equal(t, msgs2, cached2)
	})
}

func TestMessageCache_Invalidate(t *testing.T) {
	mc, err := NewMessageCache(MessageCacheConfig{
		NumCounters: 1000,
		MaxCost:     1024 * 1024,
		TTL:         5 * time.Minute,
		Metrics:     true,
	})
	require.NoError(t, err)
	defer mc.Close()

	t.Run("invalidate removes entry", func(t *testing.T) {
		sessionID := "invalidate-test"
		messages := []storage.ChatMessage{{ID: "msg", Content: "Test"}}

		mc.SetWait(sessionID, messages)

		// Verify it's cached
		_, ok := mc.Get(sessionID)
		assert.True(t, ok)

		// Invalidate
		mc.Invalidate(sessionID)

		// Wait for deletion to propagate (ristretto is async)
		time.Sleep(10 * time.Millisecond)

		// Verify it's gone
		_, ok = mc.Get(sessionID)
		assert.False(t, ok)
	})

	t.Run("invalidate nonexistent session is safe", func(t *testing.T) {
		// Should not panic
		mc.Invalidate("nonexistent-session")
	})
}

func TestMessageCache_TTLExpiration(t *testing.T) {
	// Use short TTL for testing
	mc, err := NewMessageCache(MessageCacheConfig{
		NumCounters: 1000,
		MaxCost:     1024 * 1024,
		TTL:         50 * time.Millisecond, // Very short TTL for test
		Metrics:     true,
	})
	require.NoError(t, err)
	defer mc.Close()

	t.Run("entries expire after TTL", func(t *testing.T) {
		sessionID := "ttl-test"
		messages := []storage.ChatMessage{{ID: "msg", Content: "Expires soon"}}

		mc.SetWait(sessionID, messages)

		// Verify it's cached
		_, ok := mc.Get(sessionID)
		assert.True(t, ok)

		// Wait for TTL + buffer
		time.Sleep(100 * time.Millisecond)

		// Verify it's expired
		_, ok = mc.Get(sessionID)
		assert.False(t, ok)
	})
}

func TestMessageCache_GetOrFetch(t *testing.T) {
	mc, err := NewMessageCache(MessageCacheConfig{
		NumCounters: 1000,
		MaxCost:     1024 * 1024,
		TTL:         5 * time.Minute,
		Metrics:     true,
	})
	require.NoError(t, err)
	defer mc.Close()

	t.Run("fetches on cache miss", func(t *testing.T) {
		sessionID := "fetch-test"
		fetchCalled := false
		expectedMessages := []storage.ChatMessage{{ID: "fetched", Content: "From source"}}

		messages, err := mc.GetOrFetch(sessionID, func() ([]storage.ChatMessage, error) {
			fetchCalled = true
			return expectedMessages, nil
		})

		require.NoError(t, err)
		assert.True(t, fetchCalled)
		assert.Equal(t, expectedMessages, messages)
	})

	t.Run("returns cached on hit without fetching", func(t *testing.T) {
		sessionID := "cached-test"
		cachedMessages := []storage.ChatMessage{{ID: "cached", Content: "Already here"}}

		mc.SetWait(sessionID, cachedMessages)

		fetchCalled := false
		messages, err := mc.GetOrFetch(sessionID, func() ([]storage.ChatMessage, error) {
			fetchCalled = true
			return nil, errors.New("should not fetch")
		})

		require.NoError(t, err)
		assert.False(t, fetchCalled)
		assert.Equal(t, cachedMessages, messages)
	})

	t.Run("propagates fetch errors", func(t *testing.T) {
		sessionID := "error-test"
		expectedError := errors.New("fetch failed")

		messages, err := mc.GetOrFetch(sessionID, func() ([]storage.ChatMessage, error) {
			return nil, expectedError
		})

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, messages)
	})

	t.Run("caches result after successful fetch", func(t *testing.T) {
		sessionID := "cache-after-fetch"
		expectedMessages := []storage.ChatMessage{{ID: "new", Content: "Will be cached"}}

		// First call - fetches
		_, err := mc.GetOrFetch(sessionID, func() ([]storage.ChatMessage, error) {
			return expectedMessages, nil
		})
		require.NoError(t, err)

		// Wait for cache write
		time.Sleep(10 * time.Millisecond)

		// Direct get should hit cache
		cached, ok := mc.Get(sessionID)
		assert.True(t, ok)
		assert.Equal(t, expectedMessages, cached)
	})
}

func TestMessageCache_Singleflight(t *testing.T) {
	mc, err := NewMessageCache(MessageCacheConfig{
		NumCounters: 1000,
		MaxCost:     1024 * 1024,
		TTL:         5 * time.Minute,
		Metrics:     true,
	})
	require.NoError(t, err)
	defer mc.Close()

	t.Run("prevents cache stampede with concurrent requests", func(t *testing.T) {
		sessionID := "stampede-test"
		var fetchCount int
		var fetchMu sync.Mutex

		expectedMessages := []storage.ChatMessage{{ID: "single", Content: "Fetched once"}}

		fetch := func() ([]storage.ChatMessage, error) {
			fetchMu.Lock()
			fetchCount++
			fetchMu.Unlock()

			// Simulate slow fetch
			time.Sleep(50 * time.Millisecond)
			return expectedMessages, nil
		}

		// Launch concurrent requests
		var wg sync.WaitGroup
		const numRequests = 10

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				messages, err := mc.GetOrFetch(sessionID, fetch)
				require.NoError(t, err)
				assert.Equal(t, expectedMessages, messages)
			}()
		}

		wg.Wait()

		// Despite 10 concurrent requests, fetch should only be called once
		fetchMu.Lock()
		assert.Equal(t, 1, fetchCount, "fetch should only be called once due to singleflight")
		fetchMu.Unlock()
	})
}

func TestMessageCache_HitRatio(t *testing.T) {
	mc, err := NewMessageCache(MessageCacheConfig{
		NumCounters: 1000,
		MaxCost:     1024 * 1024,
		TTL:         5 * time.Minute,
		Metrics:     true,
	})
	require.NoError(t, err)
	defer mc.Close()

	t.Run("returns 0 initially", func(t *testing.T) {
		ratio := mc.HitRatio()
		assert.Equal(t, 0.0, ratio)
	})

	t.Run("tracks hits and misses", func(t *testing.T) {
		sessionID := "ratio-test"
		messages := []storage.ChatMessage{{ID: "msg", Content: "Test"}}

		// Generate a miss
		mc.Get("nonexistent")

		// Set and generate hits
		mc.SetWait(sessionID, messages)

		// Generate 4 hits
		for i := 0; i < 4; i++ {
			mc.Get(sessionID)
		}

		// Ratio should be 4 hits / 5 total = 0.8
		ratio := mc.HitRatio()
		assert.InDelta(t, 0.8, ratio, 0.01)
	})
}

func TestMessageCache_CostFunction(t *testing.T) {
	mc, err := NewMessageCache(MessageCacheConfig{
		NumCounters: 1000,
		MaxCost:     1024 * 1024,
		TTL:         5 * time.Minute,
		Metrics:     true,
	})
	require.NoError(t, err)
	defer mc.Close()

	t.Run("handles empty messages", func(t *testing.T) {
		sessionID := "empty-test"
		mc.SetWait(sessionID, []storage.ChatMessage{})

		cached, ok := mc.Get(sessionID)
		assert.True(t, ok)
		assert.Empty(t, cached)
	})

	t.Run("handles large messages", func(t *testing.T) {
		sessionID := "large-test"
		largeContent := make([]byte, 10000)
		for i := range largeContent {
			largeContent[i] = 'a'
		}

		messages := []storage.ChatMessage{
			{ID: "large", Content: string(largeContent)},
		}

		mc.SetWait(sessionID, messages)

		cached, ok := mc.Get(sessionID)
		assert.True(t, ok)
		assert.Len(t, cached[0].Content, 10000)
	})
}

func TestMessageCache_Concurrency(t *testing.T) {
	mc, err := NewMessageCache(MessageCacheConfig{
		NumCounters: 1000,
		MaxCost:     1024 * 1024,
		TTL:         5 * time.Minute,
		Metrics:     true,
	})
	require.NoError(t, err)
	defer mc.Close()

	t.Run("handles concurrent reads and writes safely", func(t *testing.T) {
		var wg sync.WaitGroup
		const numGoroutines = 50
		const numOperations = 100

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				sessionID := "concurrent-test"

				for j := 0; j < numOperations; j++ {
					switch j % 4 {
					case 0:
						mc.Get(sessionID)
					case 1:
						mc.Set(sessionID, []storage.ChatMessage{{ID: "msg", Content: "Test"}})
					case 2:
						mc.Invalidate(sessionID)
					case 3:
						mc.HitRatio()
					}
				}
			}(i)
		}

		wg.Wait()
		// Test passes if no data race is detected
	})
}

func TestMessageCache_Close(t *testing.T) {
	mc, err := NewMessageCacheDefault()
	require.NoError(t, err)

	// Set some data
	mc.SetWait("session", []storage.ChatMessage{{ID: "msg", Content: "Test"}})

	// Close should not panic
	mc.Close()
}
