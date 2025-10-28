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

import React, { ReactNode } from 'react';
import { renderHook, RenderHookOptions, RenderHookResult } from '@testing-library/react';

/**
 * Test wrapper that provides required contexts for hook testing.
 * Currently useChatSession does not require any context providers,
 * but this wrapper can be extended to include ChatProvider or other
 * contexts as needed for future tests.
 */
export const TestProviders: React.FC<{ children: ReactNode }> = ({ children }) => {
  return <>{children}</>;
};

/**
 * Helper to render hooks with test providers.
 * Wraps @testing-library/react renderHook with TestProviders.
 *
 * @param hook - The hook function to render
 * @param options - Optional renderHook options (wrapper will be overridden)
 * @returns RenderHookResult for assertions and rerendering
 */
export function renderHookWithProviders<Result, Props>(
  hook: (props: Props) => Result,
  options?: Omit<RenderHookOptions<Props>, 'wrapper'>
): RenderHookResult<Result, Props> {
  return renderHook(hook, {
    wrapper: TestProviders,
    ...options,
  });
}
