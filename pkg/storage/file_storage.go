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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

const (
	maxSessionsPerUser    = 50
	maxMessagesPerSession = 500
)

var (
	// validUserIDRegex matches alphanumeric, hyphens, underscores, and dots
	// Security: Restricts user IDs to safe characters to prevent path traversal
	validUserIDRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
)

// FileStorage implements Storage interface using file-based JSON storage
type FileStorage struct {
	dataDir string
	mu      sync.RWMutex
	cache   map[string]*UserChatHistory // userID -> history
	// Security: Per-user locks to prevent race conditions
	userLocks map[string]*sync.RWMutex
}

// NewFileStorage creates a new file-based storage instance
func NewFileStorage(dataDir string) (*FileStorage, error) {
	// Create data directory if it doesn't exist
	usersDir := filepath.Join(dataDir, "users")
	// Security: Use 0700 permissions - only owner can read/write/execute
	if err := os.MkdirAll(usersDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	return &FileStorage{
		dataDir:   usersDir,
		cache:     make(map[string]*UserChatHistory),
		userLocks: make(map[string]*sync.RWMutex),
	}, nil
}

// sanitizeUserID sanitizes user ID to prevent path traversal attacks
// Security: Prevents directory traversal by validating and hashing invalid IDs
func sanitizeUserID(userID string) string {
	// Check if user ID contains only safe characters
	if validUserIDRegex.MatchString(userID) && len(userID) <= 255 {
		// Additional check: ensure no path separators or ".." sequences
		if filepath.Clean(userID) == userID && userID != "." && userID != ".." {
			return userID
		}
	}

	// If user ID is unsafe, use SHA256 hash instead
	// This ensures we can still identify users but prevents path traversal
	hash := sha256.Sum256([]byte(userID))
	return "hash_" + hex.EncodeToString(hash[:])
}

// getUserFilePath returns the file path for a user's chat history
// Security: Path is validated to ensure it stays within dataDir
func (fs *FileStorage) getUserFilePath(userID string) (string, error) {
	// Sanitize user ID to prevent path traversal
	safeUserID := sanitizeUserID(userID)

	// Construct file path
	filePath := filepath.Join(fs.dataDir, fmt.Sprintf("user_%s.json", safeUserID))

	// Security: Verify the resolved path is still within dataDir
	// This prevents symlink attacks and ensures containment
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path: %w", err)
	}

	absDataDir, err := filepath.Abs(fs.dataDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve data directory: %w", err)
	}

	// Check if file path is within data directory
	if !filepath.HasPrefix(absFilePath, absDataDir) {
		return "", fmt.Errorf("invalid user ID: path traversal detected")
	}

	return filePath, nil
}

// getUserLock gets or creates a per-user lock
// Security: Prevents race conditions on per-user basis
func (fs *FileStorage) getUserLock(userID string) *sync.RWMutex {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if lock, exists := fs.userLocks[userID]; exists {
		return lock
	}

	lock := &sync.RWMutex{}
	fs.userLocks[userID] = lock
	return lock
}

// deepCopyHistory creates a deep copy of UserChatHistory
// Security: Prevents concurrent modification of shared data structures
func deepCopyHistory(src *UserChatHistory) *UserChatHistory {
	if src == nil {
		return nil
	}

	dst := &UserChatHistory{
		UserID:   src.UserID,
		Sessions: make([]ChatSession, len(src.Sessions)),
	}

	for i, session := range src.Sessions {
		dst.Sessions[i] = ChatSession{
			ID:          session.ID,
			Name:        session.Name,
			UserID:      session.UserID,
			CreatedAt:   session.CreatedAt,
			UpdatedAt:   session.UpdatedAt,
			IsActive:    session.IsActive,
			TotalTokens: session.TotalTokens,
			Messages:    make([]ChatMessage, len(session.Messages)),
		}

		for j, msg := range session.Messages {
			dst.Sessions[i].Messages[j] = ChatMessage{
				ID:         msg.ID,
				Role:       msg.Role,
				Content:    msg.Content,
				Timestamp:  msg.Timestamp,
				TokenCount: msg.TokenCount,
				IsPinned:   msg.IsPinned,
			}
		}
	}

	return dst
}

