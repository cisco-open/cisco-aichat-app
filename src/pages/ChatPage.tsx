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

import React, { useState, useRef, useEffect } from 'react';
import { Button, Input, Spinner, useStyles2, Alert, ConfirmModal } from '@grafana/ui';
import { getStyles } from './ChatPage.styles';
import { MCPIntegrationState, ChatPageContentProps } from './ChatPage.types';
import { PluginPage } from '@grafana/runtime';
import { llm } from '@grafana/llm';
import { testIds } from '../components/testIds';
import { MCPIntegrationService } from '../services/MCPIntegrationService';
import { ChatMessage as ChatMessageType } from '../types/chat';
import { useChatSession } from '../hooks/useChatSession';
import { useStreamChat, ExtendedToolExecutionResult } from '../hooks/useStreamChat';
import { useToolExecution } from '../hooks/useToolExecution';
import { ChatProvider } from '../context/ChatContext';
import { ChatSidebar } from '../components/ChatSidebar';
import { ChatHistoryService } from '../services/ChatHistoryService';
import { ChatSettingsService } from '../services/ChatSettingsService';
import { ChatSettingsModal } from '../components/ChatSettingsModal';
import { PermissionGuard } from '../components/PermissionGuard';
import { ToolExecutionIndicator } from '../components/ToolExecutionIndicator';
import { ChatHeader } from '../components/ChatHeader';
import { WelcomeBackPrompt } from '../components/WelcomeBackPrompt';
import { ContextUsageGauge } from '../components/ContextUsageGauge';
import { ChatBackendService } from '../services/ChatBackendService';
import { ChatMessage } from '../components/ChatMessage';
import { isAuthError, isNotFoundError } from '../services/ChatErrors';

/**
 * Inner component that uses the chat hooks.
 * Must be wrapped in ChatProvider.
 */
