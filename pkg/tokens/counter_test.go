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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEstimationCounter verifies chars/4 calculation
func TestEstimationCounter(t *testing.T) {
	counter := NewEstimationCounter()

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty string", "", 0},
		{"1 char", "a", 1},
		{"4 chars", "abcd", 1},
		{"5 chars", "abcde", 2},
		{"8 chars", "abcdefgh", 2},
		{"9 chars", "abcdefghi", 3},
		{"100 chars", string(make([]byte, 100)), 25},
		{"101 chars", string(make([]byte, 101)), 26},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := counter.CountTokens(context.Background(), tt.text, "any-model")
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, count)
		})
	}
}

func TestEstimationCounter_SupportsAllModels(t *testing.T) {
	counter := NewEstimationCounter()

	models := []string{"gpt-4", "claude-3", "unknown-model", ""}
	for _, model := range models {
		assert.True(t, counter.SupportsModel(model))
	}
}

// TestOpenAICounter verifies OpenAI token counting
func TestOpenAICounter(t *testing.T) {
	counter, err := NewOpenAICounter()
	require.NoError(t, err)

	tests := []struct {
		name  string
		model string
	}{
		{"gpt-4", "gpt-4"},
		{"gpt-4o", "gpt-4o"},
		{"gpt-3.5-turbo", "gpt-3.5-turbo"},
		{"gpt-4-turbo", "gpt-4-turbo"},
		{"gpt-4o-mini", "gpt-4o-mini"},
		{"o1-preview", "o1-preview"},
		{"o3-mini", "o3-mini"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, counter.SupportsModel(tt.model))

			text := "Hello, how are you today? This is a test message."
			count, err := counter.CountTokens(context.Background(), text, tt.model)
			assert.NoError(t, err)
			// Token count should be reasonable (not zero, not more than chars)
			assert.Greater(t, count, 0)
			assert.LessOrEqual(t, count, len(text))
		})
	}
}

func TestOpenAICounter_DoesNotSupportClaude(t *testing.T) {
	counter, err := NewOpenAICounter()
	require.NoError(t, err)

	assert.False(t, counter.SupportsModel("claude-3-opus"))
	assert.False(t, counter.SupportsModel("claude-sonnet-4.5"))
}

func TestOpenAICounter_EncodingSelection(t *testing.T) {
	counter, err := NewOpenAICounter()
	require.NoError(t, err)

	// Test that different models give different counts for same text
	// (because they use different encodings)
	text := "Hello, world! This is a test message for token counting."

	gpt4oCount, err := counter.CountTokens(context.Background(), text, "gpt-4o")
	require.NoError(t, err)

	gpt4Count, err := counter.CountTokens(context.Background(), text, "gpt-4")
	require.NoError(t, err)

	// Both should be reasonable
	assert.Greater(t, gpt4oCount, 0)
	assert.Greater(t, gpt4Count, 0)

	// They may or may not be different depending on the text
	// Just verify they work
	t.Logf("gpt-4o count: %d, gpt-4 count: %d", gpt4oCount, gpt4Count)
}

// TestAnthropicCounter_SupportsModel verifies model detection
func TestAnthropicCounter_SupportsModel(t *testing.T) {
	counter := NewAnthropicCounter("")

	tests := []struct {
		model    string
		expected bool
	}{
		{"claude-3-opus", true},
		{"claude-sonnet-4.5", true},
		{"claude-haiku-4-5", true},
		{"gpt-4", false},
		{"o1-preview", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.expected, counter.SupportsModel(tt.model))
		})
	}
}

func TestAnthropicCounter_NoAPIKey(t *testing.T) {
	counter := NewAnthropicCounter("")

	_, err := counter.CountTokens(context.Background(), "hello", "claude-3-opus")
	assert.ErrorIs(t, err, ErrNoAPIKey)
}

// mockCounter for testing registry fallback
type mockCounter struct {
	supportsFunc func(model string) bool
	countFunc    func(ctx context.Context, text string, model string) (int, error)
}

func (m *mockCounter) SupportsModel(model string) bool {
	return m.supportsFunc(model)
}

func (m *mockCounter) CountTokens(ctx context.Context, text string, model string) (int, error) {
	return m.countFunc(ctx, text, model)
}

// TestRegistryFallback verifies fallback on unknown model
func TestRegistryFallback(t *testing.T) {
	mockOpenAI := &mockCounter{
		supportsFunc: func(model string) bool { return model == "gpt-4" },
		countFunc: func(ctx context.Context, text string, model string) (int, error) {
			return 42, nil
		},
	}

	estimator := NewEstimationCounter()
	registry := NewRegistry([]TokenCounter{mockOpenAI}, estimator)

	// Known model should use mock
	count, err := registry.CountTokens(context.Background(), "hello", "gpt-4")
	assert.NoError(t, err)
	assert.Equal(t, 42, count)

	// Unknown model should fall back to estimator
	count, err = registry.CountTokens(context.Background(), "hello world", "unknown-model")
	assert.NoError(t, err)
	assert.Equal(t, 3, count) // "hello world" = 11 chars -> (11+3)/4 = 3
}

