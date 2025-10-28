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

import { of, throwError } from 'rxjs';
import { MCPIntegrationService } from './MCPIntegrationService';
import { MCPTool, MCPToolCall } from '../types/mcp';

// Mock @grafana/runtime
const mockFetch = jest.fn();
jest.mock('@grafana/runtime', () => ({
  getBackendSrv: () => ({
    fetch: mockFetch,
  }),
}));

/**
 * Reset MCPIntegrationService static state between tests
 */
function resetMCPServiceState(): void {
  // Reset the cached availability check
  (MCPIntegrationService as unknown as { mcpClientAvailable: boolean | null }).mcpClientAvailable = null;
}

/**
 * Create a mock tool for testing
 */
function createMockTool(name: string, description: string): MCPTool {
  return {
    name,
    description,
    inputSchema: {
      type: 'object',
      properties: { query: { type: 'string' } },
      required: ['query'],
    },
  };
}

/**
 * Create a mock tool call for testing
 */
function createMockToolCall(id: string, name: string, args: Record<string, unknown>): MCPToolCall {
  return {
    id,
    type: 'function',
    function: {
      name,
      arguments: JSON.stringify(args),
    },
  };
}

describe('MCPIntegrationService', () => {
  beforeEach(() => {
    mockFetch.mockReset();
    resetMCPServiceState();
  });

  describe('isMCPClientAvailable', () => {
    it('returns true when ping endpoint succeeds', async () => {
      mockFetch.mockReturnValue(
        of({
          status: 200,
          data: {},
        })
      );

      const available = await MCPIntegrationService.isMCPClientAvailable();

      expect(available).toBe(true);
      expect(mockFetch).toHaveBeenCalledWith(
        expect.objectContaining({
          url: '/api/plugins/grafana-mcpclient-app/resources/ping',
          method: 'GET',
        })
      );
    });

    it('returns false when ping endpoint fails', async () => {
      mockFetch.mockReturnValue(throwError(() => new Error('Connection refused')));

      const available = await MCPIntegrationService.isMCPClientAvailable();

      expect(available).toBe(false);
    });

    it('caches availability check result', async () => {
      mockFetch.mockReturnValue(of({ status: 200, data: {} }));

      // First call
      await MCPIntegrationService.isMCPClientAvailable();
      // Second call should use cached value
      await MCPIntegrationService.isMCPClientAvailable();

      expect(mockFetch).toHaveBeenCalledTimes(1);
    });
  });

  describe('getAvailableTools', () => {
    it('returns tools from backend', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));
      // Mock tools list
      mockFetch.mockReturnValueOnce(
        of({
          status: 200,
          data: {
            tools: [
              createMockTool('search', 'Search tool'),
              createMockTool('query', 'Query tool'),
            ],
          },
        })
      );

      const tools = await MCPIntegrationService.getAvailableTools();

      expect(tools).toHaveLength(2);
      expect(tools[0].name).toBe('search');
      expect(tools[1].name).toBe('query');
    });

    it('returns empty array when MCP client not available', async () => {
      mockFetch.mockReturnValue(throwError(() => new Error('Not available')));

      const tools = await MCPIntegrationService.getAvailableTools();

      expect(tools).toEqual([]);
    });

    it('returns empty array on fetch error', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));
      // Mock tools list error
      mockFetch.mockReturnValueOnce(throwError(() => new Error('Fetch failed')));

      const tools = await MCPIntegrationService.getAvailableTools();

      expect(tools).toEqual([]);
    });

    it('handles missing tools array in response', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));
      // Mock empty response
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));

      const tools = await MCPIntegrationService.getAvailableTools();

      expect(tools).toEqual([]);
    });
  });

  describe('convertToolsToOpenAI', () => {
    it('converts MCP tools to OpenAI function format', () => {
      const mcpTools: MCPTool[] = [
        {
          name: 'search',
          description: 'Search for items',
          inputSchema: {
            type: 'object',
            properties: { query: { type: 'string' } },
            required: ['query'],
          },
        },
      ];

      const openAITools = MCPIntegrationService.convertToolsToOpenAI(mcpTools);

      expect(openAITools).toHaveLength(1);
      expect(openAITools[0].type).toBe('function');
      expect(openAITools[0].function.name).toBe('search');
      expect(openAITools[0].function.description).toBe('Search for items');
      expect(openAITools[0].function.parameters).toEqual({
        type: 'object',
        properties: { query: { type: 'string' } },
        required: ['query'],
      });
    });

    it('handles tools without inputSchema', () => {
      const mcpTools: MCPTool[] = [
        {
          name: 'simple-tool',
          description: 'A tool without schema',
        },
      ];

      const openAITools = MCPIntegrationService.convertToolsToOpenAI(mcpTools);

      expect(openAITools[0].function.parameters).toEqual({
        type: 'object',
        properties: {},
        required: [],
      });
    });

    it('handles empty description', () => {
      const mcpTools: MCPTool[] = [
        {
          name: 'no-desc',
          description: '',
        },
      ];

      const openAITools = MCPIntegrationService.convertToolsToOpenAI(mcpTools);

      expect(openAITools[0].function.description).toBe('');
    });
  });

  describe('callTool', () => {
    beforeEach(() => {
      // Reset availability cache for each test
      resetMCPServiceState();
    });

    it('calls backend with tool parameters', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));
      // Mock tool call
      mockFetch.mockReturnValueOnce(
        of({
          status: 200,
          data: {
            success: true,
            content: 'Tool execution result',
          },
        })
      );

      const toolCall = createMockToolCall('call-1', 'search', { query: 'test' });
      const result = await MCPIntegrationService.callTool(toolCall);

      expect(result.tool_call_id).toBe('call-1');
      expect(result.content).toBe('Tool execution result');
      expect(result.is_error).toBe(false);
      expect(mockFetch).toHaveBeenCalledWith(
        expect.objectContaining({
          url: '/api/plugins/grafana-mcpclient-app/resources/tools/call',
          method: 'POST',
          data: expect.objectContaining({
            tool_name: 'search',
            arguments: { query: 'test' },
          }),
        })
      );
    });

    it('returns error when MCP client not available', async () => {
      mockFetch.mockReturnValue(throwError(() => new Error('Not available')));

      const toolCall = createMockToolCall('call-1', 'search', { query: 'test' });
      const result = await MCPIntegrationService.callTool(toolCall);

      expect(result.is_error).toBe(true);
      expect(result.content).toContain('not available');
    });

    it('returns error on execution failure', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));
      // Mock tool call failure
      mockFetch.mockReturnValueOnce(
        of({
          status: 200,
          data: {
            success: false,
            error: 'Tool execution failed: Invalid input',
          },
        })
      );

      const toolCall = createMockToolCall('call-1', 'search', { query: 'test' });
      const result = await MCPIntegrationService.callTool(toolCall);

      expect(result.is_error).toBe(true);
      expect(result.content).toContain('Tool execution failed');
    });

    it('handles invalid JSON arguments', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));

      const toolCall: MCPToolCall = {
        id: 'call-1',
        type: 'function',
        function: {
          name: 'search',
          arguments: 'not-valid-json',
        },
      };

      const result = await MCPIntegrationService.callTool(toolCall);

      expect(result.is_error).toBe(true);
      expect(result.content).toContain('Error parsing tool arguments');
    });

    it('returns cancelled when abort signal is already aborted', async () => {
      const controller = new AbortController();
      controller.abort();

      const toolCall = createMockToolCall('call-1', 'search', { query: 'test' });
      const result = await MCPIntegrationService.callTool(toolCall, controller.signal);

      expect(result.is_error).toBe(true);
      expect(result.content).toBe('Tool call was cancelled');
    });

    it('handles network error during execution', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));
      // Mock network error
      mockFetch.mockReturnValueOnce(throwError(() => new Error('Network error')));

      const toolCall = createMockToolCall('call-1', 'search', { query: 'test' });
      const result = await MCPIntegrationService.callTool(toolCall);

      expect(result.is_error).toBe(true);
      expect(result.content).toContain('Error executing tool');
    });
  });

  describe('getIntegrationStatus', () => {
    it('returns available status with tool count', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));
      // Mock tools list
      mockFetch.mockReturnValueOnce(
        of({
          status: 200,
          data: {
            tools: [createMockTool('search', 'Search'), createMockTool('query', 'Query')],
          },
        })
      );

      const status = await MCPIntegrationService.getIntegrationStatus();

      expect(status.available).toBe(true);
      expect(status.toolCount).toBe(2);
      expect(status.message).toContain('2 tools');
    });

    it('returns unavailable status when MCP client not running', async () => {
      mockFetch.mockReturnValue(throwError(() => new Error('Not available')));

      const status = await MCPIntegrationService.getIntegrationStatus();

      expect(status.available).toBe(false);
      expect(status.toolCount).toBe(0);
      expect(status.message).toContain('not available');
    });

    it('returns available but no tools configured message', async () => {
      // Mock availability check
      mockFetch.mockReturnValueOnce(of({ status: 200, data: {} }));
      // Mock empty tools list
      mockFetch.mockReturnValueOnce(
        of({
          status: 200,
          data: { tools: [] },
        })
      );

      const status = await MCPIntegrationService.getIntegrationStatus();

      expect(status.available).toBe(true);
      expect(status.toolCount).toBe(0);
      expect(status.message).toContain('no tools configured');
    });
  });
});
