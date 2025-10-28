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
import { IconButton, Tooltip, useStyles2 } from '@grafana/ui';

export interface ChatHeaderProps {
  sessionName?: string;
  sessionId?: string;
  showSidebar: boolean;
  onShowSettings: () => void;
  onToggleSidebar: () => void;
}

const getStyles = (theme: GrafanaTheme2) => ({
  header: css`
    padding: ${theme.spacing(2)};
    border-bottom: 1px solid ${theme.colors.border.medium};
    background: ${theme.colors.background.primary};
    display: flex;
    justify-content: space-between;
    align-items: center;
    h1 {
      margin: 0;
      color: ${theme.colors.text.primary};
      font-size: ${theme.typography.h2.fontSize};
    }
  `,
  headerActions: css`
    display: flex;
    align-items: center;
    gap: ${theme.spacing(1)};
  `,
  sessionName: css`
    font-size: ${theme.typography.body.fontSize};
    color: ${theme.colors.text.secondary};
    margin: 0;
  `,
});

export function ChatHeader({
  sessionName,
  showSidebar,
  onShowSettings,
  onToggleSidebar,
}: ChatHeaderProps) {
  const s = useStyles2(getStyles);

  return (
    <div className={s.header}>
      <div>
        <h1>AI Chat Assistant</h1>
        {sessionName && (
          <p className={s.sessionName}>{sessionName}</p>
        )}
      </div>
      <div className={s.headerActions}>
        <Tooltip content="Chat settings">
          <IconButton
            name="cog"
            size="md"
            aria-label="Chat settings"
            onClick={onShowSettings}
          />
        </Tooltip>
        <Tooltip content={showSidebar ? "Hide chat history" : "Show chat history"}>
          <IconButton
            name={showSidebar ? "angle-left" : "angle-right"}
            size="md"
            aria-label={showSidebar ? "Hide chat history" : "Show chat history"}
            onClick={onToggleSidebar}
          />
        </Tooltip>
      </div>
    </div>
  );
}
