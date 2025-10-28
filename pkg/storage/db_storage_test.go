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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestDBStorage creates a temporary SQLite database for testing
func createTestDBStorage(t *testing.T) (*DBStorage, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dbstorage-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	dbURL := "file:" + dbPath

	storage, err := NewDBStorage(dbURL, log.DefaultLogger)
	require.NoError(t, err)

	cleanup := func() {
		storage.Close()
		os.RemoveAll(tmpDir)
	}

	return storage, cleanup
}

func TestDBStorage_CRUD(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"

	// Create session
	now := time.Now().UnixMilli()
	session := &ChatSession{
		ID:          "session-1",
		Name:        "Test Session",
		CreatedAt:   now,
		UpdatedAt:   now,
		IsActive:    true,
		TotalTokens: 0,
		Messages:    []ChatMessage{},
	}

	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Get session
	retrieved, err := db.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.Name, retrieved.Name)
	assert.Equal(t, userID, retrieved.UserID)
	assert.True(t, retrieved.IsActive)

	// Update session
	session.Name = "Updated Session"
	session.TotalTokens = 100
	err = db.UpdateSession(ctx, userID, session)
	require.NoError(t, err)

	// Verify update
	retrieved, err = db.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Session", retrieved.Name)
	assert.Equal(t, 100, retrieved.TotalTokens)

	// Delete session
	err = db.DeleteSession(ctx, userID, session.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = db.GetSession(ctx, userID, session.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDBStorage_Messages(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"

	// Create session
	now := time.Now().UnixMilli()
	session := &ChatSession{
		ID:        "session-msg",
		Name:      "Message Test",
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []ChatMessage{},
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Add messages
	msg1 := &ChatMessage{
		ID:         "msg-1",
		Role:       "user",
		Content:    "Hello",
		Timestamp:  now,
		TokenCount: 5,
		IsPinned:   false,
	}
	err = db.AddMessage(ctx, userID, session.ID, msg1)
	require.NoError(t, err)

	msg2 := &ChatMessage{
		ID:         "msg-2",
		Role:       "assistant",
		Content:    "Hi there!",
		Timestamp:  now + 1000,
		TokenCount: 10,
		IsPinned:   true,
	}
	err = db.AddMessage(ctx, userID, session.ID, msg2)
	require.NoError(t, err)

	// Get session and verify messages
	retrieved, err := db.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	require.Len(t, retrieved.Messages, 2)

	// Verify message order (by timestamp)
	assert.Equal(t, "msg-1", retrieved.Messages[0].ID)
	assert.Equal(t, "msg-2", retrieved.Messages[1].ID)

	// Verify message content
	assert.Equal(t, "user", retrieved.Messages[0].Role)
	assert.Equal(t, "Hello", retrieved.Messages[0].Content)
	assert.Equal(t, 5, retrieved.Messages[0].TokenCount)
	assert.False(t, retrieved.Messages[0].IsPinned)

	assert.Equal(t, "assistant", retrieved.Messages[1].Role)
	assert.Equal(t, "Hi there!", retrieved.Messages[1].Content)
	assert.Equal(t, 10, retrieved.Messages[1].TokenCount)
	assert.True(t, retrieved.Messages[1].IsPinned)

	// Verify total tokens updated
	assert.Equal(t, 15, retrieved.TotalTokens)
}

func TestDBStorage_AddMessage_IdempotentNoTokenInflation(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	session := &ChatSession{
		ID:        "session-idempotent",
		Name:      "Idempotent Test",
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []ChatMessage{},
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	msg := &ChatMessage{
		ID:         "msg-duplicate",
		Role:       "assistant",
		Content:    "same message",
		Timestamp:  now + 1,
		TokenCount: 42,
	}

	err = db.AddMessage(ctx, userID, session.ID, msg)
	require.NoError(t, err)

	// Duplicate write should be a no-op.
	err = db.AddMessage(ctx, userID, session.ID, msg)
	require.NoError(t, err)

	retrieved, err := db.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	require.Len(t, retrieved.Messages, 1)
	assert.Equal(t, 42, retrieved.TotalTokens)
}

func TestDBStorage_SetActiveSession(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create multiple sessions
	for i := 1; i <= 3; i++ {
		session := &ChatSession{
			ID:        "session-" + string(rune('0'+i)),
			Name:      "Session " + string(rune('0'+i)),
			CreatedAt: now,
			UpdatedAt: now,
			IsActive:  false,
		}
		err := db.CreateSession(ctx, userID, session)
		require.NoError(t, err)
	}

	// Set session-2 as active
	err := db.SetActiveSession(ctx, userID, "session-2")
	require.NoError(t, err)

	// Verify only session-2 is active
	sessions, err := db.GetSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 3)

	for _, s := range sessions {
		if s.ID == "session-2" {
			assert.True(t, s.IsActive, "session-2 should be active")
		} else {
			assert.False(t, s.IsActive, "%s should not be active", s.ID)
		}
	}

	// Set session-3 as active
	err = db.SetActiveSession(ctx, userID, "session-3")
	require.NoError(t, err)

	// Verify only session-3 is now active
	sessions, err = db.GetSessions(ctx, userID)
	require.NoError(t, err)

	for _, s := range sessions {
		if s.ID == "session-3" {
			assert.True(t, s.IsActive, "session-3 should be active")
		} else {
			assert.False(t, s.IsActive, "%s should not be active", s.ID)
		}
	}
}

func TestDBStorage_DeleteExpiredSessions(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create old session (10 days ago)
	oldTime := now - int64(10*24*60*60*1000)
	oldSession := &ChatSession{
		ID:        "old-session",
		Name:      "Old Session",
		CreatedAt: oldTime,
		UpdatedAt: oldTime,
	}
	err := db.CreateSession(ctx, userID, oldSession)
	require.NoError(t, err)

	// Create recent session
	recentSession := &ChatSession{
		ID:        "recent-session",
		Name:      "Recent Session",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err = db.CreateSession(ctx, userID, recentSession)
	require.NoError(t, err)

	// Verify both exist
	sessions, err := db.GetSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// Delete sessions older than 7 days
	deleted, err := db.DeleteExpiredSessions(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Verify only recent session remains
	sessions, err = db.GetSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "recent-session", sessions[0].ID)
}

func TestDBStorage_ClearAllHistory(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	user1 := "user-1"
	user2 := "user-2"
	now := time.Now().UnixMilli()

	// Create sessions for user1
	for i := 1; i <= 3; i++ {
		session := &ChatSession{
			ID:        "u1-session-" + string(rune('0'+i)),
			Name:      "Session",
			CreatedAt: now,
			UpdatedAt: now,
		}
		err := db.CreateSession(ctx, user1, session)
		require.NoError(t, err)
	}

	// Create sessions for user2
	for i := 1; i <= 2; i++ {
		session := &ChatSession{
			ID:        "u2-session-" + string(rune('0'+i)),
			Name:      "Session",
			CreatedAt: now,
			UpdatedAt: now,
		}
		err := db.CreateSession(ctx, user2, session)
		require.NoError(t, err)
	}

	// Clear user1's history
	err := db.ClearAllHistory(ctx, user1)
	require.NoError(t, err)

	// Verify user1 has no sessions
	sessions, err := db.GetSessions(ctx, user1)
	require.NoError(t, err)
	assert.Len(t, sessions, 0)

	// Verify user2's sessions are unaffected
	sessions, err = db.GetSessions(ctx, user2)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestDBStorage_Ping(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	err := db.Ping(ctx)
	assert.NoError(t, err)
}

func TestDBStorage_UpdateMessage(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session with message
	session := &ChatSession{
		ID:        "session-edit",
		Name:      "Edit Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	msg := &ChatMessage{
		ID:        "msg-edit",
		Role:      "user",
		Content:   "Original content",
		Timestamp: now,
	}
	err = db.AddMessage(ctx, userID, session.ID, msg)
	require.NoError(t, err)

	// Update message
	err = db.UpdateMessage(ctx, userID, session.ID, msg.ID, "Updated content")
	require.NoError(t, err)

	// Verify update
	retrieved, err := db.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	require.Len(t, retrieved.Messages, 1)
	assert.Equal(t, "Updated content", retrieved.Messages[0].Content)
}

func TestResilientStorage_Fallback(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create a mock primary that always fails
	mockPrimary := &failingStorage{failCount: 0}
	fallback := NewMemoryStorage()

	rs := NewResilientStorage(mockPrimary, fallback, log.DefaultLogger)

	// First few operations should trigger failures but still work via fallback
	session := &ChatSession{
		ID:        "resilient-session",
		Name:      "Resilient Test",
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Operations should succeed via fallback
	for i := 0; i < 6; i++ {
		// Each operation fails on primary, succeeds on fallback
		// After 5 failures, circuit should open
	}

	// Create session - may fail on primary but succeed on fallback
	err := rs.CreateSession(ctx, userID, session)
	assert.NoError(t, err)

	// After multiple failures, should be in degraded mode
	// Force more failures by trying operations
	for i := 0; i < 5; i++ {
		rs.GetSessions(ctx, userID)
	}

	// Should be degraded after threshold failures
	assert.True(t, rs.IsDegraded())

	// Operations should still work via fallback
	sessions, err := rs.GetSessions(ctx, userID)
	assert.NoError(t, err)
	assert.Len(t, sessions, 1)
}

// failingStorage is a mock that always fails
type failingStorage struct {
	failCount int
}

func (f *failingStorage) GetSessions(ctx context.Context, userID string) ([]ChatSession, error) {
	f.failCount++
	return nil, assert.AnError
}

func (f *failingStorage) GetSession(ctx context.Context, userID, sessionID string) (*ChatSession, error) {
	f.failCount++
	return nil, assert.AnError
}

func (f *failingStorage) CreateSession(ctx context.Context, userID string, session *ChatSession) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) UpdateSession(ctx context.Context, userID string, session *ChatSession) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) DeleteSession(ctx context.Context, userID, sessionID string) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) SetActiveSession(ctx context.Context, userID, sessionID string) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) AddMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) UpdateMessage(ctx context.Context, userID, sessionID, messageID string, content string) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) ClearAllHistory(ctx context.Context, userID string) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) DeleteExpiredSessions(ctx context.Context, retentionDays int) (int64, error) {
	f.failCount++
	return 0, assert.AnError
}

