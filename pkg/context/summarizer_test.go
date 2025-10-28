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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-aichat-app/pkg/storage"
)

// mockLLM implements LLMClient for testing
type mockLLM struct {
	response string
	err      error
	called   bool
	prompt   string
}

func (m *mockLLM) Complete(ctx context.Context, model string, prompt string) (string, error) {
	m.called = true
	m.prompt = prompt
	return m.response, m.err
}

// summarizerMockCounter implements tokens.TokenCounter for testing
type summarizerMockCounter struct {
	tokenCount int
}

func (m *summarizerMockCounter) CountTokens(ctx context.Context, text string, model string) (int, error) {
	if m.tokenCount > 0 {
		return m.tokenCount, nil
	}
	// Simple estimation: 4 chars per token
	return len(text) / 4, nil
}

func (m *summarizerMockCounter) SupportsModel(model string) bool {
	return true
}

// summarizeMockStorage extends mockStorage with summarization methods
type summarizeMockStorage struct {
	mockStorage
	savedSummary   *storage.ChatMessage
	savedOriginals []string
	saveSummaryErr error
}

func (m *summarizeMockStorage) SaveSummary(ctx context.Context, userID, sessionID string, summary *storage.ChatMessage, originalIDs []string) error {
	if m.saveSummaryErr != nil {
		return m.saveSummaryErr
	}
	m.savedSummary = summary
	m.savedOriginals = originalIDs
	return nil
}

func (m *summarizeMockStorage) GetOldestNonSummaryMessages(ctx context.Context, sessionID string, tokenLimit int) ([]storage.ChatMessage, error) {
	var result []storage.ChatMessage
	totalTokens := 0
	for _, msg := range m.messages {
		if !msg.IsSummary {
			if totalTokens+msg.TokenCount > tokenLimit && len(result) > 0 {
				break
			}
			result = append(result, msg)
			totalTokens += msg.TokenCount
		}
	}
	return result, nil
}

func TestSummarizer_IsEnabled(t *testing.T) {
	// Without LLM
	s := NewSummarizer(nil, nil, nil)
	if s.IsEnabled() {
		t.Error("Expected IsEnabled=false without LLM")
	}

	// With LLM
	s = NewSummarizer(nil, nil, &mockLLM{})
	if !s.IsEnabled() {
		t.Error("Expected IsEnabled=true with LLM")
	}
}

func TestSummarizer_ShouldSummarize(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			session: &storage.ChatSession{
				ID:          "session1",
				TotalTokens: 85000, // 85% of 100k
			},
		},
	}

	s := NewSummarizer(store, &summarizerMockCounter{}, &mockLLM{})

	// At 85%, should trigger (threshold is 80%)
	should, err := s.ShouldSummarize(context.Background(), "session1", 100000)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !should {
		t.Error("Expected should summarize at 85%")
	}

	// At 70%, should not trigger
	store.session.TotalTokens = 70000
	should, err = s.ShouldSummarize(context.Background(), "session1", 100000)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if should {
		t.Error("Expected not to summarize at 70%")
	}
}

func TestSummarizer_ShouldSummarize_AtThreshold(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			session: &storage.ChatSession{
				ID:          "session1",
				TotalTokens: 80000, // Exactly at 80%
			},
		},
	}

	s := NewSummarizer(store, &summarizerMockCounter{}, &mockLLM{})

	// At exactly 80%, should trigger (>= threshold)
	should, err := s.ShouldSummarize(context.Background(), "session1", 100000)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !should {
		t.Error("Expected should summarize at exactly 80% threshold")
	}
}

func TestSummarizer_ShouldSummarize_Disabled(t *testing.T) {
	s := NewSummarizer(nil, nil, nil) // No LLM

	should, err := s.ShouldSummarize(context.Background(), "session1", 100000)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if should {
		t.Error("Expected should=false when disabled")
	}
}

