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

// Package metrics provides Prometheus metrics infrastructure for the AI Chat Assistant plugin.
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// Registry is the gatherer used for tests and internal metric inspection.
	// Metrics are registered on Prometheus default registerer so Grafana can collect
	// them via the plugin SDK diagnostics (CollectMetrics) path.
	Registry = prometheus.DefaultGatherer

	// registerer is the metrics registerer used by this plugin.
	registerer = prometheus.DefaultRegisterer

	// PluginUp is a gauge indicating plugin health (1=healthy, 0=unhealthy).
	PluginUp prometheus.Gauge

	// once ensures single initialization of metrics.
	once sync.Once

	initialized bool
)

// LLMBuckets defines histogram buckets for LLM response times.
// Buckets are optimized for typical LLM latencies which range from
// 100ms for simple completions to 120s for complex reasoning tasks.
var LLMBuckets = []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120}

// Initialize sets up the Prometheus metrics registry and registers all collectors.
// This function is idempotent - calling it multiple times has no effect after first call.
// The namespace parameter prefixes all custom metrics (e.g., "aichat_plugin_up").
func Initialize(namespace string) {
	once.Do(func() {
		// Register baseline runtime/process/build collectors on default registry.
		registerIfMissing(prometheus.NewGoCollector())
		registerIfMissing(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
		registerIfMissing(prometheus.NewBuildInfoCollector())

		// Create and register plugin health gauge
		PluginUp = prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "plugin_up",
			Help:      "Indicates if the plugin is healthy (1) or unhealthy (0)",
		})
		mustRegister(PluginUp)

		// Set initial state to healthy
		PluginUp.Set(1)

		// Initialize LLM-specific metrics
		InitializeLLMMetrics(namespace)
		initialized = true
	})
}

// IsInitialized returns true if the metrics registry has been initialized.
func IsInitialized() bool {
	return initialized
}

// SetPluginUp updates the plugin health gauge.
func SetPluginUp(isHealthy bool) {
	if PluginUp == nil {
		return
	}
	if isHealthy {
		PluginUp.Set(1)
		return
	}
	PluginUp.Set(0)
}

func mustRegister(collector prometheus.Collector) {
	if err := registerer.Register(collector); err != nil {
		if _, exists := err.(prometheus.AlreadyRegisteredError); exists {
			return
		}
		panic(err)
	}
}

func registerIfMissing(collector prometheus.Collector) {
	if err := registerer.Register(collector); err != nil {
		if _, exists := err.(prometheus.AlreadyRegisteredError); exists {
			return
		}
		panic(err)
	}
}
