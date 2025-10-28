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

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// RequestCount tracks the total number of LLM requests.
	// Labels: status (success/error), stream (true/false)
	RequestCount *prometheus.CounterVec

	// RequestLatency tracks LLM request latency distribution in seconds.
	// Labels: status (success/error), stream (true/false)
	RequestLatency *prometheus.HistogramVec

	// TokensTotal tracks total tokens used.
	// Labels: type (prompt/completion)
	TokensTotal *prometheus.CounterVec

	// TimeToFirstToken tracks time to first token for streaming responses.
	TimeToFirstToken prometheus.Histogram

	// ErrorsTotal tracks total LLM errors by type.
	// Labels: error_type
	ErrorsTotal *prometheus.CounterVec

	// llmOnce ensures single initialization of LLM metrics.
	llmOnce sync.Once
)

// TTFTBuckets defines histogram buckets for time-to-first-token measurements.
// Buckets are optimized for TTFT which is typically faster than total latency,
// ranging from 100ms to 10s.
var TTFTBuckets = []float64{0.1, 0.25, 0.5, 1, 2, 3, 5, 10}

// InitializeLLMMetrics creates and registers LLM-specific metrics with the registry.
// This function is idempotent - calling it multiple times has no effect after first call.
// The namespace parameter prefixes all metrics (e.g., "aichat_llm_request_count").
func InitializeLLMMetrics(namespace string) {
	llmOnce.Do(func() {
		// RequestCount - Total number of LLM requests
		RequestCount = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_request_count",
				Help:      "Total number of LLM requests",
			},
			[]string{"status", "stream"},
		)
		mustRegister(RequestCount)

		// RequestLatency - LLM request latency distribution
		RequestLatency = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "llm_request_latency_seconds",
				Help:      "LLM request latency distribution in seconds",
				Buckets:   LLMBuckets,
			},
			[]string{"status", "stream"},
		)
		mustRegister(RequestLatency)

		// TokensTotal - Total tokens used
		TokensTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_tokens_total",
				Help:      "Total tokens used",
			},
			[]string{"type"},
		)
		mustRegister(TokensTotal)

		// TimeToFirstToken - Time to first token for streaming responses
		TimeToFirstToken = prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "llm_time_to_first_token_seconds",
				Help:      "Time to first token for streaming responses",
				Buckets:   TTFTBuckets,
			},
		)
		mustRegister(TimeToFirstToken)

		// ErrorsTotal - Total LLM errors by type
		ErrorsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "llm_errors_total",
				Help:      "Total LLM errors by type",
			},
			[]string{"error_type"},
		)
		mustRegister(ErrorsTotal)
	})
}
