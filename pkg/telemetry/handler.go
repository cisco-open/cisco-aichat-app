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
	"context"
	"encoding/json"
	"net/http"

	"github.com/grafana/grafana-aichat-app/pkg/metrics"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// Handler handles HTTP requests to the telemetry endpoint.
// It is stateless - metrics are recorded to the global metrics package.
type Handler struct{}

// NewHandler creates a new telemetry handler.
func NewHandler() *Handler {
	return &Handler{}
}

// ServeHTTP handles incoming telemetry requests.
// It only accepts POST requests with a JSON body containing TelemetryRequest data.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept POST method
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(TelemetryResponse{
			Recorded: false,
			Error:    "method not allowed",
		})
		return
	}

	// Decode JSON body
	var req TelemetryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TelemetryResponse{
			Recorded: false,
			Error:    "invalid JSON body",
		})
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TelemetryResponse{
			Recorded: false,
			Error:    err.Error(),
		})
		return
	}

	// Extract user from Grafana header
	user := r.Header.Get("X-Grafana-User")
	if user == "" {
		user = "anonymous"
	}

	// Record metrics
	h.recordMetrics(&req)

	// Log structured telemetry event
	h.logTelemetry(r.Context(), &req, user)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(TelemetryResponse{Recorded: true})
}

// recordMetrics updates Prometheus metrics based on the telemetry request.
func (h *Handler) recordMetrics(req *TelemetryRequest) {
	// Determine label values
	status := "success"
	if !req.Success {
		status = "error"
	}

	stream := "false"
	if req.Streaming {
		stream = "true"
	}

	// Increment request count
	metrics.RequestCount.WithLabelValues(status, stream).Inc()

	// Observe request latency (convert ms to seconds)
	metrics.RequestLatency.WithLabelValues(status, stream).Observe(float64(req.DurationMs) / 1000.0)

	// Add token counts
	metrics.TokensTotal.WithLabelValues("prompt").Add(float64(req.PromptTokens))
	metrics.TokensTotal.WithLabelValues("completion").Add(float64(req.CompletionTokens))

	// Record time to first token for streaming requests
	if req.Streaming && req.TTFTMs != nil {
		metrics.TimeToFirstToken.Observe(float64(*req.TTFTMs) / 1000.0)
	}

	// Increment error counter if request failed
	if !req.Success && req.ErrorType != "" {
		metrics.ErrorsTotal.WithLabelValues(req.ErrorType).Inc()
	}
}

// logTelemetry logs a structured telemetry event at Info level.
// NEVER logs prompt or response content (privacy per CONTEXT.md).
func (h *Handler) logTelemetry(ctx context.Context, req *TelemetryRequest, user string) {
	logger := log.DefaultLogger.FromContext(ctx)

	// Build log fields - base fields always included
	fields := []interface{}{
		"duration_ms", req.DurationMs,
		"prompt_tokens", req.PromptTokens,
		"completion_tokens", req.CompletionTokens,
		"streaming", req.Streaming,
		"source", req.Source,
		"user", user,
	}

	// Add error fields if request failed
	if !req.Success {
		fields = append(fields, "error_code", req.ErrorCode)
		fields = append(fields, "error_reason", req.ErrorType)
	}

	// Add TTFT for streaming requests
	if req.Streaming && req.TTFTMs != nil {
		fields = append(fields, "ttft_ms", *req.TTFTMs)
	}

	logger.Debug("LLM telemetry received", fields...)
}
