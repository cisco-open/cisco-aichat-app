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

import "context"

// Storage defines the interface for chat history persistence
type Storage interface {
	// Session operations
	GetSessions(ctx context.Context, userID string) ([]ChatSession, error)
	GetSession(ctx context.Context, userID, sessionID string) (*ChatSession, error)
	CreateSession(ctx context.Context, userID string, session *ChatSession) error
	UpdateSession(ctx context.Context, userID string, session *ChatSession) error
	DeleteSession(ctx context.Context, userID, sessionID string) error
	SetActiveSession(ctx context.Context, userID, sessionID string) error

	// Message operations
	AddMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error
	UpdateMessage(ctx context.Context, userID, sessionID, messageID string, content string) error

	// Bulk operations
	ClearAllHistory(ctx context.Context, userID string) error

	// Retention cleanup - deletes sessions older than retentionDays
	DeleteExpiredSessions(ctx context.Context, retentionDays int) (int64, error)

	// Health check for circuit breaker and monitoring
	Ping(ctx context.Context) error

	// Phase 11: Token-aware message operations
	SaveMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error
	SaveMessages(ctx context.Context, userID, sessionID string, messages []ChatMessage) ([]SaveResult, error)
	GetSessionMessages(ctx context.Context, userID string, params GetSessionMessagesParams) (*MessagesPage, error)
	GetMessagesByTokenBudget(ctx context.Context, sessionID string, budget int) ([]ChatMessage, error)
	UpdateMessageTokenCount(ctx context.Context, messageID string, tokenCount int) error

	// Phase 12: Summary operations for context window management
	SaveSummary(ctx context.Context, userID, sessionID string, summary *ChatMessage, originalIDs []string) error
	// GetOldestNonSummaryMessages retrieves oldest messages that aren't summaries, up to token limit
	GetOldestNonSummaryMessages(ctx context.Context, sessionID string, tokenLimit int) ([]ChatMessage, error)

	// Phase 14: Cross-session search operations
	// SearchMessages searches for messages matching query across all user sessions
	SearchMessages(ctx context.Context, params SearchParams) ([]SearchResult, error)

	// Phase 14: Message pinning for context preservation
	// UpdateMessagePinned toggles or sets the pinned state of a message
	UpdateMessagePinned(ctx context.Context, userID, sessionID, messageID string, isPinned bool) error

	// Lifecycle
	Close() error
}
