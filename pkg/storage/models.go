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

package storage

// BuildSummarizedIDSet returns message IDs covered by summary messages.
func BuildSummarizedIDSet(messages []ChatMessage) map[string]struct{} {
	covered := make(map[string]struct{})
	for _, msg := range messages {
		if !msg.IsSummary || len(msg.SummarizedIDs) == 0 {
			continue
		}
		for _, id := range msg.SummarizedIDs {
			if id == "" {
				continue
			}
			covered[id] = struct{}{}
		}
	}
	return covered
}

// FilterCompactedMessages removes messages that are already covered by summaries.
// This keeps only the active conversation surface while preserving full history on disk.
func FilterCompactedMessages(messages []ChatMessage) []ChatMessage {
	if len(messages) == 0 {
		return messages
	}

	covered := BuildSummarizedIDSet(messages)
	if len(covered) == 0 {
		return messages
	}

	filtered := make([]ChatMessage, 0, len(messages))
	for _, msg := range messages {
		if _, isCovered := covered[msg.ID]; isCovered {
			continue
		}
		filtered = append(filtered, msg)
	}

	return filtered
}

// ChatMessage represents a single message in a chat session
type ChatMessage struct {
	ID         string `json:"id"`
	Role       string `json:"role"` // "user" | "assistant"
	Content    string `json:"content"`
	Timestamp  int64  `json:"timestamp"`  // Unix milliseconds
	TokenCount int    `json:"tokenCount"` // Pre-computed token count for context management
	IsPinned   bool   `json:"isPinned"`   // Preservation flag to prevent pruning
	// Phase 12: Context window management - summarization fields
	IsSummary     bool     `json:"isSummary"`               // True if this is a summary message
	SummarizedIDs []string `json:"summarizedIds,omitempty"` // IDs of messages this summary replaces
	SummaryDepth  int      `json:"summaryDepth"`            // 0 = original, 1 = first summary, 2+ = meta-summary
}

// ChatSession represents a chat conversation session
type ChatSession struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	UserID      string        `json:"userId"`
	Messages    []ChatMessage `json:"messages"`
	CreatedAt   int64         `json:"createdAt"`
	UpdatedAt   int64         `json:"updatedAt"`
	IsActive    bool          `json:"isActive"`
	TotalTokens int           `json:"totalTokens"` // Cumulative token count for context management
}

// UserChatHistory represents all chat history for a user
type UserChatHistory struct {
	UserID   string        `json:"userId"`
	Sessions []ChatSession `json:"sessions"`
}

// PageInfo contains cursor pagination metadata
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"` // Last message ID in current page
}

// MessagesPage represents a page of messages
type MessagesPage struct {
	Messages []ChatMessage `json:"messages"`
	PageInfo PageInfo      `json:"pageInfo"`
}

// GetSessionMessagesParams for paginated message retrieval
type GetSessionMessagesParams struct {
	SessionID string
	Limit     int    // Default 50, max 100
	Cursor    string // Optional: message ID to start after
	Order     string // "asc" or "desc" (default "desc" = newest first)
}

// SaveResult reports outcome of individual message in batch save
type SaveResult struct {
	MessageID string `json:"messageId"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// SearchParams configures message search (Phase 14)
type SearchParams struct {
	UserID string
	Query  string
	Limit  int // Default 50, max 100
	Offset int // For pagination
}

// SearchResult represents a search hit (Phase 14)
type SearchResult struct {
	SessionID   string `json:"sessionId"`
	SessionName string `json:"sessionName"`
	MessageID   string `json:"messageId"`
	Content     string `json:"content"` // Snippet with highlights
	Timestamp   int64  `json:"timestamp"`
	Role        string `json:"role"`
}
