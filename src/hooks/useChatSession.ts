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

import { useState, useEffect, useCallback } from 'react';
import { ChatMessage, ChatSession } from '../types/chat';
import { ChatHistoryService } from '../services/ChatHistoryService';
import { isAuthError, isNotFoundError } from '../services/ChatErrors';

export interface UseChatSessionResult {
  // Current session data
  currentSession: ChatSession | null;
  messages: ChatMessage[];

  // All sessions
  allSessions: ChatSession[];

  // Session management
  createNewSession: (name?: string) => Promise<void>;
  switchSession: (sessionId: string) => Promise<void>;
  deleteSession: (sessionId: string) => Promise<void>;
  renameSession: (sessionId: string, newName: string) => Promise<void>;
  clearAllHistory: () => Promise<void>;

  // Message management
  addMessage: (message: ChatMessage) => Promise<void>;
  updateMessage: (
    messageId: string,
    content: string,
    toolExecutions?: Array<{ toolName: string; status: 'success' | 'error'; errorMessage?: string }>,
    options?: { persist?: boolean }
  ) => Promise<void>;

  // Returning user experience
  showWelcomePrompt: boolean;
  lastSessionForPrompt: ChatSession | null;
  dismissWelcomePrompt: () => void;
  continueLastSession: () => Promise<void>;
  startNewSessionFromPrompt: () => Promise<void>;

  // State
  isLoading: boolean;
  isCreatingSession: boolean;
  isSwitchingSession: boolean;
  isDeletingSession: boolean;
  isRenamingSession: boolean;
  isClearingHistory: boolean;
  isAddingMessage: boolean;
  isUpdatingMessage: boolean;
  error: string | null;
  authExpired: boolean;
  clearAuthExpired: () => void;
}

// Session storage key for tracking if welcome choice was made
const WELCOME_CHOICE_KEY = 'aichat_welcome_choice_made';

