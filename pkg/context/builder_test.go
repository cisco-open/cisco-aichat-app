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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/grafana/grafana-aichat-app/pkg/storage"
)

// mockStorage implements storage.Storage for testing
type mockStorage struct {
	messages []storage.ChatMessage
	session  *storage.ChatSession
}

func (m *mockStorage) GetMessagesByTokenBudget(ctx context.Context, sessionID string, budget int) ([]storage.ChatMessage, error) {
	// Always return all messages for tests - let the SlidingWindow handle budget
	// This simulates the real storage that returns messages sorted chronologically
	return m.messages, nil
}

func (m *mockStorage) GetSession(ctx context.Context, userID, sessionID string) (*storage.ChatSession, error) {
	if m.session == nil {
		return &storage.ChatSession{ID: sessionID}, nil
	}
	return m.session, nil
}

// Implement other Storage methods as no-ops for interface compliance
func (m *mockStorage) GetSessions(ctx context.Context, userID string) ([]storage.ChatSession, error) {
	return nil, nil
}
func (m *mockStorage) CreateSession(ctx context.Context, userID string, session *storage.ChatSession) error {
	return nil
}
func (m *mockStorage) UpdateSession(ctx context.Context, userID string, session *storage.ChatSession) error {
	return nil
}
func (m *mockStorage) DeleteSession(ctx context.Context, userID, sessionID string) error {
	return nil
}
func (m *mockStorage) SetActiveSession(ctx context.Context, userID, sessionID string) error {
	return nil
}
func (m *mockStorage) AddMessage(ctx context.Context, userID, sessionID string, message *storage.ChatMessage) error {
	return nil
}
func (m *mockStorage) UpdateMessage(ctx context.Context, userID, sessionID, messageID string, content string) error {
	return nil
}
func (m *mockStorage) ClearAllHistory(ctx context.Context, userID string) error { return nil }
func (m *mockStorage) DeleteExpiredSessions(ctx context.Context, retentionDays int) (int64, error) {
	return 0, nil
}
func (m *mockStorage) Ping(ctx context.Context) error { return nil }
func (m *mockStorage) SaveMessage(ctx context.Context, userID, sessionID string, message *storage.ChatMessage) error {
	return nil
}
func (m *mockStorage) SaveMessages(ctx context.Context, userID, sessionID string, messages []storage.ChatMessage) ([]storage.SaveResult, error) {
	return nil, nil
}
func (m *mockStorage) GetSessionMessages(ctx context.Context, userID string, params storage.GetSessionMessagesParams) (*storage.MessagesPage, error) {
	return nil, nil
}
func (m *mockStorage) UpdateMessageTokenCount(ctx context.Context, messageID string, tokenCount int) error {
	return nil
}
func (m *mockStorage) SaveSummary(ctx context.Context, userID, sessionID string, summary *storage.ChatMessage, originalIDs []string) error {
	return nil
}
func (m *mockStorage) GetOldestNonSummaryMessages(ctx context.Context, sessionID string, tokenLimit int) ([]storage.ChatMessage, error) {
	return nil, nil
}
func (m *mockStorage) Close() error { return nil }

// builderMockCounter implements tokens.TokenCounter for testing
type builderMockCounter struct{}

func (m *builderMockCounter) CountTokens(ctx context.Context, text string, model string) (int, error) {
	// Simple estimation: 4 chars per token
	return len(text) / 4, nil
}

func (m *builderMockCounter) SupportsModel(model string) bool {
	return true
}

func TestContextBuilder_BuildContextWindow_Basic(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Role: "user", Content: "Hello", Timestamp: 1000, TokenCount: 10},
			{ID: "2", Role: "assistant", Content: "Hi there!", Timestamp: 2000, TokenCount: 15},
			{ID: "3", Role: "user", Content: "How are you?", Timestamp: 3000, TokenCount: 20},
		},
		session: &storage.ChatSession{ID: "session1", TotalTokens: 45},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil) // No cache

	window, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		SystemPrompt:   "You are a helpful assistant.",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if window.SystemPrompt != "You are a helpful assistant." {
		t.Error("System prompt not preserved")
	}

	if len(window.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(window.Messages))
	}
}

