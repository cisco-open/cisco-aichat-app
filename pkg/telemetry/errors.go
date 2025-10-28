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

// Package telemetry provides types and utilities for LLM telemetry collection.
package telemetry

// Error type constants for classifying LLM errors.
// These are used as metric labels and in logs for error categorization.
const (
	// ErrTypeRateLimit indicates the LLM provider returned a rate limit error (HTTP 429).
	ErrTypeRateLimit = "rate_limit"

	// ErrTypeTimeout indicates the request timed out (HTTP 504).
	ErrTypeTimeout = "timeout"

	// ErrTypeAuthFailure indicates an authentication error (HTTP 401).
	ErrTypeAuthFailure = "auth_failure"

	// ErrTypeInvalidRequest indicates a malformed or invalid request (HTTP 400).
	ErrTypeInvalidRequest = "invalid_request"

	// ErrTypeContextLength indicates the context/prompt exceeded the model's token limit (HTTP 400).
	ErrTypeContextLength = "context_length"

	// ErrTypeStreamInterrupted indicates a streaming response was interrupted (HTTP 499).
	ErrTypeStreamInterrupted = "stream_interrupted"

	// ErrTypeToolCallFailed indicates an MCP tool call failed (HTTP 500).
	ErrTypeToolCallFailed = "tool_call_failed"

	// ErrTypeProviderError indicates a generic LLM provider error (HTTP 502).
	ErrTypeProviderError = "provider_error"

	// ErrTypeNetworkError indicates a network/connection error (HTTP 503).
	ErrTypeNetworkError = "network_error"

	// ErrTypeParseError indicates a response parsing error (HTTP 500).
	ErrTypeParseError = "parse_error"

	// ErrTypeUnknown indicates an unclassified error (HTTP 500).
	ErrTypeUnknown = "unknown"
)

// ErrorCode maps error types to HTTP-like numeric codes.
// These codes are used in logs and API responses for programmatic handling.
var ErrorCode = map[string]int{
	ErrTypeRateLimit:         429,
	ErrTypeTimeout:           504,
	ErrTypeAuthFailure:       401,
	ErrTypeInvalidRequest:    400,
	ErrTypeContextLength:     400,
	ErrTypeStreamInterrupted: 499, // Client closed request
	ErrTypeToolCallFailed:    500,
	ErrTypeProviderError:     502,
	ErrTypeNetworkError:      503,
	ErrTypeParseError:        500,
	ErrTypeUnknown:           500,
}

// ValidErrorTypes contains all valid error types for validation.
var ValidErrorTypes = []string{
	ErrTypeRateLimit,
	ErrTypeTimeout,
	ErrTypeAuthFailure,
	ErrTypeInvalidRequest,
	ErrTypeContextLength,
	ErrTypeStreamInterrupted,
	ErrTypeToolCallFailed,
	ErrTypeProviderError,
	ErrTypeNetworkError,
	ErrTypeParseError,
	ErrTypeUnknown,
}

// validErrorTypeMap is an internal map for O(1) validation lookups.
var validErrorTypeMap = func() map[string]bool {
	m := make(map[string]bool, len(ValidErrorTypes))
	for _, t := range ValidErrorTypes {
		m[t] = true
	}
	return m
}()

// IsValidErrorType returns true if the given error type is valid.
func IsValidErrorType(errType string) bool {
	return validErrorTypeMap[errType]
}
