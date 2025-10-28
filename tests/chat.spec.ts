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

    // Chat page should show AI Chat Assistant heading
    await expect(page.getByRole('heading', { name: 'AI Chat Assistant', level: 1 }).first()).toBeVisible({
      timeout: 10000,
    });
  });

  test('should show chat history section', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);

    // Chat History heading should be visible
    await expect(page.getByRole('heading', { name: 'Chat History' })).toBeVisible({ timeout: 10000 });
  });

  test('should show message input field', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);

    // Message input should be visible
    const input = page.getByRole('textbox', { name: /Ask me about monitoring/i });
    await expect(input).toBeVisible({ timeout: 10000 });
  });

  test('should show MCP status indicator', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);

    // MCP status should be visible (connected or not available)
    const mcpConnected = page.getByText(/MCP:.*Connected/i);
    const mcpNotAvailable = page.getByText(/MCP.*not.*available/i);

    await expect(mcpConnected.or(mcpNotAvailable)).toBeVisible({ timeout: 10000 });
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

  test('should have settings button accessible', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);

    // Settings button should be visible
    const settingsButton = page.getByRole('button', { name: /Chat settings/i });
    await expect(settingsButton).toBeVisible({ timeout: 10000 });
  });
});
