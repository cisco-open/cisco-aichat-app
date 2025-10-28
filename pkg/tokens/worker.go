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
	"sync"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

// CountJob represents a token counting job for the background worker
type CountJob struct {
	MessageID string
	Content   string
	Model     string
	ResultCh  chan CountResult
}

// CountResult contains the result of a token counting job
type CountResult struct {
	MessageID  string
	TokenCount int
	Error      error
}

// BackgroundWorker processes token counting jobs asynchronously
type BackgroundWorker struct {
	counter    TokenCounter
	jobs       chan CountJob
	numWorkers int
	stopCh     chan struct{}
	wg         sync.WaitGroup
	logger     log.Logger
	started    bool
	mu         sync.Mutex
}

// NewBackgroundWorker creates a new background worker for token counting
// counter is the TokenCounter to use for counting (typically a Registry)
// numWorkers is the number of concurrent workers (goroutines)
func NewBackgroundWorker(counter TokenCounter, numWorkers int) *BackgroundWorker {
	if numWorkers < 1 {
		numWorkers = 1
	}
	return &BackgroundWorker{
		counter:    counter,
		jobs:       make(chan CountJob, 100), // buffered channel
		numWorkers: numWorkers,
		stopCh:     make(chan struct{}),
		logger:     log.DefaultLogger,
	}
}

// Start spawns worker goroutines to process jobs
func (w *BackgroundWorker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return
	}
	w.started = true

	for i := 0; i < w.numWorkers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}

	w.logger.Debug("Background token worker started", "workers", w.numWorkers)
}

// worker is the main loop for a single worker goroutine
func (w *BackgroundWorker) worker(id int) {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopCh:
			w.logger.Debug("Worker stopping", "worker_id", id)
			return
		case job, ok := <-w.jobs:
			if !ok {
				// Channel closed
				return
			}
			w.processJob(job)
		}
	}
}

// processJob handles a single token counting job
func (w *BackgroundWorker) processJob(job CountJob) {
	ctx := context.Background()
	count, err := w.counter.CountTokens(ctx, job.Content, job.Model)

	result := CountResult{
		MessageID:  job.MessageID,
		TokenCount: count,
		Error:      err,
	}

	// Send result if channel provided
	if job.ResultCh != nil {
		select {
		case job.ResultCh <- result:
		default:
			// Result channel full or closed, log and continue
			w.logger.Warn("Could not send token count result",
				"message_id", job.MessageID,
				"error", "result channel blocked or closed",
			)
		}
	}
}

// Submit adds a job to the work queue
// If the queue is full, this will block until space is available
func (w *BackgroundWorker) Submit(job CountJob) {
	w.jobs <- job
}

// SubmitAsync adds a job to the work queue without blocking
// Returns false if the queue is full
func (w *BackgroundWorker) SubmitAsync(job CountJob) bool {
	select {
	case w.jobs <- job:
		return true
	default:
		return false
	}
}

// Stop signals all workers to stop and waits for them to finish
func (w *BackgroundWorker) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()

	w.logger.Debug("Background token worker stopped")
}

// QueueLength returns the current number of pending jobs
func (w *BackgroundWorker) QueueLength() int {
	return len(w.jobs)
}
