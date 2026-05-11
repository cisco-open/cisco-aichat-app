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
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/grafana/cisco-aichat-app/pkg/storage"
	"github.com/grafana/cisco-aichat-app/pkg/telemetry"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

var (
	// Security: Validation regex patterns
	validSessionIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
	validMessageIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
)

const (
	defaultCompactionBatchTokens = 10000
	maxAutoCompactionPasses      = 3
	errorCodeAuthRequired        = "AUTH_REQUIRED"
)

type errorResponse struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Status  int    `json:"status"`
	Error   string `json:"error,omitempty"`
}

// validateString checks if a string contains only printable characters and no control characters
// Security: Prevents injection attacks via control characters
func validateString(s string, maxLen int) bool {
	if len(s) > maxLen || len(s) == 0 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// validateMessageContent is like validateString but allows empty content
// and common whitespace (needed for streaming and multi-line messages)
func validateMessageContent(s string, maxLen int) bool {
	if len(s) > maxLen {
		return false
	}
	// Empty content is allowed for streaming messages
	if len(s) == 0 {
		return true
	}
	for _, r := range s {
		// Allow common whitespace: newline, carriage return, tab
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		// Reject other control characters
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// sanitizeContent validates and returns user content
// Note: HTML escaping removed - ReactMarkdown handles XSS protection on frontend
// Keeping function for future validation needs (length limits, etc.)
func sanitizeContent(content string) string {
	return content
}

// dedupeMessagesByID removes duplicate message IDs while preserving first-seen order.
func dedupeMessagesByID(messages []storage.ChatMessage) []storage.ChatMessage {
	if len(messages) <= 1 {
		return messages
	}

	seen := make(map[string]struct{}, len(messages))
	deduped := make([]storage.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		if _, exists := seen[msg.ID]; exists {
			continue
		}
		seen[msg.ID] = struct{}{}
		deduped = append(deduped, msg)
	}

	return deduped
}

// extractUserID extracts the user ID from the HTTP request context
// Security: Removed anonymous fallback - returns error if user not authenticated
func extractUserID(req *http.Request) (string, error) {
	// Primary: Get user from request context (set by httpadapter from plugin SDK)
	user := backend.UserFromContext(req.Context())
	if user != nil && user.Login != "" {
		return user.Login, nil
	}

	// Fallback: Try X-Grafana-User header (for testing/direct calls)
	userID := req.Header.Get("X-Grafana-User")
	if userID != "" {
		return userID, nil
	}

	// Security: No anonymous fallback - require authentication
	return "", fmt.Errorf("user not authenticated")
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.DefaultLogger.Error("Failed to encode JSON response", "error", err)
	}
}

// writeError writes an error response
func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, errorResponse{
		Message: message,
		Status:  statusCode,
		Error:   message,
	})
}

func writeAuthRequired(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, errorResponse{
		Code:    errorCodeAuthRequired,
		Message: "Authentication required",
		Status:  http.StatusUnauthorized,
		Error:   "Authentication required",
	})
}

func isStorageNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

// registerRoutes registers all HTTP routes
func (a *App) registerRoutes(mux *http.ServeMux) {
	// Telemetry endpoint for frontend LLM metrics reporting
	mux.Handle("/telemetry", telemetry.NewHandler())

	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/settings", a.handleSettings)
	mux.HandleFunc("/history", a.handleHistory)
	mux.HandleFunc("/sessions", a.handleSessions)
	mux.HandleFunc("/sessions/", a.handleSessionByID)
	mux.HandleFunc("/search", a.handleSearch)
}