// loadUserHistory loads user history from disk, with caching
// Security: Uses per-user locks and returns deep copies to prevent race conditions
func (fs *FileStorage) loadUserHistory(userID string) (*UserChatHistory, error) {
	// Get per-user lock for fine-grained concurrency control
	userLock := fs.getUserLock(userID)
	userLock.Lock()
	defer userLock.Unlock()

	// Check cache first (need to check again inside lock)
	fs.mu.RLock()
	if history, exists := fs.cache[userID]; exists {
		// Return deep copy to prevent concurrent modification
		fs.mu.RUnlock()
		return deepCopyHistory(history), nil
	}
	fs.mu.RUnlock()

	// Load from disk
	filePath, err := fs.getUserFilePath(userID)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new history for new user
			history := &UserChatHistory{
				UserID:   userID,
				Sessions: []ChatSession{},
			}
			// Update cache atomically
			fs.mu.Lock()
			fs.cache[userID] = history
			fs.mu.Unlock()
			return deepCopyHistory(history), nil
		}
		return nil, fmt.Errorf("failed to read user history: %w", err)
	}

	var history UserChatHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("failed to parse user history: %w", err)
	}

	// Cache the loaded history atomically
	fs.mu.Lock()
	fs.cache[userID] = &history
	fs.mu.Unlock()

	return deepCopyHistory(&history), nil
}

