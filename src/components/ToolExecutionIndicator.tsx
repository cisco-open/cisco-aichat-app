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
import { css, keyframes } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { useStyles2 } from '@grafana/ui';

/**
 * Status of a tool execution
 */
export type ToolExecutionStatus = 'pending' | 'executing' | 'success' | 'error';

/**
 * Information about a tool being executed
 */
export interface ToolExecutionInfo {
  id: string;
  toolName: string;
  status: ToolExecutionStatus;
  startTime: number;
  endTime?: number;
  errorMessage?: string;
}

interface ToolExecutionIndicatorProps {
  executions: ToolExecutionInfo[];
  onDismiss?: (id: string) => void;
}

/**
 * Component to display the status of MCP tool executions
 * Shows as a simple inline text line above the input, similar to "Thinking..."
 * Only shows while tool is executing, disappears immediately when done
 */
export function ToolExecutionIndicator({ executions, onDismiss }: ToolExecutionIndicatorProps) {
  const s = useStyles2(getStyles);

  // Only show pending/executing states
  const activeExecutions = executions.filter(e => e.status === 'pending' || e.status === 'executing');

  if (activeExecutions.length === 0) {
    return null;
  }

  // Show the most recent active execution
  const currentExecution = activeExecutions[activeExecutions.length - 1];

  return (
    <div className={s.container}>
      <div className={s.spinner}>
        <div className={s.spinnerCircle} />
      </div>
      <span className={s.text}>
        {currentExecution.status === 'pending' && `Queued: ${currentExecution.toolName}...`}
        {currentExecution.status === 'executing' && `Running ${currentExecution.toolName}...`}
      </span>
    </div>
  );
}

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
    display: inline-flex;
    align-items: center;
    gap: ${theme.spacing(0.5)};
    padding: ${theme.spacing(0.5)} 0;
    margin: 0;
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
  `,

  spinner: css`
    width: 10px;
    height: 10px;
    flex-shrink: 0;
  `,

  spinnerCircle: css`
    width: 100%;
    height: 100%;
    border: 1.5px solid ${theme.colors.border.weak};
    border-top-color: ${theme.colors.primary.main};
    border-radius: 50%;
    animation: ${spin} 0.8s linear infinite;
  `,

  text: css`
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
    line-height: 1;
  `,
});
