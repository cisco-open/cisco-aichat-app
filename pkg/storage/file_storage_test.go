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

func TestFileStorage_UpdateMessageTokenCount(t *testing.T) {
	store, err := NewFileStorage(t.TempDir())
	require.NoError(t, err)

	ctx := context.Background()
	userID := "user-1"
	sessionID := "session-1"
	now := time.Now().UnixMilli()

	err = store.CreateSession(ctx, userID, &ChatSession{
		ID:        sessionID,
		Name:      "Session",
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []ChatMessage{},
	})
	require.NoError(t, err)

	err = store.AddMessage(ctx, userID, sessionID, &ChatMessage{
		ID:        "msg-1",
		Role:      "assistant",
		Content:   "hello",
		Timestamp: now + 1,
	})
	require.NoError(t, err)

	err = store.UpdateMessageTokenCount(ctx, "msg-1", 42)
	require.NoError(t, err)

	session, err := store.GetSession(ctx, userID, sessionID)
	require.NoError(t, err)
	require.Len(t, session.Messages, 1)
	assert.Equal(t, 42, session.Messages[0].TokenCount)
}

func TestFileStorage_UpdateMessageTokenCount_NotFound(t *testing.T) {
	store, err := NewFileStorage(t.TempDir())
	require.NoError(t, err)

	err = store.UpdateMessageTokenCount(context.Background(), "missing-msg", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message not found")
}
