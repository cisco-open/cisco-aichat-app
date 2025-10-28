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

import { ChatSettings, defaultChatSettings } from '../types/chat';
import { PluginSettingsService, ProvisionedSettings } from './PluginSettingsService';

export class ChatSettingsService {
  private static instance: ChatSettingsService | null = null;
  private static initPromise: Promise<ChatSettingsService> | null = null;
  private settings: ChatSettings;
  private provisionedSettings: ProvisionedSettings;

  private constructor(settings: ChatSettings, provisionedSettings: ProvisionedSettings) {
    this.settings = settings;
    this.provisionedSettings = provisionedSettings;
  }

  /**
   * Get instance asynchronously (loads provisioned settings on first call)
   */
  public static async getInstance(): Promise<ChatSettingsService> {
    if (ChatSettingsService.instance) {
      return ChatSettingsService.instance;
    }

    // If initialization is already in progress, wait for it
    if (ChatSettingsService.initPromise) {
      return ChatSettingsService.initPromise;
    }

    // Start initialization
    ChatSettingsService.initPromise = (async () => {
      // Fetch provisioned settings from backend
      const provisionedSettings = await PluginSettingsService.fetchProvisionedSettings();

      // Merge: provisioned settings override defaults
      const settings: ChatSettings = {
        ...defaultChatSettings,
        ...(provisionedSettings.systemPrompt && { systemPrompt: provisionedSettings.systemPrompt }),
        ...(provisionedSettings.enableMcpTools !== undefined && { enableMcpTools: provisionedSettings.enableMcpTools }),
      };

      ChatSettingsService.instance = new ChatSettingsService(settings, provisionedSettings);
      console.log('ChatSettingsService initialized with settings:', settings);
      return ChatSettingsService.instance;
    })();

    return ChatSettingsService.initPromise;
  }

  /**
   * Get current settings (read-only)
   */
  public getSettings(): ChatSettings {
    return { ...this.settings };
  }

  /**
   * Get provisioned settings metadata
   */
  public getProvisionedSettings(): ProvisionedSettings {
    return { ...this.provisionedSettings };
  }

  /**
   * Check if a setting is provisioned
   */
  public isProvisioned(key: keyof ChatSettings): boolean {
    if (!this.provisionedSettings.provisioned) {
      return false;
    }

    // Only check for keys that exist in ProvisionedSettings
    const provisionedKey = key as keyof ProvisionedSettings;
    return this.provisionedSettings[provisionedKey] !== undefined;
  }

  /**
   * Get specific setting values (read-only)
   */
  public getMaxSessionsPerUser(): number {
    return this.settings.maxSessionsPerUser;
  }

  public getMaxMessagesPerSession(): number {
    return this.settings.maxMessagesPerSession;
  }

  public getSystemPrompt(): string {
    return this.settings.systemPrompt;
  }

  public isModelSelectionEnabled(): boolean {
    return this.settings.enableModelSelection;
  }

  public isMcpToolsEnabled(): boolean {
    return this.settings.enableMcpTools;
  }

  public isAutoGenerateSessionNamesEnabled(): boolean {
    return this.settings.autoGenerateSessionNames;
  }
}
