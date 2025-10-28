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

import { getBackendSrv } from '@grafana/runtime';

export interface ProvisionedSettings {
  systemPrompt?: string;
  maxTokens?: number;
  temperature?: number;
  enableMcpTools?: boolean;
  provisioned: boolean;
}

export class PluginSettingsService {
  private static cachedSettings: ProvisionedSettings | null = null;

  /**
   * Fetch provisioned settings from backend
   */
  static async fetchProvisionedSettings(): Promise<ProvisionedSettings> {
    // Return cached settings if available
    if (this.cachedSettings !== null) {
      return this.cachedSettings;
    }

    try {
      const response = await getBackendSrv().get('/api/plugins/grafana-aichat-app/resources/settings');

      const settings: ProvisionedSettings = {
        systemPrompt: response.systemPrompt,
        maxTokens: response.maxTokens,
        temperature: response.temperature,
        enableMcpTools: response.enableMcpTools,
        provisioned: response.provisioned || false,
      };

      // Cache the settings
      this.cachedSettings = settings;

      console.log('Loaded provisioned settings:', settings);
      return settings;
    } catch (error) {
      console.warn('Failed to fetch provisioned settings, using defaults:', error);

      // Return empty settings (defaults will be used)
      const emptySettings: ProvisionedSettings = {
        provisioned: false,
      };
      this.cachedSettings = emptySettings;
      return emptySettings;
    }
  }

  /**
   * Clear cached settings (useful for testing/development)
   */
  static clearCache(): void {
    this.cachedSettings = null;
  }
}
