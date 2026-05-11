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
	"testing"

	"github.com/grafana/cisco-aichat-app/pkg/storage"
)

func TestNewSlidingWindow(t *testing.T) {
	tests := []struct {
		name        string
		minMessages int
		expected    int
	}{
		{"positive value", 5, 5},
		{"zero defaults to 3", 0, 3},
		{"negative defaults to 3", -1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sw := NewSlidingWindow(tt.minMessages)
			if sw.MinMessages != tt.expected {
				t.Errorf("Expected MinMessages=%d, got %d", tt.expected, sw.MinMessages)
			}
		})
	}
}

func TestSlidingWindow_SelectMessages_Basic(t *testing.T) {
	sw := NewSlidingWindow(2)
	messages := []storage.ChatMessage{
		{ID: "1", Timestamp: 1000, TokenCount: 100},
		{ID: "2", Timestamp: 2000, TokenCount: 100},
		{ID: "3", Timestamp: 3000, TokenCount: 100},
	}

	// Budget for all 3
	selected := sw.SelectMessages(messages, 300)
	if len(selected) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(selected))
	}

	// Verify chronological order (oldest first)
	for i := 1; i < len(selected); i++ {
		if selected[i].Timestamp <= selected[i-1].Timestamp {
			t.Error("Messages not in chronological order")
		}
	}
}

func TestSlidingWindow_SelectMessages_BudgetLimit(t *testing.T) {
	sw := NewSlidingWindow(2)
	messages := []storage.ChatMessage{
		{ID: "1", Timestamp: 1000, TokenCount: 100},
		{ID: "2", Timestamp: 2000, TokenCount: 100},
		{ID: "3", Timestamp: 3000, TokenCount: 100},
	}

	// Budget for only 2 - should get newest 2
	selected := sw.SelectMessages(messages, 200)
	if len(selected) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(selected))
	}
	if selected[0].ID != "2" || selected[1].ID != "3" {
		t.Errorf("Expected messages 2,3, got %s,%s", selected[0].ID, selected[1].ID)
	}
}

func TestSlidingWindow_SelectMessages_MinimumGuarantee(t *testing.T) {
	sw := NewSlidingWindow(3)
	messages := []storage.ChatMessage{
		{ID: "1", Timestamp: 1000, TokenCount: 100},
		{ID: "2", Timestamp: 2000, TokenCount: 100},
		{ID: "3", Timestamp: 3000, TokenCount: 100},
	}

	// Budget for only 1 - but min is 3, so should get all 3
	selected := sw.SelectMessages(messages, 100)
	if len(selected) != 3 {
		t.Errorf("Expected 3 messages (minimum), got %d", len(selected))
	}
}

func TestSlidingWindow_SelectMessages_MinimumWithMixedBudget(t *testing.T) {
	sw := NewSlidingWindow(2)
	messages := []storage.ChatMessage{
		{ID: "1", Timestamp: 1000, TokenCount: 100},
		{ID: "2", Timestamp: 2000, TokenCount: 100},
		{ID: "3", Timestamp: 3000, TokenCount: 100},
		{ID: "4", Timestamp: 4000, TokenCount: 100},
	}

	// Budget for 3 messages (300) - min is 2
	// Should get min 2 (newest: 4, 3) plus 1 more that fits (2)
	// Processed newest first: 4(100), 3(100) = min done, then 2(100) fits (200+100=300)
	selected := sw.SelectMessages(messages, 300)
	if len(selected) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(selected))
	}
	// Should be 2, 3, 4 in chronological order
	if selected[0].ID != "2" || selected[1].ID != "3" || selected[2].ID != "4" {
		t.Errorf("Expected messages 2,3,4, got %s,%s,%s", selected[0].ID, selected[1].ID, selected[2].ID)
	}
}

func TestSlidingWindow_SelectMessages_Empty(t *testing.T) {
	sw := NewSlidingWindow(3)
	selected := sw.SelectMessages(nil, 1000)
	if selected != nil {
		t.Errorf("Expected nil for empty input, got %v", selected)
	}
}

func TestSlidingWindow_SelectMessages_EmptySlice(t *testing.T) {
	sw := NewSlidingWindow(3)
	selected := sw.SelectMessages([]storage.ChatMessage{}, 1000)
	if selected != nil {
		t.Errorf("Expected nil for empty slice, got %v", selected)
	}
}

func TestSlidingWindow_SelectMessages_SingleMessage(t *testing.T) {
	sw := NewSlidingWindow(3)
	messages := []storage.ChatMessage{
		{ID: "1", Timestamp: 1000, TokenCount: 100},
	}

	// Min is 3 but only 1 exists - should return 1
	selected := sw.SelectMessages(messages, 50)
	if len(selected) != 1 {
		t.Errorf("Expected 1 message, got %d", len(selected))
	}
	if selected[0].ID != "1" {
		t.Errorf("Expected message 1, got %s", selected[0].ID)
	}
}

