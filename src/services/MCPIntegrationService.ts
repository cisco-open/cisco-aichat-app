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
import { lastValueFrom } from 'rxjs';
import { llm } from '@grafana/llm';
import { MCPTool, MCPToolCall, MCPToolResult, isToolInputSchema } from '../types/mcp';

// Re-export types for backward compatibility
export type { MCPTool, MCPToolCall, MCPToolResult } from '../types/mcp';

/** Response type for tools list API */
interface ToolsListResponse {
  tools?: MCPTool[];
}

/** Response type for tool call API */
interface ToolCallResponse {
  success: boolean;
  content?: string;
  error?: string;
}

/** Response wrapper from Grafana's fetch API */
interface FetchResponse<T> {
  status: number;
  data: T;
}

/**
 * Service for integrating Chat app with MCP tools from the MCP Client app
 */
export class MCPIntegrationService {
  private static mcpClientAvailable: boolean | null = null;

  /**
   * Check if MCP Client app is available and has tools
   */
  static async isMCPClientAvailable(): Promise<boolean> {
    if (MCPIntegrationService.mcpClientAvailable !== null) {
      return MCPIntegrationService.mcpClientAvailable;
    }

    try {
      const response = await lastValueFrom(getBackendSrv().fetch({
        url: '/api/plugins/grafana-mcpclient-app/resources/ping',
        method: 'GET',
      }));
      MCPIntegrationService.mcpClientAvailable = response.status === 200;
      console.log('MCP Client availability check:', MCPIntegrationService.mcpClientAvailable);
      return MCPIntegrationService.mcpClientAvailable;
    } catch (error) {
      console.log('MCP Client app not available:', error);
      MCPIntegrationService.mcpClientAvailable = false;
      return false;
    }
  }

  /**
   * Get available MCP tools from the MCP Client app
   */
  static async getAvailableTools(): Promise<MCPTool[]> {
    try {
      const isAvailable = await MCPIntegrationService.isMCPClientAvailable();
      if (!isAvailable) {
        console.log('MCP Client not available, returning empty tools list');
        return [];
      }

      const response = await lastValueFrom(getBackendSrv().fetch({
        url: '/api/plugins/grafana-mcpclient-app/resources/tools',
        method: 'GET',
      }));

      const data = response.data as ToolsListResponse;
      const tools = data?.tools || [];
      console.log('Available MCP tools:', tools);
      return tools;
    } catch (error) {
      console.error('Error fetching MCP tools:', error);
      return [];
    }
  }

  /**
   * Convert MCP tools to OpenAI function calling format
   */
  static convertToolsToOpenAI(mcpTools: MCPTool[]): llm.Tool[] {
    return mcpTools.map(tool => ({
      type: 'function' as const,
      function: {
        name: tool.name,
        description: tool.description ?? '',
        parameters: isToolInputSchema(tool.inputSchema)
          ? {
              type: 'object' as const,
              properties: tool.inputSchema.properties ?? {},
              required: tool.inputSchema.required ?? []
            }
          : { type: 'object' as const, properties: {}, required: [] }
      }
    }));
  }

