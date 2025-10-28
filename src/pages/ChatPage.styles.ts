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

import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';

export const getStyles = (theme: GrafanaTheme2) => ({
  pageContainer: css`
    display: flex;
    height: calc(100vh - 100px);
    max-height: calc(100vh - 100px);
    background: ${theme.colors.background.canvas};
    overflow: hidden;
  `,
  mainContent: css`
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-height: 0;
  `,
  statusIndicator: css`
    padding: ${theme.spacing(1)};
    font-size: ${theme.typography.bodySmall.fontSize};
    color: ${theme.colors.text.secondary};
    display: flex;
    align-items: center;
    justify-content: center;
    gap: ${theme.spacing(1)};
    flex-shrink: 0;
  `,
  mcpStatusContainer: css`
    padding: ${theme.spacing(1)} ${theme.spacing(2)};
    background: ${theme.colors.background.primary};
    flex-shrink: 0;
  `,
  chatContainer: css`
    flex: 1;
    display: flex;
    flex-direction: column;
    background: ${theme.colors.background.primary};
    overflow: hidden;
    min-height: 0;
  `,
  messagesContainer: css`
    flex: 1 1 0;
    overflow-y: auto;
    overflow-x: hidden;
    padding: ${theme.spacing(2)};
    display: flex;
    flex-direction: column;
    gap: ${theme.spacing(2)};
    min-height: 0;
  `,
  messageWrapper: css`
    display: flex;
    flex-direction: column;
  `,
  userMessage: css`
    align-self: flex-end;
    max-width: 70%;
    background: ${theme.colors.primary.main};
    color: ${theme.colors.primary.contrastText};
    padding: ${theme.spacing(1.5)};
    border-radius: ${theme.shape.radius.default};
    border-bottom-right-radius: 4px;
  `,
  assistantMessage: css`
    align-self: flex-start;
    max-width: 70%;
    background: ${theme.colors.background.secondary};
    color: ${theme.colors.text.primary};
    padding: ${theme.spacing(1.5)};
    border-radius: ${theme.shape.radius.default};
    border-bottom-left-radius: 4px;
    border: 1px solid ${theme.colors.border.weak};
  `,
  messageHeader: css`
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: ${theme.spacing(0.5)};
    font-size: ${theme.typography.bodySmall.fontSize};
  `,
  timestamp: css`
    opacity: 0.7;
    font-size: ${theme.typography.bodySmall.fontSize};
  `,
  messageContent: css`
    white-space: pre-wrap;
    word-wrap: break-word;
    line-height: 1.4;
  `,
  inputContainer: css`
    display: flex;
    gap: ${theme.spacing(1)};
    padding: ${theme.spacing(2)};
    background: ${theme.colors.background.primary};
    flex-shrink: 0;
    border-top: 1px solid ${theme.colors.border.weak};
  `,
  input: css`
    flex: 1;
  `,
  emptyState: css`
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    color: ${theme.colors.text.secondary};
    h2 {
      margin-bottom: ${theme.spacing(2)};
      color: ${theme.colors.text.primary};
    }
    p {
      margin-bottom: ${theme.spacing(3)};
      max-width: 400px;
    }
  `
});
