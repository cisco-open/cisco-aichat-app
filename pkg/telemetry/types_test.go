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

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTelemetryRequestValidation_Success tests that valid requests pass validation.
func TestTelemetryRequestValidation_Success(t *testing.T) {
	tests := []struct {
		name string
		req  TelemetryRequest
	}{
		{
			name: "streaming request with all fields",
			req: TelemetryRequest{
				Streaming:        true,
				DurationMs:       1500,
				TTFTMs:           ptr(int64(500)),
				PromptTokens:     100,
				CompletionTokens: 50,
				Success:          true,
				Source:           "user",
			},
		},
		{
			name: "non-streaming request",
			req: TelemetryRequest{
				Streaming:        false,
				DurationMs:       2000,
				PromptTokens:     200,
				CompletionTokens: 100,
				Success:          true,
				Source:           "user",
			},
		},
		{
			name: "success request with no error fields",
			req: TelemetryRequest{
				Streaming:        false,
				DurationMs:       1000,
				PromptTokens:     50,
				CompletionTokens: 25,
				Success:          true,
				Source:           "system",
			},
		},
		{
			name: "zero duration is valid",
			req: TelemetryRequest{
				Streaming:        false,
				DurationMs:       0,
				PromptTokens:     10,
				CompletionTokens: 5,
				Success:          true,
				Source:           "user",
			},
		},
		{
			name: "zero tokens are valid",
			req: TelemetryRequest{
				Streaming:        false,
				DurationMs:       100,
				PromptTokens:     0,
				CompletionTokens: 0,
				Success:          true,
				Source:           "user",
			},
		},
		{
			name: "error request with valid error type",
			req: TelemetryRequest{
				Streaming:        false,
				DurationMs:       500,
				PromptTokens:     100,
				CompletionTokens: 0,
				Success:          false,
				ErrorType:        "rate_limit",
				ErrorCode:        429,
				Source:           "user",
			},
		},
		{
			name: "streaming error with TTFT",
			req: TelemetryRequest{
				Streaming:        true,
				DurationMs:       3000,
				TTFTMs:           ptr(int64(200)),
				PromptTokens:     150,
				CompletionTokens: 30,
				Success:          false,
				ErrorType:        "stream_interrupted",
				ErrorCode:        499,
				Source:           "system",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			assert.NoError(t, err, "valid request should pass validation")
		})
	}
}

// TestTelemetryRequestValidation_InvalidDuration tests that negative duration fails.
func TestTelemetryRequestValidation_InvalidDuration(t *testing.T) {
	req := TelemetryRequest{
		Streaming:        false,
		DurationMs:       -1,
		PromptTokens:     100,
		CompletionTokens: 50,
		Success:          true,
		Source:           "user",
	}

	err := req.Validate()
	require.Error(t, err, "negative duration should fail validation")
	assert.ErrorIs(t, err, ErrInvalidDuration, "error should be ErrInvalidDuration")
}

// TestTelemetryRequestValidation_InvalidTTFT tests that negative TTFT fails when streaming.
func TestTelemetryRequestValidation_InvalidTTFT(t *testing.T) {
	req := TelemetryRequest{
		Streaming:        true,
		DurationMs:       1000,
		TTFTMs:           ptr(int64(-1)),
		PromptTokens:     100,
		CompletionTokens: 50,
		Success:          true,
		Source:           "user",
	}

	err := req.Validate()
	require.Error(t, err, "negative TTFT should fail validation for streaming request")
	assert.ErrorIs(t, err, ErrInvalidTTFT, "error should be ErrInvalidTTFT")
}

// TestTelemetryRequestValidation_TTFTIgnoredWhenNotStreaming tests that TTFT is ignored for non-streaming.
func TestTelemetryRequestValidation_TTFTIgnoredWhenNotStreaming(t *testing.T) {
	// Non-streaming request with negative TTFT should pass since TTFT is only validated for streaming
	req := TelemetryRequest{
		Streaming:        false,
		DurationMs:       1000,
		TTFTMs:           ptr(int64(-1)),
		PromptTokens:     100,
		CompletionTokens: 50,
		Success:          true,
		Source:           "user",
	}

	err := req.Validate()
	assert.NoError(t, err, "TTFT validation should be skipped for non-streaming requests")
}

// TestTelemetryRequestValidation_InvalidTokens tests that negative tokens fail.
func TestTelemetryRequestValidation_InvalidTokens(t *testing.T) {
	tests := []struct {
		name          string
		promptTokens  int
		completTokens int
		expectedErr   error
	}{
		{
			name:          "negative prompt tokens",
			promptTokens:  -1,
			completTokens: 50,
			expectedErr:   ErrInvalidPromptTokens,
		},
		{
			name:          "negative completion tokens",
			promptTokens:  100,
			completTokens: -1,
			expectedErr:   ErrInvalidCompletionTokens,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TelemetryRequest{
				Streaming:        false,
				DurationMs:       1000,
				PromptTokens:     tt.promptTokens,
				CompletionTokens: tt.completTokens,
				Success:          true,
				Source:           "user",
			}

			err := req.Validate()
			require.Error(t, err, "negative tokens should fail validation")
			assert.ErrorIs(t, err, tt.expectedErr, "error should match expected error type")
		})
	}
}

