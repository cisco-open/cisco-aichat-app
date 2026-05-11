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

package context

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grafana/cisco-aichat-app/pkg/storage"
	"github.com/grafana/cisco-aichat-app/pkg/tokens"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// ContextBuilder orchestrates context window construction with caching
type ContextBuilder struct {
	storage    storage.Storage
	counter    tokens.TokenCounter
	cache      *ContextCache  // Cache for context calculations (CTX-09)
	summarizer *Summarizer    // For proactive summarization (CTX-11, optional)
	window     *SlidingWindow // Window for message selection
	logger     log.Logger
}

// NewContextBuilder creates a new builder with required dependencies.
// cache and summarizer parameters are optional - if nil, features are disabled.
func NewContextBuilder(store storage.Storage, counter tokens.TokenCounter, cache *ContextCache, summarizer *Summarizer) *ContextBuilder {
	return &ContextBuilder{
		storage:    store,
		counter:    counter,
		cache:      cache,
		summarizer: summarizer,
		window:     NewSlidingWindow(3), // Default minimum per CONTEXT.md
		logger:     log.DefaultLogger,
	}
}

// WithMinMessages configures the minimum message guarantee
func (cb *ContextBuilder) WithMinMessages(min int) *ContextBuilder {
	cb.window = NewSlidingWindow(min)
	return cb
}

// WithLogger sets a custom logger
func (cb *ContextBuilder) WithLogger(logger log.Logger) *ContextBuilder {
	cb.logger = logger
	return cb
}