// handleHealth handles GET /health
// Returns health status and cache metrics for monitoring PERF-04 target (>80% hit rate)
// Also returns preloader stats for PERF-03 monitoring
func (a *App) handleHealth(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	degraded := false
	if rs, ok := a.storage.(*storage.ResilientStorage); ok {
		degraded = rs.IsDegraded()
	}

	storageCtx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()
	storageErr := a.storage.Ping(storageCtx)
	storageHealthy := storageErr == nil

	status := "ok"
	message := "AI Chat Assistant backend is running"
	statusCode := http.StatusOK
	switch {
	case degraded:
		status = "degraded"
		message = "AI Chat is running in degraded mode (history may not persist)"
		statusCode = http.StatusServiceUnavailable
	case !storageHealthy:
		status = "error"
		message = "AI Chat storage check failed"
		statusCode = http.StatusServiceUnavailable
	}

	response := map[string]interface{}{
		"status":          status,
		"message":         message,
		"storageHealthy":  storageHealthy,
		"storageDegraded": degraded,
	}
	if storageErr != nil {
		response["storageError"] = storageErr.Error()
	}

	// Add cache metrics if available (PERF-04)
	if a.messageCache != nil {
		response["cacheHitRatio"] = a.messageCache.HitRatio()
	}

	// Add preloader stats if available (PERF-03)
	if a.preloader != nil {
		stats := a.preloader.Stats()
		response["preloader"] = map[string]interface{}{
			"trackedSessions": stats.TrackedSessions,
			"lastPreloadTime": stats.LastPreloadTime,
			"preloadCount":    stats.PreloadCount,
		}
	}

	writeJSON(w, statusCode, response)
}

// handleSettings handles GET /settings - returns provisioned plugin settings
func (a *App) handleSettings(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Parse JSONData from app settings
	var jsonData map[string]interface{}
	if len(a.appSettings.JSONData) > 0 {
		if err := json.Unmarshal(a.appSettings.JSONData, &jsonData); err != nil {
			log.DefaultLogger.Error("Failed to parse plugin settings", "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to parse plugin settings")
			return
		}
	}

	// Extract settings with defaults
	response := map[string]interface{}{
		"provisioned": len(jsonData) > 0,
	}

	if systemPrompt, ok := jsonData["systemPrompt"].(string); ok && systemPrompt != "" {
		response["systemPrompt"] = systemPrompt
	}
	if maxTokens, ok := jsonData["maxTokens"].(float64); ok && maxTokens > 0 {
		response["maxTokens"] = int(maxTokens)
	}
	if temperature, ok := jsonData["temperature"].(float64); ok {
		response["temperature"] = temperature
	}
	if enableMcpTools, ok := jsonData["enableMcpTools"].(bool); ok {
		response["enableMcpTools"] = enableMcpTools
	}

	log.DefaultLogger.Debug("Returning plugin settings", "provisioned", response["provisioned"])
	writeJSON(w, http.StatusOK, response)
}

// UserChatHistory represents the frontend's user history format
type UserChatHistory struct {
	UserID          string                `json:"userId"`
	ActiveSessionID string                `json:"activeSessionId"`
	Sessions        []storage.ChatSession `json:"sessions"`
}

