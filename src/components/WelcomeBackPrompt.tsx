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
import { Button, useStyles2 } from '@grafana/ui';

interface WelcomeBackPromptProps {
  lastSessionName: string;
  onContinue: () => void;
  onStartNew: () => void;
}

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: ${theme.spacing(4)};
    text-align: center;
    height: 100%;
  `,
  title: css`
    font-size: ${theme.typography.h3.fontSize};
    font-weight: ${theme.typography.h3.fontWeight};
    color: ${theme.colors.text.primary};
    margin: 0;
    margin-bottom: ${theme.spacing(2)};
  `,
  subtitle: css`
    color: ${theme.colors.text.secondary};
    margin: 0;
    margin-bottom: ${theme.spacing(3)};
  `,
  sessionName: css`
    font-weight: ${theme.typography.fontWeightMedium};
    color: ${theme.colors.text.primary};
  `,
  actions: css`
    display: flex;
    gap: ${theme.spacing(2)};
  `,
});

export function WelcomeBackPrompt({
  lastSessionName,
  onContinue,
  onStartNew,
}: WelcomeBackPromptProps) {
  const s = useStyles2(getStyles);

  return (
    <div className={s.container}>
      <h2 className={s.title}>Welcome Back</h2>
      <p className={s.subtitle}>
        You have an ongoing conversation:{' '}
        <span className={s.sessionName}>{lastSessionName}</span>
      </p>
      <div className={s.actions}>
        <Button variant="primary" onClick={onContinue}>
          Continue Conversation
        </Button>
        <Button variant="secondary" onClick={onStartNew}>
          Start New Chat
        </Button>
      </div>
    </div>
  );
}
