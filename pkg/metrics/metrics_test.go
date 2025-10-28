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
	"runtime"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitialize verifies that the metrics registry is properly initialized.
func TestInitialize(t *testing.T) {
	// Initialize with a test namespace
	Initialize("aichat")

	// Verify Registry is not nil after initialization
	require.NotNil(t, Registry, "Registry should not be nil after Initialize")

	// Verify PluginUp gauge was created
	require.NotNil(t, PluginUp, "PluginUp gauge should not be nil after Initialize")

	// Verify IsInitialized returns true
	assert.True(t, IsInitialized(), "IsInitialized should return true after Initialize")

	// Verify second Initialize call is idempotent (should not panic)
	assert.NotPanics(t, func() {
		Initialize("aichat")
	}, "Second Initialize call should not panic")
}

// TestPluginUpValue verifies the PluginUp gauge is set to 1 (healthy).
func TestPluginUpValue(t *testing.T) {
	// Ensure initialized
	Initialize("aichat")

	// Verify PluginUp value is 1 (healthy)
	value := testutil.ToFloat64(PluginUp)
	assert.Equal(t, float64(1), value, "PluginUp gauge should be 1 (healthy) after initialization")
}

// TestRegistryContainsGoMetrics verifies that Go runtime metrics are registered.
func TestRegistryContainsGoMetrics(t *testing.T) {
	Initialize("aichat")

	// Gather all metrics from the registry
	metrics, err := Registry.Gather()
	require.NoError(t, err, "Registry.Gather() should not error")

	// Check for go_* metrics
	var foundGoGoroutines, foundGoMemstats bool
	for _, mf := range metrics {
		name := mf.GetName()
		if name == "go_goroutines" {
			foundGoGoroutines = true
		}
		if strings.HasPrefix(name, "go_memstats_") {
			foundGoMemstats = true
		}
	}

	assert.True(t, foundGoGoroutines, "Registry should contain go_goroutines metric")
	assert.True(t, foundGoMemstats, "Registry should contain go_memstats_* metrics")
}

// TestRegistryContainsProcessMetrics verifies that process metrics are registered.
func TestRegistryContainsProcessMetrics(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process_* metrics are only guaranteed on Linux")
	}

	Initialize("aichat")

	// Gather all metrics from the registry
	metrics, err := Registry.Gather()
	require.NoError(t, err, "Registry.Gather() should not error")

	// Check for process_* metrics
	var foundProcessMetric bool
	for _, mf := range metrics {
		name := mf.GetName()
		if strings.HasPrefix(name, "process_") {
			foundProcessMetric = true
			break
		}
	}

	assert.True(t, foundProcessMetric, "Registry should contain process_* metrics")
}

// TestRegistryContainsBuildInfo verifies that the build info metric is registered.
func TestRegistryContainsBuildInfo(t *testing.T) {
	Initialize("aichat")

	// Gather all metrics from the registry
	metrics, err := Registry.Gather()
	require.NoError(t, err, "Registry.Gather() should not error")

	// Check for go_build_info metric
	var foundBuildInfo bool
	for _, mf := range metrics {
		if mf.GetName() == "go_build_info" {
			foundBuildInfo = true
			break
		}
	}

	assert.True(t, foundBuildInfo, "Registry should contain go_build_info metric")
}

// TestLLMBuckets verifies the LLMBuckets variable has the expected values.
func TestLLMBuckets(t *testing.T) {
	// Verify LLMBuckets is not empty
	require.NotEmpty(t, LLMBuckets, "LLMBuckets should not be empty")

	// Verify expected bucket count
	assert.Len(t, LLMBuckets, 10, "LLMBuckets should have 10 buckets")

	// Verify first bucket is 0.1 (100ms)
	assert.Equal(t, 0.1, LLMBuckets[0], "First LLM bucket should be 0.1")

	// Verify last bucket is 120 (2 minutes)
	assert.Equal(t, 120.0, LLMBuckets[len(LLMBuckets)-1], "Last LLM bucket should be 120")

	// Verify buckets are in ascending order
	for i := 1; i < len(LLMBuckets); i++ {
		assert.Greater(t, LLMBuckets[i], LLMBuckets[i-1],
			"LLMBuckets should be in ascending order")
	}
}
