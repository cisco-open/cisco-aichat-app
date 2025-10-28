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
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-aichat-app/pkg/tokens"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCounter is a simple mock for testing TokenService
type mockCounter struct {
	mu         sync.Mutex
	countCalls int
	countFunc  func(text string, model string) (int, error)
}

func (m *mockCounter) CountTokens(ctx context.Context, text string, model string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.countCalls++
	if m.countFunc != nil {
		return m.countFunc(text, model)
	}
	// Default: chars/4 estimation
	if len(text) == 0 {
		return 0, nil
	}
	return (len(text) + 3) / 4, nil
}

func (m *mockCounter) SupportsModel(model string) bool {
	return true
}

func (m *mockCounter) getCountCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.countCalls
}

// createTestTokenService creates a TokenService with test storage and mock counter
func createTestTokenService(t *testing.T) (*TokenService, *DBStorage, *mockCounter, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "tokenservice-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	dbURL := "file:" + dbPath

	storage, err := NewDBStorage(dbURL, log.DefaultLogger)
	require.NoError(t, err)

	counter := &mockCounter{}

	tokenService := NewTokenService(storage, counter,
		WithContextLimit(10000), // 10k for testing
		WithDefaultModel("gpt-4"),
	)

	cleanup := func() {
		tokenService.Close()
		storage.Close()
		os.RemoveAll(tmpDir)
	}

	return tokenService, storage, counter, cleanup
}

// createTestSession creates a test session with messages
func createTestSession(t *testing.T, storage *DBStorage, userID, sessionID string, messages []ChatMessage) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UnixMilli()

	session := &ChatSession{
		ID:          sessionID,
		Name:        "Test Session",
		CreatedAt:   now,
		UpdatedAt:   now,
		IsActive:    true,
		TotalTokens: 0,
		Messages:    []ChatMessage{},
	}

	err := storage.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	for _, msg := range messages {
		err := storage.AddMessage(ctx, userID, sessionID, &msg)
		require.NoError(t, err)
	}
}

func TestTokenService_EnsureTokenCounts(t *testing.T) {
	ts, storage, counter, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"

	// Create session with messages that have no token counts
	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Hello, how are you?", Timestamp: time.Now().UnixMilli(), TokenCount: 0},
		{ID: "msg-2", Role: "assistant", Content: "I'm doing well, thank you!", Timestamp: time.Now().UnixMilli() + 1, TokenCount: 0},
		{ID: "msg-3", Role: "user", Content: "Great to hear!", Timestamp: time.Now().UnixMilli() + 2, TokenCount: 0},
	}
	createTestSession(t, storage, userID, sessionID, messages)

	// Ensure token counts are computed
	err := ts.EnsureTokenCounts(ctx, messages, "gpt-4")
	require.NoError(t, err)

	// Wait a bit for background worker to complete and persist
	time.Sleep(100 * time.Millisecond)

	// Verify counts were computed
	assert.Equal(t, 3, counter.getCountCalls())

	// Verify counts were persisted to database
	session, err := storage.GetSession(ctx, userID, sessionID)
	require.NoError(t, err)

	for _, msg := range session.Messages {
		assert.Greater(t, msg.TokenCount, 0, "message %s should have token count", msg.ID)
	}
}

func TestTokenService_EnsureTokenCounts_SkipsExisting(t *testing.T) {
	ts, storage, counter, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"

	// Create session with messages that already have token counts
	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Hello", Timestamp: time.Now().UnixMilli(), TokenCount: 5},
		{ID: "msg-2", Role: "assistant", Content: "Hi there", Timestamp: time.Now().UnixMilli() + 1, TokenCount: 3},
	}
	createTestSession(t, storage, userID, sessionID, messages)

	// Ensure token counts - should skip existing
	err := ts.EnsureTokenCounts(ctx, messages, "gpt-4")
	require.NoError(t, err)

	// No counting should have occurred
	assert.Equal(t, 0, counter.getCountCalls())
}

