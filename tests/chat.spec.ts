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

import { test, expect } from './fixtures';
import { ROUTES } from '../src/constants';

test.describe('Chat page', () => {
  test('should navigate to chat page successfully', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);

    // Verify the chat page loaded (not a 404) by checking for any plugin content.
    // The plugin may show degraded UI if the backend isn't running.
    const chatHeading = page.getByRole('heading', { name: /AI Chat/i }).first();

    await expect(chatHeading).toBeVisible({ timeout: 10000 });

    // Confirm we're on the right URL
    expect(page.url()).toContain(ROUTES.Chat);
  });

  test('should load chat page without 404', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);

    // Page should not show a "not found" error
    await expect(page.getByText(/page not found/i)).not.toBeVisible({ timeout: 5000 });

    // URL should contain the chat route
    expect(page.url()).toContain(ROUTES.Chat);
  });

  test('should show chat UI elements when backend is available', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);

    // These elements depend on the backend running. If backend isn't available,
    // the page may show a degraded state. We check with a short timeout and skip
    // gracefully if elements aren't found.
    const messageInput = page.getByRole('textbox', { name: /Ask me about monitoring/i });
    const chatHistory = page.getByRole('heading', { name: /Chat History/i });
    const settingsButton = page.getByRole('button', { name: /Chat settings/i });

    // At least one of these UI elements should be present if the plugin rendered
    const anyElement = messageInput.or(chatHistory).or(settingsButton);
    const pluginRendered = await anyElement.isVisible({ timeout: 5000 }).catch(() => false);

    if (!pluginRendered) {
      // Backend not running - verify the page at least loaded without critical failure
      await expect(page.getByText(/page not found/i)).not.toBeVisible();
      test.skip(true, 'Chat UI elements not available - backend may not be running');
    }

    // If we get here, backend is running - verify the core UI
    await expect(anyElement).toBeVisible();
  });

  test('should render without critical console errors', async ({ gotoPage, page }) => {
    const criticalErrors: string[] = [];

    // Listen for console errors
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        const text = msg.text().toLowerCase();
        // Filter out expected errors
        if (
          !text.includes('net::err_') &&
          !text.includes('failed to load resource') &&
          !text.includes('failed to preload plugin') &&
          !text.includes('unknown plugin') &&
          !text.includes('cors') &&
          !text.includes('favicon')
        ) {
          criticalErrors.push(msg.text());
        }
      }
    });

    await gotoPage(`/${ROUTES.Chat}`);

    // Wait for page to stabilize
    await page.waitForTimeout(2000);

    // No critical rendering errors should occur
    const jsErrors = criticalErrors.filter(
      (e) => e.includes('TypeError') || e.includes('ReferenceError') || e.includes('SyntaxError')
    );
    expect(jsErrors).toHaveLength(0);
  });

  test('should verify plugin is enabled via API', async ({ gotoPage, page }) => {
    // Use the Grafana API to confirm the plugin is loaded and enabled
    const response = await page.request.get('/api/plugins/grafana-aichat-app/settings');
    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.enabled).toBe(true);
  });
});
