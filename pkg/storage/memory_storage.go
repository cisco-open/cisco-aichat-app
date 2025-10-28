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

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryStorage implements Storage interface using in-memory storage
// This is used as a fallback when database storage fails
// Data is ephemeral and lost on restart - this is intentional for fallback scenarios
type MemoryStorage struct {
	mu      sync.RWMutex
	history map[string]*UserChatHistory // userID -> history
}

// NewMemoryStorage creates a new in-memory storage instance
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		history: make(map[string]*UserChatHistory),
	}
}

// getOrCreateHistory gets or creates a user's history
func (ms *MemoryStorage) getOrCreateHistory(userID string) *UserChatHistory {
	if h, exists := ms.history[userID]; exists {
		return h
	}
	h := &UserChatHistory{
		UserID:   userID,
		Sessions: []ChatSession{},
	}
	ms.history[userID] = h
	return h
}

// GetSessions returns all sessions for a user
func (ms *MemoryStorage) GetSessions(ctx context.Context, userID string) ([]ChatSession, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if h, exists := ms.history[userID]; exists {
		// Return a copy to prevent modification
		sessions := make([]ChatSession, len(h.Sessions))
		copy(sessions, h.Sessions)
		return sessions, nil
	}
	return []ChatSession{}, nil
}

// GetSession returns a specific session
func (ms *MemoryStorage) GetSession(ctx context.Context, userID, sessionID string) (*ChatSession, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if h, exists := ms.history[userID]; exists {
		for i := range h.Sessions {
			if h.Sessions[i].ID == sessionID {
				// Return a copy
				session := h.Sessions[i]
				session.Messages = FilterCompactedMessages(session.Messages)
				return &session, nil
			}
		}
	}
	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// CreateSession creates a new session
func (ms *MemoryStorage) CreateSession(ctx context.Context, userID string, session *ChatSession) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	h := ms.getOrCreateHistory(userID)

	// Check for duplicate
	for _, s := range h.Sessions {
		if s.ID == session.ID {
			return fmt.Errorf("session already exists: %s", session.ID)
		}
	}

	session.UserID = userID
	if session.Messages == nil {
		session.Messages = []ChatMessage{}
	}
	h.Sessions = append(h.Sessions, *session)
	return nil
}

// UpdateSession updates an existing session
func (ms *MemoryStorage) UpdateSession(ctx context.Context, userID string, session *ChatSession) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if h, exists := ms.history[userID]; exists {
		for i := range h.Sessions {
			if h.Sessions[i].ID == session.ID {
				session.UserID = userID
				h.Sessions[i] = *session
				return nil
			}
		}
	}
	return fmt.Errorf("session not found: %s", session.ID)
}

// DeleteSession deletes a session
func (ms *MemoryStorage) DeleteSession(ctx context.Context, userID, sessionID string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if h, exists := ms.history[userID]; exists {
		for i := range h.Sessions {
			if h.Sessions[i].ID == sessionID {
				h.Sessions = append(h.Sessions[:i], h.Sessions[i+1:]...)
				return nil
			}
		}
	}
	return fmt.Errorf("session not found: %s", sessionID)
}

// SetActiveSession sets a session as active
func (ms *MemoryStorage) SetActiveSession(ctx context.Context, userID, sessionID string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if h, exists := ms.history[userID]; exists {
		found := false
		for i := range h.Sessions {
			if h.Sessions[i].ID == sessionID {
				h.Sessions[i].IsActive = true
				found = true
			} else {
				h.Sessions[i].IsActive = false
			}
		}
		if found {
			return nil
		}
	}
	return fmt.Errorf("session not found: %s", sessionID)
}

// AddMessage adds a message to a session
func (ms *MemoryStorage) AddMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if h, exists := ms.history[userID]; exists {
		for i := range h.Sessions {
			if h.Sessions[i].ID == sessionID {
				for _, existing := range h.Sessions[i].Messages {
					if existing.ID == message.ID {
						// Idempotent write: message already exists.
						return nil
					}
				}
				h.Sessions[i].Messages = append(h.Sessions[i].Messages, *message)
				h.Sessions[i].UpdatedAt = message.Timestamp
				h.Sessions[i].TotalTokens += message.TokenCount
				return nil
			}
		}
	}
	return fmt.Errorf("session not found: %s", sessionID)
}

