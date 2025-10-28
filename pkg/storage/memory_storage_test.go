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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStorage_AddMessage_Idempotent(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"
	now := time.Now().UnixMilli()

	err := store.CreateSession(ctx, userID, &ChatSession{
		ID:        sessionID,
		Name:      "Session",
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []ChatMessage{},
	})
	require.NoError(t, err)

	msg := &ChatMessage{
		ID:         "msg-1",
		Role:       "assistant",
		Content:    "hello",
		Timestamp:  now + 1,
		TokenCount: 10,
	}

	err = store.AddMessage(ctx, userID, sessionID, msg)
	require.NoError(t, err)

	// Duplicate add should be a no-op.
	err = store.AddMessage(ctx, userID, sessionID, msg)
	require.NoError(t, err)

	session, err := store.GetSession(ctx, userID, sessionID)
	require.NoError(t, err)
	require.Len(t, session.Messages, 1)
	assert.Equal(t, 10, session.TotalTokens)
}

func TestMemoryStorage_SaveMessages_Idempotent(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"
	now := time.Now().UnixMilli()

	err := store.CreateSession(ctx, userID, &ChatSession{
		ID:        sessionID,
		Name:      "Session",
		CreatedAt: now,
		UpdatedAt: now,
		Messages: []ChatMessage{
			{
				ID:         "msg-existing",
				Role:       "user",
				Content:    "existing",
				Timestamp:  now,
				TokenCount: 5,
			},
		},
		TotalTokens: 5,
	})
	require.NoError(t, err)

	results, err := store.SaveMessages(ctx, userID, sessionID, []ChatMessage{
		{
			ID:         "msg-existing",
			Role:       "user",
			Content:    "existing",
			Timestamp:  now,
			TokenCount: 5,
		},
		{
			ID:         "msg-new",
			Role:       "assistant",
			Content:    "new",
			Timestamp:  now + 1,
			TokenCount: 7,
		},
		{
			ID:         "msg-new",
			Role:       "assistant",
			Content:    "new",
			Timestamp:  now + 1,
			TokenCount: 7,
		},
	})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.True(t, results[0].Success)
	assert.True(t, results[1].Success)
	assert.True(t, results[2].Success)

	session, err := store.GetSession(ctx, userID, sessionID)
	require.NoError(t, err)
	require.Len(t, session.Messages, 2)
	assert.Equal(t, 12, session.TotalTokens)
}