func TestContextBuilder_BuildContextWindow_WithCache(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Role: "user", Content: "Hello", Timestamp: 1000, TokenCount: 10},
		},
		session: &storage.ChatSession{ID: "session1", TotalTokens: 10},
	}

	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	cb := NewContextBuilder(store, &builderMockCounter{}, cache, nil)

	// First call - cache miss
	window1, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		SystemPrompt:   "prompt",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Wait for cache to settle (ristretto is eventually consistent)
	time.Sleep(10 * time.Millisecond)

	// Second call - should hit cache
	window2, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		SystemPrompt:   "prompt",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Both should return same data
	if len(window1.Messages) != len(window2.Messages) {
		t.Error("Cache should return same window")
	}
}

func TestContextBuilder_BuildContextWindow_CacheInvalidation(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Content: "Hello", Timestamp: 1000, TokenCount: 10},
		},
		session: &storage.ChatSession{ID: "session1"},
	}

	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	cb := NewContextBuilder(store, &builderMockCounter{}, cache, nil)

	// Build and cache
	_, err = cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})
	if err != nil {
		t.Fatalf("Unexpected error on first build: %v", err)
	}

	// Wait for cache to settle
	time.Sleep(10 * time.Millisecond)

	// Invalidate
	cb.InvalidateCache("session1")

	// Next build should be cache miss (no way to verify directly, but should not panic)
	_, err = cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})
	if err != nil {
		t.Fatalf("Unexpected error after invalidation: %v", err)
	}
}

func TestContextBuilder_BuildContextWindow_BudgetLimit(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Timestamp: 1000, TokenCount: 100},
			{ID: "2", Timestamp: 2000, TokenCount: 100},
			{ID: "3", Timestamp: 3000, TokenCount: 100},
			{ID: "4", Timestamp: 4000, TokenCount: 100},
		},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil).WithMinMessages(1)

	// Budget allows only ~2 messages after system prompt
	window, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		SystemPrompt:   "", // No system prompt
		MaxTokens:      250,
		ResponseBuffer: 50,
		MinMessages:    1,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Budget is 200 tokens (250 - 50 buffer), should get 2 messages
	if len(window.Messages) != 2 {
		t.Errorf("Expected 2 messages within budget, got %d", len(window.Messages))
	}
}

func TestContextBuilder_BuildContextWindow_MinimumGuarantee(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Timestamp: 1000, TokenCount: 100},
			{ID: "2", Timestamp: 2000, TokenCount: 100},
			{ID: "3", Timestamp: 3000, TokenCount: 100},
		},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil).WithMinMessages(3)

	// Very tight budget that would normally exclude messages
	window, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		MaxTokens:      150, // Normally only ~1 message
		ResponseBuffer: 50,
		MinMessages:    3,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Despite budget, minimum 3 should be included
	if len(window.Messages) < 3 {
		t.Errorf("Expected minimum 3 messages, got %d", len(window.Messages))
	}
}

func TestContextBuilder_BuildContextWindow_ChronologicalOrder(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Timestamp: 1000, TokenCount: 10},
			{ID: "2", Timestamp: 2000, TokenCount: 10},
			{ID: "3", Timestamp: 3000, TokenCount: 10},
		},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil)

	window, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify chronological order
	for i := 1; i < len(window.Messages); i++ {
		if window.Messages[i].Timestamp <= window.Messages[i-1].Timestamp {
			t.Error("Messages not in chronological order")
		}
	}
}

func TestContextBuilder_BuildContextWindow_SessionIDRequired(t *testing.T) {
	cb := NewContextBuilder(&mockStorage{}, &builderMockCounter{}, nil, nil)

	_, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		// Missing SessionID
	})

	if err == nil {
		t.Error("Expected error for missing sessionID")
	}
}

func TestContextBuilder_BuildContextWindow_CountsSummaries(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Timestamp: 1000, TokenCount: 10, IsSummary: false},
			{ID: "2", Timestamp: 2000, TokenCount: 10, IsSummary: true},
			{ID: "3", Timestamp: 3000, TokenCount: 10, IsSummary: false},
		},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil)

	window, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if window.SummaryCount != 1 {
		t.Errorf("Expected 1 summary, got %d", window.SummaryCount)
	}
}