// UpdateMessage updates a message's content
func (ms *MemoryStorage) UpdateMessage(ctx context.Context, userID, sessionID, messageID string, content string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if h, exists := ms.history[userID]; exists {
		for i := range h.Sessions {
			if h.Sessions[i].ID == sessionID {
				for j := range h.Sessions[i].Messages {
					if h.Sessions[i].Messages[j].ID == messageID {
						h.Sessions[i].Messages[j].Content = content
						h.Sessions[i].Messages[j].TokenCount = 0
						h.Sessions[i].UpdatedAt = time.Now().UnixMilli()
						return nil
					}
				}
				return fmt.Errorf("message not found: %s", messageID)
			}
		}
	}
	return fmt.Errorf("session not found: %s", sessionID)
}

// ClearAllHistory clears all history for a user
func (ms *MemoryStorage) ClearAllHistory(ctx context.Context, userID string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.history[userID] = &UserChatHistory{
		UserID:   userID,
		Sessions: []ChatSession{},
	}
	return nil
}

// DeleteExpiredSessions deletes sessions older than retentionDays
func (ms *MemoryStorage) DeleteExpiredSessions(ctx context.Context, retentionDays int) (int64, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	cutoffMs := (time.Now().Unix() - int64(retentionDays*24*60*60)) * 1000
	var totalDeleted int64

	for _, h := range ms.history {
		activeSessions := make([]ChatSession, 0, len(h.Sessions))
		for _, session := range h.Sessions {
			if session.UpdatedAt >= cutoffMs {
				activeSessions = append(activeSessions, session)
			} else {
				totalDeleted++
			}
		}
		h.Sessions = activeSessions
	}

	return totalDeleted, nil
}

// Ping always returns nil for memory storage
func (ms *MemoryStorage) Ping(ctx context.Context) error {
	return nil
}

// SaveMessage stores a message (alias for AddMessage)
func (ms *MemoryStorage) SaveMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if h, exists := ms.history[userID]; exists {
		for i := range h.Sessions {
			if h.Sessions[i].ID == sessionID {
				for _, existing := range h.Sessions[i].Messages {
					if existing.ID == message.ID {
						// Idempotent write: message already exists.
						return nil
					}
				}
				h.Sessions[i].Messages = append(h.Sessions[i].Messages, *message)
				h.Sessions[i].UpdatedAt = message.Timestamp
				h.Sessions[i].TotalTokens += message.TokenCount
				return nil
			}
		}
	}
	return fmt.Errorf("session not found: %s", sessionID)
}

// SaveMessages stores multiple messages with partial success tracking
func (ms *MemoryStorage) SaveMessages(ctx context.Context, userID, sessionID string, messages []ChatMessage) ([]SaveResult, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	results := make([]SaveResult, len(messages))

	if len(messages) == 0 {
		return results, nil
	}

	// Find session
	h, exists := ms.history[userID]
	if !exists {
		for i, msg := range messages {
			results[i] = SaveResult{MessageID: msg.ID, Success: false, Error: "session not found"}
		}
		return results, fmt.Errorf("session not found: %s", sessionID)
	}

	var sessionIdx int = -1
	for i := range h.Sessions {
		if h.Sessions[i].ID == sessionID {
			sessionIdx = i
			break
		}
	}

	if sessionIdx == -1 {
		for i, msg := range messages {
			results[i] = SaveResult{MessageID: msg.ID, Success: false, Error: "session not found"}
		}
		return results, fmt.Errorf("session not found: %s", sessionID)
	}

	// Track existing IDs for idempotent batch writes.
	existingIDs := make(map[string]struct{}, len(h.Sessions[sessionIdx].Messages))
	for _, existing := range h.Sessions[sessionIdx].Messages {
		existingIDs[existing.ID] = struct{}{}
	}

	// Add messages
	for i, msg := range messages {
		if _, exists := existingIDs[msg.ID]; exists {
			results[i] = SaveResult{MessageID: msg.ID, Success: true}
			continue
		}

		h.Sessions[sessionIdx].Messages = append(h.Sessions[sessionIdx].Messages, msg)
		h.Sessions[sessionIdx].TotalTokens += msg.TokenCount
		if msg.Timestamp > h.Sessions[sessionIdx].UpdatedAt {
			h.Sessions[sessionIdx].UpdatedAt = msg.Timestamp
		}
		results[i] = SaveResult{MessageID: msg.ID, Success: true}
		existingIDs[msg.ID] = struct{}{}
	}

	return results, nil
}