func (f *failingStorage) Ping(ctx context.Context) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) SaveMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) SaveMessages(ctx context.Context, userID, sessionID string, messages []ChatMessage) ([]SaveResult, error) {
	f.failCount++
	return nil, assert.AnError
}

func (f *failingStorage) GetSessionMessages(ctx context.Context, userID string, params GetSessionMessagesParams) (*MessagesPage, error) {
	f.failCount++
	return nil, assert.AnError
}

func (f *failingStorage) GetMessagesByTokenBudget(ctx context.Context, sessionID string, budget int) ([]ChatMessage, error) {
	f.failCount++
	return nil, assert.AnError
}

func (f *failingStorage) UpdateMessageTokenCount(ctx context.Context, messageID string, tokenCount int) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) SaveSummary(ctx context.Context, userID, sessionID string, summary *ChatMessage, originalIDs []string) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) GetOldestNonSummaryMessages(ctx context.Context, sessionID string, tokenLimit int) ([]ChatMessage, error) {
	f.failCount++
	return nil, assert.AnError
}

func (f *failingStorage) SearchMessages(ctx context.Context, params SearchParams) ([]SearchResult, error) {
	f.failCount++
	return nil, assert.AnError
}

func (f *failingStorage) UpdateMessagePinned(ctx context.Context, userID, sessionID, messageID string, isPinned bool) error {
	f.failCount++
	return assert.AnError
}

