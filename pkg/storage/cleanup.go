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

package storage

import (
	"context"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

const (
	// DefaultRetentionDays is the default number of days to retain sessions
	DefaultRetentionDays = 30

	// DefaultCleanupInterval is the default interval between cleanup runs
	DefaultCleanupInterval = 24 * time.Hour

	// CleanupTimeout is the maximum time allowed for a cleanup operation
	CleanupTimeout = 5 * time.Minute
)

// CleanupScheduler manages background cleanup of expired sessions
type CleanupScheduler struct {
	storage       Storage
	retentionDays int
	interval      time.Duration
	ticker        *time.Ticker
	stop          chan struct{}
	stopped       chan struct{}
	logger        log.Logger
}

// NewCleanupScheduler creates a new cleanup scheduler
func NewCleanupScheduler(storage Storage, retentionDays int, logger log.Logger) *CleanupScheduler {
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}

	return &CleanupScheduler{
		storage:       storage,
		retentionDays: retentionDays,
		interval:      DefaultCleanupInterval,
		stop:          make(chan struct{}),
		stopped:       make(chan struct{}),
		logger:        logger,
	}
}

// Start begins the background cleanup scheduler
// It runs cleanup immediately on start and then every 24 hours
func (cs *CleanupScheduler) Start() {
	cs.logger.Info("Starting cleanup scheduler",
		"retentionDays", cs.retentionDays,
		"interval", cs.interval.String())

	// Run initial cleanup
	go func() {
		cs.runCleanup()
	}()

	// Start periodic cleanup
	cs.ticker = time.NewTicker(cs.interval)

	go func() {
		defer close(cs.stopped)
		for {
			select {
			case <-cs.ticker.C:
				cs.runCleanup()
			case <-cs.stop:
				cs.logger.Info("Cleanup scheduler stopped")
				return
			}
		}
	}()
}

// Stop stops the cleanup scheduler
func (cs *CleanupScheduler) Stop() {
	cs.logger.Info("Stopping cleanup scheduler")

	if cs.ticker != nil {
		cs.ticker.Stop()
	}

	close(cs.stop)

	// Wait for goroutine to finish
	select {
	case <-cs.stopped:
		// Goroutine finished
	case <-time.After(10 * time.Second):
		cs.logger.Warn("Cleanup scheduler did not stop in time")
	}
}

// runCleanup executes a single cleanup run
func (cs *CleanupScheduler) runCleanup() {
	start := time.Now()
	cs.logger.Debug("Running session cleanup", "retentionDays", cs.retentionDays)

	ctx, cancel := context.WithTimeout(context.Background(), CleanupTimeout)
	defer cancel()

	deleted, err := cs.storage.DeleteExpiredSessions(ctx, cs.retentionDays)
	if err != nil {
		cs.logger.Error("Session cleanup failed", "error", err)
		return
	}

	duration := time.Since(start)
	if deleted > 0 {
		cs.logger.Info("Session cleanup completed",
			"deleted", deleted,
			"duration", duration.String())
	} else {
		cs.logger.Debug("Session cleanup completed - no expired sessions",
			"duration", duration.String())
	}
}

// SetInterval allows customizing the cleanup interval (for testing)
func (cs *CleanupScheduler) SetInterval(interval time.Duration) {
	cs.interval = interval
}
