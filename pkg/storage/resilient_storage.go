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
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Prometheus gauge for degraded mode status
	storageDegradedGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "aichat",
			Name:      "storage_degraded",
			Help:      "Whether storage is in degraded mode (1) or normal (0)",
		},
	)
)

// ResilientStorage wraps a primary storage with circuit breaker and fallback
// It provides graceful degradation when the primary storage fails
type ResilientStorage struct {
	primary  Storage // DBStorage
	fallback Storage // MemoryStorage
	logger   log.Logger

	// Circuit breaker state
	failures     int32 // atomic counter
	threshold    int32 // failures before opening circuit
	lastFailure  time.Time
	resetTimeout time.Duration
	degraded     atomic.Bool
	mu           sync.RWMutex
}

// NewResilientStorage creates a new resilient storage wrapper
func NewResilientStorage(primary, fallback Storage, logger log.Logger) *ResilientStorage {
	return &ResilientStorage{
		primary:      primary,
		fallback:     fallback,
		logger:       logger,
		threshold:    5,                  // 5 failures before circuit opens
		resetTimeout: 30 * time.Second,   // Try primary again after 30 seconds
	}
}

// isCircuitOpen checks if we should use fallback storage
func (rs *ResilientStorage) isCircuitOpen() bool {
	if !rs.degraded.Load() {
		return false
	}

	// Check if we should try primary again
	rs.mu.RLock()
	lastFailure := rs.lastFailure
	rs.mu.RUnlock()

	if time.Since(lastFailure) > rs.resetTimeout {
		// Try to reset circuit
		rs.logger.Info("Attempting to reconnect to primary storage")
		return false
	}

	return true
}

// recordFailure records a primary storage failure
func (rs *ResilientStorage) recordFailure(err error) {
	failures := atomic.AddInt32(&rs.failures, 1)

	rs.mu.Lock()
	rs.lastFailure = time.Now()
	rs.mu.Unlock()

	if failures >= rs.threshold {
		wasOpen := rs.degraded.Swap(true)
		if !wasOpen {
			rs.logger.Warn("Primary storage circuit opened - switching to fallback",
				"failures", failures,
				"threshold", rs.threshold,
				"error", err)
			storageDegradedGauge.Set(1)
		}
	}
}

// recordSuccess records a successful primary storage operation
func (rs *ResilientStorage) recordSuccess() {
	if atomic.LoadInt32(&rs.failures) > 0 {
		atomic.StoreInt32(&rs.failures, 0)
	}

	if rs.degraded.Swap(false) {
		rs.logger.Info("Primary storage recovered - circuit closed")
		storageDegradedGauge.Set(0)
	}
}

// IsDegraded returns whether storage is in degraded mode
func (rs *ResilientStorage) IsDegraded() bool {
	return rs.degraded.Load()
}

// GetSessions returns all sessions for a user
func (rs *ResilientStorage) GetSessions(ctx context.Context, userID string) ([]ChatSession, error) {
	if rs.isCircuitOpen() {
		return rs.fallback.GetSessions(ctx, userID)
	}

	result, err := rs.primary.GetSessions(ctx, userID)
	if err != nil {
		rs.recordFailure(err)
		return rs.fallback.GetSessions(ctx, userID)
	}

	rs.recordSuccess()
	return result, nil
}

// GetSession returns a specific session
func (rs *ResilientStorage) GetSession(ctx context.Context, userID, sessionID string) (*ChatSession, error) {
	if rs.isCircuitOpen() {
		return rs.fallback.GetSession(ctx, userID, sessionID)
	}

	result, err := rs.primary.GetSession(ctx, userID, sessionID)
	if err != nil {
		// Only treat connection errors as circuit-breaking failures
		// "session not found" is a normal error, not a storage failure
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.GetSession(ctx, userID, sessionID)
		}
		return nil, err
	}

	rs.recordSuccess()
	return result, nil
}

