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

import React, { createContext, useContext, useState, ReactNode } from 'react';
import { ChatMessage } from '../types/chat';

/**
 * Result of a tool execution, used for tracking tool call outcomes.
 */
export interface ToolExecutionResult {
  toolName: string;
  status: 'success' | 'error';
  errorMessage?: string;
}

/**
 * The shape of the ChatContext value.
 * Provides shared state for streaming, tool execution, and message management.
 */
export interface ChatContextValue {
  // Message state (managed by useChatSession, passed through context)
  messages: ChatMessage[];
  addMessage: (msg: ChatMessage) => Promise<void>;
  updateMessage: (
    id: string,
    content: string,
    toolExecutions?: ToolExecutionResult[],
    options?: { persist?: boolean }
  ) => Promise<void>;

  // Streaming state
  isStreaming: boolean;
  setIsStreaming: (value: boolean) => void;
  streamError: Error | null;
  setStreamError: (error: Error | null) => void;

  // Tool execution state
  isExecutingTools: boolean;
  setIsExecutingTools: (value: boolean) => void;
  toolError: Error | null;
  setToolError: (error: Error | null) => void;

  // Current assistant message tracking (for cancellation)
  currentAssistantMessageId: string | null;
  setCurrentAssistantMessageId: (id: string | null) => void;
}

/**
 * React context for chat state. Null by default, must be used within ChatProvider.
 */
const ChatContext = createContext<ChatContextValue | null>(null);

/**
 * Hook to access the ChatContext. Throws if used outside ChatProvider.
 */
export function useChatContext(): ChatContextValue {
  const ctx = useContext(ChatContext);
  if (!ctx) {
    throw new Error('useChatContext must be used within ChatProvider');
  }
  return ctx;
}

/**
 * Props for the ChatProvider component.
 */
interface ChatProviderProps {
  children: ReactNode;
  messages: ChatMessage[];
  addMessage: (msg: ChatMessage) => Promise<void>;
  updateMessage: (
    id: string,
    content: string,
    toolExecutions?: ToolExecutionResult[],
    options?: { persist?: boolean }
  ) => Promise<void>;
}

/**
 * Provider component that wraps the chat UI and provides shared state.
 * Message management functions are passed in from useChatSession hook.
 * Streaming and tool execution state are managed internally.
 */
export function ChatProvider({ children, messages, addMessage, updateMessage }: ChatProviderProps) {
  const [isStreaming, setIsStreaming] = useState(false);
  const [streamError, setStreamError] = useState<Error | null>(null);
  const [isExecutingTools, setIsExecutingTools] = useState(false);
  const [toolError, setToolError] = useState<Error | null>(null);
  const [currentAssistantMessageId, setCurrentAssistantMessageId] = useState<string | null>(null);

  const value: ChatContextValue = {
    messages,
    addMessage,
    updateMessage,
    isStreaming,
    setIsStreaming,
    streamError,
    setStreamError,
    isExecutingTools,
    setIsExecutingTools,
    toolError,
    setToolError,
    currentAssistantMessageId,
    setCurrentAssistantMessageId,
  };

  return <ChatContext.Provider value={value}>{children}</ChatContext.Provider>;
}

export { ChatContext };
