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

import { useState, useRef, useCallback } from 'react';
import { llm } from '@grafana/llm';
import { MCPIntegrationService } from '../services/MCPIntegrationService';
import { useChatContext, ToolExecutionResult } from '../context/ChatContext';

/**
 * Status of a tool execution.
 */
export type ToolExecutionStatus = 'pending' | 'executing' | 'success' | 'error';

/**
 * Information about a tool being executed.
 * Matches the interface expected by ToolExecutionIndicator component.
 */
export interface ToolExecutionInfo {
  id: string;
  toolName: string;
  status: ToolExecutionStatus;
  startTime: number;
  endTime?: number;
  errorMessage?: string;
}

/**
 * Return type for the useToolExecution hook.
 */
export interface UseToolExecutionResult {
  /** Whether any tool is currently executing */
  isExecuting: boolean;
  /** Error from tool execution, if any */
  toolError: Error | null;
  /** List of all tool executions with their status */
  toolExecutions: ToolExecutionInfo[];
  /** Execute an array of tool calls with optional abort signal */
  executeToolCalls: (toolCalls: llm.ToolCall[], abortSignal?: AbortSignal) => Promise<ToolExecutionResult[]>;
  /** Clear all tool execution state */
  clearExecutions: () => void;
}

/**
 * Hook for executing MCP tools and tracking execution state.
 *
 * Provides:
 * - Tool call execution with abort support
 * - Execution state tracking for UI indicators
 * - Error handling with proper typing
 *
 * Must be used within a ChatProvider context.
 */
export function useToolExecution(): UseToolExecutionResult {
  const { toolError, setToolError, setIsExecutingTools } = useChatContext();
  const [toolExecutions, setToolExecutions] = useState<ToolExecutionInfo[]>([]);
  const abortControllerRef = useRef<AbortController | null>(null);

  /**
   * Add a new execution to the tracking list.
   * Returns the generated execution ID.
   */
  const addExecution = useCallback((toolName: string): string => {
    const executionId = `tool-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
    const execution: ToolExecutionInfo = {
      id: executionId,
      toolName,
      status: 'pending',
      startTime: Date.now(),
    };
    setToolExecutions(prev => [...prev, execution]);
    return executionId;
  }, []);

  /**
   * Update the status of an existing execution.
   */
  const updateExecution = useCallback((id: string, updates: Partial<ToolExecutionInfo>) => {
    setToolExecutions(prev =>
      prev.map(exec => (exec.id === id ? { ...exec, ...updates } : exec))
    );
  }, []);

  /**
   * Clear all executions from the tracking list.
   */
  const clearExecutions = useCallback(() => {
    setToolExecutions([]);
  }, []);

  /**
   * Execute an array of tool calls sequentially.
   *
   * @param toolCalls - Array of llm.ToolCall to execute
   * @param abortSignal - Optional AbortSignal for cancellation
   * @returns Array of ToolExecutionResult with status of each tool
   */
  const executeToolCalls = useCallback(async (
    toolCalls: llm.ToolCall[],
    abortSignal?: AbortSignal
  ): Promise<ToolExecutionResult[]> => {
    const results: ToolExecutionResult[] = [];
    setToolError(null);
    setIsExecutingTools(true);

    // Create internal abort controller
    abortControllerRef.current = new AbortController();

    // Link external abort signal to internal controller
    if (abortSignal) {
      abortSignal.addEventListener('abort', () => {
        abortControllerRef.current?.abort();
      });
    }

    try {
      for (const toolCall of toolCalls) {
        // Check for abort before each tool
        if (abortControllerRef.current.signal.aborted) {
          break;
        }

        const toolName = toolCall.function.name;
        const executionId = addExecution(toolName);

        try {
          updateExecution(executionId, { status: 'executing' });

          const result = await MCPIntegrationService.callTool(
            {
              id: toolCall.id,
              type: 'function',
              function: {
                name: toolName,
                arguments: toolCall.function.arguments
              }
            },
            abortControllerRef.current.signal
          );

          // Check if cancelled
          if (result.is_error && result.content.includes('cancelled')) {
            updateExecution(executionId, {
              status: 'error',
              endTime: Date.now(),
              errorMessage: 'Cancelled'
            });
            results.push({
              toolName,
              status: 'error',
              errorMessage: 'Cancelled'
            });
            break;
          }

          // Update execution status based on result
          if (result.is_error) {
            updateExecution(executionId, {
              status: 'error',
              endTime: Date.now(),
              errorMessage: result.content.substring(0, 100)
            });
            results.push({
              toolName,
              status: 'error',
              errorMessage: result.content.substring(0, 100)
            });
          } else {
            updateExecution(executionId, {
              status: 'success',
              endTime: Date.now()
            });
            results.push({
              toolName,
              status: 'success'
            });
          }
        } catch (error: unknown) {
          const errorMessage = error instanceof Error ? error.message : String(error);
          updateExecution(executionId, {
            status: 'error',
            endTime: Date.now(),
            errorMessage: errorMessage.substring(0, 100)
          });
          results.push({
            toolName,
            status: 'error',
            errorMessage: errorMessage.substring(0, 100)
          });
        }
      }

      return results;
    } catch (error: unknown) {
      const err = error instanceof Error ? error : new Error(String(error));
      setToolError(err);
      throw err;
    } finally {
      setIsExecutingTools(false);
      abortControllerRef.current = null;
    }
  }, [addExecution, updateExecution, setToolError, setIsExecutingTools]);

  return {
    isExecuting: toolExecutions.some(e => e.status === 'pending' || e.status === 'executing'),
    toolError,
    toolExecutions,
    executeToolCalls,
    clearExecutions,
  };
}