func TestSlidingWindow_SelectMessages_UnsortedInput(t *testing.T) {
	sw := NewSlidingWindow(2)
	// Messages in random order
	messages := []storage.ChatMessage{
		{ID: "3", Timestamp: 3000, TokenCount: 100},
		{ID: "1", Timestamp: 1000, TokenCount: 100},
		{ID: "2", Timestamp: 2000, TokenCount: 100},
	}

	selected := sw.SelectMessages(messages, 300)
	if len(selected) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(selected))
	}

	// Verify output is chronological regardless of input order
	if selected[0].ID != "1" || selected[1].ID != "2" || selected[2].ID != "3" {
		t.Errorf("Expected chronological order 1,2,3, got %s,%s,%s",
			selected[0].ID, selected[1].ID, selected[2].ID)
	}
}

func TestSlidingWindow_SelectMessages_OriginalUnmodified(t *testing.T) {
	sw := NewSlidingWindow(2)
	messages := []storage.ChatMessage{
		{ID: "3", Timestamp: 3000, TokenCount: 100},
		{ID: "1", Timestamp: 1000, TokenCount: 100},
		{ID: "2", Timestamp: 2000, TokenCount: 100},
	}

	// Save original order
	originalFirst := messages[0].ID

	sw.SelectMessages(messages, 300)

	// Original slice should be unchanged
	if messages[0].ID != originalFirst {
		t.Errorf("Original slice was modified: first was %s, now %s", originalFirst, messages[0].ID)
	}
}

func TestSlidingWindow_SelectMessages_ZeroBudget(t *testing.T) {
	sw := NewSlidingWindow(2)
	messages := []storage.ChatMessage{
		{ID: "1", Timestamp: 1000, TokenCount: 100},
		{ID: "2", Timestamp: 2000, TokenCount: 100},
		{ID: "3", Timestamp: 3000, TokenCount: 100},
	}

	// Zero budget - minimum guarantee kicks in
	selected := sw.SelectMessages(messages, 0)
	if len(selected) != 2 {
		t.Errorf("Expected 2 messages (minimum), got %d", len(selected))
	}
	// Should be newest 2: 2 and 3
	if selected[0].ID != "2" || selected[1].ID != "3" {
		t.Errorf("Expected messages 2,3, got %s,%s", selected[0].ID, selected[1].ID)
	}
}

func TestSlidingWindow_SelectMessages_VariableTokenCounts(t *testing.T) {
	sw := NewSlidingWindow(1)
	messages := []storage.ChatMessage{
		{ID: "1", Timestamp: 1000, TokenCount: 50},
		{ID: "2", Timestamp: 2000, TokenCount: 200},
		{ID: "3", Timestamp: 3000, TokenCount: 100},
	}

	// Budget 200: min is 1 (newest, 100 tokens), then check if others fit
	// After min (msg 3, 100 tokens), 100 budget left
	// msg 2 needs 200 - doesn't fit, STOP (pure recency - contiguous window)
	// Pure recency means we don't skip messages to include older ones
	selected := sw.SelectMessages(messages, 200)
	if len(selected) != 1 {
		t.Errorf("Expected 1 message (pure recency stops at first that doesn't fit), got %d", len(selected))
	}
	if selected[0].ID != "3" {
		t.Errorf("Expected message 3, got %s", selected[0].ID)
	}
}

func TestSlidingWindow_SelectMessages_FitsAllInBudget(t *testing.T) {
	sw := NewSlidingWindow(1)
	messages := []storage.ChatMessage{
		{ID: "1", Timestamp: 1000, TokenCount: 50},
		{ID: "2", Timestamp: 2000, TokenCount: 100},
		{ID: "3", Timestamp: 3000, TokenCount: 100},
	}

	// Budget 300: all messages fit
	selected := sw.SelectMessages(messages, 300)
	if len(selected) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(selected))
	}
	// Chronological order
	if selected[0].ID != "1" || selected[1].ID != "2" || selected[2].ID != "3" {
		t.Errorf("Expected messages 1,2,3, got %s,%s,%s", selected[0].ID, selected[1].ID, selected[2].ID)
	}
}

func TestSumTokens(t *testing.T) {
	tests := []struct {
		name     string
		messages []storage.ChatMessage
		expected int
	}{
		{
			"multiple messages",
			[]storage.ChatMessage{
				{TokenCount: 100},
				{TokenCount: 200},
				{TokenCount: 50},
			},
			350,
		},
		{
			"empty slice",
			[]storage.ChatMessage{},
			0,
		},
		{
			"nil slice",
			nil,
			0,
		},
		{
			"single message",
			[]storage.ChatMessage{{TokenCount: 42}},
			42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if total := SumTokens(tt.messages); total != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, total)
			}
		})
	}
}
