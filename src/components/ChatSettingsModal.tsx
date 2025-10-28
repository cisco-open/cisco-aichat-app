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

import React, { useState, useEffect } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import {
  Button,
  Field,
  useStyles2,
  Alert,
  Modal,
  Badge,
  CodeEditor
} from '@grafana/ui';
import { ChatSettings } from '../types/chat';
import { ChatSettingsService } from '../services/ChatSettingsService';

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    padding: ${theme.spacing(3)};
    max-width: 700px;
  `,
  section: css`
    margin-bottom: ${theme.spacing(4)};

    h3 {
      margin-bottom: ${theme.spacing(2)};
      color: ${theme.colors.text.primary};
      border-bottom: 1px solid ${theme.colors.border.medium};
      padding-bottom: ${theme.spacing(1)};
    }
  `,
  infoBox: css`
    background: ${theme.colors.background.secondary};
    border: 1px solid ${theme.colors.border.medium};
    border-radius: ${theme.shape.radius.default};
    padding: ${theme.spacing(2)};
    margin-bottom: ${theme.spacing(2)};
  `,
  settingRow: css`
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: ${theme.spacing(1.5)} 0;
    border-bottom: 1px solid ${theme.colors.border.weak};

    &:last-child {
      border-bottom: none;
    }
  `,
  settingLabel: css`
    font-weight: ${theme.typography.fontWeightMedium};
    color: ${theme.colors.text.primary};
    display: flex;
    align-items: center;
    gap: ${theme.spacing(1)};
  `,
  settingValue: css`
    color: ${theme.colors.text.secondary};
    font-family: ${theme.typography.fontFamilyMonospace};
  `,
  codeBlock: css`
    margin-top: ${theme.spacing(1)};
    max-height: 300px;
    overflow: auto;
  `,
  actions: css`
    display: flex;
    gap: ${theme.spacing(2)};
    justify-content: flex-end;
    margin-top: ${theme.spacing(3)};
    padding-top: ${theme.spacing(2)};
    border-top: 1px solid ${theme.colors.border.medium};
  `,
  description: css`
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.bodySmall.fontSize};
    margin-bottom: ${theme.spacing(2)};
  `
});

interface ChatSettingsProps {
  isOpen: boolean;
  onClose: () => void;
}

export function ChatSettingsModal({ isOpen, onClose }: ChatSettingsProps) {
  const s = useStyles2(getStyles);
  const [settings, setSettings] = useState<ChatSettings | null>(null);
  const [isProvisioned, setIsProvisioned] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  // Load settings when modal opens
  useEffect(() => {
    if (isOpen) {
      setIsLoading(true);
      ChatSettingsService.getInstance().then(service => {
        const currentSettings = service.getSettings();
        const provisionedSettings = service.getProvisionedSettings();
        setSettings(currentSettings);
        setIsProvisioned(provisionedSettings.provisioned);
        setIsLoading(false);
      });
    }
  }, [isOpen]);

  if (isLoading) {
    return (
      <Modal
        title="Chat Settings"
        isOpen={isOpen}
        onDismiss={onClose}
        className={css`min-width: 500px;`}
      >
        <div className={s.container}>
          <Alert severity="info" title="Loading settings..." />
        </div>
      </Modal>
    );
  }

  if (!settings) {
    return null;
  }

  return (
    <Modal
      title="Chat Settings"
      isOpen={isOpen}
      onDismiss={onClose}
      className={css`min-width: 600px;`}
    >
      <div className={s.container}>
        <Alert severity="info" title="Read-Only Configuration">
          Settings are controlled by deployment configuration (apps.yaml).
          Contact your administrator to modify these values.
        </Alert>

        {/* AI Behavior */}
        <div className={s.section}>
          <h3>AI Behavior</h3>
          <div className={s.description}>
            Current configuration for AI assistant behavior and capabilities.
          </div>

          <Field label="System Prompt">
            <div className={s.infoBox}>
              <div className={s.settingLabel}>
                <span>System Prompt</span>
                {isProvisioned && <Badge text="Provisioned" color="blue" />}
                {!isProvisioned && <Badge text="Default" color="orange" />}
              </div>
              <div className={s.codeBlock}>
                <CodeEditor
                  value={settings.systemPrompt}
                  language="markdown"
                  height="200px"
                  readOnly={true}
                  showLineNumbers={false}
                  showMiniMap={false}
                />
              </div>
            </div>
          </Field>

          <div className={s.infoBox}>
            <div className={s.settingRow}>
              <div className={s.settingLabel}>
                <span>Enable MCP Tools</span>
                {isProvisioned && <Badge text="Provisioned" color="blue" />}
                {!isProvisioned && <Badge text="Default" color="orange" />}
              </div>
              <div className={s.settingValue}>
                {settings.enableMcpTools ? 'Enabled' : 'Disabled'}
              </div>
            </div>

            <div className={s.settingRow}>
              <div className={s.settingLabel}>
                <span>Enable Model Selection</span>
                <Badge text="Default" color="orange" />
              </div>
              <div className={s.settingValue}>
                {settings.enableModelSelection ? 'Enabled' : 'Disabled'}
              </div>
            </div>
          </div>
        </div>

        {/* Session Management */}
        <div className={s.section}>
          <h3>Session Management</h3>
          <div className={s.description}>
            Default limits for chat sessions and message history.
          </div>

          <div className={s.infoBox}>
            <div className={s.settingRow}>
              <div className={s.settingLabel}>
                <span>Max Sessions Per User</span>
                <Badge text="Default" color="orange" />
              </div>
              <div className={s.settingValue}>{settings.maxSessionsPerUser}</div>
            </div>

            <div className={s.settingRow}>
              <div className={s.settingLabel}>
                <span>Max Messages Per Session</span>
                <Badge text="Default" color="orange" />
              </div>
              <div className={s.settingValue}>{settings.maxMessagesPerSession}</div>
            </div>

            <div className={s.settingRow}>
              <div className={s.settingLabel}>
                <span>Auto-generate Session Names</span>
                <Badge text="Default" color="orange" />
              </div>
              <div className={s.settingValue}>
                {settings.autoGenerateSessionNames ? 'Enabled' : 'Disabled'}
              </div>
            </div>
          </div>
        </div>

        <div className={s.actions}>
          <Button onClick={onClose}>Close</Button>
        </div>
      </div>
    </Modal>
  );
}
