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
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLLMMetricsRegistered verifies that all 5 LLM metrics are registered.
func TestLLMMetricsRegistered(t *testing.T) {
	// Ensure metrics are initialized
	Initialize("aichat")

	// Verify all 5 LLM metric variables are not nil (registered)
	require.NotNil(t, RequestCount, "RequestCount should be registered")
	require.NotNil(t, RequestLatency, "RequestLatency should be registered")
	require.NotNil(t, TokensTotal, "TokensTotal should be registered")
	require.NotNil(t, TimeToFirstToken, "TimeToFirstToken should be registered")
	require.NotNil(t, ErrorsTotal, "ErrorsTotal should be registered")

	// Use each metric once so they appear in Gather()
	RequestCount.WithLabelValues("success", "false").Inc()
	RequestLatency.WithLabelValues("success", "false").Observe(1.0)
	TokensTotal.WithLabelValues("prompt").Add(1)
	TimeToFirstToken.Observe(0.5)
	ErrorsTotal.WithLabelValues("unknown").Inc()

	// Verify all 5 metrics appear in Registry.Gather()
	metrics, err := Registry.Gather()
	require.NoError(t, err, "Registry.Gather() should not error")

	// Track which LLM metrics we find
	expectedMetrics := []string{
		"aichat_llm_request_count",
		"aichat_llm_request_latency_seconds",
		"aichat_llm_tokens_total",
		"aichat_llm_time_to_first_token_seconds",
		"aichat_llm_errors_total",
	}

	foundMetrics := make(map[string]bool)
	for _, mf := range metrics {
		foundMetrics[mf.GetName()] = true
	}

	// Verify all 5 LLM metrics are present
	for _, name := range expectedMetrics {
		assert.True(t, foundMetrics[name], "Registry should contain %s metric", name)
	}
}

// TestLLMMetricsLabels verifies that metrics have the correct labels.
func TestLLMMetricsLabels(t *testing.T) {
	// Ensure metrics are initialized
	Initialize("aichat")

	// Test RequestCount has status and stream labels by incrementing with those labels
	// Get baseline values first (in case other tests ran)
	baseSuccessTrue := testutil.ToFloat64(RequestCount.WithLabelValues("success", "true"))
	baseErrorFalse := testutil.ToFloat64(RequestCount.WithLabelValues("error", "false"))

	RequestCount.WithLabelValues("success", "true").Inc()
	RequestCount.WithLabelValues("error", "false").Inc()

	// Verify RequestCount incremented correctly
	assert.Equal(t, baseSuccessTrue+1, testutil.ToFloat64(RequestCount.WithLabelValues("success", "true")),
		"RequestCount should support status=success, stream=true labels")
	assert.Equal(t, baseErrorFalse+1, testutil.ToFloat64(RequestCount.WithLabelValues("error", "false")),
		"RequestCount should support status=error, stream=false labels")

	// Test RequestLatency has status and stream labels
	RequestLatency.WithLabelValues("success", "true").Observe(1.5)
	RequestLatency.WithLabelValues("success", "false").Observe(2.5)

	// Use GatherAndCount to verify RequestLatency exists
	count, err := testutil.GatherAndCount(Registry, "aichat_llm_request_latency_seconds")
	require.NoError(t, err, "GatherAndCount should not error for RequestLatency")
	assert.Greater(t, count, 0, "RequestLatency should have at least one metric")

	// Test TokensTotal has type label by checking increment
	basePrompt := testutil.ToFloat64(TokensTotal.WithLabelValues("prompt"))
	baseCompletion := testutil.ToFloat64(TokensTotal.WithLabelValues("completion"))

	TokensTotal.WithLabelValues("prompt").Add(100)
	TokensTotal.WithLabelValues("completion").Add(50)

	assert.Equal(t, basePrompt+100, testutil.ToFloat64(TokensTotal.WithLabelValues("prompt")),
		"TokensTotal should support type=prompt label")
	assert.Equal(t, baseCompletion+50, testutil.ToFloat64(TokensTotal.WithLabelValues("completion")),
		"TokensTotal should support type=completion label")

	// Test ErrorsTotal has error_type label by checking increment
	baseRateLimit := testutil.ToFloat64(ErrorsTotal.WithLabelValues("rate_limit"))
	baseTimeout := testutil.ToFloat64(ErrorsTotal.WithLabelValues("timeout"))

	ErrorsTotal.WithLabelValues("rate_limit").Inc()
	ErrorsTotal.WithLabelValues("timeout").Inc()

	assert.Equal(t, baseRateLimit+1, testutil.ToFloat64(ErrorsTotal.WithLabelValues("rate_limit")),
		"ErrorsTotal should support error_type=rate_limit label")
	assert.Equal(t, baseTimeout+1, testutil.ToFloat64(ErrorsTotal.WithLabelValues("timeout")),
		"ErrorsTotal should support error_type=timeout label")
}