func TestSummarizer_SummarizeOldMessages_Basic(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Role: "user", Content: "Hello", Timestamp: 1000, TokenCount: 100},
				{ID: "2", Role: "assistant", Content: "Hi there!", Timestamp: 2000, TokenCount: 150},
				{ID: "3", Role: "user", Content: "How are you?", Timestamp: 3000, TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1", TotalTokens: 350},
		},
	}

	llm := &mockLLM{response: "The user greeted the assistant and asked how they were doing."}

	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	err := s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify LLM was called
	if !llm.called {
		t.Error("Expected LLM to be called")
	}

	// Verify summary was saved
	if store.savedSummary == nil {
		t.Fatal("Expected summary to be saved")
	}

	if !store.savedSummary.IsSummary {
		t.Error("Expected IsSummary=true")
	}

	if store.savedSummary.SummaryDepth != 1 {
		t.Errorf("Expected SummaryDepth=1, got %d", store.savedSummary.SummaryDepth)
	}

	// Verify original IDs are recorded
	if len(store.savedOriginals) != 3 {
		t.Errorf("Expected 3 original IDs, got %d", len(store.savedOriginals))
	}

	// Verify summary role is system
	if store.savedSummary.Role != "system" {
		t.Errorf("Expected Role=system, got %s", store.savedSummary.Role)
	}
}

func TestSummarizer_SummarizeOldMessages_Disabled(t *testing.T) {
	s := NewSummarizer(nil, nil, nil) // No LLM

	// Should return nil without error
	err := s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")
	if err != nil {
		t.Fatalf("Expected no error when disabled, got: %v", err)
	}
}

func TestSummarizer_SummarizeOldMessages_NoMessages(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{}, // Empty
			session:  &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	err := s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// LLM should not be called for empty batch
	if llm.called {
		t.Error("LLM should not be called when no messages to summarize")
	}
}

func TestSummarizer_SummarizeOldMessages_MaxDepth(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Content: "Summary of summaries", SummaryDepth: 3, TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Meta summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	// Should skip because depth >= max (3)
	err := s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// LLM should not be called when max depth reached
	if llm.called {
		t.Error("LLM should not be called when max depth reached")
	}
}

func TestSummarizer_SummarizeOldMessages_DepthIncrement(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Content: "Previous summary", SummaryDepth: 1, TokenCount: 100, IsSummary: false},
				{ID: "2", Content: "Another", SummaryDepth: 2, TokenCount: 100, IsSummary: false},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Combined summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	err := s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// New summary depth should be max(1,2) + 1 = 3
	if store.savedSummary.SummaryDepth != 3 {
		t.Errorf("Expected SummaryDepth=3, got %d", store.savedSummary.SummaryDepth)
	}
}

func TestSummarizer_SummarizeOldMessages_LLMError(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Content: "Hello", TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{err: errors.New("LLM unavailable")}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	err := s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")
	if err == nil {
		t.Error("Expected error when LLM fails")
	}

	if !strings.Contains(err.Error(), "failed to generate summary") {
		t.Errorf("Expected error about summary generation, got: %v", err)
	}
}

func TestSummarizer_SummarizeOldMessages_SaveError(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Content: "Hello", TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
		saveSummaryErr: errors.New("database error"),
	}

	llm := &mockLLM{response: "Summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	err := s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")
	if err == nil {
		t.Error("Expected error when save fails")
	}

	if !strings.Contains(err.Error(), "failed to save summary") {
		t.Errorf("Expected error about save failure, got: %v", err)
	}
}

func TestSummarizer_InlineTimestamp(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Timestamp: 3000, TokenCount: 100}, // Not earliest
				{ID: "2", Timestamp: 1000, TokenCount: 100}, // Earliest
				{ID: "3", Timestamp: 2000, TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	err := s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Summary should have earliest timestamp (1000)
	if store.savedSummary.Timestamp != 1000 {
		t.Errorf("Expected timestamp 1000 (inline at earliest), got %d", store.savedSummary.Timestamp)
	}
}

func TestSummarizer_Prompt_NoMetaCommentary(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Role: "user", Content: "Test message", TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")

	// Check prompt includes instruction to avoid meta-commentary
	if !strings.Contains(llm.prompt, "Do NOT include phrases like") {
		t.Error("Prompt should instruct against meta-commentary")
	}

	if !strings.Contains(llm.prompt, "natural context") {
		t.Error("Prompt should request natural context")
	}

	if !strings.Contains(llm.prompt, "Key decisions") {
		t.Error("Prompt should focus on key decisions")
	}
}

func TestSummarizer_Prompt_IncludesMessages(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Role: "user", Content: "Hello Claude", TokenCount: 100},
				{ID: "2", Role: "assistant", Content: "Hello there!", TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")

	// Check prompt includes message content
	if !strings.Contains(llm.prompt, "[User]: Hello Claude") {
		t.Error("Prompt should include user message with role label")
	}

	if !strings.Contains(llm.prompt, "[Assistant]: Hello there!") {
		t.Error("Prompt should include assistant message with role label")
	}
}