// BuildContextWindow constructs a context window fitting within token limits.
// Uses cache.GetOrBuild for performance (CTX-09, CTX-10: <100ms target).
// Triggers proactive summarization when context reaches 80% threshold (CTX-11).
func (cb *ContextBuilder) BuildContextWindow(ctx context.Context, opts BuildOptions) (*ContextWindow, error) {
	start := time.Now()
	defer func() {
		cb.logger.Debug("BuildContextWindow completed",
			"sessionId", opts.SessionID,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	// Apply defaults
	opts = opts.WithDefaults()

	// Validate required fields
	if opts.SessionID == "" {
		return nil, fmt.Errorf("sessionID is required")
	}

	// Check if proactive summarization is needed (CTX-11: 80% threshold)
	if cb.summarizer != nil && cb.summarizer.IsEnabled() {
		shouldSummarize, err := cb.summarizer.ShouldSummarize(ctx, opts.SessionID, opts.MaxTokens)
		if err != nil {
			cb.logger.Warn("Failed to check summarization threshold", "error", err.Error())
		} else if shouldSummarize {
			cb.logger.Info("Triggering proactive summarization", "sessionID", opts.SessionID)
			if err := cb.summarizer.SummarizeOldMessages(ctx, opts.UserID, opts.SessionID, opts.Model); err != nil {
				cb.logger.Warn("Proactive summarization failed", "error", err.Error())
				// Continue with context build even if summarization fails (graceful degradation)
			} else {
				// Invalidate cache since context changed
				cb.InvalidateCache(opts.SessionID)
			}
		}
	}

	// If cache is available, use GetOrBuild pattern
	if cb.cache != nil {
		cacheKey := CacheKey(opts.SessionID, opts.SystemPrompt)
		return cb.cache.GetOrBuild(cacheKey, func() (*ContextWindow, error) {
			return cb.buildContextWindowUncached(ctx, opts)
		})
	}

	// No cache - build directly
	return cb.buildContextWindowUncached(ctx, opts)
}

// buildContextWindowUncached performs the actual context window construction
func (cb *ContextBuilder) buildContextWindowUncached(ctx context.Context, opts BuildOptions) (*ContextWindow, error) {
	// Count system prompt tokens
	systemPromptTokens := 0
	if opts.SystemPrompt != "" {
		var err error
		systemPromptTokens, err = cb.counter.CountTokens(ctx, opts.SystemPrompt, opts.Model)
		if err != nil {
			cb.logger.Warn("Failed to count system prompt tokens, using estimation",
				"error", err.Error(),
			)
			// Fallback estimation: ~4 chars per token
			systemPromptTokens = len(opts.SystemPrompt) / 4
		}
	}

	// Calculate available budget for messages
	availableBudget := opts.AvailableBudget(systemPromptTokens)
	if availableBudget < 0 {
		// System prompt exceeds budget - truncate it per CONTEXT.md
		cb.logger.Warn("System prompt exceeds available budget, truncating",
			"systemPromptTokens", systemPromptTokens,
			"maxTokens", opts.MaxTokens,
			"responseBuffer", opts.ResponseBuffer,
		)
		// Leave 1000 tokens for minimum messages
		maxPromptTokens := opts.MaxTokens - opts.ResponseBuffer - 1000
		if maxPromptTokens < 100 {
			maxPromptTokens = 100 // Minimum viable prompt
		}
		truncatedPrompt := TruncateToTokens(ctx, opts.SystemPrompt, maxPromptTokens, cb.counter, opts.Model)
		opts.SystemPrompt = truncatedPrompt
		systemPromptTokens, _ = cb.counter.CountTokens(ctx, truncatedPrompt, opts.Model)
		availableBudget = opts.AvailableBudget(systemPromptTokens)
	}

	// Get messages from storage (uses GetMessagesByTokenBudget from Phase 11)
	// This already handles pinned message priority
	messages, err := cb.storage.GetMessagesByTokenBudget(ctx, opts.SessionID, availableBudget)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	// Preserve error messages - they should never be pruned from context
	// This helps the AI understand previous failures and avoid repeating them
	var errorMessages []storage.ChatMessage
	var regularMessages []storage.ChatMessage
	errorTokens := 0
	for i := range messages {
		if isErrorMessage(&messages[i]) {
			cb.logger.Debug("Preserving error message", "messageId", messages[i].ID)
			errorMessages = append(errorMessages, messages[i])
			errorTokens += messages[i].TokenCount
		} else {
			regularMessages = append(regularMessages, messages[i])
		}
	}

	// Adjust budget for regular messages (error messages are always included)
	adjustedBudget := availableBudget - errorTokens
	if adjustedBudget < 0 {
		adjustedBudget = 0
	}

	// Apply sliding window selection to regular messages
	// GetMessagesByTokenBudget returns messages sorted chronologically by storage
	// Our window.SelectMessages ensures minimum guarantee and chronological output
	selected := cb.window.SelectMessages(regularMessages, adjustedBudget)

	// Merge error messages back into selection and sort chronologically
	if len(errorMessages) > 0 {
		selected = append(selected, errorMessages...)
		// Sort by timestamp to maintain chronological order
		for i := 0; i < len(selected)-1; i++ {
			for j := i + 1; j < len(selected); j++ {
				if selected[i].Timestamp > selected[j].Timestamp {
					selected[i], selected[j] = selected[j], selected[i]
				}
			}
		}
	}

	// Check if any truncation occurred (if total exceeds budget due to minimum guarantee)
	wasTruncated := false
	totalMessageTokens := SumTokens(selected)
	if totalMessageTokens > availableBudget && len(selected) > opts.MinMessages {
		wasTruncated = true
	}

	// Count summary messages
	summaryCount := 0
	for _, msg := range selected {
		if msg.IsSummary {
			summaryCount++
		}
	}

	// Build the context window
	window := &ContextWindow{
		SystemPrompt: opts.SystemPrompt,
		Messages:     selected,
		TotalTokens:  systemPromptTokens + totalMessageTokens,
		WasTruncated: wasTruncated,
		SummaryCount: summaryCount,
	}

	return window, nil
}

// GetContextUsage returns the percentage of context budget used
func (cb *ContextBuilder) GetContextUsage(ctx context.Context, sessionID string, maxTokens int) (float64, error) {
	session, err := cb.storage.GetSession(ctx, "", sessionID)
	if err != nil {
		return 0, err
	}

	if maxTokens == 0 {
		maxTokens = 100000 // Default
	}

	return float64(session.TotalTokens) / float64(maxTokens) * 100, nil
}

// InvalidateCache removes cached context for a session
// Should be called when new message is added (CTX-11)
func (cb *ContextBuilder) InvalidateCache(sessionID string) {
	if cb.cache != nil {
		cb.cache.Invalidate(sessionID)
	}
}

// isErrorMessage checks if a message likely contains error content.
// Used for implicit preservation (error messages are never pruned from context)
// to help the AI understand previous failures and avoid repeating them.
func isErrorMessage(msg *storage.ChatMessage) bool {
	if msg.Role != "assistant" {
		return false
	}
	content := strings.ToLower(msg.Content)
	// Check for common error indicators
	errorPatterns := []string{
		"error:",
		"failed to",
		"exception:",
		"could not",
		"unable to",
		"invalid",
	}
	for _, pattern := range errorPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	return false
}
