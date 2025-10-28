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
import { css, cx, keyframes } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Tooltip, useStyles2 } from '@grafana/ui';
import { CircularProgressbar, buildStyles } from 'react-circular-progressbar';
import 'react-circular-progressbar/dist/styles.css';

// Color constants
const COLORS = {
  normal: '#3274D9',      // Grafana blue
  warning: '#FF5630',     // Red for 90%+
  trail: '#2C3235'        // Dark background
};

const spin = keyframes`
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
`;

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    width: 18px;
    height: 18px;
    transition: transform 0.2s;

    &:hover {
      transform: scale(1.08);
    }
  `,
  clickable: css`
    cursor: pointer;
  `,
  spinning: css`
    animation: ${spin} 1s linear infinite;
  `
});

interface ContextUsageGaugeProps {
  usedTokens: number;
  maxTokens: number;
  onCompactClick?: () => void;
  isCompacting?: boolean;
}

export function ContextUsageGauge({
  usedTokens,
  maxTokens,
  onCompactClick,
  isCompacting = false
}: ContextUsageGaugeProps) {
  const s = useStyles2(getStyles);

  // Calculate percentage (handle edge cases)
  const percentage = maxTokens > 0 ? (usedTokens / maxTokens) * 100 : 0;
  const clampedPercentage = Math.min(percentage, 100);
  const distanceToCompaction = Math.max(0, 100 - percentage);

  // Determine states
  const isWarning = percentage >= 90;
  const canCompact = percentage >= 50 && onCompactClick !== undefined;
  const isClickable = canCompact && !isCompacting;

  // Determine path color
  const pathColor = isWarning ? COLORS.warning : COLORS.normal;

  // Build tooltip content
  const tooltipContent = (
    <div>
      <div>{usedTokens.toLocaleString()} / {maxTokens.toLocaleString()} tokens used</div>
      {!isCompacting && (
        <div style={{ marginTop: '4px' }}>
          {distanceToCompaction <= 0
            ? '0% away from auto-compaction'
            : `${distanceToCompaction.toFixed(1)}% away from auto-compaction`}
        </div>
      )}
      {canCompact && !isCompacting && (
        <div style={{ marginTop: '4px', fontStyle: 'italic' }}>
          Click to compact conversation
        </div>
      )}
      {isCompacting && (
        <div style={{ marginTop: '4px', fontStyle: 'italic' }}>
          Compacting...
        </div>
      )}
    </div>
  );

  const handleClick = () => {
    if (isClickable && onCompactClick) {
      onCompactClick();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if ((e.key === 'Enter' || e.key === ' ') && isClickable && onCompactClick) {
      e.preventDefault();
      onCompactClick();
    }
  };

  return (
    <Tooltip content={tooltipContent} placement="bottom">
      <div
        className={cx(
          s.container,
          isClickable && s.clickable,
          isCompacting && s.spinning
        )}
        onClick={handleClick}
        onKeyDown={handleKeyDown}
        role={isClickable ? 'button' : undefined}
        tabIndex={isClickable ? 0 : undefined}
        aria-label={`Context usage indicator${isClickable ? '. Click to compact.' : ''}`}
      >
        <CircularProgressbar
          value={clampedPercentage}
          styles={buildStyles({
            pathColor: pathColor,
            trailColor: COLORS.trail,
            pathTransitionDuration: 0.5,
            strokeLinecap: 'round'
          })}
        />
      </div>
    </Tooltip>
  );
}
