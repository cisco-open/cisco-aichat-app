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
	"errors"
	"fmt"
)

// TelemetryRequest represents the telemetry data sent from the frontend after an LLM call.
type TelemetryRequest struct {
	// Streaming indicates if this was a streaming request.
	Streaming bool `json:"streaming"`

	// DurationMs is the total request duration in milliseconds.
	DurationMs int64 `json:"duration_ms"`

	// TTFTMs is the time to first token in milliseconds (streaming only, optional).
	TTFTMs *int64 `json:"ttft_ms,omitempty"`

	// PromptTokens is the number of tokens in the prompt.
	PromptTokens int `json:"prompt_tokens"`

	// CompletionTokens is the number of tokens in the completion.
	CompletionTokens int `json:"completion_tokens"`

	// Success indicates if the request completed successfully.
	Success bool `json:"success"`

	// ErrorType is the error classification if Success is false.
	ErrorType string `json:"error_type,omitempty"`

	// ErrorCode is the numeric error code if Success is false.
	ErrorCode int `json:"error_code,omitempty"`

	// Source indicates if the request was user or system initiated.
	Source string `json:"source"`
}

// TelemetryResponse represents the response sent back to the frontend.
type TelemetryResponse struct {
	// Recorded indicates if the telemetry was successfully recorded.
	Recorded bool `json:"recorded"`

	// Error is the error message if recording failed.
	Error string `json:"error,omitempty"`
}

// Validation errors
var (
	ErrInvalidDuration         = errors.New("duration_ms must be >= 0")
	ErrInvalidTTFT             = errors.New("ttft_ms must be >= 0")
	ErrInvalidPromptTokens     = errors.New("prompt_tokens must be >= 0")
	ErrInvalidCompletionTokens = errors.New("completion_tokens must be >= 0")
	ErrInvalidErrorType        = errors.New("error_type must be valid when success is false")
	ErrInvalidSource           = errors.New("source must be 'user' or 'system'")
)

// Validate validates the TelemetryRequest fields.
func (r *TelemetryRequest) Validate() error {
	// Validate duration
	if r.DurationMs < 0 {
		return ErrInvalidDuration
	}

	// Validate TTFT for streaming requests
	if r.Streaming && r.TTFTMs != nil && *r.TTFTMs < 0 {
		return ErrInvalidTTFT
	}

	// Validate token counts
	if r.PromptTokens < 0 {
		return ErrInvalidPromptTokens
	}
	if r.CompletionTokens < 0 {
		return ErrInvalidCompletionTokens
	}

	// Validate error type for failed requests
	if !r.Success {
		if r.ErrorType == "" || !IsValidErrorType(r.ErrorType) {
			return fmt.Errorf("%w: got %q", ErrInvalidErrorType, r.ErrorType)
		}
	}

	// Validate source
	if r.Source != "user" && r.Source != "system" {
		return fmt.Errorf("%w: got %q", ErrInvalidSource, r.Source)
	}

	return nil
}
