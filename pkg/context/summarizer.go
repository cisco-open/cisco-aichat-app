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

	"github.com/google/uuid"
	"github.com/grafana/cisco-aichat-app/pkg/storage"
	"github.com/grafana/cisco-aichat-app/pkg/tokens"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// LLMClient is the interface for making LLM calls for summarization.
// Implementations should use the same provider as the main chat.
//
// NOTE: Concrete implementation will be wired via App initialization,
// following the same pattern as TokenService in Phase 11. The App struct
// will create an LLMClientAdapter that wraps the existing LLM provider
// (OpenAI/Anthropic client) to implement the LLMClient.Complete method.
type LLMClient interface {
	// Complete generates a completion for the given prompt
	Complete(ctx context.Context, model string, prompt string) (string, error)
}

// SummarizerConfig configures the summarization behavior
type SummarizerConfig struct {
	BatchTokens      int           // Target batch size in tokens (~10k per CONTEXT.md)
	TriggerThreshold float64       // Threshold to trigger summarization (0.8 = 80%)
	MaxSummaryDepth  int           // Maximum recursive summarization depth (default 3)
	SummaryTimeout   time.Duration // Timeout for LLM call
}

// DefaultSummarizerConfig returns sensible defaults per CONTEXT.md
func DefaultSummarizerConfig() SummarizerConfig {
	return SummarizerConfig{
		BatchTokens:      10000,             // ~10k tokens per batch
		TriggerThreshold: 0.80,              // 80% of budget triggers summarization
		MaxSummaryDepth:  3,                 // Limit recursive summarization
		SummaryTimeout:   30 * time.Second,  // Timeout for LLM summarization call
	}
}

// Summarizer compresses old messages via LLM to manage context window size.
// When context reaches 80% of budget, it proactively summarizes oldest messages
// to prevent abrupt message loss while preserving key decisions and outcomes.
type Summarizer struct {
	storage storage.Storage
	counter tokens.TokenCounter
	llm     LLMClient
	config  SummarizerConfig
	logger  log.Logger
}

// NewSummarizer creates a new summarizer with the given dependencies.
// llm parameter is optional - if nil, summarization is disabled.
func NewSummarizer(store storage.Storage, counter tokens.TokenCounter, llm LLMClient) *Summarizer {
	return &Summarizer{
		storage: store,
		counter: counter,
		llm:     llm,
		config:  DefaultSummarizerConfig(),
		logger:  log.DefaultLogger,
	}
}

// WithConfig applies custom configuration
func (s *Summarizer) WithConfig(cfg SummarizerConfig) *Summarizer {
	s.config = cfg
	return s
}

// WithLogger sets a custom logger
func (s *Summarizer) WithLogger(logger log.Logger) *Summarizer {
	s.logger = logger
	return s
}

// IsEnabled returns true if summarization is available (LLM client provided).
// This allows graceful handling when LLM is not configured.
func (s *Summarizer) IsEnabled() bool {
	return s.llm != nil
}

// ShouldSummarize checks if summarization should be triggered.
// Returns true if session is at or above threshold of context budget (80% by default).
func (s *Summarizer) ShouldSummarize(ctx context.Context, sessionID string, maxTokens int) (bool, error) {
	if !s.IsEnabled() {
		return false, nil
	}

	session, err := s.storage.GetSession(ctx, "", sessionID)
	if err != nil {
		return false, err
	}

	threshold := float64(maxTokens) * s.config.TriggerThreshold
	return float64(session.TotalTokens) >= threshold, nil
}

