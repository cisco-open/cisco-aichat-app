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

import React from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Icon, Spinner, useStyles2 } from '@grafana/ui';
import { SearchResult } from '../types/chat';

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  `,
  resultsList: css`
    flex: 1;
    overflow-y: auto;
    padding: ${theme.spacing(1)};
  `,
  resultItem: css`
    display: flex;
    flex-direction: column;
    padding: ${theme.spacing(1.5)};
    margin-bottom: ${theme.spacing(1)};
    border-radius: ${theme.shape.borderRadius()};
    background: ${theme.colors.background.secondary};
    border: 1px solid transparent;
    cursor: pointer;
    transition: all 0.2s;

    &:hover {
      background: ${theme.colors.action.hover};
      border-color: ${theme.colors.border.medium};
    }

    &:last-child {
      margin-bottom: 0;
    }
  `,
  sessionName: css`
    font-size: ${theme.typography.size.xs};
    color: ${theme.colors.text.secondary};
    margin-bottom: ${theme.spacing(0.5)};
    display: flex;
    align-items: center;
    gap: ${theme.spacing(0.5)};
  `,
  roleIcon: css`
    color: ${theme.colors.text.disabled};
  `,
  contentSnippet: css`
    font-size: ${theme.typography.size.sm};
    color: ${theme.colors.text.primary};
    line-height: 1.4;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    margin-bottom: ${theme.spacing(0.5)};

    mark {
      background: ${theme.colors.warning.main}40;
      color: ${theme.colors.text.primary};
      padding: 0 2px;
      border-radius: 2px;
    }
  `,
  timestamp: css`
    font-size: ${theme.typography.size.xs};
    color: ${theme.colors.text.disabled};
    display: flex;
    align-items: center;
    gap: ${theme.spacing(0.5)};
  `,
  emptyState: css`
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: ${theme.spacing(4)};
    text-align: center;
    color: ${theme.colors.text.secondary};
    height: 100%;
  `,
  emptyIcon: css`
    margin-bottom: ${theme.spacing(1)};
    color: ${theme.colors.text.disabled};
  `,
  emptyText: css`
    font-size: ${theme.typography.size.sm};
    margin-bottom: ${theme.spacing(0.5)};
  `,
  emptyHint: css`
    font-size: ${theme.typography.size.xs};
    color: ${theme.colors.text.disabled};
  `,
  loadingState: css`
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: ${theme.spacing(4)};
    height: 100%;
    gap: ${theme.spacing(1)};
  `,
  loadingText: css`
    font-size: ${theme.typography.size.sm};
    color: ${theme.colors.text.secondary};
  `,
  resultsHeader: css`
    padding: ${theme.spacing(1)} ${theme.spacing(1.5)};
    font-size: ${theme.typography.size.xs};
    color: ${theme.colors.text.secondary};
    border-bottom: 1px solid ${theme.colors.border.weak};
  `
});

interface SearchResultsListProps {
  results: SearchResult[];
  isLoading: boolean;
  onResultClick: (sessionId: string, messageId: string) => void;
}

/**
 * Safely render backend-highlighted snippets.
 * Only <mark> tags are interpreted; everything else renders as plain text.
 */
function renderHighlightedSnippet(snippet: string): React.ReactNode {
  const tokens = snippet.split(/(<\/?mark>)/gi);
  let inMark = false;
  const nodes: React.ReactNode[] = [];

  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    const lower = token.toLowerCase();

    if (lower === '<mark>') {
      inMark = true;
      continue;
    }
    if (lower === '</mark>') {
      inMark = false;
      continue;
    }
    if (!token) {
      continue;
    }

    if (inMark) {
      nodes.push(<mark key={`mark-${i}`}>{token}</mark>);
    } else {
      nodes.push(<span key={`text-${i}`}>{token}</span>);
    }
  }

  return nodes;
}

/**
 * Format timestamp to relative time string
 */
function formatRelativeTime(timestamp: number): string {
  const now = Date.now();
  const diff = now - timestamp;
  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (days > 7) {
    return new Date(timestamp).toLocaleDateString([], {
      month: 'short',
      day: 'numeric'
    });
  } else if (days > 0) {
    return `${days}d ago`;
  } else if (hours > 0) {
    return `${hours}h ago`;
  } else if (minutes > 0) {
    return `${minutes}m ago`;
  } else {
    return 'Just now';
  }
}

/**
 * SearchResultsList displays full-text search results
 * with highlighted snippets and navigation to message
 */
export function SearchResultsList({
  results,
  isLoading,
  onResultClick
}: SearchResultsListProps) {
  const styles = useStyles2(getStyles);

  // Loading state
  if (isLoading) {
    return (
      <div className={styles.loadingState}>
        <Spinner size="lg" />
        <span className={styles.loadingText}>Searching messages...</span>
      </div>
    );
  }

  // Empty state
  if (results.length === 0) {
    return (
      <div className={styles.emptyState}>
        <Icon name="search" size="xxl" className={styles.emptyIcon} />
        <div className={styles.emptyText}>No messages found</div>
        <div className={styles.emptyHint}>Try a different search term</div>
      </div>
    );
  }

  // Results list
  return (
    <div className={styles.container}>
      <div className={styles.resultsHeader}>
        {results.length} result{results.length !== 1 ? 's' : ''} found
      </div>
      <div className={styles.resultsList}>
        {results.map((result) => (
          <button
            key={`${result.sessionId}-${result.messageId}`}
            className={styles.resultItem}
            onClick={() => onResultClick(result.sessionId, result.messageId)}
            type="button"
          >
            <div className={styles.sessionName}>
              <Icon
                name={result.role === 'user' ? 'user' : 'message'}
                size="xs"
                className={styles.roleIcon}
              />
              {result.sessionName}
            </div>
            <div className={styles.contentSnippet}>{renderHighlightedSnippet(result.content)}</div>
            <div className={styles.timestamp}>
              <Icon name="clock-nine" size="xs" />
              {formatRelativeTime(result.timestamp)}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

export default SearchResultsList;