func TestContextBuilder_BuildContextWindow_Performance(t *testing.T) {
	// Create large message set
	messages := make([]storage.ChatMessage, 1000)
	for i := 0; i < 1000; i++ {
		messages[i] = storage.ChatMessage{
			ID:         fmt.Sprintf("msg-%d", i),
			Timestamp:  int64(i * 1000),
			TokenCount: 100,
		}
	}

	store := &mockStorage{messages: messages}
	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil)

	start := time.Now()
	_, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID: "session1",
		MaxTokens: 100000,
	})
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Performance target: <100ms
	if duration > 100*time.Millisecond {
		t.Errorf("BuildContextWindow took %v, expected <100ms", duration)
	}

	t.Logf("Performance: BuildContextWindow completed in %v", duration)
}

func TestContextBuilder_InvalidateCache_NilCache(t *testing.T) {
	cb := NewContextBuilder(&mockStorage{}, &builderMockCounter{}, nil, nil)

	// Should not panic when cache is nil
	cb.InvalidateCache("session1")
}

func TestContextBuilder_GetContextUsage(t *testing.T) {
	store := &mockStorage{
		session: &storage.ChatSession{ID: "session1", TotalTokens: 50000},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil)

	usage, err := cb.GetContextUsage(context.Background(), "session1", 100000)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := 50.0 // 50000 / 100000 * 100
	if usage != expected {
		t.Errorf("Expected usage %.2f%%, got %.2f%%", expected, usage)
	}
}

func TestContextBuilder_GetContextUsage_DefaultMaxTokens(t *testing.T) {
	store := &mockStorage{
		session: &storage.ChatSession{ID: "session1", TotalTokens: 25000},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil)

	// Pass 0 to use default
	usage, err := cb.GetContextUsage(context.Background(), "session1", 0)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := 25.0 // 25000 / 100000 * 100
	if usage != expected {
		t.Errorf("Expected usage %.2f%%, got %.2f%%", expected, usage)
	}
}

func TestContextBuilder_WithMinMessages(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Timestamp: 1000, TokenCount: 10},
			{ID: "2", Timestamp: 2000, TokenCount: 10},
			{ID: "3", Timestamp: 3000, TokenCount: 10},
			{ID: "4", Timestamp: 4000, TokenCount: 10},
			{ID: "5", Timestamp: 5000, TokenCount: 10},
		},
	}

	// Test with min=5
	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil).WithMinMessages(5)

	window, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		MaxTokens:      100, // Would only fit ~1-2 messages
		ResponseBuffer: 50,
		MinMessages:    5,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// All 5 should be included due to minimum guarantee
	if len(window.Messages) != 5 {
		t.Errorf("Expected 5 messages (minimum guarantee), got %d", len(window.Messages))
	}
}

func TestContextBuilder_EmptyMessages(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil)

	window, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		SystemPrompt:   "You are helpful.",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(window.Messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(window.Messages))
	}

	if window.SystemPrompt != "You are helpful." {
		t.Error("System prompt should still be included")
	}
}

func TestContextBuilder_TotalTokensCalculation(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Timestamp: 1000, TokenCount: 100},
			{ID: "2", Timestamp: 2000, TokenCount: 200},
			{ID: "3", Timestamp: 3000, TokenCount: 300},
		},
	}

	cb := NewContextBuilder(store, &builderMockCounter{}, nil, nil)

	window, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:    "session1",
		SystemPrompt: "Test prompt here.", // ~4 tokens with our mock
		MaxTokens:    10000,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Messages total: 100 + 200 + 300 = 600
	// System prompt: ~4 tokens
	expectedMessageTokens := 600
	if window.TotalTokens < expectedMessageTokens {
		t.Errorf("Expected TotalTokens >= %d, got %d", expectedMessageTokens, window.TotalTokens)
	}
}

func TestContextBuilder_CacheKeyVariation(t *testing.T) {
	store := &mockStorage{
		messages: []storage.ChatMessage{
			{ID: "1", Content: "Hello", Timestamp: 1000, TokenCount: 10},
		},
	}

	cache, err := NewContextCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	cb := NewContextBuilder(store, &builderMockCounter{}, cache, nil)

	// Build with prompt A
	window1, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		SystemPrompt:   "Prompt A",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // Let cache settle

	// Build with prompt B - should be different cache key
	window2, err := cb.BuildContextWindow(context.Background(), BuildOptions{
		SessionID:      "session1",
		SystemPrompt:   "Prompt B",
		MaxTokens:      10000,
		ResponseBuffer: 100,
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// System prompts should be different (not returning cached value from different prompt)
	if window1.SystemPrompt == window2.SystemPrompt {
		t.Error("Different system prompts should produce different cached entries")
	}
}