  /**
   * Execute an MCP tool call with timeout and optional abort signal
   */
  static async callTool(toolCall: MCPToolCall, abortSignal?: AbortSignal): Promise<MCPToolResult> {
    const TOOL_CALL_TIMEOUT = 30000; // 30 seconds max timeout
    const MIN_WAIT_TIME = 500; // 500ms minimum wait (just to prevent flashing)

    // Create internal abort controller for timeout
    const timeoutController = new AbortController();
    const timeoutId = setTimeout(() => {
      timeoutController.abort();
    }, TOOL_CALL_TIMEOUT);

    // Link external abort signal to internal controller
    const abortHandler = () => timeoutController.abort();
    if (abortSignal) {
      abortSignal.addEventListener('abort', abortHandler);
    }

    try {
      // Check if already aborted
      if (abortSignal?.aborted) {
        return {
          tool_call_id: toolCall.id,
          content: 'Tool call was cancelled',
          is_error: true
        };
      }

      console.log('Executing tool call:', toolCall);
      const startTime = Date.now();

      const isAvailable = await MCPIntegrationService.isMCPClientAvailable();
      if (!isAvailable) {
        return {
          tool_call_id: toolCall.id,
          content: 'MCP Client app is not available. Please ensure the MCP Client plugin is installed and running.',
          is_error: true
        };
      }

      // Parse arguments from the function call
      let arguments_obj: Record<string, unknown> = {};
      try {
        arguments_obj = typeof toolCall.function.arguments === 'string'
          ? JSON.parse(toolCall.function.arguments)
          : toolCall.function.arguments;
      } catch (err) {
        console.error('Error parsing tool arguments:', err);
        return {
          tool_call_id: toolCall.id,
          content: `Error parsing tool arguments: ${err}`,
          is_error: true
        };
      }

      // Check if aborted before making the request
      if (timeoutController.signal.aborted) {
        const wasExternalAbort = abortSignal?.aborted;
        return {
          tool_call_id: toolCall.id,
          content: wasExternalAbort ? 'Tool call was cancelled' : `Tool call timed out after ${TOOL_CALL_TIMEOUT / 1000} seconds`,
          is_error: true
        };
      }

      // Create tool call promise - Grafana's fetch doesn't directly support AbortSignal,
      // so we wrap it with our abort handling
      const toolCallPromise = new Promise<FetchResponse<ToolCallResponse>>(async (resolve, reject) => {
        // Listen for abort
        const onAbort = () => {
          reject(new Error(abortSignal?.aborted ? 'Tool call was cancelled' : 'Tool call timed out'));
        };
        timeoutController.signal.addEventListener('abort', onAbort);

        try {
          const response = await lastValueFrom(getBackendSrv().fetch({
            url: '/api/plugins/grafana-mcpclient-app/resources/tools/call',
            method: 'POST',
            data: {
              tool_name: toolCall.function.name,
              arguments: arguments_obj
            }
          }));
          resolve(response as FetchResponse<ToolCallResponse>);
        } catch (error) {
          reject(error);
        } finally {
          timeoutController.signal.removeEventListener('abort', onAbort);
        }
      });

      const response = await toolCallPromise;
      const executionTime = Date.now() - startTime;

      // Only add minimal wait if execution was very fast (prevents UI flashing)
      if (executionTime < MIN_WAIT_TIME) {
        const remainingWait = MIN_WAIT_TIME - executionTime;
        console.log(`Tool executed in ${executionTime}ms, adding ${remainingWait}ms to prevent UI flashing`);
        await new Promise(resolve => setTimeout(resolve, remainingWait));
      }

      const data = response.data;
      console.log('Tool call response:', data, `(completed in ${Date.now() - startTime}ms)`);

      return {
        tool_call_id: toolCall.id,
        content: data.success ? (data.content ?? '') : `Tool execution failed: ${data.error}`,
        is_error: !data.success
      };
    } catch (error: unknown) {
      console.error('Error executing tool call:', error);
      const errorMessage = error instanceof Error ? error.message : String(error);
      const isCancelled = errorMessage.includes('cancelled');
      const isTimeout = errorMessage.includes('timed out');
      return {
        tool_call_id: toolCall.id,
        content: isCancelled ? 'Tool call was cancelled' :
                 isTimeout ? `Tool call timed out after ${TOOL_CALL_TIMEOUT / 1000} seconds` :
                 `Error executing tool "${toolCall.function.name}": ${errorMessage}`,
        is_error: true
      };
    } finally {
      clearTimeout(timeoutId);
      if (abortSignal) {
        abortSignal.removeEventListener('abort', abortHandler);
      }
    }
  }

  /**
   * Get MCP integration status for UI display
   */
  static async getIntegrationStatus(): Promise<{
    available: boolean;
    toolCount: number;
    message: string;
  }> {
    const available = await MCPIntegrationService.isMCPClientAvailable();

    if (!available) {
      return {
        available: false,
        toolCount: 0,
        message: 'MCP Client app not available. Install and configure MCP servers to enable enhanced AI capabilities.'
      };
    }

    const tools = await MCPIntegrationService.getAvailableTools();
    return {
      available: true,
      toolCount: tools.length,
      message: tools.length > 0
        ? `Connected to MCP Client with ${tools.length} tools available`
        : 'MCP Client available but no tools configured'
    };
  }
}



export default MCPIntegrationService;