// TestLLMMetricsIdempotent verifies that double initialization is safe.
func TestLLMMetricsIdempotent(t *testing.T) {
	// Initialize twice - should not panic
	assert.NotPanics(t, func() {
		Initialize("aichat")
		Initialize("aichat")
	}, "Double Initialize should not panic")

	// Also test InitializeLLMMetrics directly
	assert.NotPanics(t, func() {
		InitializeLLMMetrics("aichat")
		InitializeLLMMetrics("aichat")
	}, "Double InitializeLLMMetrics should not panic")

	// Verify metrics are still accessible after double init
	require.NotNil(t, RequestCount, "RequestCount should not be nil after double init")
	require.NotNil(t, RequestLatency, "RequestLatency should not be nil after double init")
	require.NotNil(t, TokensTotal, "TokensTotal should not be nil after double init")
	require.NotNil(t, TimeToFirstToken, "TimeToFirstToken should not be nil after double init")
	require.NotNil(t, ErrorsTotal, "ErrorsTotal should not be nil after double init")
}

// TestTTFTBuckets verifies the TTFTBuckets variable has the expected values.
func TestTTFTBuckets(t *testing.T) {
	// Verify TTFTBuckets is not empty
	require.NotEmpty(t, TTFTBuckets, "TTFTBuckets should not be empty")

	// Verify expected bucket count (8 buckets for TTFT)
	assert.Len(t, TTFTBuckets, 8, "TTFTBuckets should have 8 buckets")

	// Verify first bucket is 0.1 (100ms)
	assert.Equal(t, 0.1, TTFTBuckets[0], "First TTFT bucket should be 0.1")

	// Verify last bucket is 10 (10 seconds)
	assert.Equal(t, 10.0, TTFTBuckets[len(TTFTBuckets)-1], "Last TTFT bucket should be 10")

	// Verify buckets are in ascending order
	for i := 1; i < len(TTFTBuckets); i++ {
		assert.Greater(t, TTFTBuckets[i], TTFTBuckets[i-1],
			"TTFTBuckets should be in ascending order")
	}
}

// TestTimeToFirstTokenHistogram verifies that TTFT histogram accepts observations.
func TestTimeToFirstTokenHistogram(t *testing.T) {
	// Ensure metrics are initialized
	Initialize("aichat")

	// Observe some values
	TimeToFirstToken.Observe(0.5)  // 500ms
	TimeToFirstToken.Observe(1.0)  // 1s
	TimeToFirstToken.Observe(2.5)  // 2.5s

	// Gather metrics and verify histogram exists
	metrics, err := Registry.Gather()
	require.NoError(t, err, "Registry.Gather() should not error")

	var found bool
	for _, mf := range metrics {
		if mf.GetName() == "aichat_llm_time_to_first_token_seconds" {
			found = true
			// Verify it has histogram type
			assert.True(t, strings.Contains(mf.GetType().String(), "HISTOGRAM"),
				"TimeToFirstToken should be a histogram type")
			break
		}
	}

	assert.True(t, found, "TimeToFirstToken histogram should be registered")
}