// handleHistory handles GET/POST/DELETE /history - gets, saves, or clears user's chat history
// GET: Returns the user's complete chat history
// POST: Accepts the full history object from frontend and syncs sessions
// DELETE: Clears all chat history for the user
func (a *App) handleHistory(w http.ResponseWriter, req *http.Request) {
	// Security: Extract and validate user ID
	userID, err := extractUserID(req)
	if err != nil {
		log.DefaultLogger.Warn("Authentication failed", "error", err)
		writeAuthRequired(w)
		return
	}

	switch req.Method {
	case http.MethodGet:
		a.handleGetHistory(w, req, userID)
	case http.MethodPost:
		a.handlePostHistory(w, req, userID)
	case http.MethodDelete:
		a.handleClearAll(w, req, userID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleGetHistory returns the user's complete chat history
func (a *App) handleGetHistory(w http.ResponseWriter, req *http.Request, userID string) {
	sessions, err := a.storage.GetSessions(req.Context(), userID)
	if err != nil {
		log.DefaultLogger.Error("Failed to list sessions", "userID", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to fetch sessions")
		return
	}

	// Ensure empty array, not null in JSON
	if sessions == nil {
		sessions = []storage.ChatSession{}
	}

	// Find the most recently updated session as the active one
	var activeSessionID string
	if len(sessions) > 0 {
		// Sessions are already sorted by updatedAt descending from storage
		activeSessionID = sessions[0].ID
	}

	history := UserChatHistory{
		UserID:          userID,
		ActiveSessionID: activeSessionID,
		Sessions:        sessions,
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    history,
	})
}

// handlePostHistory saves the user's chat history from frontend
func (a *App) handlePostHistory(w http.ResponseWriter, req *http.Request, userID string) {
	var history UserChatHistory
	if err := json.NewDecoder(req.Body).Decode(&history); err != nil {
		log.DefaultLogger.Warn("Invalid request body", "error", err)
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Process each session in the history
	for _, session := range history.Sessions {
		// Security: Validate session ID
		if !validSessionIDRegex.MatchString(session.ID) {
			log.DefaultLogger.Warn("Invalid session ID in history", "sessionID", session.ID)
			continue
		}

		// Security: Validate and sanitize session name
		if !validateString(session.Name, 256) {
			log.DefaultLogger.Warn("Invalid session name in history", "sessionID", session.ID)
			continue
		}
		session.Name = sanitizeContent(session.Name)

		// Security: Validate messages
		validMessages := make([]storage.ChatMessage, 0, len(session.Messages))
		for _, msg := range session.Messages {
			if !validMessageIDRegex.MatchString(msg.ID) {
				continue
			}
			if !validateMessageContent(msg.Content, 100000) {
				continue
			}
			msg.Content = sanitizeContent(msg.Content)
			validMessages = append(validMessages, msg)
		}
		session.Messages = dedupeMessagesByID(validMessages)

		// Check if session exists
		existing, err := a.storage.GetSession(req.Context(), userID, session.ID)
		if err != nil || existing == nil {
			// Create new session
			if err := a.storage.CreateSession(req.Context(), userID, &session); err != nil {
				log.DefaultLogger.Error("Failed to create session from history", "sessionID", session.ID, "error", err)
			}
		} else {
			// Update existing session
			if err := a.storage.UpdateSession(req.Context(), userID, &session); err != nil {
				log.DefaultLogger.Error("Failed to update session from history", "sessionID", session.ID, "error", err)
			}
		}

		// Save messages to the messages table (Phase 15 fix: messages must be in aichat_messages)
		for _, msg := range session.Messages {
			if err := a.storage.SaveMessage(req.Context(), userID, session.ID, &msg); err != nil {
				// Log but continue - message may already exist
				log.DefaultLogger.Debug("Failed to save message", "sessionID", session.ID, "messageID", msg.ID, "error", err)
			}
		}

		// Invalidate message cache for this session
		if a.messageCache != nil {
			a.messageCache.Invalidate(session.ID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// handleSessions handles GET /sessions and POST /sessions
func (a *App) handleSessions(w http.ResponseWriter, req *http.Request) {
	// Security: Extract and validate user ID
	userID, err := extractUserID(req)
	if err != nil {
		log.DefaultLogger.Warn("Authentication failed", "error", err)
		writeAuthRequired(w)
		return
	}

	switch req.Method {
	case http.MethodGet:
		a.handleGetSessions(w, req, userID)
	case http.MethodPost:
		a.handleCreateSession(w, req, userID)
	case http.MethodDelete:
		// Handle /sessions/clear-all
		if strings.HasSuffix(req.URL.Path, "/clear-all") {
			a.handleClearAll(w, req, userID)
		} else {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleGetSessions handles GET /sessions
func (a *App) handleGetSessions(w http.ResponseWriter, req *http.Request, userID string) {
	sessions, err := a.storage.GetSessions(req.Context(), userID)
	if err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to get sessions", "userID", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to retrieve sessions")
		return
	}

	// Ensure empty array, not null in JSON
	if sessions == nil {
		sessions = []storage.ChatSession{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessions": sessions,
		"total":    len(sessions),
	})
}

// handleCreateSession handles POST /sessions
func (a *App) handleCreateSession(w http.ResponseWriter, req *http.Request, userID string) {
	var session storage.ChatSession
	if err := json.NewDecoder(req.Body).Decode(&session); err != nil {
		log.DefaultLogger.Warn("Invalid request body", "error", err)
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Security: Validate session ID
	if !validSessionIDRegex.MatchString(session.ID) {
		writeError(w, http.StatusBadRequest, "Invalid session ID format")
		return
	}

	// Security: Validate and sanitize session name
	if !validateString(session.Name, 256) {
		writeError(w, http.StatusBadRequest, "Invalid session name")
		return
	}
	session.Name = sanitizeContent(session.Name)

	// Security: Validate messages if present
	for i := range session.Messages {
		if !validMessageIDRegex.MatchString(session.Messages[i].ID) {
			writeError(w, http.StatusBadRequest, "Invalid message ID format")
			return
		}
		if !validateMessageContent(session.Messages[i].Content, 100000) {
			writeError(w, http.StatusBadRequest, "Invalid message content")
			return
		}
		session.Messages[i].Content = sanitizeContent(session.Messages[i].Content)
	}
	session.Messages = dedupeMessagesByID(session.Messages)

	if err := a.storage.CreateSession(req.Context(), userID, &session); err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to create session", "userID", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	// Save messages included in the session (e.g., welcome message)
	for _, msg := range session.Messages {
		if err := a.storage.SaveMessage(req.Context(), userID, session.ID, &msg); err != nil {
			// Log but continue - message save failure shouldn't fail session creation
			log.DefaultLogger.Debug("Failed to save initial message", "sessionID", session.ID, "messageID", msg.ID, "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"data":    session,
	})
}

// handleSessionByID handles operations on specific sessions
func (a *App) handleSessionByID(w http.ResponseWriter, req *http.Request) {
	// Security: Extract and validate user ID
	userID, err := extractUserID(req)
	if err != nil {
		log.DefaultLogger.Warn("Authentication failed", "error", err)
		writeAuthRequired(w)
		return
	}

	// Extract session ID from path: /sessions/{id} or /sessions/{id}/...
	path := strings.TrimPrefix(req.URL.Path, "/sessions/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusBadRequest, "Session ID required")
		return
	}

	sessionID := parts[0]

	// Security: Validate session ID format
	if !validSessionIDRegex.MatchString(sessionID) {
		writeError(w, http.StatusBadRequest, "Invalid session ID format")
		return
	}

	// Handle different endpoints
	if len(parts) >= 2 {
		switch parts[1] {
		case "messages":
			if len(parts) >= 3 {
				if len(parts) >= 4 && parts[3] == "pin" {
					// /sessions/{id}/messages/{msgId}/pin
					a.handleTogglePin(w, req, userID, sessionID, parts[2])
				} else {
					// /sessions/{id}/messages/{msgId}
					a.handleUpdateMessage(w, req, userID, sessionID, parts[2])
				}
			} else {
				// /sessions/{id}/messages
				if req.Method == http.MethodGet {
					a.handleGetSessionMessages(w, req, userID, sessionID)
				} else if req.Method == http.MethodPost {
					a.handleAddMessage(w, req, userID, sessionID)
				} else {
					writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
				}
			}
		case "activate":
			// /sessions/{id}/activate
			a.handleActivateSession(w, req, userID, sessionID)
		case "rename":
			// /sessions/{id}/rename
			a.handleRenameSession(w, req, userID, sessionID)
		case "tokens":
			// /sessions/{id}/tokens - token statistics
			a.handleGetSessionTokenStats(w, req, userID, sessionID)
		case "compact":
			// /sessions/{id}/compact
			a.handleCompactSession(w, req, userID, sessionID)
		default:
			writeError(w, http.StatusNotFound, "Endpoint not found")
		}
		return
	}

	// Handle /sessions/{id}
	switch req.Method {
	case http.MethodGet:
		a.handleGetSession(w, req, userID, sessionID)
	case http.MethodPut:
		a.handleUpdateSession(w, req, userID, sessionID)
	case http.MethodDelete:
		a.handleDeleteSession(w, req, userID, sessionID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleRenameSession handles PUT /sessions/{id}/rename
func (a *App) handleRenameSession(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	if req.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		log.DefaultLogger.Warn("Invalid request body for rename", "error", err)
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	trimmedName := strings.TrimSpace(body.Name)
	if !validateString(trimmedName, 256) {
		writeError(w, http.StatusBadRequest, "Invalid session name")
		return
	}
	trimmedName = sanitizeContent(trimmedName)

	session, err := a.storage.GetSession(req.Context(), userID, sessionID)
	if err != nil {
		log.DefaultLogger.Error("Failed to get session for rename", "userID", userID, "sessionID", sessionID, "error", err)
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}

	session.Name = trimmedName
	session.UpdatedAt = time.Now().UnixMilli()

	if err := a.storage.UpdateSession(req.Context(), userID, session); err != nil {
		log.DefaultLogger.Error("Failed to rename session", "userID", userID, "sessionID", sessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to rename session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    session,
	})
}

// handleGetSession handles GET /sessions/{id}
func (a *App) handleGetSession(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	session, err := a.storage.GetSession(req.Context(), userID, sessionID)
	if err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to get session", "userID", userID, "sessionID", sessionID, "error", err)
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    session,
	})
}

// handleUpdateSession handles PUT /sessions/{id}
func (a *App) handleUpdateSession(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	var session storage.ChatSession
	if err := json.NewDecoder(req.Body).Decode(&session); err != nil {
		log.DefaultLogger.Warn("Invalid request body", "error", err)
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Ensure session ID matches URL
	session.ID = sessionID

	// Security: Validate and sanitize session name
	if !validateString(session.Name, 256) {
		writeError(w, http.StatusBadRequest, "Invalid session name")
		return
	}
	session.Name = sanitizeContent(session.Name)

	// Security: Validate messages if present
	for i := range session.Messages {
		if !validMessageIDRegex.MatchString(session.Messages[i].ID) {
			writeError(w, http.StatusBadRequest, "Invalid message ID format")
			return
		}
		if !validateMessageContent(session.Messages[i].Content, 100000) {
			writeError(w, http.StatusBadRequest, "Invalid message content")
			return
		}
		session.Messages[i].Content = sanitizeContent(session.Messages[i].Content)
	}
	session.Messages = dedupeMessagesByID(session.Messages)

	if err := a.storage.UpdateSession(req.Context(), userID, &session); err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to update session", "userID", userID, "sessionID", sessionID, "error", err)
		if isStorageNotFoundError(err) {
			writeError(w, http.StatusNotFound, "Session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to update session")
		return
	}

	// Save messages included in the session update
	for _, msg := range session.Messages {
		if err := a.storage.SaveMessage(req.Context(), userID, session.ID, &msg); err != nil {
			// Log but continue - message save failure shouldn't fail session update
			log.DefaultLogger.Debug("Failed to save message during session update", "sessionID", session.ID, "messageID", msg.ID, "error", err)
		}
	}

	// Invalidate message cache for this session
	if a.messageCache != nil {
		a.messageCache.Invalidate(sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    session,
	})
}

// handleDeleteSession handles DELETE /sessions/{id}
func (a *App) handleDeleteSession(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	if err := a.storage.DeleteSession(req.Context(), userID, sessionID); err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to delete session", "userID", userID, "sessionID", sessionID, "error", err)
		if isStorageNotFoundError(err) {
			writeError(w, http.StatusNotFound, "Session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to delete session")
		return
	}

	// Invalidate message cache for this session
	if a.messageCache != nil {
		a.messageCache.Invalidate(sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"message":   "Session deleted successfully",
		"sessionId": sessionID,
	})
}

// handleAddMessage handles POST /sessions/{id}/messages
func (a *App) handleAddMessage(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var message storage.ChatMessage
	if err := json.NewDecoder(req.Body).Decode(&message); err != nil {
		log.DefaultLogger.Warn("Invalid request body", "error", err)
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Security: Validate message ID
	if !validMessageIDRegex.MatchString(message.ID) {
		writeError(w, http.StatusBadRequest, "Invalid message ID format")
		return
	}

	// Security: Validate and sanitize message content (empty allowed for streaming)
	if !validateMessageContent(message.Content, 100000) {
		writeError(w, http.StatusBadRequest, "Invalid message content")
		return
	}
	message.Content = sanitizeContent(message.Content)

	if err := a.storage.AddMessage(req.Context(), userID, sessionID, &message); err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to add message", "userID", userID, "sessionID", sessionID, "error", err)
		if isStorageNotFoundError(err) {
			writeError(w, http.StatusNotFound, "Session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to add message")
		return
	}

	// Invalidate message cache for this session
	if a.messageCache != nil {
		a.messageCache.Invalidate(sessionID)
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"data":    message,
	})
}

// handleUpdateMessage handles PUT /sessions/{id}/messages/{msgId}
func (a *App) handleUpdateMessage(w http.ResponseWriter, req *http.Request, userID, sessionID, messageID string) {
	if req.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Security: Validate message ID format
	if !validMessageIDRegex.MatchString(messageID) {
		writeError(w, http.StatusBadRequest, "Invalid message ID format")
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		log.DefaultLogger.Warn("Invalid request body", "error", err)
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Security: Validate and sanitize message content (empty allowed for streaming)
	if !validateMessageContent(body.Content, 100000) {
		writeError(w, http.StatusBadRequest, "Invalid message content")
		return
	}
	body.Content = sanitizeContent(body.Content)

	if err := a.storage.UpdateMessage(req.Context(), userID, sessionID, messageID, body.Content); err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to update message", "userID", userID, "sessionID", sessionID, "messageID", messageID, "error", err)
		if isStorageNotFoundError(err) {
			if strings.Contains(strings.ToLower(err.Error()), "message not found") {
				writeError(w, http.StatusNotFound, "Message not found")
			} else {
				writeError(w, http.StatusNotFound, "Session not found")
			}
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to update message")
		return
	}

	// Invalidate message cache for this session
	if a.messageCache != nil {
		a.messageCache.Invalidate(sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"messageId": messageID,
	})
}

// handleTogglePin handles PUT /sessions/{id}/messages/{msgId}/pin
// Toggles the pinned state of a message (Phase 14)
func (a *App) handleTogglePin(w http.ResponseWriter, req *http.Request, userID, sessionID, messageID string) {
	if req.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Security: Validate message ID format
	if !validMessageIDRegex.MatchString(messageID) {
		writeError(w, http.StatusBadRequest, "Invalid message ID format")
		return
	}

	// Parse body for isPinned state
	var body struct {
		IsPinned bool `json:"isPinned"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		log.DefaultLogger.Warn("Invalid request body", "error", err)
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := a.storage.UpdateMessagePinned(req.Context(), userID, sessionID, messageID, body.IsPinned); err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to update pin state", "userID", userID, "sessionID", sessionID, "messageID", messageID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to update pin state")
		return
	}

	// Invalidate message cache for this session
	if a.messageCache != nil {
		a.messageCache.Invalidate(sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"messageId": messageID,
		"isPinned":  body.IsPinned,
	})
}

// handleActivateSession handles POST /sessions/{id}/activate
func (a *App) handleActivateSession(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if err := a.storage.SetActiveSession(req.Context(), userID, sessionID); err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to activate session", "userID", userID, "sessionID", sessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to activate session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"message":   "Session activated successfully",
		"sessionId": sessionID,
	})
}

// handleClearAll handles DELETE /sessions/clear-all
func (a *App) handleClearAll(w http.ResponseWriter, req *http.Request, userID string) {
	if req.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if err := a.storage.ClearAllHistory(req.Context(), userID); err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Failed to clear all history", "userID", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to clear history")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "All chat history cleared successfully",
	})
}

// handleSearch handles GET /search - searches messages across all user sessions
// Query parameters:
//   - q: search query (required)
//   - limit: max results (default 50, max 100)
//   - offset: pagination offset (default 0)
func (a *App) handleSearch(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Security: Extract and validate user ID
	userID, err := extractUserID(req)
	if err != nil {
		log.DefaultLogger.Warn("Authentication failed for search", "error", err)
		writeAuthRequired(w)
		return
	}

	// Security: Check rate limit
	if !a.checkRateLimit(userID) {
		log.DefaultLogger.Warn("Rate limit exceeded for search", "userID", userID)
		writeError(w, http.StatusTooManyRequests, "Rate limit exceeded")
		return
	}

	// Get required query parameter
	query := req.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "Query parameter 'q' is required")
		return
	}

	// Security: Validate query string
	if !validateString(query, 1000) {
		writeError(w, http.StatusBadRequest, "Invalid search query")
		return
	}

	// Parse optional pagination parameters
	limit := 50
	if limitStr := req.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := parsePositiveInt(limitStr); err == nil {
			limit = parsedLimit
		}
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if offsetStr := req.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := parsePositiveInt(offsetStr); err == nil {
			offset = parsedOffset
		}
	}

	// Execute search
	params := storage.SearchParams{
		UserID: userID,
		Query:  query,
		Limit:  limit,
		Offset: offset,
	}

	results, err := a.storage.SearchMessages(req.Context(), params)
	if err != nil {
		// Security: Log detailed error internally, return generic error to client
		log.DefaultLogger.Error("Search failed", "userID", userID, "query", query, "error", err)
		writeError(w, http.StatusInternalServerError, "Search failed")
		return
	}

	// Return empty array if no results (not null)
	if results == nil {
		results = []storage.SearchResult{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"total":   len(results),
	})
}

// parsePositiveInt parses a string to a positive integer
func parsePositiveInt(s string) (int, error) {
	var result int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer")
		}
		result = result*10 + int(c-'0')
	}
	return result, nil
}

// handleGetSessionMessages handles GET /sessions/{id}/messages with pagination (PERF-07, PERF-08)
// Query params: limit (default 50, max 100), cursor (optional message ID for pagination)
// Returns messages with hasMore flag and nextCursor for infinite scroll
func (a *App) handleGetSessionMessages(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	// Parse pagination params
	limit := 50
	if l := req.URL.Query().Get("limit"); l != "" {
		if parsed, err := parsePositiveInt(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	cursor := req.URL.Query().Get("cursor")

	// Check cache for initial page (no cursor)
	if cursor == "" && a.messageCache != nil {
		if msgs, ok := a.messageCache.Get(sessionID); ok {
			hasMore := len(msgs) > limit
			messages := msgs
			if hasMore {
				messages = msgs[:limit]
			}
			nextCursor := ""
			if hasMore && len(messages) > 0 {
				nextCursor = messages[len(messages)-1].ID
			}
			// Track session access for pre-loading (PERF-03)
			if a.preloader != nil {
				a.preloader.TrackAccess(userID, sessionID)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"messages":   messages,
				"hasMore":    hasMore,
				"nextCursor": nextCursor,
			})
			return
		}
	}

	// Fetch from storage using existing GetSessionMessages
	params := storage.GetSessionMessagesParams{
		SessionID: sessionID,
		Limit:     limit + 1, // Fetch extra to determine hasMore
		Cursor:    cursor,
	}
	page, err := a.storage.GetSessionMessages(req.Context(), userID, params)
	if err != nil {
		log.DefaultLogger.Error("Failed to get session messages", "sessionID", sessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to get messages")
		return
	}

	hasMore := len(page.Messages) > limit
	messages := page.Messages
	if messages == nil {
		messages = []storage.ChatMessage{} // Ensure empty array, not null in JSON
	}
	if hasMore {
		messages = messages[:limit]
	}

	// Cache initial page only
	if cursor == "" && a.messageCache != nil {
		a.messageCache.Set(sessionID, messages)
	}

	nextCursor := ""
	if hasMore && len(messages) > 0 {
		nextCursor = messages[len(messages)-1].ID
	}

	// Track session access for pre-loading (PERF-03)
	if a.preloader != nil {
		a.preloader.TrackAccess(userID, sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"messages":   messages,
		"hasMore":    hasMore,
		"nextCursor": nextCursor,
	})
}

// handleGetSessionTokenStats handles GET /sessions/{id}/tokens
// Returns token statistics for a session including total tokens, context limit, and usage percentage
func (a *App) handleGetSessionTokenStats(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	if req.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if TokenService is available
	if a.tokenService == nil {
		// Fallback: compute stats directly from session data
		session, err := a.storage.GetSession(req.Context(), userID, sessionID)
		if err != nil {
			log.DefaultLogger.Error("Failed to get session for token stats", "userID", userID, "sessionID", sessionID, "error", err)
			writeError(w, http.StatusNotFound, "Session not found")
			return
		}

		// Compute stats manually
		var totalTokens, uncountedMsgs int
		for _, msg := range session.Messages {
			totalTokens += msg.TokenCount
			if msg.TokenCount == 0 {
				uncountedMsgs++
			}
		}

		// Default context limit (100k)
		contextLimit := storage.DefaultContextLimit
		contextUsage := 0.0
		if contextLimit > 0 {
			contextUsage = (float64(totalTokens) / float64(contextLimit)) * 100.0
		}

		writeJSON(w, http.StatusOK, storage.TokenStats{
			SessionID:     sessionID,
			TotalTokens:   totalTokens,
			ContextLimit:  contextLimit,
			ContextUsage:  contextUsage,
			MessageCount:  len(session.Messages),
			UncountedMsgs: uncountedMsgs,
		})
		return
	}

	// Use TokenService for stats
	stats, err := a.tokenService.GetSessionTokenStats(req.Context(), userID, sessionID)
	if err != nil {
		log.DefaultLogger.Error("Failed to get session token stats", "userID", userID, "sessionID", sessionID, "error", err)
		writeError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Auto-compaction when threshold is reached.
	threshold := a.autoCompactThreshold
	if threshold <= 0 {
		threshold = 100
	}
	if stats.ContextUsage >= threshold {
		for pass := 0; pass < maxAutoCompactionPasses && stats.ContextUsage >= threshold; pass++ {
			compacted, compactErr := a.compactSession(req.Context(), userID, sessionID)
			if compactErr != nil {
				log.DefaultLogger.Warn("Auto-compaction failed", "userID", userID, "sessionID", sessionID, "error", compactErr)
				break
			}
			if !compacted {
				break
			}

			updatedStats, statsErr := a.tokenService.GetSessionTokenStats(req.Context(), userID, sessionID)
			if statsErr != nil {
				log.DefaultLogger.Warn("Failed to refresh token stats after auto-compaction", "userID", userID, "sessionID", sessionID, "error", statsErr)
				break
			}
			stats = updatedStats
		}
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleCompactSession handles POST /sessions/{id}/compact
// Triggers one compaction pass for the session.
func (a *App) handleCompactSession(w http.ResponseWriter, req *http.Request, userID, sessionID string) {
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	compacted, err := a.compactSession(req.Context(), userID, sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "does not support") {
			writeError(w, http.StatusNotImplemented, "Compaction is not supported by current storage backend")
			return
		}
		log.DefaultLogger.Error("Failed to compact session", "userID", userID, "sessionID", sessionID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to compact session")
		return
	}

	response := map[string]interface{}{
		"success":   true,
		"compacted": compacted,
	}

	if a.tokenService != nil {
		if stats, statsErr := a.tokenService.GetSessionTokenStats(req.Context(), userID, sessionID); statsErr == nil {
			response["tokenStats"] = stats
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// compactSession summarizes the oldest unsummarized messages once.
// Returns true when compaction changed the session.
func (a *App) compactSession(ctx context.Context, userID, sessionID string) (bool, error) {
	batchTokens := a.compactionBatchTokens
	if batchTokens <= 0 {
		batchTokens = defaultCompactionBatchTokens
	}

	messages, err := a.storage.GetOldestNonSummaryMessages(ctx, sessionID, batchTokens)
	if err != nil {
		return false, err
	}
	if len(messages) == 0 {
		return false, nil
	}

	// Skip tiny batches to avoid noisy summaries.
	if len(messages) < 2 {
		return false, nil
	}

	summaryContent := buildCompactionSummary(messages)
	if strings.TrimSpace(summaryContent) == "" {
		return false, nil
	}

	earliestTimestamp := messages[0].Timestamp
	for _, msg := range messages {
		if msg.Timestamp < earliestTimestamp {
			earliestTimestamp = msg.Timestamp
		}
	}

	originalIDs := make([]string, 0, len(messages))
	for _, msg := range messages {
		originalIDs = append(originalIDs, msg.ID)
	}

	summary := &storage.ChatMessage{
		ID:         fmt.Sprintf("summary_%d", time.Now().UnixNano()),
		Role:       "system",
		Content:    summaryContent,
		Timestamp:  earliestTimestamp,
		TokenCount: estimateTokenCount(summaryContent),
		IsSummary:  true,
	}

	if err := a.storage.SaveSummary(ctx, userID, sessionID, summary, originalIDs); err != nil {
		return false, err
	}

	if a.messageCache != nil {
		a.messageCache.Invalidate(sessionID)
	}

	return true, nil
}

// buildCompactionSummary creates a deterministic compact summary when backend compaction is triggered.
func buildCompactionSummary(messages []storage.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Compacted summary of %d earlier messages:\n", len(messages)))

	const maxLines = 20
	const maxCharsPerLine = 220
	const maxTotalChars = 3200

	lineCount := 0
	for _, msg := range messages {
		if lineCount >= maxLines || builder.Len() >= maxTotalChars {
			break
		}

		content := strings.TrimSpace(msg.Content)
		content = strings.ReplaceAll(content, "\n", " ")
		content = strings.ReplaceAll(content, "\r", " ")
		content = strings.Join(strings.Fields(content), " ")
		if content == "" {
			continue
		}
		if len(content) > maxCharsPerLine {
			content = content[:maxCharsPerLine] + "..."
		}

		role := msg.Role
		if role == "" {
			role = "message"
		}
		builder.WriteString(fmt.Sprintf("- %s: %s\n", role, content))
		lineCount++
	}

	return strings.TrimSpace(builder.String())
}

// estimateTokenCount provides a lightweight token estimate for backend summaries.
func estimateTokenCount(content string) int {
	if content == "" {
		return 0
	}
	// Conservative approximation for mixed natural language.
	return (len(content) + 3) / 4
}