// SummarizeOldMessages compresses oldest non-summary messages into a summary.
// Called proactively when session approaches context budget (80% threshold).
//
// The summarization process:
// 1. Retrieves oldest non-summary messages up to batch token limit
// 2. Checks max depth to prevent infinite recursive summarization
// 3. Groups tool call + result pairs for atomic summarization
// 4. Generates natural language summary via LLM
// 5. Stores summary with references to original message IDs
// 6. Summary uses earliest original timestamp for inline positioning
func (s *Summarizer) SummarizeOldMessages(ctx context.Context, userID, sessionID, model string) error {
	if !s.IsEnabled() {
		return nil // Silently skip if LLM not available
	}

	s.logger.Info("Starting summarization", "sessionID", sessionID)

	// Get batch of oldest non-summary messages
	messages, err := s.storage.GetOldestNonSummaryMessages(ctx, sessionID, s.config.BatchTokens)
	if err != nil {
		return fmt.Errorf("failed to get messages for summarization: %w", err)
	}

	if len(messages) == 0 {
		s.logger.Debug("No messages to summarize", "sessionID", sessionID)
		return nil
	}

	// Check max depth - find highest depth in batch
	maxDepth := 0
	for _, msg := range messages {
		if msg.SummaryDepth > maxDepth {
			maxDepth = msg.SummaryDepth
		}
	}

	if maxDepth >= s.config.MaxSummaryDepth {
		s.logger.Warn("Max summarization depth reached, skipping",
			"sessionID", sessionID,
			"maxDepth", s.config.MaxSummaryDepth,
		)
		return nil
	}

	// Group tool calls with their results for atomic summarization
	groupedMessages := s.groupToolCalls(messages)

	// Generate summary via LLM
	summaryCtx, cancel := context.WithTimeout(ctx, s.config.SummaryTimeout)
	defer cancel()

	summaryText, err := s.generateSummary(summaryCtx, groupedMessages, model)
	if err != nil {
		return fmt.Errorf("failed to generate summary: %w", err)
	}

	// Count tokens in summary
	summaryTokens, _ := s.counter.CountTokens(ctx, summaryText, model)

	// Create summary message
	// Use earliest message timestamp so summary appears inline at original position
	earliestTimestamp := messages[0].Timestamp
	for _, msg := range messages {
		if msg.Timestamp < earliestTimestamp {
			earliestTimestamp = msg.Timestamp
		}
	}

	summary := &storage.ChatMessage{
		ID:           uuid.New().String(),
		Role:         "system", // Summaries appear as system context
		Content:      summaryText,
		Timestamp:    earliestTimestamp, // Inline at original position per CONTEXT.md
		TokenCount:   summaryTokens,
		IsSummary:    true,
		SummaryDepth: maxDepth + 1,
	}

	// Extract original message IDs
	originalIDs := make([]string, len(messages))
	for i, msg := range messages {
		originalIDs[i] = msg.ID
	}

	// Save summary with references
	if err := s.storage.SaveSummary(ctx, userID, sessionID, summary, originalIDs); err != nil {
		return fmt.Errorf("failed to save summary: %w", err)
	}

	s.logger.Info("Summarization complete",
		"sessionID", sessionID,
		"originalMessages", len(messages),
		"originalTokens", s.sumTokens(messages),
		"summaryTokens", summaryTokens,
		"compressionRatio", float64(s.sumTokens(messages))/float64(max(summaryTokens, 1)),
	)

	return nil
}

// groupToolCalls ensures tool call + result pairs stay together.
// Per CONTEXT.md: atomic summarization of tool interactions.
func (s *Summarizer) groupToolCalls(messages []storage.ChatMessage) []storage.ChatMessage {
	// For now, return as-is - tool call detection would require
	// inspecting message content for function_call patterns.
	// This can be enhanced when tool call format is determined.

	// Messages are already in chronological order from GetOldestNonSummaryMessages
	return messages
}

// generateSummary creates a concise summary via LLM.
// Per CONTEXT.md: natural context, no meta-commentary, preserve decisions/outcomes.
func (s *Summarizer) generateSummary(ctx context.Context, messages []storage.ChatMessage, model string) (string, error) {
	prompt := s.buildSummarizationPrompt(messages)

	summary, err := s.llm.Complete(ctx, model, prompt)
	if err != nil {
		return "", err
	}

	// Trim any leading/trailing whitespace
	return strings.TrimSpace(summary), nil
}

// buildSummarizationPrompt constructs the prompt per CONTEXT.md requirements.
// The prompt instructs the LLM to:
// - Focus on key decisions and outcomes
// - Write as natural context (not meta-commentary)
// - Preserve tool call results as outcomes
// - Stay concise but preserve important details
func (s *Summarizer) buildSummarizationPrompt(messages []storage.ChatMessage) string {
	var sb strings.Builder

	sb.WriteString(`Summarize the following conversation segment into a concise paragraph.

Focus on:
- Key decisions made and their outcomes
- What was tried and what worked or failed
- Tool/function call results (condense to outcomes only)
- User preferences or requests that should be remembered

Write as natural context, not as a summary. Do NOT include phrases like:
- "In summary"
- "The user discussed"
- "The conversation covered"
- "[N messages summarized]"

Keep the summary under 500 tokens. Be concise but preserve important details.

Conversation:
`)

	for _, msg := range messages {
		role := msg.Role
		if role == "assistant" {
			role = "Assistant"
		} else if role == "user" {
			role = "User"
		} else if role == "system" {
			role = "System"
		}

		// Truncate very long messages in prompt to avoid exceeding limits
		content := msg.Content
		if len(content) > 2000 {
			content = content[:2000] + "... [content truncated for summarization]"
		}

		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	return sb.String()
}

// sumTokens calculates total tokens in messages
func (s *Summarizer) sumTokens(messages []storage.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += msg.TokenCount
	}
	return total
}