// GetSessionMessages returns paginated messages for a session
func (ms *MemoryStorage) GetSessionMessages(ctx context.Context, userID string, params GetSessionMessagesParams) (*MessagesPage, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Find session
	h, exists := ms.history[userID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	var session *ChatSession
	for i := range h.Sessions {
		if h.Sessions[i].ID == params.SessionID {
			session = &h.Sessions[i]
			break
		}
	}

	if session == nil {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	// Apply defaults
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	order := params.Order
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	// Copy messages for sorting
	messages := make([]ChatMessage, len(session.Messages))
	copy(messages, session.Messages)

	// Sort by timestamp
	if order == "desc" {
		// Sort newest first
		for i := 0; i < len(messages)-1; i++ {
			for j := i + 1; j < len(messages); j++ {
				if messages[j].Timestamp > messages[i].Timestamp {
					messages[i], messages[j] = messages[j], messages[i]
				}
			}
		}
	}
	// asc order is natural (oldest first)

	// Find cursor position
	startIdx := 0
	if params.Cursor != "" {
		for i, msg := range messages {
			if msg.ID == params.Cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// Slice messages
	endIdx := startIdx + limit + 1
	if endIdx > len(messages) {
		endIdx = len(messages)
	}

	pageMessages := messages[startIdx:endIdx]

	// Determine pagination info
	hasNextPage := len(pageMessages) > limit
	if hasNextPage {
		pageMessages = pageMessages[:limit]
	}

	var endCursor string
	if len(pageMessages) > 0 {
		endCursor = pageMessages[len(pageMessages)-1].ID
	}

	return &MessagesPage{
		Messages: pageMessages,
		PageInfo: PageInfo{
			HasNextPage: hasNextPage,
			EndCursor:   endCursor,
		},
	}, nil
}

// GetMessagesByTokenBudget retrieves messages fitting within token limit
func (ms *MemoryStorage) GetMessagesByTokenBudget(ctx context.Context, sessionID string, budget int) ([]ChatMessage, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Find session across all users
	var session *ChatSession
	for _, h := range ms.history {
		for i := range h.Sessions {
			if h.Sessions[i].ID == sessionID {
				session = &h.Sessions[i]
				break
			}
		}
		if session != nil {
			break
		}
	}

	if session == nil {
		return []ChatMessage{}, nil
	}

	// Copy messages and sort newest first
	messages := make([]ChatMessage, len(session.Messages))
	copy(messages, session.Messages)
	messages = FilterCompactedMessages(messages)
	for i := 0; i < len(messages)-1; i++ {
		for j := i + 1; j < len(messages); j++ {
			if messages[j].Timestamp > messages[i].Timestamp {
				messages[i], messages[j] = messages[j], messages[i]
			}
		}
	}

	// First pass: include all pinned messages
	var selected []ChatMessage
	remainingBudget := budget

	for _, msg := range messages {
		if msg.IsPinned {
			selected = append(selected, msg)
			remainingBudget -= msg.TokenCount
		}
	}

	// Second pass: add unpinned messages newest-to-oldest until budget exhausted
	for _, msg := range messages {
		if !msg.IsPinned && remainingBudget >= msg.TokenCount {
			selected = append(selected, msg)
			remainingBudget -= msg.TokenCount
		}
	}

	// Sort by timestamp ascending (oldest first) for LLM context
	for i := 0; i < len(selected)-1; i++ {
		for j := i + 1; j < len(selected); j++ {
			if selected[j].Timestamp < selected[i].Timestamp {
				selected[i], selected[j] = selected[j], selected[i]
			}
		}
	}

	return selected, nil
}

// UpdateMessageTokenCount updates the token count for a message
func (ms *MemoryStorage) UpdateMessageTokenCount(ctx context.Context, messageID string, tokenCount int) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for _, h := range ms.history {
		for i := range h.Sessions {
			for j := range h.Sessions[i].Messages {
				if h.Sessions[i].Messages[j].ID == messageID {
					h.Sessions[i].Messages[j].TokenCount = tokenCount
					return nil
				}
			}
		}
	}

	return fmt.Errorf("message not found: %s", messageID)
}

// GetOldestNonSummaryMessages retrieves oldest messages that aren't summaries, up to token limit
func (ms *MemoryStorage) GetOldestNonSummaryMessages(ctx context.Context, sessionID string, tokenLimit int) ([]ChatMessage, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Find session across all users
	var session *ChatSession
	for _, h := range ms.history {
		for i := range h.Sessions {
			if h.Sessions[i].ID == sessionID {
				session = &h.Sessions[i]
				break
			}
		}
		if session != nil {
			break
		}
	}

	if session == nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	covered := BuildSummarizedIDSet(session.Messages)

	// Filter non-summary messages and sort by timestamp (oldest first)
	var nonSummary []ChatMessage
	for _, msg := range session.Messages {
		if _, isCovered := covered[msg.ID]; isCovered {
			continue
		}
		if !msg.IsSummary {
			nonSummary = append(nonSummary, msg)
		}
	}

	// Sort by timestamp ascending (oldest first)
	for i := 0; i < len(nonSummary)-1; i++ {
		for j := i + 1; j < len(nonSummary); j++ {
			if nonSummary[j].Timestamp < nonSummary[i].Timestamp {
				nonSummary[i], nonSummary[j] = nonSummary[j], nonSummary[i]
			}
		}
	}

	// Accumulate messages until token limit
	var result []ChatMessage
	totalTokens := 0
	for _, msg := range nonSummary {
		if totalTokens+msg.TokenCount > tokenLimit && len(result) > 0 {
			break
		}
		result = append(result, msg)
		totalTokens += msg.TokenCount
	}

	return result, nil
}

// SaveSummary stores a summary message and links it to original messages
func (ms *MemoryStorage) SaveSummary(ctx context.Context, userID, sessionID string, summary *ChatMessage, originalIDs []string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Find session
	h, exists := ms.history[userID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	var sessionIdx int = -1
	for i := range h.Sessions {
		if h.Sessions[i].ID == sessionID {
			sessionIdx = i
			break
		}
	}

	if sessionIdx == -1 {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	// Calculate summary depth from original messages
	var maxDepth int
	for _, origID := range originalIDs {
		for _, msg := range h.Sessions[sessionIdx].Messages {
			if msg.ID == origID && msg.SummaryDepth > maxDepth {
				maxDepth = msg.SummaryDepth
			}
		}
	}

	// Set summary metadata
	summary.IsSummary = true
	summary.SummarizedIDs = originalIDs
	summary.SummaryDepth = maxDepth + 1

	// Add summary message
	h.Sessions[sessionIdx].Messages = append(h.Sessions[sessionIdx].Messages, *summary)
	h.Sessions[sessionIdx].UpdatedAt = summary.Timestamp
	h.Sessions[sessionIdx].TotalTokens += summary.TokenCount

	return nil
}

// SearchMessages performs in-memory substring search across all user sessions
// Phase 14: This is a basic implementation for fallback - no FTS capabilities
func (ms *MemoryStorage) SearchMessages(ctx context.Context, params SearchParams) ([]SearchResult, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var results []SearchResult

	h, exists := ms.history[params.UserID]
	if !exists {
		return results, nil
	}

	// Simple substring search (no FTS capabilities in memory storage)
	query := strings.ToLower(params.Query)
	count := 0

	for _, session := range h.Sessions {
		for _, msg := range session.Messages {
			if strings.Contains(strings.ToLower(msg.Content), query) {
				// Create snippet with match context
				snippet := createSnippet(msg.Content, params.Query, 64)
				results = append(results, SearchResult{
					SessionID:   session.ID,
					SessionName: session.Name,
					MessageID:   msg.ID,
					Content:     snippet,
					Timestamp:   msg.Timestamp,
					Role:        msg.Role,
				})
				count++
				if params.Limit > 0 && count >= params.Limit {
					return results, nil
				}
			}
		}
	}

	// Apply offset
	if params.Offset > 0 && params.Offset < len(results) {
		results = results[params.Offset:]
	} else if params.Offset >= len(results) {
		return []SearchResult{}, nil
	}

	return results, nil
}

// createSnippet creates a text snippet around the first match
func createSnippet(content, query string, maxLen int) string {
	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerContent, lowerQuery)
	if idx == -1 {
		if len(content) > maxLen {
			return content[:maxLen] + "..."
		}
		return content
	}

	// Calculate window around match
	start := idx - 20
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 40
	if end > len(content) {
		end = len(content)
	}

	snippet := ""
	if start > 0 {
		snippet = "..."
	}
	// Add highlight markers around match
	matchStart := idx - start
	matchEnd := matchStart + len(query)
	snippetContent := content[start:end]
	if matchStart >= 0 && matchEnd <= len(snippetContent) {
		snippet += snippetContent[:matchStart] + "<mark>" + snippetContent[matchStart:matchEnd] + "</mark>" + snippetContent[matchEnd:]
	} else {
		snippet += snippetContent
	}
	if end < len(content) {
		snippet += "..."
	}

	return snippet
}

// UpdateMessagePinned updates the pinned state of a message
// Phase 14: Message pinning for context preservation
func (ms *MemoryStorage) UpdateMessagePinned(ctx context.Context, userID, sessionID, messageID string, isPinned bool) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	h, exists := ms.history[userID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	for i := range h.Sessions {
		if h.Sessions[i].ID == sessionID {
			for j := range h.Sessions[i].Messages {
				if h.Sessions[i].Messages[j].ID == messageID {
					h.Sessions[i].Messages[j].IsPinned = isPinned
					return nil
				}
			}
			return fmt.Errorf("message not found: %s", messageID)
		}
	}

	return fmt.Errorf("session not found: %s", sessionID)
}

// Close is a no-op for memory storage
func (ms *MemoryStorage) Close() error {
	return nil
}
