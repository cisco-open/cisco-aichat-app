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

export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  timestamp: number;
  toolExecutions?: Array<{
    toolName: string;
    status: 'success' | 'error';
    errorMessage?: string;
  }>;
  // Summary message fields (used for collapsed summarized messages)
  isSummary?: boolean;
  summarizedIds?: string[];
  summaryDepth?: number;
}

export interface ChatSession {
  id: string;
  name: string;
  userId: string;
  messages: ChatMessage[];
  createdAt: number;
  updatedAt: number;
  isActive: boolean;
}

export interface UserChatHistory {
  userId: string;
  sessions: ChatSession[];
  activeSessionId: string | null;
}

export interface ChatHistoryStorage {
  [userId: string]: UserChatHistory;
}

export interface ChatSettings {
  maxSessionsPerUser: number;
  maxMessagesPerSession: number;
  systemPrompt: string;
  enableModelSelection: boolean;
  enableMcpTools: boolean;
  autoGenerateSessionNames: boolean;
}

/**
 * Search result from backend FTS search
 * Content includes backend-generated <mark> tokens for highlighting matches
 */
export interface SearchResult {
  sessionId: string;
  sessionName: string;
  messageId: string;
  content: string;  // Text with <mark> highlight tokens
  timestamp: number;
  role: string;
}

export const defaultChatSettings: ChatSettings = {
  maxSessionsPerUser: 50,
  maxMessagesPerSession: 500,
  systemPrompt: 'You are an expert SRE and monitoring assistant with deep knowledge of Grafana, Prometheus, and observability best practices.',
  enableModelSelection: false,
  enableMcpTools: true,
  autoGenerateSessionNames: true,
};
