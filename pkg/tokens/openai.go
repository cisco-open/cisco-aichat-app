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
	"fmt"
	"strings"
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

// OpenAICounter counts tokens for OpenAI models using tiktoken-go
type OpenAICounter struct {
	mu     sync.RWMutex
	codecs map[tokenizer.Encoding]tokenizer.Codec
}

// NewOpenAICounter creates a new OpenAI token counter with pre-initialized codecs
func NewOpenAICounter() (*OpenAICounter, error) {
	counter := &OpenAICounter{
		codecs: make(map[tokenizer.Encoding]tokenizer.Codec),
	}

	// Pre-initialize commonly used encodings
	o200k, err := tokenizer.Get(tokenizer.O200kBase)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize O200kBase encoding: %w", err)
	}
	counter.codecs[tokenizer.O200kBase] = o200k

	cl100k, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Cl100kBase encoding: %w", err)
	}
	counter.codecs[tokenizer.Cl100kBase] = cl100k

	return counter, nil
}

// SupportsModel returns true for OpenAI models (gpt-*, o1*, o3*, o4*, text-*)
func (c *OpenAICounter) SupportsModel(model string) bool {
	model = strings.ToLower(model)
	return strings.HasPrefix(model, "gpt-") ||
		strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4") ||
		strings.HasPrefix(model, "text-") ||
		strings.HasPrefix(model, "code-") ||
		strings.HasPrefix(model, "davinci") ||
		strings.HasPrefix(model, "curie") ||
		strings.HasPrefix(model, "babbage") ||
		strings.HasPrefix(model, "ada") ||
		strings.HasPrefix(model, "ft:gpt-")
}

// CountTokens counts tokens in the given text for the specified OpenAI model
func (c *OpenAICounter) CountTokens(ctx context.Context, text string, model string) (int, error) {
	encoding := c.encodingForModel(model)

	c.mu.RLock()
	codec, ok := c.codecs[encoding]
	c.mu.RUnlock()

	if !ok {
		// Try to load the encoding if not pre-cached
		c.mu.Lock()
		codec, ok = c.codecs[encoding]
		if !ok {
			var err error
			codec, err = tokenizer.Get(encoding)
			if err != nil {
				c.mu.Unlock()
				return 0, fmt.Errorf("failed to get encoding %s: %w", encoding, err)
			}
			c.codecs[encoding] = codec
		}
		c.mu.Unlock()
	}

	count, err := codec.Count(text)
	if err != nil {
		return 0, fmt.Errorf("failed to count tokens: %w", err)
	}

	return count, nil
}

// encodingForModel returns the appropriate encoding for the given model
func (c *OpenAICounter) encodingForModel(model string) tokenizer.Encoding {
	model = strings.ToLower(model)

	// O200kBase encoding (newer models)
	if strings.HasPrefix(model, "gpt-4o") ||
		strings.HasPrefix(model, "gpt-4.1") ||
		strings.HasPrefix(model, "gpt-4.5") ||
		strings.HasPrefix(model, "gpt-5") ||
		strings.HasPrefix(model, "chatgpt-4o") ||
		strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4") {
		return tokenizer.O200kBase
	}

	// Cl100kBase encoding (GPT-4, GPT-3.5-turbo, embeddings, fine-tuned)
	if strings.HasPrefix(model, "gpt-4") ||
		strings.HasPrefix(model, "gpt-3.5") ||
		strings.HasPrefix(model, "gpt-35") ||
		strings.HasPrefix(model, "text-embedding") ||
		strings.HasPrefix(model, "ft:gpt-4") ||
		strings.HasPrefix(model, "ft:gpt-3.5") ||
		strings.HasPrefix(model, "ft:davinci-002") ||
		strings.HasPrefix(model, "ft:babbage-002") {
		return tokenizer.Cl100kBase
	}

	// P50kBase for Codex models
	if strings.HasPrefix(model, "code-davinci") ||
		strings.HasPrefix(model, "code-cushman") ||
		strings.HasPrefix(model, "text-davinci-003") ||
		strings.HasPrefix(model, "text-davinci-002") ||
		strings.HasPrefix(model, "davinci-codex") ||
		strings.HasPrefix(model, "cushman-codex") {
		return tokenizer.P50kBase
	}

	// P50kEdit for edit models
	if strings.HasPrefix(model, "text-davinci-edit") ||
		strings.HasPrefix(model, "code-davinci-edit") {
		return tokenizer.P50kEdit
	}

	// R50kBase for older models
	if strings.HasPrefix(model, "text-davinci-001") ||
		strings.HasPrefix(model, "text-curie") ||
		strings.HasPrefix(model, "text-babbage") ||
		strings.HasPrefix(model, "text-ada") ||
		strings.HasPrefix(model, "davinci") ||
		strings.HasPrefix(model, "curie") ||
		strings.HasPrefix(model, "babbage") ||
		strings.HasPrefix(model, "ada") ||
		strings.HasPrefix(model, "text-similarity") ||
		strings.HasPrefix(model, "text-search") ||
		strings.HasPrefix(model, "code-search") {
		return tokenizer.R50kBase
	}

	// Default to Cl100kBase for unknown OpenAI models
	return tokenizer.Cl100kBase
}
