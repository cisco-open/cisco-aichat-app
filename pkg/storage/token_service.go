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
	"strconv"
	"sync"

	"github.com/grafana/cisco-aichat-app/pkg/tokens"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// Default context window limit (100k tokens)
const DefaultContextLimit = 100000

// TokenStats contains token statistics for a session
type TokenStats struct {
	SessionID     string  `json:"sessionId"`
	TotalTokens   int     `json:"totalTokens"`   // Sum of all message tokens
	ContextLimit  int     `json:"contextLimit"`  // Configured limit (default 100k)
	ContextUsage  float64 `json:"contextUsage"`  // totalTokens / contextLimit as percentage
	MessageCount  int     `json:"messageCount"`
	UncountedMsgs int     `json:"uncountedMsgs"` // Messages with token_count = 0
}

// TokenService orchestrates lazy token counting and session tracking
type TokenService struct {
	storage      Storage
	counter      tokens.TokenCounter
	worker       *tokens.BackgroundWorker
	defaultModel string
	contextLimit int
	logger       log.Logger
	mu           sync.Mutex
}

// TokenServiceOption is a functional option for TokenService configuration
type TokenServiceOption func(*TokenService)

// WithContextLimit sets a custom context limit
func WithContextLimit(limit int) TokenServiceOption {
	return func(ts *TokenService) {
		if limit > 0 {
			ts.contextLimit = limit
		}
	}
}

// WithDefaultModel sets the default model for token counting
func WithDefaultModel(model string) TokenServiceOption {
	return func(ts *TokenService) {
		if model != "" {
			ts.defaultModel = model
		}
	}
}

// WithLogger sets a custom logger
func WithLogger(logger log.Logger) TokenServiceOption {
	return func(ts *TokenService) {
		if logger != nil {
			ts.logger = logger
		}
	}
}

// NewTokenService creates a new TokenService
// storage: for DB operations
// counter: TokenCounter implementation (typically a Registry)
// opts: functional options for configuration
func NewTokenService(storage Storage, counter tokens.TokenCounter, opts ...TokenServiceOption) *TokenService {
	// Check for context limit from environment variable
	contextLimit := DefaultContextLimit
	if envLimit := os.Getenv("AICHAT_CONTEXT_LIMIT"); envLimit != "" {
		if parsed, err := strconv.Atoi(envLimit); err == nil && parsed > 0 {
			contextLimit = parsed
		}
	}

	ts := &TokenService{
		storage:      storage,
		counter:      counter,
		defaultModel: "gpt-4", // Default model for counting
		contextLimit: contextLimit,
		logger:       log.DefaultLogger,
	}

	// Apply options
	for _, opt := range opts {
		opt(ts)
	}

	// Start background worker with 4 workers
	ts.worker = tokens.NewBackgroundWorker(counter, 4)
	ts.worker.Start()

	ts.logger.Info("TokenService initialized",
		"contextLimit", ts.contextLimit,
		"defaultModel", ts.defaultModel,
	)

	return ts
}

// EnsureTokenCounts ensures all messages have token counts computed
// Messages with TokenCount == 0 are counted via the background worker
// Per CONTEXT.md: lazy counting on first read, persist after calculation
// All counting goes through the background worker (async-only)
func (ts *TokenService) EnsureTokenCounts(ctx context.Context, messages []ChatMessage, model string) error {
	if model == "" {
		model = ts.defaultModel
	}

	// Find messages that need counting
	var needsCounting []ChatMessage
	for _, msg := range messages {
		if msg.TokenCount == 0 && msg.Content != "" {
			needsCounting = append(needsCounting, msg)
		}
	}

	if len(needsCounting) == 0 {
		return nil // All messages already have counts
	}

	ts.logger.Debug("Counting tokens for messages",
		"total", len(messages),
		"needsCounting", len(needsCounting),
		"model", model,
	)

	// Submit all jobs and collect result channels
	type jobResult struct {
		messageID string
		resultCh  chan tokens.CountResult
	}
	jobs := make([]jobResult, 0, len(needsCounting))

	for _, msg := range needsCounting {
		resultCh := make(chan tokens.CountResult, 1)
		job := tokens.CountJob{
			MessageID: msg.ID,
			Content:   msg.Content,
			Model:     model,
			ResultCh:  resultCh,
		}
		ts.worker.Submit(job)
		jobs = append(jobs, jobResult{messageID: msg.ID, resultCh: resultCh})
	}

	// Wait for all results and persist counts
	for _, jr := range jobs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-jr.resultCh:
			if result.Error != nil {
				ts.logger.Warn("Token counting failed for message",
					"messageID", jr.messageID,
					"error", result.Error,
				)
				continue // Skip this message but continue with others
			}

			// Persist the computed count to database
			if err := ts.storage.UpdateMessageTokenCount(ctx, result.MessageID, result.TokenCount); err != nil {
				ts.logger.Warn("Failed to persist token count",
					"messageID", result.MessageID,
					"tokenCount", result.TokenCount,
					"error", err,
				)
				// Continue even if persistence fails - we computed the count
			}
		}
	}

	return nil
}

// GetSessionTokenStats returns token statistics for a session
// Triggers lazy token counting for any uncounted messages
func (ts *TokenService) GetSessionTokenStats(ctx context.Context, userID, sessionID string) (*TokenStats, error) {
	// Get session to verify ownership and get message list
	session, err := ts.storage.GetSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}

	// Trigger lazy token counting for messages without counts
	if err := ts.EnsureTokenCounts(ctx, session.Messages, ts.defaultModel); err != nil {
		ts.logger.Warn("Failed to ensure token counts", "sessionID", sessionID, "error", err)
		// Continue anyway - we'll report uncounted messages in stats
	}

	// Re-fetch session to get updated token counts
	session, err = ts.storage.GetSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}

	// Count tokens and uncounted messages
	var totalTokens int
	var uncountedMsgs int
	for _, msg := range session.Messages {
		totalTokens += msg.TokenCount
		if msg.TokenCount == 0 {
			uncountedMsgs++
		}
	}

	// Calculate context usage percentage
	contextUsage := 0.0
	if ts.contextLimit > 0 {
		contextUsage = (float64(totalTokens) / float64(ts.contextLimit)) * 100.0
	}

	return &TokenStats{
		SessionID:     sessionID,
		TotalTokens:   totalTokens,
		ContextLimit:  ts.contextLimit,
		ContextUsage:  contextUsage,
		MessageCount:  len(session.Messages),
		UncountedMsgs: uncountedMsgs,
	}, nil
}

// RecalculateSessionTokens recalculates the total token count for a session
// by summing all message token counts and updating the session
func (ts *TokenService) RecalculateSessionTokens(ctx context.Context, userID, sessionID string) error {
	// Get session with all messages
	session, err := ts.storage.GetSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}

	// Sum all message token counts
	var totalTokens int
	for _, msg := range session.Messages {
		totalTokens += msg.TokenCount
	}

	// Update session's total tokens
	session.TotalTokens = totalTokens
	if err := ts.storage.UpdateSession(ctx, userID, session); err != nil {
		return err
	}

	ts.logger.Debug("Recalculated session tokens",
		"sessionID", sessionID,
		"totalTokens", totalTokens,
	)

	return nil
}

// GetContextLimit returns the configured context limit
func (ts *TokenService) GetContextLimit() int {
	return ts.contextLimit
}

// Close stops the background worker and cleans up resources
func (ts *TokenService) Close() {
	if ts.worker != nil {
		ts.worker.Stop()
	}
}