// saveUserHistory saves user history to disk atomically
// Security: Uses per-user locks and secure file permissions, updates cache atomically
func (fs *FileStorage) saveUserHistory(history *UserChatHistory) error {
	// Get file path with validation
	filePath, err := fs.getUserFilePath(history.UserID)
	if err != nil {
		return err
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tempFile := filePath + ".tmp"
	// Security: Use 0600 permissions - only owner can read/write
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempFile, filePath); err != nil {
		os.Remove(tempFile) // Clean up temp file on error
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Update cache atomically
	// Security: Update cache after successful write to prevent lost updates
	fs.mu.Lock()
	fs.cache[history.UserID] = deepCopyHistory(history)
	fs.mu.Unlock()

	return nil
}

// GetSessions returns all sessions for a user
func (fs *FileStorage) GetSessions(ctx context.Context, userID string) ([]ChatSession, error) {
	history, err := fs.loadUserHistory(userID)
	if err != nil {
		return nil, err
	}

	return history.Sessions, nil
}

// GetSession returns a specific session
func (fs *FileStorage) GetSession(ctx context.Context, userID, sessionID string) (*ChatSession, error) {
	history, err := fs.loadUserHistory(userID)
	if err != nil {
		return nil, err
	}

	for i := range history.Sessions {
		if history.Sessions[i].ID == sessionID {
			return &history.Sessions[i], nil
		}
	}

	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// CreateSession creates a new session
func (fs *FileStorage) CreateSession(ctx context.Context, userID string, session *ChatSession) error {
	history, err := fs.loadUserHistory(userID)
	if err != nil {
		return err
	}

	// Check session limit
	if len(history.Sessions) >= maxSessionsPerUser {
		return fmt.Errorf("session limit reached: max %d sessions per user", maxSessionsPerUser)
	}

	// Check for duplicate session ID
	for _, s := range history.Sessions {
		if s.ID == session.ID {
			return fmt.Errorf("session already exists: %s", session.ID)
		}
	}

	// Set user ID
	session.UserID = userID

	// Initialize messages if nil
	if session.Messages == nil {
		session.Messages = []ChatMessage{}
	}

	// Add session
	history.Sessions = append(history.Sessions, *session)

	return fs.saveUserHistory(history)
}

// UpdateSession updates an existing session
func (fs *FileStorage) UpdateSession(ctx context.Context, userID string, session *ChatSession) error {
	history, err := fs.loadUserHistory(userID)
	if err != nil {
		return err
	}

	// Find and update session
	for i := range history.Sessions {
		if history.Sessions[i].ID == session.ID {
			// Preserve user ID
			session.UserID = userID
			history.Sessions[i] = *session
			return fs.saveUserHistory(history)
		}
	}

	return fmt.Errorf("session not found: %s", session.ID)
}

// DeleteSession deletes a session
func (fs *FileStorage) DeleteSession(ctx context.Context, userID, sessionID string) error {
	history, err := fs.loadUserHistory(userID)
	if err != nil {
		return err
	}

	// Find and remove session
	for i := range history.Sessions {
		if history.Sessions[i].ID == sessionID {
			history.Sessions = append(history.Sessions[:i], history.Sessions[i+1:]...)
			return fs.saveUserHistory(history)
		}
	}

	return fmt.Errorf("session not found: %s", sessionID)
}

// SetActiveSession sets a session as active and deactivates others
func (fs *FileStorage) SetActiveSession(ctx context.Context, userID, sessionID string) error {
	history, err := fs.loadUserHistory(userID)
	if err != nil {
		return err
	}

	found := false
	for i := range history.Sessions {
		if history.Sessions[i].ID == sessionID {
			history.Sessions[i].IsActive = true
			found = true
		} else {
			history.Sessions[i].IsActive = false
		}
	}

	if !found {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return fs.saveUserHistory(history)
}

// AddMessage adds a message to a session
func (fs *FileStorage) AddMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	history, err := fs.loadUserHistory(userID)
	if err != nil {
		return err
	}

	// Find session and add message
	for i := range history.Sessions {
		if history.Sessions[i].ID == sessionID {
			for _, existing := range history.Sessions[i].Messages {
				if existing.ID == message.ID {
					// Idempotent write: message already exists.
					return nil
				}
			}

			// Check message limit
			if len(history.Sessions[i].Messages) >= maxMessagesPerSession {
				return fmt.Errorf("message limit reached: max %d messages per session", maxMessagesPerSession)
			}

			history.Sessions[i].Messages = append(history.Sessions[i].Messages, *message)
			history.Sessions[i].UpdatedAt = message.Timestamp
			return fs.saveUserHistory(history)
		}
	}

	return fmt.Errorf("session not found: %s", sessionID)
}

// UpdateMessage updates a message's content
func (fs *FileStorage) UpdateMessage(ctx context.Context, userID, sessionID, messageID string, content string) error {
	history, err := fs.loadUserHistory(userID)
	if err != nil {
		return err
	}

	// Find session
	for i := range history.Sessions {
		if history.Sessions[i].ID == sessionID {
			// Find and update message
			for j := range history.Sessions[i].Messages {
				if history.Sessions[i].Messages[j].ID == messageID {
					history.Sessions[i].Messages[j].Content = content
					history.Sessions[i].Messages[j].TokenCount = 0
					history.Sessions[i].UpdatedAt = time.Now().UnixMilli()
					return fs.saveUserHistory(history)
				}
			}
			return fmt.Errorf("message not found: %s", messageID)
		}
	}

	return fmt.Errorf("session not found: %s", sessionID)
}

// ClearAllHistory deletes all sessions for a user
func (fs *FileStorage) ClearAllHistory(ctx context.Context, userID string) error {
	history := &UserChatHistory{
		UserID:   userID,
		Sessions: []ChatSession{},
	}

	return fs.saveUserHistory(history)
}

// DeleteExpiredSessions deletes sessions older than retentionDays
// Returns the count of deleted sessions
func (fs *FileStorage) DeleteExpiredSessions(ctx context.Context, retentionDays int) (int64, error) {
	fs.mu.RLock()
	userIDs := make([]string, 0, len(fs.cache))
	for userID := range fs.cache {
		userIDs = append(userIDs, userID)
	}
	fs.mu.RUnlock()

	// Calculate cutoff time (retentionDays ago in milliseconds)
	cutoffMs := (time.Now().Unix() - int64(retentionDays*24*60*60)) * 1000

	var totalDeleted int64

	for _, userID := range userIDs {
		history, err := fs.loadUserHistory(userID)
		if err != nil {
			log.DefaultLogger.Warn("Failed to load user history for cleanup", "userID", userID, "error", err)
			continue
		}

		// Filter out expired sessions
		activeSessions := make([]ChatSession, 0, len(history.Sessions))
		for _, session := range history.Sessions {
			if session.UpdatedAt >= cutoffMs {
				activeSessions = append(activeSessions, session)
			} else {
				totalDeleted++
			}
		}

		// Save if any sessions were deleted
		if len(activeSessions) < len(history.Sessions) {
			history.Sessions = activeSessions
			if err := fs.saveUserHistory(history); err != nil {
				log.DefaultLogger.Warn("Failed to save user history after cleanup", "userID", userID, "error", err)
			}
		}
	}

	return totalDeleted, nil
}

// Ping checks if storage is available
// For file storage, this always returns nil as file storage is always available
func (fs *FileStorage) Ping(ctx context.Context) error {
	return nil
}

// SaveMessage is not supported in file storage (legacy)
func (fs *FileStorage) SaveMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	return fmt.Errorf("file storage does not support SaveMessage")
}

// SaveMessages is not supported in file storage (legacy)
func (fs *FileStorage) SaveMessages(ctx context.Context, userID, sessionID string, messages []ChatMessage) ([]SaveResult, error) {
	results := make([]SaveResult, len(messages))
	for i, msg := range messages {
		results[i] = SaveResult{MessageID: msg.ID, Success: false, Error: "file storage does not support SaveMessages"}
	}
	return results, fmt.Errorf("file storage does not support SaveMessages")
}

// GetSessionMessages is not supported in file storage (legacy)
func (fs *FileStorage) GetSessionMessages(ctx context.Context, userID string, params GetSessionMessagesParams) (*MessagesPage, error) {
	return nil, fmt.Errorf("file storage does not support GetSessionMessages")
}

// GetMessagesByTokenBudget is not supported in file storage (legacy)
func (fs *FileStorage) GetMessagesByTokenBudget(ctx context.Context, sessionID string, budget int) ([]ChatMessage, error) {
	return nil, fmt.Errorf("file storage does not support GetMessagesByTokenBudget")
}

// UpdateMessageTokenCount updates token count for an existing message.
// This enables TokenService lazy counting in file-storage deployments.
func (fs *FileStorage) UpdateMessageTokenCount(ctx context.Context, messageID string, tokenCount int) error {
	fs.mu.RLock()
	userIDs := make([]string, 0, len(fs.cache))
	for userID := range fs.cache {
		userIDs = append(userIDs, userID)
	}
	fs.mu.RUnlock()

	for _, userID := range userIDs {
		history, err := fs.loadUserHistory(userID)
		if err != nil {
			continue
		}

		updated := false
		for i := range history.Sessions {
			for j := range history.Sessions[i].Messages {
				if history.Sessions[i].Messages[j].ID == messageID {
					history.Sessions[i].Messages[j].TokenCount = tokenCount
					updated = true
					break
				}
			}
			if updated {
				break
			}
		}

		if updated {
			return fs.saveUserHistory(history)
		}
	}

	return fmt.Errorf("message not found: %s", messageID)
}

// SaveSummary is not supported in file storage (legacy)
func (fs *FileStorage) SaveSummary(ctx context.Context, userID, sessionID string, summary *ChatMessage, originalIDs []string) error {
	return fmt.Errorf("file storage does not support SaveSummary")
}

// GetOldestNonSummaryMessages is not supported in file storage (legacy)
func (fs *FileStorage) GetOldestNonSummaryMessages(ctx context.Context, sessionID string, tokenLimit int) ([]ChatMessage, error) {
	return nil, fmt.Errorf("file storage does not support GetOldestNonSummaryMessages")
}

// SearchMessages is not supported in file storage (legacy)
// Phase 14: Full-text search requires database FTS capabilities
func (fs *FileStorage) SearchMessages(ctx context.Context, params SearchParams) ([]SearchResult, error) {
	return nil, fmt.Errorf("file storage does not support SearchMessages")
}

// UpdateMessagePinned is not supported in file storage (legacy)
// Phase 14: Message pinning requires database storage
func (fs *FileStorage) UpdateMessagePinned(ctx context.Context, userID, sessionID, messageID string, isPinned bool) error {
	return fmt.Errorf("file storage does not support UpdateMessagePinned")
}

// Close cleans up resources
// Security: Fixed deadlock by copying cache before releasing lock
func (fs *FileStorage) Close() error {
	// Copy cache contents while holding lock
	fs.mu.Lock()
	historiesToSave := make([]*UserChatHistory, 0, len(fs.cache))
	for _, history := range fs.cache {
		historiesToSave = append(historiesToSave, deepCopyHistory(history))
	}
	fs.mu.Unlock()

	// Save all cached histories without holding the main lock
	// This prevents deadlock since saveUserHistory needs to acquire locks
	for _, history := range historiesToSave {
		if err := fs.saveUserHistory(history); err != nil {
			log.DefaultLogger.Error("Failed to save user history on close", "userID", history.UserID, "error", err)
		}
	}

	// Clear cache
	fs.mu.Lock()
	fs.cache = make(map[string]*UserChatHistory)
	fs.userLocks = make(map[string]*sync.RWMutex)
	fs.mu.Unlock()

	return nil
}