// CreateSession creates a new session
func (rs *ResilientStorage) CreateSession(ctx context.Context, userID string, session *ChatSession) error {
	if rs.isCircuitOpen() {
		return rs.fallback.CreateSession(ctx, userID, session)
	}

	err := rs.primary.CreateSession(ctx, userID, session)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.CreateSession(ctx, userID, session)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// UpdateSession updates an existing session
func (rs *ResilientStorage) UpdateSession(ctx context.Context, userID string, session *ChatSession) error {
	if rs.isCircuitOpen() {
		return rs.fallback.UpdateSession(ctx, userID, session)
	}

	err := rs.primary.UpdateSession(ctx, userID, session)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.UpdateSession(ctx, userID, session)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// DeleteSession deletes a session
func (rs *ResilientStorage) DeleteSession(ctx context.Context, userID, sessionID string) error {
	if rs.isCircuitOpen() {
		return rs.fallback.DeleteSession(ctx, userID, sessionID)
	}

	err := rs.primary.DeleteSession(ctx, userID, sessionID)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.DeleteSession(ctx, userID, sessionID)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// SetActiveSession sets a session as active
func (rs *ResilientStorage) SetActiveSession(ctx context.Context, userID, sessionID string) error {
	if rs.isCircuitOpen() {
		return rs.fallback.SetActiveSession(ctx, userID, sessionID)
	}

	err := rs.primary.SetActiveSession(ctx, userID, sessionID)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.SetActiveSession(ctx, userID, sessionID)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// AddMessage adds a message to a session
func (rs *ResilientStorage) AddMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	if rs.isCircuitOpen() {
		return rs.fallback.AddMessage(ctx, userID, sessionID, message)
	}

	err := rs.primary.AddMessage(ctx, userID, sessionID, message)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.AddMessage(ctx, userID, sessionID, message)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// UpdateMessage updates a message's content
func (rs *ResilientStorage) UpdateMessage(ctx context.Context, userID, sessionID, messageID string, content string) error {
	if rs.isCircuitOpen() {
		return rs.fallback.UpdateMessage(ctx, userID, sessionID, messageID, content)
	}

	err := rs.primary.UpdateMessage(ctx, userID, sessionID, messageID, content)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.UpdateMessage(ctx, userID, sessionID, messageID, content)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// ClearAllHistory clears all history for a user
func (rs *ResilientStorage) ClearAllHistory(ctx context.Context, userID string) error {
	if rs.isCircuitOpen() {
		return rs.fallback.ClearAllHistory(ctx, userID)
	}

	err := rs.primary.ClearAllHistory(ctx, userID)
	if err != nil {
		rs.recordFailure(err)
		return rs.fallback.ClearAllHistory(ctx, userID)
	}

	rs.recordSuccess()
	return nil
}

// DeleteExpiredSessions deletes expired sessions
func (rs *ResilientStorage) DeleteExpiredSessions(ctx context.Context, retentionDays int) (int64, error) {
	if rs.isCircuitOpen() {
		return rs.fallback.DeleteExpiredSessions(ctx, retentionDays)
	}

	count, err := rs.primary.DeleteExpiredSessions(ctx, retentionDays)
	if err != nil {
		rs.recordFailure(err)
		return rs.fallback.DeleteExpiredSessions(ctx, retentionDays)
	}

	rs.recordSuccess()
	return count, nil
}

// Ping checks storage connectivity
func (rs *ResilientStorage) Ping(ctx context.Context) error {
	// Always try primary for Ping to detect recovery
	err := rs.primary.Ping(ctx)
	if err != nil {
		rs.recordFailure(err)
		// Return nil since fallback is always available
		return nil
	}

	rs.recordSuccess()
	return nil
}

// SaveMessage saves a message with circuit breaker
func (rs *ResilientStorage) SaveMessage(ctx context.Context, userID, sessionID string, message *ChatMessage) error {
	if rs.isCircuitOpen() {
		return rs.fallback.SaveMessage(ctx, userID, sessionID, message)
	}

	err := rs.primary.SaveMessage(ctx, userID, sessionID, message)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.SaveMessage(ctx, userID, sessionID, message)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// SaveMessages saves multiple messages with circuit breaker
func (rs *ResilientStorage) SaveMessages(ctx context.Context, userID, sessionID string, messages []ChatMessage) ([]SaveResult, error) {
	if rs.isCircuitOpen() {
		return rs.fallback.SaveMessages(ctx, userID, sessionID, messages)
	}

	results, err := rs.primary.SaveMessages(ctx, userID, sessionID, messages)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.SaveMessages(ctx, userID, sessionID, messages)
		}
		return results, err
	}

	rs.recordSuccess()
	return results, nil
}

// GetSessionMessages returns paginated messages with circuit breaker
func (rs *ResilientStorage) GetSessionMessages(ctx context.Context, userID string, params GetSessionMessagesParams) (*MessagesPage, error) {
	if rs.isCircuitOpen() {
		return rs.fallback.GetSessionMessages(ctx, userID, params)
	}

	result, err := rs.primary.GetSessionMessages(ctx, userID, params)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.GetSessionMessages(ctx, userID, params)
		}
		return nil, err
	}

	rs.recordSuccess()
	return result, nil
}

// GetMessagesByTokenBudget retrieves messages by budget with circuit breaker
func (rs *ResilientStorage) GetMessagesByTokenBudget(ctx context.Context, sessionID string, budget int) ([]ChatMessage, error) {
	if rs.isCircuitOpen() {
		return rs.fallback.GetMessagesByTokenBudget(ctx, sessionID, budget)
	}

	result, err := rs.primary.GetMessagesByTokenBudget(ctx, sessionID, budget)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.GetMessagesByTokenBudget(ctx, sessionID, budget)
		}
		return nil, err
	}

	rs.recordSuccess()
	return result, nil
}

