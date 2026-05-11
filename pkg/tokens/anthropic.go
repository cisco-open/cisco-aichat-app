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

package tokens

import (
	"context"
	"errors"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var (
	// ErrNoAPIKey is returned when no Anthropic API key is provided
	ErrNoAPIKey = errors.New("anthropic API key not provided")
)

// AnthropicCounter counts tokens for Claude models using the Anthropic API
type AnthropicCounter struct {
	client anthropic.Client
	apiKey string
}

// NewAnthropicCounter creates a new Anthropic token counter
// apiKey should be from ANTHROPIC_API_KEY environment variable
// If empty, counter will return ErrNoAPIKey on CountTokens calls
func NewAnthropicCounter(apiKey string) *AnthropicCounter {
	counter := &AnthropicCounter{
		apiKey: apiKey,
	}
	if apiKey != "" {
		counter.client = anthropic.NewClient(option.WithAPIKey(apiKey))
	}
	return counter
}

// SupportsModel returns true for Claude models (claude*)
func (c *AnthropicCounter) SupportsModel(model string) bool {
	model = strings.ToLower(model)
	return strings.HasPrefix(model, "claude")
}

// CountTokens counts tokens using the Anthropic API
func (c *AnthropicCounter) CountTokens(ctx context.Context, text string, model string) (int, error) {
	if c.apiKey == "" {
		return 0, ErrNoAPIKey
	}

	// Map model string to SDK model constant
	sdkModel := c.mapModel(model)

	// Create message for token counting
	params := anthropic.MessageCountTokensParams{
		Model: sdkModel,
		Messages: []anthropic.MessageParam{
			{
				Role: "user",
				Content: []anthropic.ContentBlockParamUnion{
					anthropic.NewTextBlock(text),
				},
			},
		},
	}

	result, err := c.client.Messages.CountTokens(ctx, params)
	if err != nil {
		return 0, err
	}

	return int(result.InputTokens), nil
}

// mapModel maps a model string to the SDK model type
func (c *AnthropicCounter) mapModel(model string) anthropic.Model {
	model = strings.ToLower(model)

	// Map common model names to SDK constants
	switch {
	// Claude Opus 4.5
	case strings.Contains(model, "opus-4-5") || strings.Contains(model, "opus-4.5"):
		return anthropic.ModelClaudeOpus4_5
	case strings.Contains(model, "opus-4") || strings.Contains(model, "opus-4.0"):
		return anthropic.ModelClaudeOpus4_0

	// Claude Sonnet 4.5
	case strings.Contains(model, "sonnet-4-5") || strings.Contains(model, "sonnet-4.5"):
		return anthropic.ModelClaudeSonnet4_5
	case strings.Contains(model, "sonnet-4") || strings.Contains(model, "sonnet-4.0"):
		return anthropic.ModelClaudeSonnet4_0

	// Claude 3.7 Sonnet
	case strings.Contains(model, "3-7-sonnet") || strings.Contains(model, "3.7-sonnet"):
		return "claude-3-7-sonnet-latest"

	// Claude 3.5 Haiku
	case strings.Contains(model, "3-5-haiku") || strings.Contains(model, "3.5-haiku"):
		return "claude-3-5-haiku-latest"

	// Claude Haiku 4.5
	case strings.Contains(model, "haiku-4-5") || strings.Contains(model, "haiku-4.5"):
		return anthropic.ModelClaudeHaiku4_5

	// Default to Sonnet 4.5 for unrecognized claude models
	default:
		return anthropic.ModelClaudeSonnet4_5
	}
}
