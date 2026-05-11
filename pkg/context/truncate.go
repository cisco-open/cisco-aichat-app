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
	"strings"
	"unicode/utf8"

	"github.com/grafana/cisco-aichat-app/pkg/storage"
	"github.com/grafana/cisco-aichat-app/pkg/tokens"
)

// TruncationMarker is appended to truncated content
const TruncationMarker = " [truncated]"

// TruncateMessage shortens message content to fit token budget
// Returns a copy with truncated content and updated token count
func TruncateMessage(ctx context.Context, msg storage.ChatMessage, maxTokens int, counter tokens.TokenCounter, model string) storage.ChatMessage {
	if msg.TokenCount <= maxTokens {
		return msg
	}

	// Calculate available tokens for content (minus marker)
	markerTokens, _ := counter.CountTokens(ctx, TruncationMarker, model)
	targetTokens := maxTokens - markerTokens
	if targetTokens < 10 {
		// Can't fit meaningful content, just return marker
		msg.Content = TruncationMarker
		msg.TokenCount = markerTokens
		return msg
	}

	// Binary search for right truncation point
	truncated := binarySearchTruncation(ctx, msg.Content, targetTokens, counter, model)
	msg.Content = truncated + TruncationMarker

	// Recount actual tokens
	msg.TokenCount, _ = counter.CountTokens(ctx, msg.Content, model)

	return msg
}

// binarySearchTruncation finds the longest prefix that fits within token budget
func binarySearchTruncation(ctx context.Context, content string, maxTokens int, counter tokens.TokenCounter, model string) string {
	// Handle empty content
	if len(content) == 0 {
		return ""
	}

	low, high := 0, len(content)

	for low < high {
		mid := (low + high + 1) / 2
		prefix := safeSubstring(content, mid)
		tokenCount, _ := counter.CountTokens(ctx, prefix, model)

		if tokenCount <= maxTokens {
			low = mid
		} else {
			high = mid - 1
		}
	}

	result := safeSubstring(content, low)

	// Try to truncate at word boundary for cleaner output
	if lastSpace := strings.LastIndex(result, " "); lastSpace > len(result)/2 {
		return result[:lastSpace]
	}

	return result
}

// safeSubstring returns a substring that doesn't break UTF-8 runes
func safeSubstring(s string, length int) string {
	if length >= len(s) {
		return s
	}
	if length <= 0 {
		return ""
	}

	// Find valid UTF-8 boundary
	for length > 0 && !utf8.RuneStart(s[length]) {
		length--
	}

	return s[:length]
}

// TruncateToTokens is a simpler variant that truncates any string to fit token budget
// Adds truncation marker when content is shortened
func TruncateToTokens(ctx context.Context, content string, maxTokens int, counter tokens.TokenCounter, model string) string {
	currentTokens, _ := counter.CountTokens(ctx, content, model)
	if currentTokens <= maxTokens {
		return content
	}

	// Account for marker in budget
	markerTokens, _ := counter.CountTokens(ctx, TruncationMarker, model)
	targetTokens := maxTokens - markerTokens
	if targetTokens < 1 {
		return TruncationMarker
	}

	truncated := binarySearchTruncation(ctx, content, targetTokens, counter, model)
	return truncated + TruncationMarker
}