func TestTokenService_EnsureTokenCounts_MixedCounts(t *testing.T) {
	ts, storage, counter, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"

	// Mix of messages with and without counts
	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Hello", Timestamp: time.Now().UnixMilli(), TokenCount: 5},       // Has count
		{ID: "msg-2", Role: "assistant", Content: "Hi there", Timestamp: time.Now().UnixMilli() + 1, TokenCount: 0}, // No count
		{ID: "msg-3", Role: "user", Content: "How are you?", Timestamp: time.Now().UnixMilli() + 2, TokenCount: 8},  // Has count
	}
	createTestSession(t, storage, userID, sessionID, messages)

	// Ensure token counts
	err := ts.EnsureTokenCounts(ctx, messages, "gpt-4")
	require.NoError(t, err)

	// Wait for background worker
	time.Sleep(100 * time.Millisecond)

	// Only one message should have been counted (msg-2)
	assert.Equal(t, 1, counter.getCountCalls())
}

func TestTokenService_GetSessionTokenStats(t *testing.T) {
	ts, storage, _, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"

	// Create session with messages with various token counts
	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Hello", Timestamp: time.Now().UnixMilli(), TokenCount: 100},
		{ID: "msg-2", Role: "assistant", Content: "Hi", Timestamp: time.Now().UnixMilli() + 1, TokenCount: 200},
		{ID: "msg-3", Role: "user", Content: "Test", Timestamp: time.Now().UnixMilli() + 2, TokenCount: 0}, // Uncounted
		{ID: "msg-4", Role: "assistant", Content: "Reply", Timestamp: time.Now().UnixMilli() + 3, TokenCount: 300},
	}
	createTestSession(t, storage, userID, sessionID, messages)

	stats, err := ts.GetSessionTokenStats(ctx, userID, sessionID)
	require.NoError(t, err)

	assert.Equal(t, sessionID, stats.SessionID)
	assert.Equal(t, 600, stats.TotalTokens)         // 100 + 200 + 0 + 300
	assert.Equal(t, 10000, stats.ContextLimit)      // Test limit
	assert.Equal(t, 6.0, stats.ContextUsage)        // (600 / 10000) * 100 = 6%
	assert.Equal(t, 4, stats.MessageCount)
	assert.Equal(t, 1, stats.UncountedMsgs)         // msg-3 has 0 tokens
}

func TestTokenService_GetSessionTokenStats_NotFound(t *testing.T) {
	ts, _, _, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()

	_, err := ts.GetSessionTokenStats(ctx, "user-1", "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTokenService_RecalculateSessionTokens(t *testing.T) {
	ts, storage, _, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"

	// Create session
	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Hello", Timestamp: time.Now().UnixMilli(), TokenCount: 100},
		{ID: "msg-2", Role: "assistant", Content: "Hi", Timestamp: time.Now().UnixMilli() + 1, TokenCount: 200},
		{ID: "msg-3", Role: "user", Content: "Bye", Timestamp: time.Now().UnixMilli() + 2, TokenCount: 150},
	}
	createTestSession(t, storage, userID, sessionID, messages)

	// Manually set session total tokens to wrong value
	session, err := storage.GetSession(ctx, userID, sessionID)
	require.NoError(t, err)
	session.TotalTokens = 9999 // Wrong value
	err = storage.UpdateSession(ctx, userID, session)
	require.NoError(t, err)

	// Recalculate
	err = ts.RecalculateSessionTokens(ctx, userID, sessionID)
	require.NoError(t, err)

	// Verify correct total
	session, err = storage.GetSession(ctx, userID, sessionID)
	require.NoError(t, err)
	assert.Equal(t, 450, session.TotalTokens) // 100 + 200 + 150
}

func TestTokenService_ContextLimit(t *testing.T) {
	t.Run("default context limit", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "tokenservice-default-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		dbPath := filepath.Join(tmpDir, "test.db")
		storage, err := NewDBStorage("file:"+dbPath, log.DefaultLogger)
		require.NoError(t, err)
		defer storage.Close()

		counter := &mockCounter{}

		// Without WithContextLimit option, should use default
		ts := NewTokenService(storage, counter)
		defer ts.Close()

		assert.Equal(t, DefaultContextLimit, ts.GetContextLimit())
	})

	t.Run("custom context limit", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "tokenservice-custom-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		dbPath := filepath.Join(tmpDir, "test.db")
		storage, err := NewDBStorage("file:"+dbPath, log.DefaultLogger)
		require.NoError(t, err)
		defer storage.Close()

		counter := &mockCounter{}

		ts := NewTokenService(storage, counter, WithContextLimit(50000))
		defer ts.Close()

		assert.Equal(t, 50000, ts.GetContextLimit())
	})

	t.Run("context usage calculation", func(t *testing.T) {
		ts, storage, _, cleanup := createTestTokenService(t)
		defer cleanup()

		ctx := context.Background()
		userID := "user-1"
		sessionID := "session-1"

		// Create session with tokens that give us a specific percentage
		// 10000 limit, 2500 tokens = 25%
		messages := []ChatMessage{
			{ID: "msg-1", Role: "user", Content: "Test", Timestamp: time.Now().UnixMilli(), TokenCount: 2500},
		}
		createTestSession(t, storage, userID, sessionID, messages)

		stats, err := ts.GetSessionTokenStats(ctx, userID, sessionID)
		require.NoError(t, err)

		assert.Equal(t, 2500, stats.TotalTokens)
		assert.Equal(t, 10000, stats.ContextLimit)
		assert.InDelta(t, 25.0, stats.ContextUsage, 0.01)
	})
}

