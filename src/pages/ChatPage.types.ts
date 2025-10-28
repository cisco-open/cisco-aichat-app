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

import { MCPTool } from '../types/mcp';
import { ChatMessage as ChatMessageType } from '../types/chat';
import { useChatSession } from '../hooks/useChatSession';

/** MCP integration state */
export interface MCPIntegrationState {
  available: boolean;
  toolCount: number;
  message: string;
  tools: MCPTool[];
}

/** Props for ChatPageContent component */
export interface ChatPageContentProps {
  currentSession: ReturnType<typeof useChatSession>['currentSession'];
  messages: ChatMessageType[];
  createNewSession: () => Promise<void>;
  renameSession: (id: string, name: string) => Promise<void>;
  addMessage: (msg: ChatMessageType) => Promise<void>;
  llmEnabled: boolean | null;
  setError: (error: string | null) => void;
  mcpIntegration: MCPIntegrationState;
  sessionLoading: boolean;
  sessionError: string | null;
  error: string | null;
  authExpired: boolean;
  onSignInAgain: () => void;
  onReload: () => void;
  showSidebar: boolean;
  setShowSidebar: (show: boolean) => void;
  showSettings: boolean;
  setShowSettings: (show: boolean) => void;
  allSessions: ReturnType<typeof useChatSession>['allSessions'];
  switchSession: ReturnType<typeof useChatSession>['switchSession'];
  deleteSession: ReturnType<typeof useChatSession>['deleteSession'];
  clearAllHistory: ReturnType<typeof useChatSession>['clearAllHistory'];
  isCreatingSession: boolean;
  isSwitchingSession: boolean;
  isDeletingSession: boolean;
  isRenamingSession: boolean;
  isClearingHistory: boolean;
  // Welcome prompt for returning users
  showWelcomePrompt: boolean;
  lastSessionForPrompt: ReturnType<typeof useChatSession>['lastSessionForPrompt'];
  continueLastSession: ReturnType<typeof useChatSession>['continueLastSession'];
  startNewSessionFromPrompt: ReturnType<typeof useChatSession>['startNewSessionFromPrompt'];
  // Context usage
  contextUsage: { usedTokens: number; maxTokens: number } | null;
  onCompactClick: () => Promise<void>;
  isCompacting: boolean;
}