// UpdateMessageTokenCount updates token count with circuit breaker
func (rs *ResilientStorage) UpdateMessageTokenCount(ctx context.Context, messageID string, tokenCount int) error {
	if rs.isCircuitOpen() {
		return rs.fallback.UpdateMessageTokenCount(ctx, messageID, tokenCount)
	}

	err := rs.primary.UpdateMessageTokenCount(ctx, messageID, tokenCount)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.UpdateMessageTokenCount(ctx, messageID, tokenCount)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// SaveSummary saves a summary message with circuit breaker
func (rs *ResilientStorage) SaveSummary(ctx context.Context, userID, sessionID string, summary *ChatMessage, originalIDs []string) error {
	if rs.isCircuitOpen() {
		return rs.fallback.SaveSummary(ctx, userID, sessionID, summary, originalIDs)
	}

	err := rs.primary.SaveSummary(ctx, userID, sessionID, summary, originalIDs)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.SaveSummary(ctx, userID, sessionID, summary, originalIDs)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// GetOldestNonSummaryMessages retrieves oldest non-summary messages with circuit breaker
func (rs *ResilientStorage) GetOldestNonSummaryMessages(ctx context.Context, sessionID string, tokenLimit int) ([]ChatMessage, error) {
	if rs.isCircuitOpen() {
		return rs.fallback.GetOldestNonSummaryMessages(ctx, sessionID, tokenLimit)
	}

	result, err := rs.primary.GetOldestNonSummaryMessages(ctx, sessionID, tokenLimit)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.GetOldestNonSummaryMessages(ctx, sessionID, tokenLimit)
		}
		return nil, err
	}

	rs.recordSuccess()
	return result, nil
}

// SearchMessages searches for messages with circuit breaker (Phase 14)
func (rs *ResilientStorage) SearchMessages(ctx context.Context, params SearchParams) ([]SearchResult, error) {
	if rs.isCircuitOpen() {
		return rs.fallback.SearchMessages(ctx, params)
	}

	result, err := rs.primary.SearchMessages(ctx, params)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.SearchMessages(ctx, params)
		}
		return nil, err
	}

	rs.recordSuccess()
	return result, nil
}

// UpdateMessagePinned updates message pinned state with circuit breaker (Phase 14)
func (rs *ResilientStorage) UpdateMessagePinned(ctx context.Context, userID, sessionID, messageID string, isPinned bool) error {
	if rs.isCircuitOpen() {
		return rs.fallback.UpdateMessagePinned(ctx, userID, sessionID, messageID, isPinned)
	}

	err := rs.primary.UpdateMessagePinned(ctx, userID, sessionID, messageID, isPinned)
	if err != nil {
		if isStorageError(err) {
			rs.recordFailure(err)
			return rs.fallback.UpdateMessagePinned(ctx, userID, sessionID, messageID, isPinned)
		}
		return err
	}

	rs.recordSuccess()
	return nil
}

// Close closes both storages
func (rs *ResilientStorage) Close() error {
	if err := rs.primary.Close(); err != nil {
		rs.logger.Error("Failed to close primary storage", "error", err)
	}
	return rs.fallback.Close()
}

// isStorageError determines if an error is a storage infrastructure error
// vs a logical error (like "not found")
func isStorageError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// These are logical errors, not infrastructure failures
	if contains(errStr, "not found") ||
		contains(errStr, "already exists") ||
		contains(errStr, "limit reached") {
		return false
	}
	// Everything else is treated as infrastructure error
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
