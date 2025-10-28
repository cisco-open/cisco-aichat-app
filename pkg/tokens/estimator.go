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

import "context"

// EstimationCounter provides fallback token estimation using chars/4 approximation
// This is used when:
// - Model is unknown
// - Provider-specific counter fails
// - No API key available for API-based counting
type EstimationCounter struct{}

// NewEstimationCounter creates a new estimation-based token counter
func NewEstimationCounter() *EstimationCounter {
	return &EstimationCounter{}
}

// SupportsModel always returns true - estimation works for all models
func (c *EstimationCounter) SupportsModel(model string) bool {
	return true
}

// CountTokens estimates tokens as characters/4 (rounded up)
// This is a reasonable approximation for most LLM tokenizers
// Never returns an error - estimation always succeeds
func (c *EstimationCounter) CountTokens(ctx context.Context, text string, model string) (int, error) {
	// chars/4 rounded up: (len + 3) / 4
	return (len(text) + 3) / 4, nil
}
