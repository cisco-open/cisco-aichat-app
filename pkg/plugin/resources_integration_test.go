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

package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grafana/grafana-aichat-app/pkg/storage"
	"github.com/grafana/grafana-aichat-app/pkg/tokens"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	// Import database drivers for test database operations
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "modernc.org/sqlite"
)

type authErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"status"`
	Error   string `json:"error"`
}

// TestHandleGetSessionTokenStats_Integration tests the full API flow
// with real TokenService instead of fallback path
func TestHandleGetSessionTokenStats_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "plugin-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize storage
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewDBStorage("file:"+dbPath, log.DefaultLogger)
	require.NoError(t, err)
	defer store.Close()

	// Initialize token counting infrastructure (same as NewApp)
	estimator := tokens.NewEstimationCounter()
	registry := tokens.NewRegistry(nil, estimator) // Use estimation only for test

	tokenService := storage.NewTokenService(
		store,
		registry,
		storage.WithContextLimit(10000), // 10k for testing
	)
	defer tokenService.Close()

	// Create App with initialized tokenService
	app := &App{
		storage:      store,
		tokenService: tokenService,
		rateLimiters: make(map[string]*rate.Limiter),
	}

	// Create test session with messages
	ctx := context.Background()
	userID := "test-user"
	sessionID := "test-session"
	now := time.Now().UnixMilli()

	session := &storage.ChatSession{
		ID:        sessionID,
		Name:      "Integration Test Session",
		CreatedAt: now,
		UpdatedAt: now,
		IsActive:  true,
	}
	err = store.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Add messages with known token counts
	messages := []storage.ChatMessage{
		{ID: "msg-1", Role: "user", Content: "Hello world", Timestamp: now, TokenCount: 100},
		{ID: "msg-2", Role: "assistant", Content: "Hi there!", Timestamp: now + 1, TokenCount: 200},
		{ID: "msg-3", Role: "user", Content: "How are you?", Timestamp: now + 2, TokenCount: 0}, // Uncounted
	}
	for _, msg := range messages {
		err := store.AddMessage(ctx, userID, sessionID, &msg)
		require.NoError(t, err)
	}

	// Create HTTP request
	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/tokens", nil)
	req.Header.Set("X-Grafana-User", userID)

	// Create response recorder
	rr := httptest.NewRecorder()

	// Call handler directly
	app.handleGetSessionTokenStats(rr, req, userID, sessionID)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code)

	var stats storage.TokenStats
	err = json.NewDecoder(rr.Body).Decode(&stats)
	require.NoError(t, err)

	// Verify stats match expected values
	assert.Equal(t, sessionID, stats.SessionID)
	assert.Equal(t, 300, stats.TotalTokens) // 100 + 200 + 0
	assert.Equal(t, 10000, stats.ContextLimit)
	assert.InDelta(t, 3.0, stats.ContextUsage, 0.01) // (300/10000)*100 = 3%
	assert.Equal(t, 3, stats.MessageCount)
	assert.Equal(t, 1, stats.UncountedMsgs) // msg-3 has 0 tokens
}

// TestHandleGetSessionTokenStats_UsesTokenService verifies that the handler
// uses TokenService when available (not the fallback path)
func TestHandleGetSessionTokenStats_UsesTokenService(t *testing.T) {
	// This test verifies TokenService path is taken by checking
	// that the context limit from TokenService is used, not the default

	tmpDir, err := os.MkdirTemp("", "plugin-tokenservice-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewDBStorage("file:"+dbPath, log.DefaultLogger)
	require.NoError(t, err)
	defer store.Close()

	estimator := tokens.NewEstimationCounter()
	registry := tokens.NewRegistry(nil, estimator)

	// Use custom context limit different from default (100000)
	customLimit := 50000
	tokenService := storage.NewTokenService(
		store,
		registry,
		storage.WithContextLimit(customLimit),
	)
	defer tokenService.Close()

	app := &App{
		storage:      store,
		tokenService: tokenService,
		rateLimiters: make(map[string]*rate.Limiter),
	}

	ctx := context.Background()
	userID := "test-user"
	sessionID := "test-session"
	now := time.Now().UnixMilli()

	session := &storage.ChatSession{
		ID:        sessionID,
		Name:      "Test",
		CreatedAt: now,
		UpdatedAt: now,
		IsActive:  true,
	}
	err = store.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/tokens", nil)
	req.Header.Set("X-Grafana-User", userID)
	rr := httptest.NewRecorder()

	app.handleGetSessionTokenStats(rr, req, userID, sessionID)

	assert.Equal(t, http.StatusOK, rr.Code)

	var stats storage.TokenStats
	err = json.NewDecoder(rr.Body).Decode(&stats)
	require.NoError(t, err)

	// The context limit should be from TokenService (50000), not default (100000)
	// This proves TokenService path was used, not fallback
	assert.Equal(t, customLimit, stats.ContextLimit)
}

// TestHandleGetSessionTokenStats_FallbackPath verifies the fallback behavior
// when TokenService is nil
func TestHandleGetSessionTokenStats_FallbackPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plugin-fallback-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := storage.NewDBStorage("file:"+dbPath, log.DefaultLogger)
	require.NoError(t, err)
	defer store.Close()

	// Create App WITHOUT tokenService (nil)
	app := &App{
		storage:      store,
		tokenService: nil, // Explicitly nil to test fallback
		rateLimiters: make(map[string]*rate.Limiter),
	}

	ctx := context.Background()
	userID := "test-user"
	sessionID := "test-session"
	now := time.Now().UnixMilli()

	session := &storage.ChatSession{
		ID:        sessionID,
		Name:      "Fallback Test",
		CreatedAt: now,
		UpdatedAt: now,
		IsActive:  true,
	}
	err = store.CreateSession(ctx, userID, session)
	require.NoError(t, err)

	// Add a message
	msg := storage.ChatMessage{ID: "msg-1", Role: "user", Content: "Test", Timestamp: now, TokenCount: 500}
	err = store.AddMessage(ctx, userID, sessionID, &msg)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/sessions/"+sessionID+"/tokens", nil)
	req.Header.Set("X-Grafana-User", userID)
	rr := httptest.NewRecorder()

	app.handleGetSessionTokenStats(rr, req, userID, sessionID)

	assert.Equal(t, http.StatusOK, rr.Code)

	var stats storage.TokenStats
	err = json.NewDecoder(rr.Body).Decode(&stats)
	require.NoError(t, err)

	// Fallback uses DefaultContextLimit (100000)
	assert.Equal(t, storage.DefaultContextLimit, stats.ContextLimit)
	assert.Equal(t, 500, stats.TotalTokens)
	assert.Equal(t, 1, stats.MessageCount)
}

func TestAuthErrorContract_History(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rr := httptest.NewRecorder()

	app.handleHistory(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)

	var resp authErrorResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "AUTH_REQUIRED", resp.Code)
	assert.Equal(t, "Authentication required", resp.Message)
	assert.Equal(t, http.StatusUnauthorized, resp.Status)
	assert.Equal(t, "Authentication required", resp.Error)
}