// TestRegistryErrorFallback verifies fallback on counter error
func TestRegistryErrorFallback(t *testing.T) {
	mockOpenAI := &mockCounter{
		supportsFunc: func(model string) bool { return model == "gpt-4" },
		countFunc: func(ctx context.Context, text string, model string) (int, error) {
			return 0, errors.New("mock error")
		},
	}

	estimator := NewEstimationCounter()
	registry := NewRegistry([]TokenCounter{mockOpenAI}, estimator)

	// Even for known model, if error occurs, fall back to estimator
	count, err := registry.CountTokens(context.Background(), "hello world", "gpt-4")
	assert.NoError(t, err)
	assert.Equal(t, 3, count) // Falls back to estimation
}

func TestRegistry_SupportsAllModels(t *testing.T) {
	registry := NewRegistry([]TokenCounter{}, NewEstimationCounter())
	assert.True(t, registry.SupportsModel("anything"))
}

// TestBackgroundWorker verifies async job processing
func TestBackgroundWorker(t *testing.T) {
	counter := NewEstimationCounter()
	worker := NewBackgroundWorker(counter, 2)

	worker.Start()
	defer worker.Stop()

	// Submit jobs with result channels
	resultCh := make(chan CountResult, 5)

	jobs := []CountJob{
		{MessageID: "msg1", Content: "hello", Model: "test", ResultCh: resultCh},
		{MessageID: "msg2", Content: "hello world", Model: "test", ResultCh: resultCh},
		{MessageID: "msg3", Content: "a longer message here", Model: "test", ResultCh: resultCh},
	}

	for _, job := range jobs {
		worker.Submit(job)
	}

	// Collect results
	results := make(map[string]int)
	timeout := time.After(5 * time.Second)

	for i := 0; i < len(jobs); i++ {
		select {
		case result := <-resultCh:
			assert.NoError(t, result.Error)
			results[result.MessageID] = result.TokenCount
		case <-timeout:
			t.Fatal("timeout waiting for results")
		}
	}

	// Verify all jobs completed
	assert.Len(t, results, 3)
	assert.Equal(t, 2, results["msg1"])  // "hello" = 5 chars -> 2 tokens
	assert.Equal(t, 3, results["msg2"])  // "hello world" = 11 chars -> 3 tokens
	assert.Equal(t, 6, results["msg3"])  // "a longer message here" = 21 chars -> 6 tokens
}

func TestBackgroundWorker_SubmitAsync(t *testing.T) {
	counter := NewEstimationCounter()
	worker := NewBackgroundWorker(counter, 1)
	// Don't start the worker to fill the queue

	// Fill the queue (100 capacity)
	for i := 0; i < 100; i++ {
		ok := worker.SubmitAsync(CountJob{MessageID: "test"})
		assert.True(t, ok)
	}

	// Next one should fail (queue full)
	ok := worker.SubmitAsync(CountJob{MessageID: "overflow"})
	assert.False(t, ok)
}

func TestBackgroundWorker_ConcurrentProcessing(t *testing.T) {
	counter := NewEstimationCounter()
	worker := NewBackgroundWorker(counter, 4) // 4 workers

	worker.Start()
	defer worker.Stop()

	// Submit many jobs concurrently
	numJobs := 100
	resultCh := make(chan CountResult, numJobs)
	var wg sync.WaitGroup

	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			worker.Submit(CountJob{
				MessageID: "msg",
				Content:   "test message",
				Model:     "test",
				ResultCh:  resultCh,
			})
		}(i)
	}

	wg.Wait()

	// Collect all results
	collected := 0
	timeout := time.After(10 * time.Second)
	for collected < numJobs {
		select {
		case result := <-resultCh:
			assert.NoError(t, result.Error)
			collected++
		case <-timeout:
			t.Fatalf("timeout: only collected %d of %d results", collected, numJobs)
		}
	}

	assert.Equal(t, numJobs, collected)
}

func TestBackgroundWorker_QueueLength(t *testing.T) {
	counter := NewEstimationCounter()
	worker := NewBackgroundWorker(counter, 1)
	// Don't start worker

	assert.Equal(t, 0, worker.QueueLength())

	worker.SubmitAsync(CountJob{MessageID: "1"})
	assert.Equal(t, 1, worker.QueueLength())

	worker.SubmitAsync(CountJob{MessageID: "2"})
	assert.Equal(t, 2, worker.QueueLength())
}

func TestBackgroundWorker_DoubleStart(t *testing.T) {
	counter := NewEstimationCounter()
	worker := NewBackgroundWorker(counter, 2)

	worker.Start()
	worker.Start() // Should be no-op
	worker.Stop()

	// Should not panic or error
}

func TestBackgroundWorker_StopWithoutStart(t *testing.T) {
	counter := NewEstimationCounter()
	worker := NewBackgroundWorker(counter, 2)

	worker.Stop() // Should be no-op
}
