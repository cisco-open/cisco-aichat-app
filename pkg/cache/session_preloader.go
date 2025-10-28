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

// Package cache provides in-memory caching for chat data.
package cache

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/grafana-aichat-app/pkg/storage"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

const (
	// DefaultMaxSessions is the default number of sessions to pre-load
	DefaultMaxSessions = 10

	// DefaultPreloadRefreshInterval is the default interval between pre-load refreshes
	DefaultPreloadRefreshInterval = 5 * time.Minute

	// PreloadPageSize is the number of messages to pre-load per session
	PreloadPageSize = 50

	// PreloadTimeout is the maximum time for a pre-load operation
	PreloadTimeout = 30 * time.Second
)

// MessageFetcher defines the interface for fetching session messages.
// This allows SessionPreloader to work with any storage implementation.
type MessageFetcher interface {
	GetSessionMessages(ctx context.Context, userID string, params storage.GetSessionMessagesParams) (*storage.MessagesPage, error)
}

// sessionAccess tracks access information for a session
type sessionAccess struct {
	UserID     string
	SessionID  string
	AccessTime time.Time
}

// accessKey generates a key for the access map
func accessKey(userID, sessionID string) string {
	return userID + ":" + sessionID
}

// SessionPreloader tracks session access patterns and pre-loads recent sessions
// into the cache for improved performance (PERF-03).
type SessionPreloader struct {
	storage         MessageFetcher
	cache           *MessageCache
	accessTimes     map[string]sessionAccess // key: "userID:sessionID"
	mu              sync.RWMutex
	maxSessions     int
	refreshInterval time.Duration
	cancel          context.CancelFunc
	done            chan struct{}
	logger          log.Logger
	lastPreloadTime time.Time
	preloadCount    atomic.Int64
}

// PreloaderOption configures the SessionPreloader
type PreloaderOption func(*SessionPreloader)

// WithMaxSessions sets the maximum number of sessions to pre-load
func WithMaxSessions(n int) PreloaderOption {
	return func(p *SessionPreloader) {
		if n > 0 {
			p.maxSessions = n
		}
	}
}

// WithRefreshInterval sets the interval between pre-load refreshes
func WithRefreshInterval(d time.Duration) PreloaderOption {
	return func(p *SessionPreloader) {
		if d > 0 {
			p.refreshInterval = d
		}
	}
}

// WithPreloaderLogger sets the logger for the preloader
func WithPreloaderLogger(l log.Logger) PreloaderOption {
	return func(p *SessionPreloader) {
		if l != nil {
			p.logger = l
		}
	}
}

// NewSessionPreloader creates a new SessionPreloader
func NewSessionPreloader(storage MessageFetcher, cache *MessageCache, opts ...PreloaderOption) *SessionPreloader {
	p := &SessionPreloader{
		storage:         storage,
		cache:           cache,
		accessTimes:     make(map[string]sessionAccess),
		maxSessions:     DefaultMaxSessions,
		refreshInterval: DefaultPreloadRefreshInterval,
		done:            make(chan struct{}),
		logger:          log.DefaultLogger,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// TrackAccess records that a session was accessed.
// This is called from API handlers to track session popularity.
func (p *SessionPreloader) TrackAccess(userID, sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := accessKey(userID, sessionID)
	p.accessTimes[key] = sessionAccess{
		UserID:     userID,
		SessionID:  sessionID,
		AccessTime: time.Now(),
	}
}

// GetRecentSessions returns the most recently accessed sessions, sorted by access time.
// Limited to maxSessions.
func (p *SessionPreloader) GetRecentSessions() []sessionAccess {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Convert map to slice
	sessions := make([]sessionAccess, 0, len(p.accessTimes))
	for _, access := range p.accessTimes {
		sessions = append(sessions, access)
	}

	// Sort by access time, newest first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].AccessTime.After(sessions[j].AccessTime)
	})

	// Limit to maxSessions
	if len(sessions) > p.maxSessions {
		sessions = sessions[:p.maxSessions]
	}

	return sessions
}

// PreloadRecentSessions fetches and caches messages for recent sessions.
// Called on startup and periodically.
func (p *SessionPreloader) PreloadRecentSessions(ctx context.Context) {
	sessions := p.GetRecentSessions()
	if len(sessions) == 0 {
		p.logger.Debug("No sessions to pre-load")
		return
	}

	p.logger.Info("Pre-loading recent sessions", "count", len(sessions))

	loaded := 0
	for _, access := range sessions {
		select {
		case <-ctx.Done():
			p.logger.Debug("Pre-load cancelled", "loaded", loaded)
			return
		default:
		}

		// Create timeout context for individual session
		sessionCtx, cancel := context.WithTimeout(ctx, PreloadTimeout)

		// Fetch first page of messages
		params := storage.GetSessionMessagesParams{
			SessionID: access.SessionID,
			Limit:     PreloadPageSize,
		}

		page, err := p.storage.GetSessionMessages(sessionCtx, access.UserID, params)
		cancel()

		if err != nil {
			p.logger.Debug("Failed to pre-load session",
				"sessionID", access.SessionID,
				"error", err)
			continue
		}

		if len(page.Messages) == 0 {
			continue
		}

		// Cache the messages
		p.cache.Set(access.SessionID, page.Messages)
		loaded++

		p.logger.Debug("Pre-loaded session",
			"sessionID", access.SessionID,
			"messages", len(page.Messages))
	}

	p.mu.Lock()
	p.lastPreloadTime = time.Now()
	p.mu.Unlock()

	p.preloadCount.Add(1)

	p.logger.Info("Pre-load complete", "loaded", loaded, "total", len(sessions))
}

// Start launches the background pre-load goroutine.
// It runs PreloadRecentSessions on startup and periodically.
func (p *SessionPreloader) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	p.logger.Info("Starting session preloader",
		"maxSessions", p.maxSessions,
		"refreshInterval", p.refreshInterval.String())

	// Run initial pre-load
	go func() {
		p.PreloadRecentSessions(ctx)
	}()

	// Start periodic refresh
	ticker := time.NewTicker(p.refreshInterval)

	go func() {
		defer close(p.done)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.PreloadRecentSessions(ctx)
			case <-ctx.Done():
				p.logger.Info("Session preloader stopped")
				return
			}
		}
	}()
}

// Stop gracefully shuts down the preloader.
func (p *SessionPreloader) Stop() {
	p.logger.Info("Stopping session preloader")

	if p.cancel != nil {
		p.cancel()
	}

	// Wait for goroutine to finish
	select {
	case <-p.done:
		// Goroutine finished
	case <-time.After(10 * time.Second):
		p.logger.Warn("Session preloader did not stop in time")
	}
}

// PreloaderStats contains statistics for monitoring
type PreloaderStats struct {
	TrackedSessions int       `json:"trackedSessions"`
	LastPreloadTime time.Time `json:"lastPreloadTime"`
	PreloadCount    int64     `json:"preloadCount"`
}

// Stats returns preloader statistics for monitoring
func (p *SessionPreloader) Stats() PreloaderStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PreloaderStats{
		TrackedSessions: len(p.accessTimes),
		LastPreloadTime: p.lastPreloadTime,
		PreloadCount:    p.preloadCount.Load(),
	}
}