func TestTokenService_EmptyMessages(t *testing.T) {
	ts, _, counter, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()

	// Call with empty slice
	err := ts.EnsureTokenCounts(ctx, []ChatMessage{}, "gpt-4")
	require.NoError(t, err)

	// No counting should occur
	assert.Equal(t, 0, counter.getCountCalls())
}

func TestTokenService_SkipsEmptyContent(t *testing.T) {
	ts, storage, counter, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"

	// Message with empty content should be skipped
	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "", Timestamp: time.Now().UnixMilli(), TokenCount: 0},
		{ID: "msg-2", Role: "user", Content: "Hello", Timestamp: time.Now().UnixMilli() + 1, TokenCount: 0},
	}
	createTestSession(t, storage, userID, sessionID, messages)

	err := ts.EnsureTokenCounts(ctx, messages, "gpt-4")
	require.NoError(t, err)

	// Wait for background worker
	time.Sleep(100 * time.Millisecond)

	// Only one message should have been counted (msg-2 with content)
	assert.Equal(t, 1, counter.getCountCalls())
}

func TestTokenService_UsesDefaultModel(t *testing.T) {
	ts, storage, counter, cleanup := createTestTokenService(t)
	defer cleanup()

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"

	var usedModel string
	counter.countFunc = func(text string, model string) (int, error) {
		usedModel = model
		return 10, nil
	}

	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Hello", Timestamp: time.Now().UnixMilli(), TokenCount: 0},
	}
	createTestSession(t, storage, userID, sessionID, messages)

	// Call with empty model - should use default
	err := ts.EnsureTokenCounts(ctx, messages, "")
	require.NoError(t, err)

	// Wait for background worker
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, "gpt-4", usedModel)
}

// Integration test with real token counter
func TestTokenService_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "tokenservice-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	storage, err := NewDBStorage("file:"+dbPath, log.DefaultLogger)
	require.NoError(t, err)
	defer storage.Close()

	// Use real estimation counter
	estimator := tokens.NewEstimationCounter()
	registry := tokens.NewRegistry(nil, estimator)

	ts := NewTokenService(storage, registry, WithContextLimit(100000))
	defer ts.Close()

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"

	// Create session with real messages
	now := time.Now().UnixMilli()
	session := &ChatSession{
		ID:        sessionID,
		Name:      "Integration Test",
		CreatedAt: now,
		UpdatedAt: now,
		IsActive:  true,
	}
	err = storage.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	messages := []ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Hello, how are you doing today?", Timestamp: now, TokenCount: 0},
		{ID: "msg-2", Role: "assistant", Content: "I am doing great, thank you for asking!", Timestamp: now + 1, TokenCount: 0},
	}
	for _, msg := range messages {
		err := storage.AddMessage(ctx, userID, sessionID, &msg)
		require.NoError(t, err)
	}

	// Ensure token counts
	err = ts.EnsureTokenCounts(ctx, messages, "gpt-4")
	require.NoError(t, err)

	// Wait for background worker
	time.Sleep(200 * time.Millisecond)

	// Get stats
	stats, err := ts.GetSessionTokenStats(ctx, userID, sessionID)
	require.NoError(t, err)

	// Verify reasonable stats
	assert.Equal(t, sessionID, stats.SessionID)
	assert.Greater(t, stats.TotalTokens, 0)
	assert.Equal(t, 100000, stats.ContextLimit)
	assert.Greater(t, stats.ContextUsage, 0.0)
	assert.Equal(t, 2, stats.MessageCount)
	assert.Equal(t, 0, stats.UncountedMsgs) // All should be counted now
}
