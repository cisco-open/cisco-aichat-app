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
import { MemoryRouter } from 'react-router-dom';
import { AppRootProps, PluginType } from '@grafana/data';
import { render } from '@testing-library/react';
import App from './App';

// Mock all services to prevent backend calls and side effects
jest.mock('../../services/ChatBackendService', () => ({
  ChatBackendService: {
    getInstance: jest.fn().mockReturnValue({
      isLLMAvailable: jest.fn().mockResolvedValue(false),
      checkLLMAvailability: jest.fn().mockResolvedValue(false),
    }),
  },
}));

jest.mock('../../services/ChatHistoryService', () => ({
  ChatHistoryService: {
    getInstance: jest.fn().mockReturnValue({
      getSessions: jest.fn().mockReturnValue([]),
      subscribe: jest.fn().mockReturnValue(() => {}),
      getActiveSession: jest.fn().mockReturnValue(null),
    }),
  },
}));

jest.mock('../../services/MCPIntegrationService', () => ({
  MCPIntegrationService: {
    getInstance: jest.fn().mockReturnValue({
      isAvailable: jest.fn().mockResolvedValue(false),
      getAvailableTools: jest.fn().mockResolvedValue([]),
    }),
  },
}));

describe('Components/App', () => {
  let props: AppRootProps;

  beforeEach(() => {
    jest.clearAllMocks();

    props = {
      basename: 'a/grafana-aichat-app',
      meta: {
        id: 'grafana-aichat-app',
        name: 'AI Chat Assistant',
        type: PluginType.app,
        enabled: true,
        jsonData: {},
      },
      query: {},
      path: '',
      onNavChanged: jest.fn(),
    } as unknown as AppRootProps;
  });

  test('renders without throwing an error', () => {
    // This test verifies the App component can be rendered without crashing
    // The actual routing and page content is tested in E2E tests
    expect(() => {
      render(
        <MemoryRouter>
          <App {...props} />
        </MemoryRouter>
      );
    }).not.toThrow();
  });
});
