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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/cisco-aichat-app/pkg/storage"
)

func TestContextCache_GetSet(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	key := CacheKey("session1", "system prompt")
	window := &ContextWindow{
		SystemPrompt: "system prompt",
		Messages: []storage.ChatMessage{
			{ID: "1", Content: "Hello"},
		},
		TotalTokens: 100,
	}

	// Set and wait for visibility
	cache.SetWait(key, window)

	// Get should return the cached value
	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("Expected cache hit")
	}

	if got.TotalTokens != 100 {
		t.Errorf("Expected TotalTokens=100, got %d", got.TotalTokens)
	}

	if len(got.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(got.Messages))
	}

	if got.Messages[0].Content != "Hello" {
		t.Errorf("Expected message content 'Hello', got '%s'", got.Messages[0].Content)
	}
}

func TestContextCache_CacheKey(t *testing.T) {
	// Same session + prompt = same key
	key1 := CacheKey("session1", "prompt")
	key2 := CacheKey("session1", "prompt")
	if key1 != key2 {
		t.Error("Same inputs should produce same key")
	}

	// Different session = different key
	key3 := CacheKey("session2", "prompt")
	if key1 == key3 {
		t.Error("Different sessions should produce different keys")
	}

	// Different prompt = different key
	key4 := CacheKey("session1", "different prompt")
	if key1 == key4 {
		t.Error("Different prompts should produce different keys")
	}

	// Keys should contain session ID for debugging
	if len(key1) < 20 {
		t.Error("Key should be reasonably long")
	}
}

func TestContextCache_CacheMiss(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Non-existent key should return miss
	_, ok := cache.Get("nonexistent-key")
	if ok {
		t.Error("Expected cache miss for non-existent key")
	}
}

func TestContextCache_InvalidateKey(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	key := CacheKey("session1", "prompt")
	window := &ContextWindow{TotalTokens: 100}

	cache.SetWait(key, window)

	// Verify it's cached
	if _, ok := cache.Get(key); !ok {
		t.Fatal("Expected cache hit before invalidation")
	}

	// Invalidate specific key
	cache.InvalidateKey(key)

	// Small delay for ristretto deletion
	time.Sleep(10 * time.Millisecond)

	// Should be gone
	if _, ok := cache.Get(key); ok {
		t.Error("Expected cache miss after key invalidation")
	}
}

func TestContextCache_Invalidate(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Note: Invalidate uses "ctx:{sessionID}" without hash
	// For this test, we set a key with just the session prefix
	sessionID := "session-for-invalidate"
	prefixKey := "ctx:" + sessionID

	window := &ContextWindow{TotalTokens: 100}
	cache.SetWait(prefixKey, window)

	// Verify it's cached
	if _, ok := cache.Get(prefixKey); !ok {
		t.Fatal("Expected cache hit before invalidation")
	}

	// Invalidate by session
	cache.Invalidate(sessionID)

	// Small delay for ristretto deletion
	time.Sleep(10 * time.Millisecond)

	// Should be gone
	if _, ok := cache.Get(prefixKey); ok {
		t.Error("Expected cache miss after session invalidation")
	}
}

