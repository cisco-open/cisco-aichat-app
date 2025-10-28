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

import { useRef, useCallback, useEffect } from 'react';
import { Subscription } from 'rxjs';
import { llm } from '@grafana/llm';
import { useChatContext, ToolExecutionResult } from '../context/ChatContext';
import { ChatMessage } from '../types/chat';

/**
 * Extended streaming chunk choice interface.
 * The @grafana/llm types for ChatCompletionsChunk don't expose finish_reason,
 * but the OpenAI streaming API does include it at runtime.
 */
interface StreamingChunkChoice extends llm.ChatCompletionsChunk {
  finish_reason?: string | null;
}

/**
 * Extended tool execution result that includes the MCP tool result content.
 */
export interface ExtendedToolExecutionResult extends ToolExecutionResult {
  toolCallId: string;
  content: string;
}

/**
 * Result interface for the useStreamChat hook.
 */
export interface UseStreamChatResult {
  /** Whether a stream is currently active */
  isStreaming: boolean;
  /** Error that occurred during streaming, if any */
  streamError: Error | null;
  /**
   * Start a streaming chat completion request.
   * @param userMessage - The user's message to send
   * @param systemPrompt - The system prompt to use
   * @param mcpTools - MCP tools available for the LLM to use
   * @param onToolCallsComplete - Callback invoked when tool calls are ready for execution
   */
  startStream: (
    userMessage: string,
    systemPrompt: string,
    mcpTools: llm.Tool[],
    onToolCallsComplete: (toolCalls: llm.ToolCall[], abortSignal: AbortSignal) => Promise<ExtendedToolExecutionResult[]>
  ) => Promise<void>;
  /** Cancel the current stream and cleanup */
  cancelStream: () => void;
}

/**
 * Hook for managing SSE streaming chat completions with proper RxJS subscription cleanup.
 * Handles tool call accumulation, tool result follow-up streaming, message updates, and error states.
 */
