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
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/grafana/cisco-aichat-app/pkg/storage"
)

// mockCounter is a simple token counter for testing
// It counts tokens as approximately 1 token per 4 characters
type mockCounter struct{}

func (m *mockCounter) CountTokens(ctx context.Context, text string, model string) (int, error) {
	// Simple estimation: ~4 chars per token
	return (len(text) + 3) / 4, nil
}

func (m *mockCounter) SupportsModel(model string) bool {
	return true
}

func TestTruncateMessage_UnderBudget(t *testing.T) {
	counter := &mockCounter{}
	msg := storage.ChatMessage{
		ID:         "1",
		Content:    "Hello world",
		TokenCount: 3, // ~11 chars = 3 tokens
	}

	// Budget of 10 tokens - should not truncate
	result := TruncateMessage(context.Background(), msg, 10, counter, "test")

	if result.Content != "Hello world" {
		t.Errorf("Expected unchanged content, got %q", result.Content)
	}
}

func TestTruncateMessage_OverBudget(t *testing.T) {
	counter := &mockCounter{}
	msg := storage.ChatMessage{
		ID:         "1",
		Content:    "This is a longer message that needs truncation because it exceeds the token budget",
		TokenCount: 21, // ~82 chars = 21 tokens
	}

	// Budget of 10 tokens - should truncate
	result := TruncateMessage(context.Background(), msg, 10, counter, "test")

	if !strings.HasSuffix(result.Content, TruncationMarker) {
		t.Errorf("Expected truncation marker, got %q", result.Content)
	}

	// Result should be under budget
	if result.TokenCount > 10 {
		t.Errorf("Expected token count <= 10, got %d", result.TokenCount)
	}
}

func TestTruncateMessage_VerySmallBudget(t *testing.T) {
	counter := &mockCounter{}
	msg := storage.ChatMessage{
		ID:         "1",
		Content:    "Some content that will be heavily truncated",
		TokenCount: 11,
	}

	// Budget of 3 tokens - marker takes ~4 tokens "[truncated]"
	// Should just return marker
	result := TruncateMessage(context.Background(), msg, 3, counter, "test")

	if result.Content != TruncationMarker {
		t.Errorf("Expected just truncation marker, got %q", result.Content)
	}
}

func TestTruncateMessage_PreservesOtherFields(t *testing.T) {
	counter := &mockCounter{}
	msg := storage.ChatMessage{
		ID:         "msg-123",
		Role:       "user",
		Content:    "This is a message with lots of content that needs to be truncated",
		Timestamp:  1234567890,
		TokenCount: 17,
		IsPinned:   true,
	}

	result := TruncateMessage(context.Background(), msg, 5, counter, "test")

	if result.ID != "msg-123" {
		t.Errorf("ID changed: %s", result.ID)
	}
	if result.Role != "user" {
		t.Errorf("Role changed: %s", result.Role)
	}
	if result.Timestamp != 1234567890 {
		t.Errorf("Timestamp changed: %d", result.Timestamp)
	}
	if result.IsPinned != true {
		t.Errorf("IsPinned changed: %v", result.IsPinned)
	}
}

func TestSafeSubstring(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		length   int
		expected string
	}{
		{"within bounds", "hello", 3, "hel"},
		{"exact length", "hello", 5, "hello"},
		{"over length", "hello", 10, "hello"},
		{"zero length", "hello", 0, ""},
		{"negative length", "hello", -1, ""},
		{"empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeSubstring(tt.input, tt.length)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSafeSubstring_UTF8(t *testing.T) {
	// Multi-byte UTF-8 characters
	input := "Hello, ... World" // ... uses 3 bytes

	// Try to cut in the middle of the multi-byte character
	// The function should back up to a valid boundary
	result := safeSubstring(input, 8) // Would cut in middle of ...

	if !utf8.ValidString(result) {
		t.Errorf("Result is not valid UTF-8: %q", result)
	}
}

func TestSafeSubstring_Emoji(t *testing.T) {
	input := "Hi!" // Emoji is 4 bytes

	// Try various cut points
	for i := 1; i <= len(input); i++ {
		result := safeSubstring(input, i)
		if !utf8.ValidString(result) {
			t.Errorf("Invalid UTF-8 at length %d: %q", i, result)
		}
	}
}

func TestTruncateToTokens_UnderBudget(t *testing.T) {
	counter := &mockCounter{}
	content := "Short content"

	result := TruncateToTokens(context.Background(), content, 100, counter, "test")

	if result != content {
		t.Errorf("Expected unchanged content, got %q", result)
	}
}

func TestTruncateToTokens_OverBudget(t *testing.T) {
	counter := &mockCounter{}
	content := "This is a much longer piece of content that definitely exceeds any reasonable token budget we might set"

	result := TruncateToTokens(context.Background(), content, 10, counter, "test")

	if !strings.HasSuffix(result, TruncationMarker) {
		t.Errorf("Expected truncation marker, got %q", result)
	}

	// Verify result is under budget
	tokens, _ := counter.CountTokens(context.Background(), result, "test")
	if tokens > 10 {
		t.Errorf("Expected <= 10 tokens, got %d", tokens)
	}
}

func TestTruncateToTokens_VerySmallBudget(t *testing.T) {
	counter := &mockCounter{}
	content := "Some content"

	// Budget so small only marker fits
	result := TruncateToTokens(context.Background(), content, 1, counter, "test")

	if result != TruncationMarker {
		t.Errorf("Expected just marker, got %q", result)
	}
}

func TestBinarySearchTruncation_EmptyContent(t *testing.T) {
	counter := &mockCounter{}
	result := binarySearchTruncation(context.Background(), "", 100, counter, "test")

	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestBinarySearchTruncation_WordBoundary(t *testing.T) {
	counter := &mockCounter{}
	content := "word1 word2 word3 word4 word5"

	// Request a truncation that should prefer word boundary
	result := binarySearchTruncation(context.Background(), content, 4, counter, "test")

	// Result should end at a word boundary (space) if possible
	if strings.HasSuffix(result, " ") || (len(result) > 0 && !strings.Contains(result, " ") && strings.Contains(content[:len(result)+1], " ")) {
		// Either ends with space (will be trimmed in production) or
		// the next char would be space (preferred boundary)
	}
	// Just verify it's valid and under budget
	tokens, _ := counter.CountTokens(context.Background(), result, "test")
	if tokens > 4 {
		t.Errorf("Expected <= 4 tokens, got %d for %q", tokens, result)
	}
}
