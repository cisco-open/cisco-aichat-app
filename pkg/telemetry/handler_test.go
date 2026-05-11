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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/cisco-aichat-app/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// init ensures metrics are initialized before tests run.
func init() {
	metrics.Initialize("aichat")
}

// TestHandlerMethodNotAllowed verifies that GET/PUT/DELETE return 405.
func TestHandlerMethodNotAllowed(t *testing.T) {
	handler := NewHandler()

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/telemetry", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusMethodNotAllowed, w.Code, "%s should return 405", method)

			var resp TelemetryResponse
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err, "response should be valid JSON")
			assert.False(t, resp.Recorded, "recorded should be false")
			assert.NotEmpty(t, resp.Error, "error message should not be empty")
		})
	}
}

// TestHandlerInvalidJSON verifies that malformed JSON returns 400.
func TestHandlerInvalidJSON(t *testing.T) {
	handler := NewHandler()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty body",
			body: "",
		},
		{
			name: "malformed JSON",
			body: "{invalid json}",
		},
		{
			name: "unclosed brace",
			body: `{"streaming": true`,
		},
		{
			name: "trailing comma",
			body: `{"streaming": true,}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "invalid JSON should return 400")

			var resp TelemetryResponse
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err, "response should be valid JSON")
			assert.False(t, resp.Recorded, "recorded should be false")
			assert.Contains(t, resp.Error, "invalid JSON", "error should mention invalid JSON")
		})
	}
}

// TestHandlerValidationFailure verifies that invalid TelemetryRequest returns 400.
func TestHandlerValidationFailure(t *testing.T) {
	handler := NewHandler()

	tests := []struct {
		name        string
		req         TelemetryRequest
		expectError string
	}{
		{
			name: "negative duration",
			req: TelemetryRequest{
				DurationMs: -1,
				Source:     "user",
				Success:    true,
			},
			expectError: "duration_ms",
		},
		{
			name: "invalid source",
			req: TelemetryRequest{
				DurationMs: 1000,
				Source:     "invalid",
				Success:    true,
			},
			expectError: "source",
		},
		{
			name: "missing error type on failure",
			req: TelemetryRequest{
				DurationMs: 1000,
				Source:     "user",
				Success:    false,
				ErrorType:  "",
			},
			expectError: "error_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.req)
			req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "validation failure should return 400")

			var resp TelemetryResponse
			err := json.NewDecoder(w.Body).Decode(&resp)
			require.NoError(t, err, "response should be valid JSON")
			assert.False(t, resp.Recorded, "recorded should be false")
			assert.Contains(t, resp.Error, tt.expectError, "error should mention %s", tt.expectError)
		})
	}
}

// TestHandlerSuccess_NonStreaming verifies valid non-streaming request handling.
func TestHandlerSuccess_NonStreaming(t *testing.T) {
	handler := NewHandler()

	// Get baseline metric values
	baseCount := testutil.ToFloat64(metrics.RequestCount.WithLabelValues("success", "false"))
	basePromptTokens := testutil.ToFloat64(metrics.TokensTotal.WithLabelValues("prompt"))
	baseCompletionTokens := testutil.ToFloat64(metrics.TokensTotal.WithLabelValues("completion"))

	telReq := TelemetryRequest{
		Streaming:        false,
		DurationMs:       1500,
		PromptTokens:     100,
		CompletionTokens: 50,
		Success:          true,
		Source:           "user",
	}

	body, _ := json.Marshal(telReq)
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Grafana-User", "testuser")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code, "valid request should return 200")

	var resp TelemetryResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err, "response should be valid JSON")
	assert.True(t, resp.Recorded, "recorded should be true")
	assert.Empty(t, resp.Error, "error should be empty")

	// Verify metrics were recorded
	newCount := testutil.ToFloat64(metrics.RequestCount.WithLabelValues("success", "false"))
	assert.Equal(t, baseCount+1, newCount, "RequestCount should be incremented")

	newPromptTokens := testutil.ToFloat64(metrics.TokensTotal.WithLabelValues("prompt"))
	assert.Equal(t, basePromptTokens+100, newPromptTokens, "prompt tokens should be added")

	newCompletionTokens := testutil.ToFloat64(metrics.TokensTotal.WithLabelValues("completion"))
	assert.Equal(t, baseCompletionTokens+50, newCompletionTokens, "completion tokens should be added")
}

// TestHandlerSuccess_Streaming verifies valid streaming request handling.
func TestHandlerSuccess_Streaming(t *testing.T) {
	handler := NewHandler()

	// Get baseline metric values
	baseCount := testutil.ToFloat64(metrics.RequestCount.WithLabelValues("success", "true"))

	ttft := int64(500)
	telReq := TelemetryRequest{
		Streaming:        true,
		DurationMs:       3000,
		TTFTMs:           &ttft,
		PromptTokens:     150,
		CompletionTokens: 75,
		Success:          true,
		Source:           "system",
	}

	body, _ := json.Marshal(telReq)
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code, "valid streaming request should return 200")

	var resp TelemetryResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err, "response should be valid JSON")
	assert.True(t, resp.Recorded, "recorded should be true")

	// Verify RequestCount was incremented with stream=true
	newCount := testutil.ToFloat64(metrics.RequestCount.WithLabelValues("success", "true"))
	assert.Equal(t, baseCount+1, newCount, "RequestCount should be incremented for streaming")

	// TTFT metric was recorded (histogram - verify by gathering)
	count, err := testutil.GatherAndCount(metrics.Registry, "aichat_llm_time_to_first_token_seconds")
	require.NoError(t, err, "GatherAndCount should not error")
	assert.Greater(t, count, 0, "TTFT histogram should have observations")
}

// TestHandlerError verifies error request handling.
func TestHandlerError(t *testing.T) {
	handler := NewHandler()

	// Get baseline metric values
	baseErrorCount := testutil.ToFloat64(metrics.ErrorsTotal.WithLabelValues("rate_limit"))
	baseRequestCount := testutil.ToFloat64(metrics.RequestCount.WithLabelValues("error", "false"))

	telReq := TelemetryRequest{
		Streaming:        false,
		DurationMs:       500,
		PromptTokens:     100,
		CompletionTokens: 0,
		Success:          false,
		ErrorType:        "rate_limit",
		ErrorCode:        429,
		Source:           "user",
	}

	body, _ := json.Marshal(telReq)
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code, "error request should still return 200")

	var resp TelemetryResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err, "response should be valid JSON")
	assert.True(t, resp.Recorded, "recorded should be true for error telemetry")

	// Verify ErrorsTotal was incremented
	newErrorCount := testutil.ToFloat64(metrics.ErrorsTotal.WithLabelValues("rate_limit"))
	assert.Equal(t, baseErrorCount+1, newErrorCount, "ErrorsTotal should be incremented")

	// Verify RequestCount was incremented with status=error
	newRequestCount := testutil.ToFloat64(metrics.RequestCount.WithLabelValues("error", "false"))
	assert.Equal(t, baseRequestCount+1, newRequestCount, "RequestCount should be incremented with status=error")
}

// TestHandlerAnonymousUser verifies that missing X-Grafana-User is handled.
func TestHandlerAnonymousUser(t *testing.T) {
	handler := NewHandler()

	telReq := TelemetryRequest{
		Streaming:        false,
		DurationMs:       1000,
		PromptTokens:     50,
		CompletionTokens: 25,
		Success:          true,
		Source:           "user",
	}

	body, _ := json.Marshal(telReq)
	// Don't set X-Grafana-User header
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should still succeed
	assert.Equal(t, http.StatusOK, w.Code, "request without user header should succeed")

	var resp TelemetryResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err, "response should be valid JSON")
	assert.True(t, resp.Recorded, "recorded should be true")
}

// TestHandlerContentType verifies handler works without explicit Content-Type.
func TestHandlerContentType(t *testing.T) {
	handler := NewHandler()

	telReq := TelemetryRequest{
		Streaming:        false,
		DurationMs:       500,
		PromptTokens:     25,
		CompletionTokens: 10,
		Success:          true,
		Source:           "user",
	}

	body, _ := json.Marshal(telReq)
	// Don't set Content-Type header - Go's json.Decoder should still work
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "request without Content-Type should succeed")

	var resp TelemetryResponse
	err := json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err, "response should be valid JSON")
	assert.True(t, resp.Recorded, "recorded should be true")
}

// TestNewHandler verifies handler creation.
func TestNewHandler(t *testing.T) {
	handler := NewHandler()
	require.NotNil(t, handler, "NewHandler should return non-nil handler")
}