func (f *failingStorage) Close() error {
	return nil
}

func TestCleanupScheduler_RunsOnStart(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cleanup-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "cleanup-test.db")
	dbURL := "file:" + dbPath

	db, err := NewDBStorage(dbURL, log.DefaultLogger)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create old session (10 days ago)
	oldTime := now - int64(10*24*60*60*1000)
	oldSession := &ChatSession{
		ID:        "old-cleanup-session",
		Name:      "Old Session",
		CreatedAt: oldTime,
		UpdatedAt: oldTime,
	}
	err = db.CreateSession(ctx, userID, oldSession)
	require.NoError(t, err)

	// Create recent session
	recentSession := &ChatSession{
		ID:        "recent-cleanup-session",
		Name:      "Recent Session",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err = db.CreateSession(ctx, userID, recentSession)
	require.NoError(t, err)

	// Verify both exist
	sessions, err := db.GetSessions(ctx, userID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// Create and start cleanup scheduler with 7-day retention
	scheduler := NewCleanupScheduler(db, 7, log.DefaultLogger)
	scheduler.Start()

	// Wait for cleanup to run
	time.Sleep(500 * time.Millisecond)

	scheduler.Stop()

	// Verify old session was cleaned up
	sessions, err = db.GetSessions(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, "recent-cleanup-session", sessions[0].ID)
}

func TestMemoryStorage_AllOperations(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	ms := NewMemoryStorage()

	// Create session
	session := &ChatSession{
		ID:        "mem-session",
		Name:      "Memory Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := ms.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Get sessions
	sessions, err := ms.GetSessions(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)

	// Add message
	msg := &ChatMessage{
		ID:         "mem-msg",
		Role:       "user",
		Content:    "Hello",
		Timestamp:  now,
		TokenCount: 5,
	}
	err = ms.AddMessage(ctx, userID, session.ID, msg)
	require.NoError(t, err)

	// Get session with message
	retrieved, err := ms.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	assert.Len(t, retrieved.Messages, 1)
	assert.Equal(t, 5, retrieved.TotalTokens)

	// Update message
	err = ms.UpdateMessage(ctx, userID, session.ID, msg.ID, "Updated")
	require.NoError(t, err)

	retrieved, err = ms.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", retrieved.Messages[0].Content)

	// Set active session
	err = ms.SetActiveSession(ctx, userID, session.ID)
	require.NoError(t, err)

	retrieved, err = ms.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	assert.True(t, retrieved.IsActive)

	// Delete session
	err = ms.DeleteSession(ctx, userID, session.ID)
	require.NoError(t, err)

	sessions, err = ms.GetSessions(ctx, userID)
	require.NoError(t, err)
	assert.Len(t, sessions, 0)

	// Ping should always succeed
	err = ms.Ping(ctx)
	require.NoError(t, err)
}

func TestFactory_DefaultsToFileStorage(t *testing.T) {
	// Ensure no AICHAT_DATABASE_URL is set
	os.Unsetenv("AICHAT_DATABASE_URL")

	tmpDir, err := os.MkdirTemp("", "factory-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	storage, err := NewStorage(tmpDir, log.DefaultLogger)
	require.NoError(t, err)
	defer storage.Close()

	// Should be FileStorage (not ResilientStorage)
	_, isFile := storage.(*FileStorage)
	assert.True(t, isFile, "Expected FileStorage when no AICHAT_DATABASE_URL")
}

func TestFactory_UsesDatabaseURL(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "factory-db-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Set database URL
	dbPath := filepath.Join(tmpDir, "factory-test.db")
	t.Setenv("AICHAT_DATABASE_URL", "file:"+dbPath)

	storage, err := NewStorage(tmpDir, log.DefaultLogger)
	require.NoError(t, err)
	defer storage.Close()

	// Strict consistency mode is default: DBStorage without runtime memory fallback.
	_, isDBStorage := storage.(*DBStorage)
	assert.True(t, isDBStorage, "Expected DBStorage when AICHAT_DATABASE_URL is set")
}

func TestFactory_RuntimeFallbackOptIn(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "factory-db-fallback-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "factory-fallback.db")
	t.Setenv("AICHAT_DATABASE_URL", "file:"+dbPath)
	t.Setenv("AICHAT_ENABLE_RUNTIME_MEMORY_FALLBACK", "true")

	storage, err := NewStorage(tmpDir, log.DefaultLogger)
	require.NoError(t, err)
	defer storage.Close()

	_, isResilient := storage.(*ResilientStorage)
	assert.True(t, isResilient, "Expected ResilientStorage when runtime memory fallback is enabled")
}

func TestDBStorage_SaveMessage(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session
	session := &ChatSession{
		ID:        "save-msg-session",
		Name:      "SaveMessage Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// SaveMessage is alias for AddMessage
	msg := &ChatMessage{
		ID:         "save-msg-1",
		Role:       "user",
		Content:    "Test content",
		Timestamp:  now,
		TokenCount: 25,
		IsPinned:   false,
	}
	err = db.SaveMessage(ctx, userID, session.ID, msg)
	require.NoError(t, err)

	// Verify message saved
	retrieved, err := db.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	require.Len(t, retrieved.Messages, 1)
	assert.Equal(t, 25, retrieved.Messages[0].TokenCount)
	assert.Equal(t, 25, retrieved.TotalTokens)
}

func TestDBStorage_SaveMessages(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session
	session := &ChatSession{
		ID:        "batch-save-session",
		Name:      "Batch Save Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Batch save messages
	messages := []ChatMessage{
		{ID: "batch-1", Role: "user", Content: "Message 1", Timestamp: now, TokenCount: 10},
		{ID: "batch-2", Role: "assistant", Content: "Message 2", Timestamp: now + 1000, TokenCount: 20},
		{ID: "batch-3", Role: "user", Content: "Message 3", Timestamp: now + 2000, TokenCount: 15},
	}

	results, err := db.SaveMessages(ctx, userID, session.ID, messages)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// All should succeed
	for i, result := range results {
		assert.True(t, result.Success, "Message %d should succeed", i)
		assert.Equal(t, messages[i].ID, result.MessageID)
		assert.Empty(t, result.Error)
	}

	// Verify messages saved
	retrieved, err := db.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	assert.Len(t, retrieved.Messages, 3)
	assert.Equal(t, 45, retrieved.TotalTokens) // 10 + 20 + 15
}

func TestDBStorage_SaveMessages_SessionNotFound(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"

	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Test", Timestamp: time.Now().UnixMilli()},
	}

	results, err := db.SaveMessages(ctx, userID, "nonexistent-session", messages)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")

	// Results should indicate failure
	require.Len(t, results, 1)
	assert.False(t, results[0].Success)
}

func TestDBStorage_GetSessionMessages_Pagination(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session
	session := &ChatSession{
		ID:        "pagination-session",
		Name:      "Pagination Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Add 10 messages
	for i := 0; i < 10; i++ {
		msg := &ChatMessage{
			ID:        "page-msg-" + string(rune('0'+i)),
			Role:      "user",
			Content:   "Message " + string(rune('0'+i)),
			Timestamp: now + int64(i*1000),
		}
		err = db.AddMessage(ctx, userID, session.ID, msg)
		require.NoError(t, err)
	}

	// Get first page (limit 3)
	params := GetSessionMessagesParams{
		SessionID: session.ID,
		Limit:     3,
		Order:     "desc", // Newest first
	}
	page1, err := db.GetSessionMessages(ctx, userID, params)
	require.NoError(t, err)

	assert.Len(t, page1.Messages, 3)
	assert.True(t, page1.PageInfo.HasNextPage)
	assert.NotEmpty(t, page1.PageInfo.EndCursor)

	// Verify order (newest first)
	assert.Equal(t, "page-msg-9", page1.Messages[0].ID)
	assert.Equal(t, "page-msg-8", page1.Messages[1].ID)
	assert.Equal(t, "page-msg-7", page1.Messages[2].ID)

	// Get second page using cursor
	params.Cursor = page1.PageInfo.EndCursor
	page2, err := db.GetSessionMessages(ctx, userID, params)
	require.NoError(t, err)

	assert.Len(t, page2.Messages, 3)
	assert.True(t, page2.PageInfo.HasNextPage)

	// Verify continuation
	assert.Equal(t, "page-msg-6", page2.Messages[0].ID)
	assert.Equal(t, "page-msg-5", page2.Messages[1].ID)
	assert.Equal(t, "page-msg-4", page2.Messages[2].ID)
}

func TestDBStorage_GetSessionMessages_Order(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session
	session := &ChatSession{
		ID:        "order-session",
		Name:      "Order Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Add messages
	for i := 0; i < 5; i++ {
		msg := &ChatMessage{
			ID:        "order-msg-" + string(rune('0'+i)),
			Role:      "user",
			Content:   "Message",
			Timestamp: now + int64(i*1000),
		}
		err = db.AddMessage(ctx, userID, session.ID, msg)
		require.NoError(t, err)
	}

	// Test ascending order (oldest first)
	paramsAsc := GetSessionMessagesParams{
		SessionID: session.ID,
		Limit:     10,
		Order:     "asc",
	}
	pageAsc, err := db.GetSessionMessages(ctx, userID, paramsAsc)
	require.NoError(t, err)

	assert.Equal(t, "order-msg-0", pageAsc.Messages[0].ID)
	assert.Equal(t, "order-msg-4", pageAsc.Messages[4].ID)

	// Test descending order (newest first)
	paramsDesc := GetSessionMessagesParams{
		SessionID: session.ID,
		Limit:     10,
		Order:     "desc",
	}
	pageDesc, err := db.GetSessionMessages(ctx, userID, paramsDesc)
	require.NoError(t, err)

	assert.Equal(t, "order-msg-4", pageDesc.Messages[0].ID)
	assert.Equal(t, "order-msg-0", pageDesc.Messages[4].ID)
}

func TestDBStorage_GetSessionMessages_DefaultLimit(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session
	session := &ChatSession{
		ID:        "default-limit-session",
		Name:      "Default Limit Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Add a few messages
	for i := 0; i < 3; i++ {
		msg := &ChatMessage{
			ID:        "limit-msg-" + string(rune('0'+i)),
			Role:      "user",
			Content:   "Message",
			Timestamp: now + int64(i*1000),
		}
		err = db.AddMessage(ctx, userID, session.ID, msg)
		require.NoError(t, err)
	}

	// Test with no limit specified (should default to 50)
	params := GetSessionMessagesParams{
		SessionID: session.ID,
		// Limit not set
	}
	page, err := db.GetSessionMessages(ctx, userID, params)
	require.NoError(t, err)

	// Should return all 3 messages
	assert.Len(t, page.Messages, 3)
	assert.False(t, page.PageInfo.HasNextPage)
}

func TestDBStorage_GetMessagesByTokenBudget(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session
	session := &ChatSession{
		ID:        "budget-session",
		Name:      "Token Budget Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Add messages with known token counts
	messages := []struct {
		id       string
		tokens   int
		isPinned bool
	}{
		{"budget-msg-1", 100, false}, // oldest
		{"budget-msg-2", 200, true},  // pinned
		{"budget-msg-3", 150, false},
		{"budget-msg-4", 100, false},
		{"budget-msg-5", 50, false}, // newest
	}

	for i, m := range messages {
		msg := &ChatMessage{
			ID:         m.id,
			Role:       "user",
			Content:    "Message",
			Timestamp:  now + int64(i*1000),
			TokenCount: m.tokens,
			IsPinned:   m.isPinned,
		}
		err = db.AddMessage(ctx, userID, session.ID, msg)
		require.NoError(t, err)
	}

	// Test with budget of 350 tokens
	// Should include: pinned (200) + newest unpinned fitting (50+100 = 150)
	result, err := db.GetMessagesByTokenBudget(ctx, session.ID, 350)
	require.NoError(t, err)

	// Pinned always included (200 tokens)
	// Then newest to oldest until budget exhausted:
	// budget-msg-5 (50) -> 250 remaining -> fits
	// budget-msg-4 (100) -> 150 remaining -> fits
	// budget-msg-3 (150) -> 0 remaining -> fits exactly
	// Total: 200 + 50 + 100 = 350

	// Result should be in chronological order (oldest first)
	assert.GreaterOrEqual(t, len(result), 2) // At least pinned + some unpinned

	// Verify pinned message is included
	hasPinned := false
	for _, msg := range result {
		if msg.ID == "budget-msg-2" {
			hasPinned = true
			break
		}
	}
	assert.True(t, hasPinned, "Pinned message should always be included")

	// Verify chronological order
	for i := 1; i < len(result); i++ {
		assert.LessOrEqual(t, result[i-1].Timestamp, result[i].Timestamp, "Messages should be in chronological order")
	}
}

func TestDBStorage_GetMessagesByTokenBudget_PinnedPriority(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session
	session := &ChatSession{
		ID:        "pinned-priority-session",
		Name:      "Pinned Priority Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Add messages - pinned message should always be included even if it exceeds budget alone
	msg1 := &ChatMessage{
		ID:         "pinned-msg",
		Role:       "user",
		Content:    "Important pinned message",
		Timestamp:  now,
		TokenCount: 300,
		IsPinned:   true,
	}
	err = db.AddMessage(ctx, userID, session.ID, msg1)
	require.NoError(t, err)

	msg2 := &ChatMessage{
		ID:         "unpinned-msg",
		Role:       "assistant",
		Content:    "Regular message",
		Timestamp:  now + 1000,
		TokenCount: 50,
		IsPinned:   false,
	}
	err = db.AddMessage(ctx, userID, session.ID, msg2)
	require.NoError(t, err)

	// Budget smaller than pinned message
	result, err := db.GetMessagesByTokenBudget(ctx, session.ID, 100)
	require.NoError(t, err)

	// Pinned should still be included
	require.Len(t, result, 1)
	assert.Equal(t, "pinned-msg", result[0].ID)
}

func TestDBStorage_UpdateMessageTokenCount(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	// Create session with message
	session := &ChatSession{
		ID:        "token-update-session",
		Name:      "Token Update Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	msg := &ChatMessage{
		ID:         "token-update-msg",
		Role:       "user",
		Content:    "Test message",
		Timestamp:  now,
		TokenCount: 0, // Initially 0
	}
	err = db.AddMessage(ctx, userID, session.ID, msg)
	require.NoError(t, err)

	// Update token count
	err = db.UpdateMessageTokenCount(ctx, msg.ID, 42)
	require.NoError(t, err)

	// Verify update
	retrieved, err := db.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	require.Len(t, retrieved.Messages, 1)
	assert.Equal(t, 42, retrieved.Messages[0].TokenCount)
}

func TestDBStorage_UpdateMessageTokenCount_NotFound(t *testing.T) {
	db, cleanup := createTestDBStorage(t)
	defer cleanup()

	ctx := context.Background()

	err := db.UpdateMessageTokenCount(ctx, "nonexistent-message", 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message not found")
}

func TestMemoryStorage_NewMethods(t *testing.T) {
	ctx := context.Background()
	userID := "test-user"
	now := time.Now().UnixMilli()

	ms := NewMemoryStorage()

	// Create session
	session := &ChatSession{
		ID:        "mem-new-session",
		Name:      "Memory New Methods Test",
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := ms.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Test SaveMessages
	messages := []ChatMessage{
		{ID: "mem-batch-1", Role: "user", Content: "Msg 1", Timestamp: now, TokenCount: 10},
		{ID: "mem-batch-2", Role: "assistant", Content: "Msg 2", Timestamp: now + 1000, TokenCount: 20},
	}
	results, err := ms.SaveMessages(ctx, userID, session.ID, messages)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.True(t, results[0].Success)
	assert.True(t, results[1].Success)

	// Test GetSessionMessages
	params := GetSessionMessagesParams{
		SessionID: session.ID,
		Limit:     10,
	}
	page, err := ms.GetSessionMessages(ctx, userID, params)
	require.NoError(t, err)
	assert.Len(t, page.Messages, 2)

	// Test GetMessagesByTokenBudget
	budget, err := ms.GetMessagesByTokenBudget(ctx, session.ID, 100)
	require.NoError(t, err)
	assert.Len(t, budget, 2)

	// Test UpdateMessageTokenCount
	err = ms.UpdateMessageTokenCount(ctx, "mem-batch-1", 99)
	require.NoError(t, err)

	// Verify update
	retrieved, err := ms.GetSession(ctx, userID, session.ID)
	require.NoError(t, err)
	for _, msg := range retrieved.Messages {
		if msg.ID == "mem-batch-1" {
			assert.Equal(t, 99, msg.TokenCount)
		}
	}
}