// TestTelemetryRequestValidation_InvalidErrorType tests that unknown error type fails when !Success.
func TestTelemetryRequestValidation_InvalidErrorType(t *testing.T) {
	tests := []struct {
		name      string
		errorType string
	}{
		{
			name:      "empty error type",
			errorType: "",
		},
		{
			name:      "unknown error type",
			errorType: "unknown_error_xyz",
		},
		{
			name:      "typo in error type",
			errorType: "rate-limit", // wrong: should be rate_limit
		},
		{
			name:      "case sensitive",
			errorType: "RATE_LIMIT", // wrong: should be lowercase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TelemetryRequest{
				Streaming:        false,
				DurationMs:       1000,
				PromptTokens:     100,
				CompletionTokens: 0,
				Success:          false,
				ErrorType:        tt.errorType,
				ErrorCode:        500,
				Source:           "user",
			}

			err := req.Validate()
			require.Error(t, err, "invalid error type should fail validation")
			assert.ErrorIs(t, err, ErrInvalidErrorType, "error should be ErrInvalidErrorType")
		})
	}
}

// TestTelemetryRequestValidation_ErrorTypeIgnoredWhenSuccess tests that error type is not validated for success.
func TestTelemetryRequestValidation_ErrorTypeIgnoredWhenSuccess(t *testing.T) {
	// Success request with invalid error type should pass since it's not checked
	req := TelemetryRequest{
		Streaming:        false,
		DurationMs:       1000,
		PromptTokens:     100,
		CompletionTokens: 50,
		Success:          true,
		ErrorType:        "invalid_error_type_xyz",
		Source:           "user",
	}

	err := req.Validate()
	assert.NoError(t, err, "error type validation should be skipped for success requests")
}

// TestTelemetryRequestValidation_InvalidSource tests that invalid source fails.
func TestTelemetryRequestValidation_InvalidSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "empty source",
			source: "",
		},
		{
			name:   "invalid source",
			source: "unknown",
		},
		{
			name:   "case sensitive user",
			source: "User", // wrong: should be lowercase
		},
		{
			name:   "case sensitive system",
			source: "SYSTEM", // wrong: should be lowercase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TelemetryRequest{
				Streaming:        false,
				DurationMs:       1000,
				PromptTokens:     100,
				CompletionTokens: 50,
				Success:          true,
				Source:           tt.source,
			}

			err := req.Validate()
			require.Error(t, err, "invalid source should fail validation")
			assert.ErrorIs(t, err, ErrInvalidSource, "error should be ErrInvalidSource")
		})
	}
}

// TestIsValidErrorType tests error type validation function.
func TestIsValidErrorType(t *testing.T) {
	// Test all valid error types
	validTypes := []string{
		"rate_limit",
		"timeout",
		"auth_failure",
		"invalid_request",
		"context_length",
		"stream_interrupted",
		"tool_call_failed",
		"provider_error",
		"network_error",
		"parse_error",
		"unknown",
	}

	for _, errType := range validTypes {
		t.Run("valid_"+errType, func(t *testing.T) {
			assert.True(t, IsValidErrorType(errType), "%s should be a valid error type", errType)
		})
	}

	// Test invalid error types
	invalidTypes := []string{
		"",
		"invalid",
		"RATE_LIMIT",
		"rate-limit",
		"ratelimit",
		"error",
		"something_random",
	}

	for _, errType := range invalidTypes {
		t.Run("invalid_"+errType, func(t *testing.T) {
			assert.False(t, IsValidErrorType(errType), "%s should be an invalid error type", errType)
		})
	}
}

// TestErrorCodeMapping tests that ErrorCode map contains all valid error types.
func TestErrorCodeMapping(t *testing.T) {
	validTypes := []string{
		"rate_limit",
		"timeout",
		"auth_failure",
		"invalid_request",
		"context_length",
		"stream_interrupted",
		"tool_call_failed",
		"provider_error",
		"network_error",
		"parse_error",
		"unknown",
	}

	for _, errType := range validTypes {
		t.Run(errType, func(t *testing.T) {
			code, exists := ErrorCode[errType]
			assert.True(t, exists, "ErrorCode should contain %s", errType)
			assert.Greater(t, code, 0, "error code should be positive")
		})
	}
}

// Helper function to create pointer to int64
func ptr(v int64) *int64 {
	return &v
}