func TestSummarizer_Prompt_TruncatesLongMessages(t *testing.T) {
	// Create a very long message
	longContent := strings.Repeat("x", 3000)
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Role: "user", Content: longContent, TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")

	// Check prompt truncates long content (max 2000 chars)
	if len(llm.prompt) > 5000 {
		t.Error("Prompt should truncate very long messages")
	}

	if !strings.Contains(llm.prompt, "[content truncated for summarization]") {
		t.Error("Prompt should indicate truncation")
	}
}

func TestSummarizer_ConfigCustomization(t *testing.T) {
	s := NewSummarizer(nil, nil, nil).WithConfig(SummarizerConfig{
		BatchTokens:      5000,
		TriggerThreshold: 0.70,
		MaxSummaryDepth:  2,
		SummaryTimeout:   10 * time.Second,
	})

	if s.config.BatchTokens != 5000 {
		t.Errorf("Expected BatchTokens=5000, got %d", s.config.BatchTokens)
	}

	if s.config.TriggerThreshold != 0.70 {
		t.Errorf("Expected TriggerThreshold=0.70, got %f", s.config.TriggerThreshold)
	}

	if s.config.MaxSummaryDepth != 2 {
		t.Errorf("Expected MaxSummaryDepth=2, got %d", s.config.MaxSummaryDepth)
	}

	if s.config.SummaryTimeout != 10*time.Second {
		t.Errorf("Expected SummaryTimeout=10s, got %v", s.config.SummaryTimeout)
	}
}

func TestSummarizer_DefaultConfig(t *testing.T) {
	config := DefaultSummarizerConfig()

	if config.BatchTokens != 10000 {
		t.Errorf("Expected default BatchTokens=10000, got %d", config.BatchTokens)
	}

	if config.TriggerThreshold != 0.80 {
		t.Errorf("Expected default TriggerThreshold=0.80, got %f", config.TriggerThreshold)
	}

	if config.MaxSummaryDepth != 3 {
		t.Errorf("Expected default MaxSummaryDepth=3, got %d", config.MaxSummaryDepth)
	}

	if config.SummaryTimeout != 30*time.Second {
		t.Errorf("Expected default SummaryTimeout=30s, got %v", config.SummaryTimeout)
	}
}

func TestSummarizer_SummaryHasUUID(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Content: "Test", TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")

	// Summary should have a UUID
	if store.savedSummary.ID == "" {
		t.Error("Summary should have an ID")
	}

	// UUID format check (8-4-4-4-12 = 36 chars)
	if len(store.savedSummary.ID) != 36 {
		t.Errorf("Expected UUID format (36 chars), got %d chars", len(store.savedSummary.ID))
	}
}

func TestSummarizer_SummaryContent(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Content: "Test", TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "  Summary with whitespace  "}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")

	// Summary content should be trimmed
	if store.savedSummary.Content != "Summary with whitespace" {
		t.Errorf("Expected trimmed content, got: %q", store.savedSummary.Content)
	}
}

func TestSummarizer_SummaryTokenCount(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Content: "Test", TokenCount: 100},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Summary text here"}
	counter := &summarizerMockCounter{tokenCount: 50}
	s := NewSummarizer(store, counter, llm)

	s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")

	// Summary should have token count
	if store.savedSummary.TokenCount != 50 {
		t.Errorf("Expected TokenCount=50, got %d", store.savedSummary.TokenCount)
	}
}

func TestSummarizer_SystemRoleMessages(t *testing.T) {
	store := &summarizeMockStorage{
		mockStorage: mockStorage{
			messages: []storage.ChatMessage{
				{ID: "1", Role: "system", Content: "You are helpful", TokenCount: 100},
				{ID: "2", Role: "user", Content: "Hello", TokenCount: 50},
			},
			session: &storage.ChatSession{ID: "session1"},
		},
	}

	llm := &mockLLM{response: "Summary"}
	s := NewSummarizer(store, &summarizerMockCounter{}, llm)

	s.SummarizeOldMessages(context.Background(), "user1", "session1", "gpt-4")

	// Prompt should include system messages properly labeled
	if !strings.Contains(llm.prompt, "[System]: You are helpful") {
		t.Error("Prompt should include system message with System label")
	}
}
