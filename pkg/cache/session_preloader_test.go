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

package cache

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/grafana-aichat-app/pkg/storage"
)

// mockMessageFetcher implements MessageFetcher for testing
type mockMessageFetcher struct {
	messages map[string][]storage.ChatMessage // key: sessionID
}

func newMockMessageFetcher() *mockMessageFetcher {
	return &mockMessageFetcher{
		messages: make(map[string][]storage.ChatMessage),
	}
}

func (m *mockMessageFetcher) GetSessionMessages(ctx context.Context, userID string, params storage.GetSessionMessagesParams) (*storage.MessagesPage, error) {
	msgs, ok := m.messages[params.SessionID]
	if !ok {
		return &storage.MessagesPage{Messages: []storage.ChatMessage{}}, nil
	}

	// Apply limit
	limit := params.Limit
	if limit == 0 || limit > len(msgs) {
		limit = len(msgs)
	}

	return &storage.MessagesPage{
		Messages: msgs[:limit],
		PageInfo: storage.PageInfo{
			HasNextPage: len(msgs) > limit,
			EndCursor:   "",
		},
	}, nil
}

func (m *mockMessageFetcher) addSession(sessionID string, messages []storage.ChatMessage) {
	m.messages[sessionID] = messages
}

func TestSessionPreloader_TrackAccess(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()
	preloader := NewSessionPreloader(fetcher, cache)

	// Track access for a session
	preloader.TrackAccess("user1", "session1")

	// Verify access was tracked
	sessions := preloader.GetRecentSessions()
	if len(sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(sessions))
	}

	if sessions[0].UserID != "user1" {
		t.Errorf("Expected userID 'user1', got '%s'", sessions[0].UserID)
	}
	if sessions[0].SessionID != "session1" {
		t.Errorf("Expected sessionID 'session1', got '%s'", sessions[0].SessionID)
	}

	// Track another session
	preloader.TrackAccess("user2", "session2")

	sessions = preloader.GetRecentSessions()
	if len(sessions) != 2 {
		t.Fatalf("Expected 2 sessions, got %d", len(sessions))
	}
}

func TestSessionPreloader_TrackAccessUpdateTime(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()
	preloader := NewSessionPreloader(fetcher, cache)

	// Track access
	preloader.TrackAccess("user1", "session1")
	time.Sleep(10 * time.Millisecond)

	// Track the same session again - should update time
	preloader.TrackAccess("user1", "session1")

	sessions := preloader.GetRecentSessions()
	if len(sessions) != 1 {
		t.Fatalf("Expected 1 session after re-access, got %d", len(sessions))
	}
}

func TestSessionPreloader_GetRecentSessionsSorted(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()
	preloader := NewSessionPreloader(fetcher, cache)

	// Track multiple sessions with delays
	preloader.TrackAccess("user1", "session1")
	time.Sleep(10 * time.Millisecond)
	preloader.TrackAccess("user2", "session2")
	time.Sleep(10 * time.Millisecond)
	preloader.TrackAccess("user3", "session3")

	sessions := preloader.GetRecentSessions()
	if len(sessions) != 3 {
		t.Fatalf("Expected 3 sessions, got %d", len(sessions))
	}

	// Should be sorted newest first
	if sessions[0].SessionID != "session3" {
		t.Errorf("Expected session3 first (newest), got %s", sessions[0].SessionID)
	}
	if sessions[1].SessionID != "session2" {
		t.Errorf("Expected session2 second, got %s", sessions[1].SessionID)
	}
	if sessions[2].SessionID != "session1" {
		t.Errorf("Expected session1 third (oldest), got %s", sessions[2].SessionID)
	}
}

func TestSessionPreloader_GetRecentSessionsLimited(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()
	preloader := NewSessionPreloader(fetcher, cache, WithMaxSessions(2))

	// Track more sessions than max
	preloader.TrackAccess("user1", "session1")
	time.Sleep(5 * time.Millisecond)
	preloader.TrackAccess("user2", "session2")
	time.Sleep(5 * time.Millisecond)
	preloader.TrackAccess("user3", "session3")

	sessions := preloader.GetRecentSessions()
	if len(sessions) != 2 {
		t.Fatalf("Expected 2 sessions (max limit), got %d", len(sessions))
	}

	// Should have the 2 most recent
	if sessions[0].SessionID != "session3" {
		t.Errorf("Expected session3 first, got %s", sessions[0].SessionID)
	}
	if sessions[1].SessionID != "session2" {
		t.Errorf("Expected session2 second, got %s", sessions[1].SessionID)
	}
}

