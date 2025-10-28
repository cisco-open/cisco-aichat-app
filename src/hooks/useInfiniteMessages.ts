/**
 * Copyright 2025 Cisco Systems, Inc. and its affiliates
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

import { useState, useCallback, useRef, useEffect } from 'react';
import { ChatMessage } from '../types/chat';
import { ChatBackendService } from '../services/ChatBackendService';

export interface UseInfiniteMessagesResult {
  /** Loaded messages in chronological order (oldest first) */
  messages: ChatMessage[];
  /** True during initial load */
  isInitialLoading: boolean;
  /** True while loading more (older) messages */
  isLoadingMore: boolean;
  /** True if there are more (older) messages to load */
  hasMore: boolean;
  /** Error message if loading failed after retries */
  error: string | null;
  /** Load more (older) messages */
  loadMore: () => Promise<void>;
  /** Refresh messages (clear and reload) */
  refresh: () => Promise<void>;
  /** Append a new message to the list (for optimistic updates) */
  appendMessage: (message: ChatMessage) => void;
  /** Update a message in the list (for streaming updates) */
  updateMessage: (messageId: string, updates: Partial<ChatMessage>) => void;
}

const INITIAL_LIMIT = 50; // Per CONTEXT.md: Initial page size
const LOAD_MORE_LIMIT = 25; // Per CONTEXT.md: Smaller batches for infinite scroll
const MAX_RETRIES = 3;

/**
 * Hook for infinite scroll message loading with retry logic
 *
 * Features:
 * - Initial load of 50 most recent messages (PERF-07)
 * - Infinite scroll loads 25 older messages at a time (PERF-08)
 * - 2-3 silent retries before showing error (PERF-09)
 * - Optimistic message append/update for real-time feel
 *
 * @param sessionId - Current session ID to load messages from
 */
export function useInfiniteMessages(sessionId: string | undefined): UseInfiniteMessagesResult {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [isInitialLoading, setIsInitialLoading] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const cursorRef = useRef<string | null>(null);
  const sessionIdRef = useRef<string | undefined>(undefined);

  // Helper for retries with exponential backoff
  const fetchWithRetry = useCallback(async <T>(
    fn: () => Promise<T>,
    retries = MAX_RETRIES
  ): Promise<T> => {
    let lastError: Error | null = null;
    for (let i = 0; i < retries; i++) {
      try {
        return await fn();
      } catch (err) {
        lastError = err as Error;
        if (i < retries - 1) {
          // Exponential backoff: 500ms, 1000ms, 2000ms
          await new Promise(r => setTimeout(r, Math.pow(2, i) * 500));
        }
      }
    }
    throw lastError;
  }, []);

  // Load initial messages when sessionId changes
  useEffect(() => {
    // Track session changes to avoid stale updates
    sessionIdRef.current = sessionId;

    if (!sessionId) {
      setMessages([]);
      setHasMore(false);
      setError(null);
      cursorRef.current = null;
      return;
    }

    const loadInitial = async () => {
      setIsInitialLoading(true);
      setError(null);
      setMessages([]);
      cursorRef.current = null;

      try {
        const backend = ChatBackendService.getInstance();
        const page = await fetchWithRetry(() =>
          backend.getSessionMessagesPage(sessionId, INITIAL_LIMIT)
        );

        // Only update state if session hasn't changed during fetch
        if (sessionIdRef.current === sessionId) {
          setMessages(page.messages);
          setHasMore(page.hasMore);
          cursorRef.current = page.nextCursor;
        }
      } catch (err) {
        if (sessionIdRef.current === sessionId) {
          setError('Failed to load messages. Please try again.');
          console.error('Failed to load messages:', err);
        }
      } finally {
        if (sessionIdRef.current === sessionId) {
          setIsInitialLoading(false);
        }
      }
    };

    loadInitial();
  }, [sessionId, fetchWithRetry]);

  // Load more (older) messages
  const loadMore = useCallback(async () => {
    if (!sessionId || isLoadingMore || !hasMore) {
      return;
    }

    setIsLoadingMore(true);
    try {
      const backend = ChatBackendService.getInstance();
      const page = await fetchWithRetry(() =>
        backend.getSessionMessagesPage(
          sessionId,
          LOAD_MORE_LIMIT,
          cursorRef.current || undefined
        )
      );

      // Only update if session hasn't changed
      if (sessionIdRef.current === sessionId) {
        // Prepend older messages (they go at the top/beginning)
        setMessages(prev => [...page.messages, ...prev]);
        setHasMore(page.hasMore);
        cursorRef.current = page.nextCursor;
      }
    } catch (err) {
      if (sessionIdRef.current === sessionId) {
        setError('Failed to load older messages.');
        console.error('Failed to load more messages:', err);
      }
    } finally {
      if (sessionIdRef.current === sessionId) {
        setIsLoadingMore(false);
      }
    }
  }, [sessionId, isLoadingMore, hasMore, fetchWithRetry]);

  // Refresh messages (clear and reload) - triggers effect
  const refresh = useCallback(async () => {
    cursorRef.current = null;
    setMessages([]);
    setHasMore(true);
    setError(null);

    if (!sessionId) {
      return;
    }

    setIsInitialLoading(true);
    try {
      const backend = ChatBackendService.getInstance();
      const page = await fetchWithRetry(() =>
        backend.getSessionMessagesPage(sessionId, INITIAL_LIMIT)
      );

      if (sessionIdRef.current === sessionId) {
        setMessages(page.messages);
        setHasMore(page.hasMore);
        cursorRef.current = page.nextCursor;
      }
    } catch (err) {
      if (sessionIdRef.current === sessionId) {
        setError('Failed to refresh messages.');
        console.error('Failed to refresh messages:', err);
      }
    } finally {
      if (sessionIdRef.current === sessionId) {
        setIsInitialLoading(false);
      }
    }
  }, [sessionId, fetchWithRetry]);

  // Append a new message (for optimistic updates when user sends)
  const appendMessage = useCallback((message: ChatMessage) => {
    setMessages(prev => [...prev, message]);
  }, []);

  // Update a message in the list (for streaming content updates)
  const updateMessage = useCallback((messageId: string, updates: Partial<ChatMessage>) => {
    setMessages(prev => prev.map(msg =>
      msg.id === messageId ? { ...msg, ...updates } : msg
    ));
  }, []);

  return {
    messages,
    isInitialLoading,
    isLoadingMore,
    hasMore,
    error,
    loadMore,
    refresh,
    appendMessage,
    updateMessage,
  };
}
