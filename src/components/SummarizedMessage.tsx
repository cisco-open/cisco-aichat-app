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

import React, { useState } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Icon, useStyles2 } from '@grafana/ui';

interface SummarizedMessageProps {
  summaryText: string;
  summarizedCount: number;  // Number of messages this summary represents
  timestamp: number;
  summaryDepth?: number;    // 1 = first summary, 2+ = meta-summary
}

/**
 * Formats a timestamp as relative time (e.g., "2h ago", "3d ago")
 */
function formatRelativeTime(timestamp: number): string {
  const now = Date.now();
  const diff = now - timestamp;

  const seconds = Math.floor(diff / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (days > 0) {
    return `${days}d ago`;
  }
  if (hours > 0) {
    return `${hours}h ago`;
  }
  if (minutes > 0) {
    return `${minutes}m ago`;
  }
  return 'just now';
}

/**
 * SummarizedMessage Component
 *
 * Displays a summarized message in a collapsed/expandable format.
 * Used to show summaries of older messages in the chat history.
 *
 * Features:
 * - Collapsed by default with dimmed appearance
 * - Expandable to show full summary text
 * - Shows count of messages summarized
 * - Shows relative timestamp
 * - Visual distinction from regular messages
 */
export function SummarizedMessage({
  summaryText,
  summarizedCount,
  timestamp,
  summaryDepth = 1,
}: SummarizedMessageProps) {
  const [isExpanded, setIsExpanded] = useState(false);
  const s = useStyles2(getStyles);

  const handleToggle = () => {
    setIsExpanded(!isExpanded);
  };

  const label = summaryDepth > 1
    ? `Meta-summary of ${summarizedCount} summaries`
    : `Summary of ${summarizedCount} messages`;

  return (
    <div className={s.summaryWrapper}>
      <button
        className={s.summaryHeader}
        onClick={handleToggle}
        type="button"
        aria-expanded={isExpanded}
        aria-controls="summary-content"
      >
        <Icon name={isExpanded ? 'angle-down' : 'angle-right'} />
        <span className={s.summaryLabel}>{label}</span>
        <span className={s.summaryTime}>{formatRelativeTime(timestamp)}</span>
      </button>
      {isExpanded && (
        <div id="summary-content" className={s.summaryContent}>
          {summaryText}
        </div>
      )}
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => ({
  summaryWrapper: css`
    background: ${theme.colors.background.secondary};
    border-left: 3px solid ${theme.colors.info.main};
    opacity: 0.8;
    margin: ${theme.spacing(1)} 0;
    border-radius: ${theme.shape.radius.default};
  `,
  summaryHeader: css`
    display: flex;
    align-items: center;
    gap: ${theme.spacing(1)};
    width: 100%;
    padding: ${theme.spacing(1)};
    background: transparent;
    border: none;
    cursor: pointer;
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.size.sm};
    &:hover {
      background: ${theme.colors.action.hover};
    }
  `,
  summaryLabel: css`
    font-style: italic;
  `,
  summaryTime: css`
    margin-left: auto;
    font-size: ${theme.typography.size.xs};
  `,
  summaryContent: css`
    padding: ${theme.spacing(1)} ${theme.spacing(2)};
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.size.sm};
    line-height: 1.5;
    border-top: 1px solid ${theme.colors.border.weak};
  `,
});
