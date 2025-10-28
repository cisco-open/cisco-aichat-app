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
	"sort"

	"github.com/grafana/grafana-aichat-app/pkg/storage"
)

// SlidingWindow selects messages based on recency and token budget
type SlidingWindow struct {
	MinMessages int // Minimum messages to include regardless of budget
}

// NewSlidingWindow creates a window with configured minimum messages
func NewSlidingWindow(minMessages int) *SlidingWindow {
	if minMessages < 1 {
		minMessages = 3 // Default per CONTEXT.md
	}
	return &SlidingWindow{MinMessages: minMessages}
}

// SelectMessages returns newest messages fitting budget, output chronologically
// Input: messages in any order (will be sorted by timestamp desc internally)
// Output: selected messages in chronological order (oldest first)
func (sw *SlidingWindow) SelectMessages(messages []storage.ChatMessage, budget int) []storage.ChatMessage {
	if len(messages) == 0 {
		return nil
	}

	// Sort by timestamp descending (newest first) for selection
	sorted := make([]storage.ChatMessage, len(messages))
	copy(sorted, messages)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp > sorted[j].Timestamp
	})

	// Select messages newest-first
	var selected []storage.ChatMessage
	usedTokens := 0

	for i, msg := range sorted {
		// Always include minimum messages even if over budget
		if i < sw.MinMessages {
			selected = append(selected, msg)
			usedTokens += msg.TokenCount
			continue
		}

		// Check budget for remaining messages
		if usedTokens+msg.TokenCount > budget {
			break
		}
		selected = append(selected, msg)
		usedTokens += msg.TokenCount
	}

	// Reverse to chronological order (oldest first for LLM)
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}

	return selected
}

// SumTokens calculates total tokens in a message slice
func SumTokens(messages []storage.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += msg.TokenCount
	}
	return total
}