export function useChatSession(): UseChatSessionResult {
  const [currentSession, setCurrentSession] = useState<ChatSession | null>(null);
  const [allSessions, setAllSessions] = useState<ChatSession[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isCreatingSession, setIsCreatingSession] = useState(false);
  const [isSwitchingSession, setIsSwitchingSession] = useState(false);
  const [isDeletingSession, setIsDeletingSession] = useState(false);
  const [isRenamingSession, setIsRenamingSession] = useState(false);
  const [isClearingHistory, setIsClearingHistory] = useState(false);
  const [isAddingMessage, setIsAddingMessage] = useState(false);
  const [isUpdatingMessage, setIsUpdatingMessage] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [authExpired, setAuthExpired] = useState(false);

  // Returning user experience state
  const [showWelcomePrompt, setShowWelcomePrompt] = useState(false);
  const [lastSessionForPrompt, setLastSessionForPrompt] = useState<ChatSession | null>(null);

  const handleAuthAwareError = useCallback((err: unknown, fallbackMessage: string) => {
    if (isAuthError(err)) {
      setAuthExpired(true);
      setError('Your session has expired. Sign in again and reload.');
      return;
    }
    setError(fallbackMessage);
  }, []);

  const clearAuthExpired = useCallback(() => {
    setAuthExpired(false);
    setError(null);
  }, []);

  // Load initial session and all sessions
  useEffect(() => {
    const loadInitialData = async () => {
      try {
        setIsLoading(true);
        const historyService = await ChatHistoryService.getInstance();
        const activeSession = await historyService.getActiveSession();
        const sessions = await historyService.getAllSessions();

        // Check if this is a returning user
        const welcomeChoiceMade = sessionStorage.getItem(WELCOME_CHOICE_KEY);
        const isReturningUser = activeSession !== null && !welcomeChoiceMade;

        if (isReturningUser) {
          // Show welcome prompt instead of auto-loading the session
          setShowWelcomePrompt(true);
          setLastSessionForPrompt(activeSession);
          // Don't set currentSession yet - user needs to choose
          setCurrentSession(null);
        } else {
          // Normal flow - load the session
          setCurrentSession(activeSession);
        }

        setAllSessions(sessions);
        setAuthExpired(false);
        setError(null);
      } catch (err) {
        console.error('Failed to load chat session:', err);
        handleAuthAwareError(err, 'Failed to load chat history');
      } finally {
        setIsLoading(false);
      }
    };

    loadInitialData();
  }, [handleAuthAwareError]);

  // Keep current session aligned with sidebar session list after background reconciliation.
  useEffect(() => {
    if (isLoading || !currentSession) {
      return;
    }
    const stillPresent = allSessions.some((session) => session.id === currentSession.id);
    if (!stillPresent) {
      setCurrentSession(allSessions.length > 0 ? allSessions[0] : null);
    }
  }, [allSessions, currentSession, isLoading]);

  // Get messages from current session
  const messages = currentSession?.messages || [];

  // Create a new session
  const createNewSession = useCallback(async (name?: string) => {
    if (authExpired) {
      setError('Your session has expired. Sign in again and reload.');
      return;
    }
    try {
      setIsCreatingSession(true);
      const historyService = await ChatHistoryService.getInstance();
      const newSession = await historyService.createSession(name);
      const updatedSessions = await historyService.getAllSessions();

      setCurrentSession(newSession);
      setAllSessions(updatedSessions);
      setAuthExpired(false);
      setError(null);
    } catch (err) {
      console.error('Failed to create new session:', err);
      handleAuthAwareError(err, 'Failed to create new session');
    } finally {
      setIsCreatingSession(false);
    }
  }, [authExpired, handleAuthAwareError]);

  // Switch to a different session
  const switchSession = useCallback(async (sessionId: string) => {
    if (authExpired) {
      setError('Your session has expired. Sign in again and reload.');
      return;
    }
    try {
      setIsSwitchingSession(true);
      const historyService = await ChatHistoryService.getInstance();
      const session = await historyService.switchToSession(sessionId);
      if (session) {
        setCurrentSession(session);
        setAuthExpired(false);
        setError(null);
      } else {
        setError('Session not found');
      }
    } catch (err) {
      console.error('Failed to switch session:', err);
      handleAuthAwareError(err, 'Failed to switch session');
    } finally {
      setIsSwitchingSession(false);
    }
  }, [authExpired, handleAuthAwareError]);

  // Delete a session
  const deleteSession = useCallback(async (sessionId: string) => {
    if (authExpired) {
      setError('Your session has expired. Sign in again and reload.');
      return;
    }
    try {
      setIsDeletingSession(true);
      const historyService = await ChatHistoryService.getInstance();
      const success = await historyService.deleteSession(sessionId);
      if (success) {
        const updatedSessions = await historyService.getAllSessions();
        setAllSessions(updatedSessions);

        // If we deleted the current session, get the new active session
        if (currentSession?.id === sessionId) {
          if (updatedSessions.length > 0) {
            const newActiveSession = await historyService.getActiveSession();
            setCurrentSession(newActiveSession);
          } else {
            setCurrentSession(null);
          }
        }
        setAuthExpired(false);
        setError(null);
      } else {
        // Session can already be gone in backend (stale sidebar state/race). Re-sync before surfacing error.
        const refreshedSessions = await historyService.getAllSessions();
        const sessionStillExists = refreshedSessions.some((s) => s.id === sessionId);
        setAllSessions(refreshedSessions);

        if (!sessionStillExists) {
          if (currentSession?.id === sessionId) {
            if (refreshedSessions.length > 0) {
              const newActiveSession = await historyService.getActiveSession();
              setCurrentSession(newActiveSession);
            } else {
              setCurrentSession(null);
            }
          }
          setAuthExpired(false);
          setError(null);
        } else {
          setError('Failed to delete session');
        }
      }
    } catch (err) {
      console.error('Failed to delete session:', err);
      handleAuthAwareError(err, 'Failed to delete session');
    } finally {
      setIsDeletingSession(false);
    }
  }, [authExpired, currentSession, handleAuthAwareError]);

  // Rename a session
  const renameSession = useCallback(async (sessionId: string, newName: string) => {
    if (authExpired) {
      setError('Your session has expired. Sign in again and reload.');
      return;
    }
    try {
      setIsRenamingSession(true);
      const historyService = await ChatHistoryService.getInstance();

      // Optimistic update
      if (currentSession?.id === sessionId) {
        setCurrentSession(prev => prev ? { ...prev, name: newName, updatedAt: Date.now() } : null);
      }
      setAllSessions(prev => prev.map(s =>
        s.id === sessionId ? { ...s, name: newName, updatedAt: Date.now() } : s
      ));

      const success = await historyService.renameSession(sessionId, newName);
      if (success) {
        const updatedSessions = await historyService.getAllSessions();
        setAllSessions(updatedSessions);
        setAuthExpired(false);
        setError(null);
      } else {
        setError('Failed to rename session');
        // Revert optimistic update
        const historyServiceRevert = await ChatHistoryService.getInstance();
        const revertedSessions = await historyServiceRevert.getAllSessions();
        setAllSessions(revertedSessions);
        if (currentSession?.id === sessionId) {
          const revertedSession = revertedSessions.find((s: ChatSession) => s.id === sessionId);
          if (revertedSession) {
            setCurrentSession(revertedSession);
          }
        }
      }
    } catch (err) {
      console.error('Failed to rename session:', err);
      handleAuthAwareError(err, 'Failed to rename session');
      // Revert optimistic update
      const historyService = await ChatHistoryService.getInstance();
      const revertedSessions = await historyService.getAllSessions();
      setAllSessions(revertedSessions);
      if (currentSession?.id === sessionId) {
        const revertedSession = revertedSessions.find((s: ChatSession) => s.id === sessionId);
        if (revertedSession) {
          setCurrentSession(revertedSession);
        }
      }
    } finally {
      setIsRenamingSession(false);
    }
  }, [authExpired, currentSession, handleAuthAwareError]);

  // Clear all history
  const clearAllHistory = useCallback(async () => {
    if (authExpired) {
      setError('Your session has expired. Sign in again and reload.');
      return;
    }
    try {
      setIsClearingHistory(true);
      const historyService = await ChatHistoryService.getInstance();
      await historyService.clearAllHistory();
      setCurrentSession(null);
      setAllSessions([]);
      setAuthExpired(false);
      setError(null);
    } catch (err) {
      console.error('Failed to clear history:', err);
      handleAuthAwareError(err, 'Failed to clear history');
    } finally {
      setIsClearingHistory(false);
    }
  }, [authExpired, handleAuthAwareError]);

  // Dismiss the welcome prompt and mark choice as made
  const dismissWelcomePrompt = useCallback(() => {
    sessionStorage.setItem(WELCOME_CHOICE_KEY, 'true');
    setShowWelcomePrompt(false);
    setLastSessionForPrompt(null);
  }, []);

  // Continue with the last session (from welcome prompt)
  const continueLastSession = useCallback(async () => {
    if (lastSessionForPrompt) {
      sessionStorage.setItem(WELCOME_CHOICE_KEY, 'true');
      setShowWelcomePrompt(false);
      setCurrentSession(lastSessionForPrompt);
      setLastSessionForPrompt(null);
    }
  }, [lastSessionForPrompt]);

  // Start a new session from the welcome prompt
  const startNewSessionFromPrompt = useCallback(async () => {
    sessionStorage.setItem(WELCOME_CHOICE_KEY, 'true');
    setShowWelcomePrompt(false);
    setLastSessionForPrompt(null);
    await createNewSession();
  }, [createNewSession]);

  // Add a message to the current session
  const addMessage = useCallback(async (message: ChatMessage) => {
    if (authExpired) {
      setError('Your session has expired. Sign in again and reload.');
      return;
    }
    try {
      setIsAddingMessage(true);
      const historyService = await ChatHistoryService.getInstance();

      // Optimistic update
      setCurrentSession(prev => {
        if (!prev) {
          return null;
        }
        return {
          ...prev,
          messages: [...prev.messages, message],
          updatedAt: Date.now()
        };
      });

      await historyService.addMessage(message, currentSession?.id);

      // Update sessions list to reflect the updated timestamp
      const updatedSessions = await historyService.getAllSessions();
      setAllSessions(updatedSessions);
      setAuthExpired(false);
      setError(null);
    } catch (err) {
      console.error('Failed to add message:', err);
      if (isAuthError(err)) {
        handleAuthAwareError(err, 'Failed to save message');
        throw err;
      }
      if (isNotFoundError(err)) {
        // Session disappeared in backend; reload canonical session list and recover UI state.
        const historyService = await ChatHistoryService.getInstance();
        const refreshedSessions = await historyService.getAllSessions();
        setAllSessions(refreshedSessions);
        if (refreshedSessions.length === 0) {
          setCurrentSession(null);
        } else {
          const nextActive = await historyService.getActiveSession();
          setCurrentSession(nextActive);
        }
        setError('Session no longer exists. Please continue in an active chat.');
        throw err;
      }
      handleAuthAwareError(err, 'Failed to save message');
      // Revert optimistic update
      const historyService = await ChatHistoryService.getInstance();
      const revertedSession = await historyService.getActiveSession();
      setCurrentSession(revertedSession);
    } finally {
      setIsAddingMessage(false);
    }
  }, [authExpired, currentSession?.id, handleAuthAwareError]);

  // Update a message in the current session
  const updateMessage = useCallback(async (
    messageId: string,
    content: string,
    toolExecutions?: Array<{ toolName: string; status: 'success' | 'error'; errorMessage?: string }>,
    options?: { persist?: boolean }
  ) => {
    if (authExpired) {
      setError('Your session has expired. Sign in again and reload.');
      return;
    }
    const persist = options?.persist !== false;

    // Always apply optimistic UI update first.
    setCurrentSession(prev => {
      if (!prev) {
        return null;
      }
      return {
        ...prev,
        messages: prev.messages.map(msg =>
          msg.id === messageId ? { ...msg, content, ...(toolExecutions && { toolExecutions }) } : msg
        ),
        updatedAt: Date.now()
      };
    });

    // For high-frequency streaming chunks we keep updates local-only and defer persistence.
    if (!persist) {
      return;
    }

    try {
      setIsUpdatingMessage(true);
      const historyService = await ChatHistoryService.getInstance();

      await historyService.updateMessage(messageId, content, toolExecutions, currentSession?.id);

      // Update sessions list to reflect the updated timestamp
      const updatedSessions = await historyService.getAllSessions();
      setAllSessions(updatedSessions);
      setAuthExpired(false);
      setError(null);
    } catch (err) {
      console.error('Failed to update message:', err);
      if (isAuthError(err)) {
        setAuthExpired(true);
        setError('Your session has expired. Sign in again and reload.');
        throw err;
      } else if (isNotFoundError(err)) {
        const historyService = await ChatHistoryService.getInstance();
        const refreshedSessions = await historyService.getAllSessions();
        setAllSessions(refreshedSessions);
        if (refreshedSessions.length === 0) {
          setCurrentSession(null);
        } else {
          const nextActive = await historyService.getActiveSession();
          setCurrentSession(nextActive);
        }
        setError('Session no longer exists. Please continue in an active chat.');
        throw err;
      } else {
        const errorMessage = err instanceof Error ? err.message : String(err);
        setError(errorMessage || 'Failed to update message');
      }
      // Revert optimistic update
      const historyService = await ChatHistoryService.getInstance();
      const revertedSession = await historyService.getActiveSession();
      setCurrentSession(revertedSession);
    } finally {
      setIsUpdatingMessage(false);
    }
  }, [authExpired, currentSession?.id]);

  return {
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
    showWelcomePrompt,
    lastSessionForPrompt,
    dismissWelcomePrompt,
    continueLastSession,
    startNewSessionFromPrompt,
    isLoading,
    isCreatingSession,
    isSwitchingSession,
    isDeletingSession,
    isRenamingSession,
    isClearingHistory,
    isAddingMessage,
    isUpdatingMessage,
    error,
    authExpired,
    clearAuthExpired
  };
}
