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

package tokens

import (
	"context"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// TokenCounter defines the interface for counting tokens in text
type TokenCounter interface {
	// CountTokens returns the number of tokens in the given text for the specified model
	CountTokens(ctx context.Context, text string, model string) (int, error)

	// SupportsModel returns true if this counter can handle the given model
	SupportsModel(model string) bool
}

// Registry holds multiple token counters and routes requests to the appropriate one
type Registry struct {
	counters  []TokenCounter
	estimator TokenCounter
	logger    log.Logger
}

// NewRegistry creates a new token counter registry
// counters are checked in order; first one that supports the model is used
// estimator is used as fallback when no counter matches or on errors
func NewRegistry(counters []TokenCounter, estimator TokenCounter) *Registry {
	return &Registry{
		counters:  counters,
		estimator: estimator,
		logger:    log.DefaultLogger,
	}
}

// CountTokens counts tokens using the appropriate counter for the model
// Falls back to estimator on errors or unknown models
func (r *Registry) CountTokens(ctx context.Context, text string, model string) (int, error) {
	// Find first counter that supports this model
	for _, counter := range r.counters {
		if counter.SupportsModel(model) {
			count, err := counter.CountTokens(ctx, text, model)
			if err != nil {
				// Log the error and fall back to estimation
				r.logger.Warn("Token counter failed, falling back to estimation",
					"model", model,
					"error", err.Error(),
				)
				return r.estimator.CountTokens(ctx, text, model)
			}
			return count, nil
		}
	}

	// No counter supports this model, use estimator
	r.logger.Debug("No counter for model, using estimation", "model", model)
	return r.estimator.CountTokens(ctx, text, model)
}

// SupportsModel returns true (registry supports all models via fallback)
func (r *Registry) SupportsModel(model string) bool {
	return true
}
