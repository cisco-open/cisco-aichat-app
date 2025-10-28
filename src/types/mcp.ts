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

/**
 * MCP type definitions for aichat.
 * These types mirror mcpclient's type shapes for consistency while remaining decoupled.
 */

/**
 * JSON Schema structure for MCP tool input parameters.
 * Matches the JSON Schema 'object' type structure used by MCP servers.
 */
export interface MCPToolInputSchema {
  type: 'object';
  properties?: Record<string, unknown>;
  required?: string[];
}

/**
 * Type guard to validate if a value conforms to MCPToolInputSchema.
 * Used for safely narrowing unknown inputSchema values from API responses.
 */
export function isToolInputSchema(value: unknown): value is MCPToolInputSchema {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const obj = value as Record<string, unknown>;
  return obj.type === 'object';
}

/**
 * MCP tool definition as returned by the mcpclient API.
 */
export interface MCPTool {
  name: string;
  description: string;
  serverId?: string;
  serverName?: string;
  parameters?: Record<string, unknown>;
  inputSchema?: MCPToolInputSchema;
}

/**
 * Tool call request matching OpenAI function calling format.
 * Used when the LLM requests execution of an MCP tool.
 */
export interface MCPToolCall {
  id: string;
  type: 'function';
  function: {
    name: string;
    arguments: string;
  };
}

/**
 * Result from executing an MCP tool call.
 * Returned to the LLM to continue the conversation with tool output.
 */
export interface MCPToolResult {
  tool_call_id: string;
  content: string;
  is_error?: boolean;
}
