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

import React, { useCallback, useRef, useEffect } from 'react';
import InfiniteScroll from 'react-infinite-scroll-component';
import { Spinner, useStyles2 } from '@grafana/ui';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { ChatMessage as ChatMessageType } from '../types/chat';
import { ChatMessage } from './ChatMessage';

const getStyles = (theme: GrafanaTheme2) => ({
  scrollContainer: css`
    flex: 1;
    min-height: 0;
    overflow: auto;
    display: flex;
    flex-direction: column-reverse;
  `,
  scrollInner: css`
    display: flex;
    flex-direction: column-reverse;
    padding: ${theme.spacing(2)};
    gap: ${theme.spacing(2)};
  `,
  loader: css`
    text-align: center;
    padding: 8px;
  `,
  endMessage: css`
    text-align: center;
    opacity: 0.5;
    padding: 8px;
    font-size: 12px;
  `,
  error: css`
    text-align: center;
    color: ${theme.colors.error.text};
    padding: ${theme.spacing(2)};
  `,
});

interface InfiniteMessageListProps {
  /** Messages to display in chronological order */
  messages: ChatMessageType[];
  /** Whether there are more (older) messages to load */
  hasMore: boolean;
  /** Whether currently loading more messages */
  isLoadingMore: boolean;
  /** Function to load more (older) messages */
  onLoadMore: () => Promise<void>;
  /** Whether AI is currently streaming a response */
  isStreaming: boolean;
  /** Error message if loading failed */
  error?: string | null;
}

/**
 * InfiniteMessageList Component
 *
 * Renders messages with reverse infinite scroll for loading older messages.
 * Uses column-reverse CSS to maintain scroll position when prepending messages (PERF-10).
 *
 * Features:
 * - Reverse infinite scroll (scroll up to load older messages)
 * - Scroll position preserved via column-reverse
 * - Spinner indicator while loading more
 * - "Beginning of conversation" message when all loaded
 */
export function InfiniteMessageList({
  messages,
  hasMore,
  isLoadingMore,
  onLoadMore,
  isStreaming,
  error,
}: InfiniteMessageListProps) {
  const styles = useStyles2(getStyles);
  const loadingRef = useRef(false);

  // Prevent double-trigger of loadMore (Pitfall 4 from research)
  const handleLoadMore = useCallback(async () => {
    if (loadingRef.current || isLoadingMore) {
      return;
    }
    loadingRef.current = true;
    try {
      await onLoadMore();
    } finally {
      loadingRef.current = false;
    }
  }, [onLoadMore, isLoadingMore]);

  // Reset loading ref when isLoadingMore changes
  useEffect(() => {
    if (!isLoadingMore) {
      loadingRef.current = false;
    }
  }, [isLoadingMore]);

  // For InfiniteScroll with inverse={true}, we need to reverse the message order
  // because the component renders from bottom to top
  const reversedMessages = [...messages].reverse();

  return (
    <div
      id="messagesScrollable"
      className={styles.scrollContainer}
    >
      <InfiniteScroll
        dataLength={reversedMessages.length}
        next={handleLoadMore}
        hasMore={hasMore && !isStreaming}
        loader={
          <div className={styles.loader}>
            <Spinner size="sm" />
          </div>
        }
        endMessage={
          messages.length > 0 && !hasMore ? (
            <div className={styles.endMessage}>
              Beginning of conversation
            </div>
          ) : null
        }
        className={styles.scrollInner}
        scrollableTarget="messagesScrollable"
        inverse={true}
      >
        {error && (
          <div className={styles.error}>{error}</div>
        )}
        {reversedMessages.map((message) => (
          <ChatMessage
            key={message.id}
            message={message}
          />
        ))}
      </InfiniteScroll>
    </div>
  );
}
