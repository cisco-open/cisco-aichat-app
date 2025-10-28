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

test.describe('navigating app', () => {
  test('chat page should render successfully', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);
    // Chat page should show the AI Chat interface
    await expect(page.getByRole('heading', { name: /AI Chat/i }).or(page.getByText(/Chat/i).first())).toBeVisible({
      timeout: 10000,
    });
  });

  test('chat page should show message input', async ({ gotoPage, page }) => {
    await gotoPage(`/${ROUTES.Chat}`);
    // Chat page should have a text input area for messages
    const messageInput = page.getByRole('textbox', { name: /Ask me about monitoring/i });
    await expect(messageInput).toBeVisible({ timeout: 10000 });
  });
});