function ChatPageContent(props: ChatPageContentProps) {
  const {
    currentSession,
    messages: _messages, // Still passed but using paginatedMessages from hook
    createNewSession,
    renameSession,
    addMessage,
    llmEnabled,
    setError,
    mcpIntegration,
    sessionLoading,
    sessionError,
    error,
    authExpired,
    onSignInAgain,
    onReload,
    showSidebar,
    setShowSidebar,
    showSettings,
    setShowSettings,
    allSessions,
    switchSession,
    deleteSession,
    clearAllHistory,
    isCreatingSession,
    isSwitchingSession,
    isDeletingSession,
    isRenamingSession,
    isClearingHistory,
    // Welcome prompt for returning users
    showWelcomePrompt,
    lastSessionForPrompt,
    continueLastSession,
    startNewSessionFromPrompt,
    // Context usage
    contextUsage,
    onCompactClick,
    isCompacting,
  } = props;
  const s = useStyles2(getStyles);
  const [input, setInput] = useState('');
  const [showCompactConfirm, setShowCompactConfirm] = useState(false);
  const messagesContainerRef = useRef<HTMLDivElement>(null);
  const { isStreaming, streamError, startStream, cancelStream } = useStreamChat();
  const { toolExecutions, clearExecutions } = useToolExecution();

  // Use messages directly from useChatSession - single source of truth
  // (Removed useInfiniteMessages to fix sync issues causing duplicate messages)

  const handleCompactClick = () => {
    if (isStreaming) {
      return; // Prevent compaction during streaming
    }
    setShowCompactConfirm(true);
  };

  const handleCompactConfirm = () => {
    setShowCompactConfirm(false);
    onCompactClick?.();
  };

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    if (messagesContainerRef.current) {
      messagesContainerRef.current.scrollTop = messagesContainerRef.current.scrollHeight;
    }
  }, [_messages]);

  // Update error state when stream error occurs
  useEffect(() => {
    if (streamError) {
      if (isAuthError(streamError)) {
        setError('Your session has expired. Sign in again and reload.');
      } else {
        setError('Failed to get response from AI assistant. Please try again.');
      }
    }
  }, [streamError, setError]);

  const sendMessage = async () => {
    if (!input.trim() || isStreaming || authExpired) {
      return;
    }

    // Check LLM availability before sending
    if (llmEnabled === false) {
      setError('LLM service is not available. Please configure an LLM provider.');
      return;
    }

    // If no current session, create one
    if (!currentSession) {
      await createNewSession();
      return;
    }

    const userMessage: ChatMessageType = {
      id: `msg_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`,
      role: 'user',
      content: input.trim(),
      timestamp: Date.now()
    };

    try {
      // Auto-generate session name from first user message if it's the default name
      if (currentSession.messages.length === 1 && currentSession.name.startsWith('Chat ')) {
        const settingsService = await ChatSettingsService.getInstance();
        if (settingsService.isAutoGenerateSessionNamesEnabled()) {
          const historyService = await ChatHistoryService.getInstance();
          const smartName = historyService.generateSessionName(userMessage.content);
          await renameSession(currentSession.id, smartName);
        }
      }

      await addMessage(userMessage);
      // Note: sync effect handles adding to paginatedMessages
      setInput('');
      setError(null);

      // Get configured system prompt from settings
      const settingsService = await ChatSettingsService.getInstance();
      const baseSystemPrompt = settingsService.getSystemPrompt();
      const mcpToolsEnabled = settingsService.isMcpToolsEnabled();

      // Prepare MCP tools if available and enabled
      const mcpToolsForLLM = mcpIntegration.available && mcpIntegration.tools.length > 0 && mcpToolsEnabled
        ? MCPIntegrationService.convertToolsToOpenAI(mcpIntegration.tools)
        : [];

      // Enhanced system prompt with MCP capabilities if tools are available
      const systemPrompt = mcpIntegration.available && mcpIntegration.tools.length > 0 && mcpToolsEnabled
        ? `${baseSystemPrompt} You have access to ${mcpIntegration.toolCount} specialized tools from connected MCP servers that can help you provide more detailed and accurate information about monitoring systems, dashboards, and infrastructure.

When appropriate, use the available tools to:
- Query Grafana dashboards and data sources
- Retrieve monitoring metrics and alerts
- Analyze system performance and health
- Access configuration information

Provide helpful, accurate, and concise responses. When you use tools, explain what information you're gathering and how it helps answer the user's question.`
        : baseSystemPrompt;

      // Start streaming with tool execution callback
      await startStream(
        userMessage.content,
        systemPrompt,
        mcpToolsForLLM,
        async (toolCalls: llm.ToolCall[], abortSignal: AbortSignal): Promise<ExtendedToolExecutionResult[]> => {
          const results: ExtendedToolExecutionResult[] = [];

          for (const toolCall of toolCalls) {
            if (abortSignal.aborted) {
              break;
            }

            const toolName = toolCall.function.name;

            try {
              const result = await MCPIntegrationService.callTool({
                id: toolCall.id,
                type: 'function',
                function: {
                  name: toolName,
                  arguments: toolCall.function.arguments
                }
              }, abortSignal);

              if (abortSignal.aborted) {
                break;
              }

              results.push({
                toolName,
                toolCallId: toolCall.id,
                content: result.content,
                status: result.is_error ? 'error' : 'success',
                errorMessage: result.is_error ? result.content.substring(0, 100) : undefined
              });
            } catch (err: unknown) {
              if (abortSignal.aborted) {
                break;
              }

              const errorMessage = err instanceof Error ? err.message : String(err);
              results.push({
                toolName,
                toolCallId: toolCall.id,
                content: JSON.stringify({ error: 'Tool execution failed: ' + errorMessage }),
                status: 'error',
                errorMessage: errorMessage.substring(0, 100)
              });
            }
          }

          clearExecutions();
          return results;
        }
      );
    } catch (err) {
      console.error('Failed to send chat message:', err);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  };

  const handleCancelGeneration = () => {
    cancelStream();
    clearExecutions();
  };

  return (
    <div data-testid={testIds.chat.container} className={s.pageContainer}>
      {/* Chat Sidebar */}
      {showSidebar && (
        <ChatSidebar
          sessions={allSessions}
          currentSessionId={currentSession?.id || null}
          onSessionSelect={switchSession}
          onSessionCreate={createNewSession}
          onSessionDelete={deleteSession}
          onSessionRename={renameSession}
          onClearHistory={clearAllHistory}
          onCollapse={() => setShowSidebar(false)}
          isCreatingSession={isCreatingSession}
          isSwitchingSession={isSwitchingSession}
          isDeletingSession={isDeletingSession}
          isRenamingSession={isRenamingSession}
          isClearingHistory={isClearingHistory}
        />
      )}

      {/* Main Chat Area */}
      <div className={s.mainContent}>
        {/* Header */}
        <ChatHeader
          sessionName={currentSession?.name}
          showSidebar={showSidebar}
          onShowSettings={() => setShowSettings(true)}
          onToggleSidebar={() => setShowSidebar(!showSidebar)}
        />

        {/* Status Indicators */}
        {(llmEnabled === null || llmEnabled === false || error || sessionError) && (
          <div style={{ padding: '16px' }}>
            {llmEnabled === null && (
              <div className={s.statusIndicator}>
                <Spinner size="sm" /> Checking LLM availability...
              </div>
            )}
            {llmEnabled === false && (
              <Alert severity="warning" title="LLM Service Not Available">
                The LLM service is not configured or enabled. Please configure an LLM provider
                in the LLM App under Administration - Plugins - LLM App.
              </Alert>
            )}
            {sessionError && (
              <Alert severity="error" title="Session Error">
                {sessionError}
              </Alert>
            )}
            {error && (
              <Alert severity="error" title="Chat Error">
                {error}
              </Alert>
            )}
          </div>
        )}

        {authExpired && (
          <div style={{ padding: '16px' }}>
            <Alert severity="warning" title="Session expired">
              Your SSO session expired. Sign in again to continue using AI Chat.
              <div style={{ marginTop: '8px', display: 'flex', gap: '8px' }}>
                <Button size="sm" variant="primary" onClick={onSignInAgain}>
                  Sign in again
                </Button>
                <Button size="sm" variant="secondary" onClick={onReload}>
                  Reload
                </Button>
              </div>
            </Alert>
          </div>
        )}

        {/* MCP Integration Status */}
        {llmEnabled === true && (
          <div className={s.mcpStatusContainer}>
            <div className={s.statusIndicator}>
              {mcpIntegration.available
                ? <>MCP: {mcpIntegration.message}</>
                : <>Info: {mcpIntegration.message}</>
              }
            </div>
          </div>
        )}

        {/* Chat Content */}
        {sessionLoading ? (
          <div className={s.emptyState}>
            <Spinner size="lg" />
            <h2>Loading chat history...</h2>
          </div>
        ) : showWelcomePrompt && lastSessionForPrompt ? (
          <WelcomeBackPrompt
            lastSessionName={lastSessionForPrompt.name}
            onContinue={continueLastSession}
            onStartNew={startNewSessionFromPrompt}
          />
        ) : !currentSession ? (
          <div className={s.emptyState}>
            <h2>Welcome to AI Chat</h2>
            <p>Create a new chat session to get started with your AI assistant.</p>
            <Button onClick={() => createNewSession()} variant="primary">
              Start New Chat
            </Button>
          </div>
        ) : (
          <div className={s.chatContainer}>
            {/* Messages display - using _messages directly from useChatSession */}
            <div className={s.messagesContainer} ref={messagesContainerRef}>
              {_messages.map((message) => (
                <ChatMessage
                  key={message.id}
                  message={message}
                />
              ))}
              {/* Streaming indicator */}
              {isStreaming && _messages.length > 0 && _messages[_messages.length - 1]?.content === '' && (
                <div className={s.messageWrapper}>
                  <div className={s.assistantMessage}>
                    <div className={s.messageHeader}>
                      <strong>AI Assistant</strong>
                    </div>
                    <div className={s.messageContent}>
                      <Spinner size="sm" /> Thinking...
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* Tool Execution Indicator */}
            <ToolExecutionIndicator
              executions={toolExecutions}
              onDismiss={() => {}}
            />

            <div className={s.inputContainer}>
              {/* Context Usage Gauge */}
              {contextUsage && contextUsage.maxTokens > 0 && (
                <ContextUsageGauge
                  usedTokens={contextUsage.usedTokens}
                  maxTokens={contextUsage.maxTokens}
                  onCompactClick={!isStreaming ? handleCompactClick : undefined}
                  isCompacting={isCompacting}
                />
              )}
              <Input
                value={input}
                onChange={(e) => setInput(e.currentTarget.value)}
                onKeyDown={handleKeyPress}
                placeholder={llmEnabled === false ? "LLM service not available" : "Ask me about monitoring, alerts, or observability..."}
                disabled={isStreaming || llmEnabled === false || authExpired}
                className={s.input}
              />
              {isStreaming ? (
                <Button onClick={handleCancelGeneration} variant="destructive">
                  Cancel
                </Button>
              ) : (
                <Button
                  onClick={sendMessage}
                  disabled={!input.trim() || llmEnabled === false || authExpired}
                  variant="primary"
                >
                  Send
                </Button>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Chat Settings Modal */}
      <ChatSettingsModal
        isOpen={showSettings}
        onClose={() => setShowSettings(false)}
      />

      {/* Compact Confirmation Modal */}
      <ConfirmModal
        isOpen={showCompactConfirm}
        title="Compact Conversation"
        body="This will summarize older messages to free up context space. Recent messages will be preserved. This action cannot be undone."
        confirmText="Compact"
        onConfirm={handleCompactConfirm}
        onDismiss={() => setShowCompactConfirm(false)}
      />
    </div>
  );
}

/**
 * Main ChatPage component.
 * Wraps ChatPageContent with ChatProvider for shared state.
 */
function ChatPage() {
  const [showSidebar, setShowSidebar] = useState(true);
  const [showSettings, setShowSettings] = useState(false);
  const [llmEnabled, setLlmEnabled] = useState<boolean | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [mcpIntegration, setMcpIntegration] = useState<MCPIntegrationState>({
    available: false,
    toolCount: 0,
    message: 'Checking MCP integration...',
    tools: []
  });

  // Context usage state
  const [contextUsage, setContextUsage] = useState<{ usedTokens: number; maxTokens: number } | null>(null);
  const [isCompacting, setIsCompacting] = useState(false);

  // Use the chat session hook
  const {
    currentSession,
    messages,
    allSessions,
    createNewSession,
    switchSession,
    deleteSession,
    renameSession,
    clearAllHistory,
    addMessage,
    updateMessage,
    isLoading: sessionLoading,
    isCreatingSession,
    isSwitchingSession,
    isDeletingSession,
    isRenamingSession,
    isClearingHistory,
    error: sessionError,
    authExpired,
    clearAuthExpired,
    // Returning user experience
    showWelcomePrompt,
    lastSessionForPrompt,
    continueLastSession,
    startNewSessionFromPrompt,
  } = useChatSession();

  const onSignInAgain = () => {
    clearAuthExpired();
    window.location.assign('/login');
  };

  const onReload = () => {
    window.location.reload();
  };

  // Check LLM availability on mount
  useEffect(() => {
    const checkLLMAvailability = async () => {
      try {
        const enabled = await llm.enabled();
        setLlmEnabled(enabled);
        if (!enabled) {
          setError('LLM service is not configured. Please configure an LLM provider in the LLM App.');
        }
      } catch (err: unknown) {
        console.error('Error checking LLM availability:', err);
        setLlmEnabled(false);
        setError('Unable to connect to LLM service. Please check the LLM App configuration.');
      }
    };
    checkLLMAvailability();
  }, []);

  // Initialize MCP integration on mount
  useEffect(() => {
    const initializeMCPIntegration = async () => {
      try {
        const status = await MCPIntegrationService.getIntegrationStatus();
        const tools = status.available ? await MCPIntegrationService.getAvailableTools() : [];
        setMcpIntegration({
          available: status.available,
          toolCount: status.toolCount,
          message: status.message,
          tools: tools
        });
        console.log('MCP Integration initialized:', { status, toolCount: tools.length });
      } catch (err: unknown) {
        console.error('Error initializing MCP integration:', err);
        setMcpIntegration({
          available: false,
          toolCount: 0,
          message: 'Error connecting to MCP services',
          tools: []
        });
      }
    };
    initializeMCPIntegration();
  }, []);

  const lastMessage = messages.length > 0 ? messages[messages.length - 1] : null;
  const lastMessageSignature = lastMessage ? `${lastMessage.id}:${lastMessage.content.length}` : '';

  // Fetch token stats when session changes, message count changes, or latest message content grows.
  // Debounced to avoid excessive requests during streaming.
  useEffect(() => {
    const fetchTokenStats = async () => {
      if (!currentSession?.id) {
        setContextUsage(null);
        return;
      }
      try {
        const backendService = ChatBackendService.getInstance();
        const stats = await backendService.getSessionTokenStats(currentSession.id);
        setContextUsage({
          usedTokens: stats.totalTokens,
          maxTokens: stats.contextLimit,
        });
      } catch (err) {
        if (isNotFoundError(err)) {
          // Session was deleted or replaced; clear gauge and avoid noisy console errors.
          setContextUsage(null);
          return;
        }
        console.error('Failed to fetch token stats:', err);
        // Don't block UI on failure - just log and continue
      }
    };
    const timer = window.setTimeout(() => {
      void fetchTokenStats();
    }, 400);

    return () => window.clearTimeout(timer);
  }, [currentSession?.id, messages.length, lastMessageSignature]);

  // Handle compaction request
  const handleCompact = async () => {
    if (!currentSession?.id) {
      return;
    }
    setIsCompacting(true);
    try {
      const backendService = ChatBackendService.getInstance();
      await backendService.triggerCompaction(currentSession.id);
      // Refresh token stats after compaction
      const stats = await backendService.getSessionTokenStats(currentSession.id);
      setContextUsage({
        usedTokens: stats.totalTokens,
        maxTokens: stats.contextLimit,
      });
    } catch (err) {
      console.error('Compaction failed:', err);
      setError('Failed to compact conversation');
    } finally {
      setIsCompacting(false);
    }
  };

  return (
    <PermissionGuard appName="AI Chat Assistant">
      <PluginPage>
        <ChatProvider
          messages={messages}
          addMessage={addMessage}
          updateMessage={updateMessage}
        >
          <ChatPageContent
            currentSession={currentSession}
            messages={messages}
            createNewSession={createNewSession}
            renameSession={renameSession}
            addMessage={addMessage}
            llmEnabled={llmEnabled}
            setError={setError}
            mcpIntegration={mcpIntegration}
            sessionLoading={sessionLoading}
            sessionError={sessionError}
            error={error}
            authExpired={authExpired}
            onSignInAgain={onSignInAgain}
            onReload={onReload}
            showSidebar={showSidebar}
            setShowSidebar={setShowSidebar}
            showSettings={showSettings}
            setShowSettings={setShowSettings}
            allSessions={allSessions}
            switchSession={switchSession}
            deleteSession={deleteSession}
            clearAllHistory={clearAllHistory}
            isCreatingSession={isCreatingSession}
            isSwitchingSession={isSwitchingSession}
            isDeletingSession={isDeletingSession}
            isRenamingSession={isRenamingSession}
            isClearingHistory={isClearingHistory}
            showWelcomePrompt={showWelcomePrompt}
            lastSessionForPrompt={lastSessionForPrompt}
            continueLastSession={continueLastSession}
            startNewSessionFromPrompt={startNewSessionFromPrompt}
            contextUsage={contextUsage}
            onCompactClick={handleCompact}
            isCompacting={isCompacting}
          />
        </ChatProvider>
      </PluginPage>
    </PermissionGuard>
  );
}

export default ChatPage;