func TestContextCache_GetOrBuild_CacheHit(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	key := CacheKey("session1", "prompt")
	window := &ContextWindow{TotalTokens: 100}
	cache.SetWait(key, window)

	buildCalled := false
	got, err := cache.GetOrBuild(key, func() (*ContextWindow, error) {
		buildCalled = true
		return &ContextWindow{TotalTokens: 200}, nil
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if buildCalled {
		t.Error("Build function should not be called on cache hit")
	}

	if got.TotalTokens != 100 {
		t.Errorf("Expected cached value (100), got %d", got.TotalTokens)
	}
}

func TestContextCache_GetOrBuild_CacheMiss(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	key := CacheKey("session1", "prompt")

	buildCalled := false
	got, err := cache.GetOrBuild(key, func() (*ContextWindow, error) {
		buildCalled = true
		return &ContextWindow{TotalTokens: 200}, nil
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !buildCalled {
		t.Error("Build function should be called on cache miss")
	}

	if got.TotalTokens != 200 {
		t.Errorf("Expected built value (200), got %d", got.TotalTokens)
	}
}

func TestContextCache_GetOrBuild_Singleflight(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	key := CacheKey("session1", "prompt")

	var buildCount int32
	var wg sync.WaitGroup

	// Start 10 concurrent requests for the same key
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.GetOrBuild(key, func() (*ContextWindow, error) {
				atomic.AddInt32(&buildCount, 1)
				time.Sleep(50 * time.Millisecond) // Simulate work
				return &ContextWindow{TotalTokens: 100}, nil
			})
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()

	// Only one build should have occurred (singleflight)
	if buildCount != 1 {
		t.Errorf("Expected build to be called once (singleflight), called %d times", buildCount)
	}
}

func TestContextCache_GetOrBuild_Error(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	key := CacheKey("session1", "prompt")
	expectedErr := "build error"

	_, err = cache.GetOrBuild(key, func() (*ContextWindow, error) {
		return nil, &testError{msg: expectedErr}
	})

	if err == nil {
		t.Fatal("Expected error from build function")
	}

	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestContextCache_HitRatio(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Initial ratio should be 0
	if ratio := cache.HitRatio(); ratio != 0 {
		t.Errorf("Expected initial ratio 0, got %f", ratio)
	}

	key := CacheKey("session1", "prompt")
	cache.SetWait(key, &ContextWindow{})

	// Hit
	cache.Get(key)
	// Miss
	cache.Get("nonexistent")

	// Give ristretto time to update metrics
	time.Sleep(10 * time.Millisecond)

	ratio := cache.HitRatio()
	// Should be ~0.5 (1 hit, 1 miss)
	if ratio < 0.3 || ratio > 0.7 {
		t.Errorf("Expected ratio around 0.5, got %f", ratio)
	}
}

func TestContextCache_Metrics(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Metrics should be non-nil (enabled by default)
	if cache.Metrics() == nil {
		t.Error("Expected metrics to be enabled by default")
	}
}

func TestContextCache_CostBasedEviction(t *testing.T) {
	// Create tiny cache to force eviction
	cache, err := NewContextCache(CacheConfig{
		NumCounters: 100,
		MaxCost:     1000, // Only 1KB
		Metrics:     true,
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Add entry that exceeds cache size
	bigWindow := &ContextWindow{
		SystemPrompt: string(make([]byte, 500)), // 500 bytes
		Messages: []storage.ChatMessage{
			{Content: string(make([]byte, 600))}, // 600 bytes
		},
	}

	key := CacheKey("session1", "prompt")
	cache.SetWait(key, bigWindow)

	// The entry may be evicted due to size
	// This test just verifies no panic occurs with oversized entries
}

func TestContextCache_CustomConfig(t *testing.T) {
	cfg := CacheConfig{
		NumCounters: 1000,
		MaxCost:     1 << 20, // 1MB
		Metrics:     false,
	}

	cache, err := NewContextCache(cfg)
	if err != nil {
		t.Fatalf("Failed to create cache with custom config: %v", err)
	}
	defer cache.Close()

	// Should work normally
	key := CacheKey("session1", "prompt")
	cache.SetWait(key, &ContextWindow{TotalTokens: 42})

	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("Expected cache hit")
	}

	if got.TotalTokens != 42 {
		t.Errorf("Expected TotalTokens=42, got %d", got.TotalTokens)
	}

	// Metrics should be nil when disabled
	if cache.Metrics() != nil {
		t.Error("Expected metrics to be nil when disabled")
	}
}

func TestContextCache_DefaultConfig(t *testing.T) {
	cfg := DefaultCacheConfig()

	if cfg.NumCounters != 100000 {
		t.Errorf("Expected NumCounters=100000, got %d", cfg.NumCounters)
	}

	if cfg.MaxCost != 1<<27 {
		t.Errorf("Expected MaxCost=128MB, got %d", cfg.MaxCost)
	}

	if !cfg.Metrics {
		t.Error("Expected Metrics=true by default")
	}
}

func TestContextCache_MultipleSessionsIndependent(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Set different windows for different sessions
	key1 := CacheKey("session1", "prompt")
	key2 := CacheKey("session2", "prompt")

	cache.SetWait(key1, &ContextWindow{TotalTokens: 100})
	cache.SetWait(key2, &ContextWindow{TotalTokens: 200})

	// Both should be retrievable independently
	got1, ok1 := cache.Get(key1)
	got2, ok2 := cache.Get(key2)

	if !ok1 || !ok2 {
		t.Fatal("Expected both cache hits")
	}

	if got1.TotalTokens != 100 {
		t.Errorf("Session1: expected 100, got %d", got1.TotalTokens)
	}

	if got2.TotalTokens != 200 {
		t.Errorf("Session2: expected 200, got %d", got2.TotalTokens)
	}
}

func TestContextCache_SameSessionDifferentPrompts(t *testing.T) {
	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	// Same session but different system prompts = different cache entries
	key1 := CacheKey("session1", "prompt A")
	key2 := CacheKey("session1", "prompt B")

	cache.SetWait(key1, &ContextWindow{TotalTokens: 100})
	cache.SetWait(key2, &ContextWindow{TotalTokens: 200})

	got1, ok1 := cache.Get(key1)
	got2, ok2 := cache.Get(key2)

	if !ok1 || !ok2 {
		t.Fatal("Expected both cache hits")
	}

	if got1.TotalTokens != 100 {
		t.Errorf("Prompt A: expected 100, got %d", got1.TotalTokens)
	}

	if got2.TotalTokens != 200 {
		t.Errorf("Prompt B: expected 200, got %d", got2.TotalTokens)
	}
}