export function useStreamChat(): UseStreamChatResult {
  const FIRST_TOKEN_TIMEOUT_MS = 15000;
  const INACTIVITY_TIMEOUT_MS = 20000;

  const {
    messages,
    addMessage,
    updateMessage,
    isStreaming,
    setIsStreaming,
    setStreamError,
    streamError,
    setCurrentAssistantMessageId,
    setIsExecutingTools,
  } = useChatContext();

  const subscriptionRef = useRef<Subscription | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const runIdRef = useRef(0);
  const firstTokenTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inactivityTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearWatchdogs = useCallback(() => {
    if (firstTokenTimerRef.current) {
      clearTimeout(firstTokenTimerRef.current);
      firstTokenTimerRef.current = null;
    }
    if (inactivityTimerRef.current) {
      clearTimeout(inactivityTimerRef.current);
      inactivityTimerRef.current = null;
    }
  }, []);

  const timeoutStream = useCallback((assistantMessageId: string, reason: string, runId: number) => {
    if (runId !== runIdRef.current) {
      return;
    }
    console.info('[ChatStreamEvent]', { event: 'stream_timeout', reason, timestamp: Date.now() });
    subscriptionRef.current?.unsubscribe();
    subscriptionRef.current = null;
    abortControllerRef.current?.abort();
    abortControllerRef.current = null;
    clearWatchdogs();
    setIsExecutingTools(false);
    setIsStreaming(false);
    setCurrentAssistantMessageId(null);
    setStreamError(new Error('Response timed out. Please sign in again and retry.'));
    void updateMessage(
      assistantMessageId,
      'The response timed out. Your session may have expired. Please sign in again and retry.',
      undefined,
      { persist: true }
    ).catch((persistErr) => console.error('Failed to persist timeout error message:', persistErr));
  }, [clearWatchdogs, setCurrentAssistantMessageId, setIsExecutingTools, setIsStreaming, setStreamError, updateMessage]);

  const startFirstTokenWatchdog = useCallback((assistantMessageId: string, runId: number) => {
    if (firstTokenTimerRef.current) {
      clearTimeout(firstTokenTimerRef.current);
    }
    firstTokenTimerRef.current = setTimeout(() => {
      timeoutStream(assistantMessageId, 'first_token', runId);
    }, FIRST_TOKEN_TIMEOUT_MS);
  }, [timeoutStream]);

  const resetInactivityWatchdog = useCallback((assistantMessageId: string, runId: number) => {
    if (inactivityTimerRef.current) {
      clearTimeout(inactivityTimerRef.current);
    }
    inactivityTimerRef.current = setTimeout(() => {
      timeoutStream(assistantMessageId, 'inactivity', runId);
    }, INACTIVITY_TIMEOUT_MS);
  }, [timeoutStream]);

  /**
   * Start a streaming chat completion request.
   */
  const startStream = useCallback(async (
    userMessage: string,
    systemPrompt: string,
    mcpTools: llm.Tool[],
    onToolCallsComplete: (toolCalls: llm.ToolCall[], abortSignal: AbortSignal) => Promise<ExtendedToolExecutionResult[]>
  ) => {
    // Cancel and invalidate any existing run
    runIdRef.current += 1;
    const runId = runIdRef.current;
    subscriptionRef.current?.unsubscribe();
    subscriptionRef.current = null;
    abortControllerRef.current?.abort();
    clearWatchdogs();
    abortControllerRef.current = new AbortController();
    const runAbortSignal = abortControllerRef.current.signal;
    const isRunActive = () =>
      runId === runIdRef.current && !runAbortSignal.aborted;

    setIsExecutingTools(false);
    setIsStreaming(true);
    setStreamError(null);

    // Create assistant placeholder message
    const assistantMessageId = `assistant_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;
    const assistantMessage: ChatMessage = {
      id: assistantMessageId,
      role: 'assistant',
      content: '',
      timestamp: Date.now()
    };
    setCurrentAssistantMessageId(assistantMessageId);
    await addMessage(assistantMessage);
    startFirstTokenWatchdog(assistantMessageId, runId);

    // Build LLM messages (system + recent history + user message)
    // Filter out messages with empty content (like the just-added assistant placeholder)
    const llmMessages: llm.Message[] = [
      { role: 'system', content: systemPrompt },
      ...messages.slice(-10).filter(msg => msg.content && msg.content.trim()).map(msg => ({
        role: msg.role as 'user' | 'assistant',
        content: msg.content
      })),
      { role: 'user', content: userMessage }
    ];
    console.log('[useStreamChat] Starting stream with messages:', JSON.stringify(llmMessages));

    // Create streaming request with tools if provided
    const streamOptions: llm.ChatCompletionsRequest = {
      model: llm.Model.BASE,
      messages: llmMessages,
      ...(mcpTools.length > 0 && { tools: mcpTools })
    };

    const stream = llm.streamChatCompletions(streamOptions);

    // Track accumulated state
    let accumulatedToolCalls: llm.ToolCall[] = [];
    let accumulatedContent = '';
    let toolCallsCompleted = false;

    subscriptionRef.current = stream.subscribe({
      next: async (chunk: llm.ChatCompletionsResponse<StreamingChunkChoice>) => {
        if (!isRunActive()) {
          return;
        }
        if (firstTokenTimerRef.current) {
          clearTimeout(firstTokenTimerRef.current);
          firstTokenTimerRef.current = null;
        }
        resetInactivityWatchdog(assistantMessageId, runId);

        console.log('[useStreamChat] Received chunk:', JSON.stringify(chunk));
        const choice = chunk.choices[0];
        const delta = choice?.delta;

        if (delta && llm.isToolCallsMessage(delta)) {
          // Accumulate tool calls from chunks
          for (const tc of delta.tool_calls) {
            const index = tc.index ?? 0;
            if (!accumulatedToolCalls[index]) {
              accumulatedToolCalls[index] = {
                id: tc.id ?? '',
                type: 'function',
                function: { name: tc.function?.name ?? '', arguments: tc.function?.arguments ?? '' }
              };
            } else {
              if (tc.function?.name) {
                accumulatedToolCalls[index].function.name += tc.function.name;
              }
              if (tc.function?.arguments) {
                accumulatedToolCalls[index].function.arguments += tc.function.arguments;
              }
            }
          }
        } else if (choice?.finish_reason === 'tool_calls' && !toolCallsCompleted) {
          toolCallsCompleted = true;
          setIsExecutingTools(true);

          // Execute tool calls via callback
          let toolResults: ExtendedToolExecutionResult[];
          try {
            clearWatchdogs();
            toolResults = await onToolCallsComplete(accumulatedToolCalls, runAbortSignal);
          } catch (err: unknown) {
            if (!isRunActive()) {
              return;
            }
            const error = err instanceof Error ? err : new Error(String(err));
            setStreamError(error);
            setIsExecutingTools(false);
            setIsStreaming(false);
            setCurrentAssistantMessageId(null);
            abortControllerRef.current = null;
            clearWatchdogs();
            void updateMessage(
              assistantMessageId,
              'Tool execution failed. Please try again.',
              undefined,
              { persist: true }
            ).catch((persistErr) => console.error('Failed to persist tool execution error message:', persistErr));
            return;
          }
          if (!isRunActive()) {
            return;
          }

          // Convert tool results to ToolExecutionResult for UI tracking
          const completedToolExecutions: ToolExecutionResult[] = toolResults.map(r => ({
            toolName: r.toolName,
            status: r.status,
            errorMessage: r.errorMessage
          }));

          setIsExecutingTools(false);

          // Build assistant message with tool calls for follow-up
          const assistantMessageWithToolCalls: llm.Message = {
            role: 'assistant',
            content: undefined,
            tool_calls: accumulatedToolCalls
          };

          // Build tool result messages
          const toolResultMessages: llm.Message[] = toolResults.map(result => ({
            role: 'tool' as const,
            tool_call_id: result.toolCallId,
            content: result.content
          }));

          // Follow-up request with tool results (no tools to prevent recursion)
          const followUpRequest: llm.ChatCompletionsRequest = {
            model: llm.Model.BASE,
            messages: [
              ...llmMessages,
              assistantMessageWithToolCalls,
              ...toolResultMessages
            ]
            // No tools in follow-up to prevent recursive tool calls
          };

          const finalStream = llm.streamChatCompletions(followUpRequest);
          const accumulatedStream = finalStream.pipe(llm.accumulateContent());
          let finalContent = '';
          startFirstTokenWatchdog(assistantMessageId, runId);

          subscriptionRef.current = accumulatedStream.subscribe({
            next: (content: string) => {
              if (!isRunActive()) {
                return;
              }
              if (firstTokenTimerRef.current) {
                clearTimeout(firstTokenTimerRef.current);
                firstTokenTimerRef.current = null;
              }
              resetInactivityWatchdog(assistantMessageId, runId);
              finalContent = content;
              void updateMessage(assistantMessageId, content, completedToolExecutions, { persist: false });
            },
            complete: () => {
              if (!isRunActive()) {
                return;
              }
              if (finalContent) {
                void updateMessage(assistantMessageId, finalContent, completedToolExecutions, { persist: true })
                  .catch((err) => console.error('Failed to persist final tool response:', err));
              }
              setIsExecutingTools(false);
              setIsStreaming(false);
              setCurrentAssistantMessageId(null);
              abortControllerRef.current = null;
              subscriptionRef.current = null;
              clearWatchdogs();
            },
            error: (err: unknown) => {
              if (!isRunActive()) {
                return;
              }
              const error = err instanceof Error ? err : new Error(String(err));
              console.error('Final streaming error:', error);
              setStreamError(error);
              setIsExecutingTools(false);
              setIsStreaming(false);
              setCurrentAssistantMessageId(null);
              abortControllerRef.current = null;
              clearWatchdogs();
              void updateMessage(
                assistantMessageId,
                'Error getting final response after tool execution. Please try again.',
                undefined,
                { persist: true }
              ).catch((persistErr) => console.error('Failed to persist final stream error message:', persistErr));
            }
          });
        } else if (delta && llm.isContentMessage(delta)) {
          if (!isRunActive()) {
            return;
          }
          accumulatedContent += delta.content;
          void updateMessage(assistantMessageId, accumulatedContent, undefined, { persist: false });
        }
      },
      complete: () => {
        if (!isRunActive()) {
          return;
        }
        console.log('[useStreamChat] Stream completed, toolCallsCompleted:', toolCallsCompleted, 'accumulatedContent length:', accumulatedContent.length);
        // Only set streaming to false if we haven't started a follow-up stream
        if (!toolCallsCompleted) {
          // Ensure final content is saved before completing
          if (accumulatedContent) {
            void updateMessage(assistantMessageId, accumulatedContent, undefined, { persist: true })
              .catch((err) => console.error('Failed to persist final streamed content:', err));
          }
          setIsExecutingTools(false);
          setIsStreaming(false);
          setCurrentAssistantMessageId(null);
          abortControllerRef.current = null;
          clearWatchdogs();
        }
        subscriptionRef.current = null;
      },
      error: (err: unknown) => {
        if (!isRunActive()) {
          return;
        }
        console.error('[useStreamChat] Stream error:', err);
        const error = err instanceof Error ? err : new Error(String(err));
        setStreamError(error);
        setIsExecutingTools(false);
        setIsStreaming(false);
        setCurrentAssistantMessageId(null);
        abortControllerRef.current = null;
        clearWatchdogs();
        void updateMessage(assistantMessageId, 'Error processing your request. Please try again.', undefined, { persist: true })
          .catch((persistErr) => console.error('Failed to persist stream error message:', persistErr));
      }
    });
  }, [
    addMessage,
    clearWatchdogs,
    messages,
    resetInactivityWatchdog,
    setCurrentAssistantMessageId,
    setIsExecutingTools,
    setIsStreaming,
    setStreamError,
    startFirstTokenWatchdog,
    updateMessage
  ]);

  /**
   * Cancel the current stream and cleanup all resources.
   */
  const cancelStream = useCallback(() => {
    // Invalidate current run so stale async callbacks no-op.
    runIdRef.current += 1;

    subscriptionRef.current?.unsubscribe();
    subscriptionRef.current = null;
    abortControllerRef.current?.abort();
    abortControllerRef.current = null;
    clearWatchdogs();
    setIsExecutingTools(false);
    setStreamError(null);
    setIsStreaming(false);
    setCurrentAssistantMessageId(null);
  }, [clearWatchdogs, setIsStreaming, setCurrentAssistantMessageId, setIsExecutingTools, setStreamError]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      subscriptionRef.current?.unsubscribe();
      abortControllerRef.current?.abort();
      clearWatchdogs();
    };
  }, [clearWatchdogs]);

  return {
    isStreaming,
    streamError,
    startStream,
    cancelStream,
  };
}
