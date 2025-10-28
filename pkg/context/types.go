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
	"github.com/grafana/grafana-aichat-app/pkg/storage"
)

// ContextWindow represents a ready-to-send LLM context
type ContextWindow struct {
	SystemPrompt string                `json:"systemPrompt"`
	Messages     []storage.ChatMessage `json:"messages"`
	TotalTokens  int                   `json:"totalTokens"`
	WasTruncated bool                  `json:"wasTruncated"` // True if any message was truncated
	SummaryCount int                   `json:"summaryCount"` // Number of summary messages included
}

// BuildOptions configures context window building
type BuildOptions struct {
	SessionID      string // Required: session to build context for
	UserID         string // Required: user owning the session
	SystemPrompt   string // System prompt to include
	Model          string // Model name for token counting
	MaxTokens      int    // Total budget (default 100000)
	ResponseBuffer int    // Reserved for response (default 3000)
	MinMessages    int    // Minimum recent messages (default 3)
}

// DefaultBuildOptions returns sensible defaults per CONTEXT.md
func DefaultBuildOptions() BuildOptions {
	return BuildOptions{
		MaxTokens:      100000, // 100k default context limit
		ResponseBuffer: 3000,   // 3k response buffer (within 2-4k range)
		MinMessages:    3,      // Minimum 3 recent messages
	}
}

// WithDefaults fills in missing fields with defaults
func (opts BuildOptions) WithDefaults() BuildOptions {
	defaults := DefaultBuildOptions()
	if opts.MaxTokens == 0 {
		opts.MaxTokens = defaults.MaxTokens
	}
	if opts.ResponseBuffer == 0 {
		opts.ResponseBuffer = defaults.ResponseBuffer
	}
	if opts.MinMessages == 0 {
		opts.MinMessages = defaults.MinMessages
	}
	return opts
}

// AvailableBudget returns tokens available for messages (after system prompt and buffer)
func (opts BuildOptions) AvailableBudget(systemPromptTokens int) int {
	return opts.MaxTokens - opts.ResponseBuffer - systemPromptTokens
}