func TestSessionPreloader_PreloadRecentSessions(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()

	// Add test data
	fetcher.addSession("session1", []storage.ChatMessage{
		{ID: "msg1", Content: "Hello", Role: "user"},
		{ID: "msg2", Content: "Hi there", Role: "assistant"},
	})
	fetcher.addSession("session2", []storage.ChatMessage{
		{ID: "msg3", Content: "Question", Role: "user"},
	})

	preloader := NewSessionPreloader(fetcher, cache)

	// Track sessions
	preloader.TrackAccess("user1", "session1")
	preloader.TrackAccess("user2", "session2")

	// Pre-load
	preloader.PreloadRecentSessions(context.Background())

	// Give ristretto time to process (eventually consistent)
	time.Sleep(50 * time.Millisecond)

	// Verify cache contains messages
	msgs1, ok := cache.Get("session1")
	if !ok {
		t.Error("Expected session1 to be cached")
	} else if len(msgs1) != 2 {
		t.Errorf("Expected 2 messages for session1, got %d", len(msgs1))
	}

	msgs2, ok := cache.Get("session2")
	if !ok {
		t.Error("Expected session2 to be cached")
	} else if len(msgs2) != 1 {
		t.Errorf("Expected 1 message for session2, got %d", len(msgs2))
	}
}

func TestSessionPreloader_PreloadSkipsEmptySessions(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()
	// session1 has no messages (not added to fetcher)

	preloader := NewSessionPreloader(fetcher, cache)

	// Track empty session
	preloader.TrackAccess("user1", "session1")

	// Pre-load should not error
	preloader.PreloadRecentSessions(context.Background())

	// Empty session should not be cached
	_, ok := cache.Get("session1")
	if ok {
		t.Error("Empty session should not be cached")
	}
}

func TestSessionPreloader_Stats(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()
	fetcher.addSession("session1", []storage.ChatMessage{
		{ID: "msg1", Content: "Test", Role: "user"},
	})

	preloader := NewSessionPreloader(fetcher, cache)

	// Initial stats
	stats := preloader.Stats()
	if stats.TrackedSessions != 0 {
		t.Errorf("Expected 0 tracked sessions initially, got %d", stats.TrackedSessions)
	}
	if stats.PreloadCount != 0 {
		t.Errorf("Expected 0 preload count initially, got %d", stats.PreloadCount)
	}

	// Track and preload
	preloader.TrackAccess("user1", "session1")
	preloader.PreloadRecentSessions(context.Background())

	stats = preloader.Stats()
	if stats.TrackedSessions != 1 {
		t.Errorf("Expected 1 tracked session, got %d", stats.TrackedSessions)
	}
	if stats.PreloadCount != 1 {
		t.Errorf("Expected 1 preload count, got %d", stats.PreloadCount)
	}
	if stats.LastPreloadTime.IsZero() {
		t.Error("Expected lastPreloadTime to be set")
	}
}

func TestSessionPreloader_StartStop(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()
	preloader := NewSessionPreloader(fetcher, cache,
		WithRefreshInterval(100*time.Millisecond))

	// Start
	preloader.Start()

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Stop
	preloader.Stop()

	// Should complete without hanging
}

func TestSessionPreloader_Options(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()

	preloader := NewSessionPreloader(fetcher, cache,
		WithMaxSessions(5),
		WithRefreshInterval(10*time.Minute),
	)

	if preloader.maxSessions != 5 {
		t.Errorf("Expected maxSessions 5, got %d", preloader.maxSessions)
	}
	if preloader.refreshInterval != 10*time.Minute {
		t.Errorf("Expected refreshInterval 10m, got %v", preloader.refreshInterval)
	}
}

func TestSessionPreloader_CancelledContext(t *testing.T) {
	cache, err := NewMessageCacheDefault()
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Close()

	fetcher := newMockMessageFetcher()
	fetcher.addSession("session1", []storage.ChatMessage{
		{ID: "msg1", Content: "Test", Role: "user"},
	})

	preloader := NewSessionPreloader(fetcher, cache)
	preloader.TrackAccess("user1", "session1")

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should not panic or hang
	preloader.PreloadRecentSessions(ctx)
}
