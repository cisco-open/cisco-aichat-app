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

import { config } from '@grafana/runtime';
import { ChatMessage, ChatSession, UserChatHistory } from '../types/chat';
import { ChatSettingsService } from './ChatSettingsService';
import { ChatStorageService } from './ChatStorageService';
import { AuthError } from './ChatErrors';

export class ChatHistoryService {
  private static instance: ChatHistoryService | null = null;
  private static initPromise: Promise<ChatHistoryService> | null = null;
  private settingsService: ChatSettingsService;
  private storageService: ChatStorageService;

  private constructor(settingsService: ChatSettingsService) {
    this.settingsService = settingsService;
    this.storageService = ChatStorageService.getInstance();
  }

  public static async getInstance(): Promise<ChatHistoryService> {
    if (ChatHistoryService.instance) {
      return ChatHistoryService.instance;
    }

    // If initialization is already in progress, wait for it
    if (ChatHistoryService.initPromise) {
      return ChatHistoryService.initPromise;
    }

    // Start initialization
    ChatHistoryService.initPromise = (async () => {
      const settingsService = await ChatSettingsService.getInstance();
      ChatHistoryService.instance = new ChatHistoryService(settingsService);
      return ChatHistoryService.instance;
    })();

    return ChatHistoryService.initPromise;
  }

  /**
   * Get the current Grafana user ID
   */
  private getCurrentUserId(): string {
    const user = config.bootData.user;
    // Keep frontend storage key aligned with backend auth identity (login).
    const userId = user?.login || user?.id?.toString();
    if (!userId) {
      throw new AuthError('Authentication required');
    }
    return userId;
  }

  /**
   * Get user's chat history
   */
  public async getUserHistory(): Promise<UserChatHistory> {
    const userId = this.getCurrentUserId();
    return await this.storageService.getUserHistory(userId);
  }

  /**
   * Create a new chat session
   * Uses dedicated storageService.createSession() which calls POST /sessions on backend
   */
  public async createSession(name?: string): Promise<ChatSession> {
    const userId = this.getCurrentUserId();
    const userHistory = await this.storageService.getUserHistory(userId);

    // Enforce max sessions limit before creating
    const maxSessions = this.settingsService.getMaxSessionsPerUser();
    if (userHistory.sessions.length >= maxSessions) {
      // Delete oldest sessions to make room
      const sessionsToRemove = userHistory.sessions
        .sort((a, b) => a.updatedAt - b.updatedAt)
        .slice(0, userHistory.sessions.length - maxSessions + 1);

      for (const session of sessionsToRemove) {
        await this.storageService.deleteSession(userId, session.id, true);
      }
    }

    const sessionId = `session_${Date.now()}_${crypto.randomUUID().replace(/-/g, '').slice(0, 9)}`;
    const newSession: ChatSession = {
      id: sessionId,
      name: name || `Chat ${userHistory.sessions.length + 1}`,
      userId,
      messages: [{
        id: `welcome_${Date.now()}`,
        role: 'assistant',
        content: 'Hello! I\'m your AI assistant. I can help you with monitoring, observability, and Grafana-related questions. How can I assist you today?',
        timestamp: Date.now()
      }],
      createdAt: Date.now(),
      updatedAt: Date.now(),
      isActive: true
    };

    // Use dedicated createSession which calls POST /sessions on backend
    const createdSession = await this.storageService.createSession(userId, newSession, true);

    return createdSession;
  }

  /**
   * Get current active session or create one if none exists
   * Loads the full session with messages from the backend
   */
  public async getActiveSession(): Promise<ChatSession> {
    const userId = this.getCurrentUserId();
    const userHistory = await this.getUserHistory();

    if (userHistory.activeSessionId) {
      // Fetch the full session with messages
      const fullSession = await this.storageService.getSession(userId, userHistory.activeSessionId);
      if (fullSession) {
        return fullSession;
      }
    }

    // No active session found, create a new one
    return await this.createSession();
  }

  /**
   * Switch to a different session
   * Fetches the full session with messages from the backend
   */
  public async switchToSession(sessionId: string): Promise<ChatSession | null> {
    const userId = this.getCurrentUserId();
    const userHistory = await this.storageService.getUserHistory(userId);

    // Check if session exists in user's sessions
    const sessionExists = userHistory.sessions.some(s => s.id === sessionId);
    if (!sessionExists) {
      return null;
    }

    // Update active session ID
    userHistory.activeSessionId = sessionId;
    await this.storageService.saveUserHistory(userHistory);

    // Fetch and return the full session with messages
    return await this.storageService.getSession(userId, sessionId);
  }

  /**
   * Add a message to the current active session
   * Uses dedicated storageService.addMessage() to avoid full-session rewrite per message.
   */
  public async addMessage(message: ChatMessage, sessionId?: string): Promise<void> {
    const userId = this.getCurrentUserId();
    let targetSessionId = sessionId;

    // Fallback to active session if caller didn't pass one
    if (!targetSessionId) {
      const userHistory = await this.storageService.getUserHistory(userId);
      targetSessionId = userHistory.activeSessionId ?? undefined;
    }

    if (!targetSessionId) {
      return;
    }

    await this.storageService.addMessage(userId, targetSessionId, message, true);
  }

  /**
   * Update a message in the current active session
   * Uses dedicated storageService.updateMessage() instead of full session rewrite.
   */
  public async updateMessage(
    messageId: string,
    content: string,
    toolExecutions?: Array<{ toolName: string; status: 'success' | 'error'; errorMessage?: string }>,
    sessionId?: string
  ): Promise<void> {
    const userId = this.getCurrentUserId();
    let targetSessionId = sessionId;

    // Fallback to active session if caller didn't pass one
    if (!targetSessionId) {
      const userHistory = await this.storageService.getUserHistory(userId);
      targetSessionId = userHistory.activeSessionId ?? undefined;
    }

    if (!targetSessionId) {
      return;
    }

    await this.storageService.updateMessage(
      userId,
      targetSessionId,
      messageId,
      content,
      toolExecutions,
      true
    );
  }

  /**
   * Delete a session
   * Uses dedicated storageService.deleteSession() which calls DELETE /sessions/{id} on backend
   */
  public async deleteSession(sessionId: string): Promise<boolean> {
    const userId = this.getCurrentUserId();
    const userHistory = await this.storageService.getUserHistory(userId);

    // Check if session exists
    const sessionIndex = userHistory.sessions.findIndex(s => s.id === sessionId);
    if (sessionIndex === -1) {
      return false;
    }

    // Use dedicated deleteSession which calls DELETE /sessions/{id} on backend
    // This handles localStorage update and activeSessionId update internally
    await this.storageService.deleteSession(userId, sessionId, true);

    return true;
  }

  /**
   * Rename a session
   * Uses dedicated storageService.renameSession() which calls PUT /sessions/{id}/rename on backend
   */
  public async renameSession(sessionId: string, newName: string): Promise<boolean> {
    const userId = this.getCurrentUserId();
    const userHistory = await this.storageService.getUserHistory(userId);

    const session = userHistory.sessions.find(s => s.id === sessionId);
    if (!session) {
      return false;
    }

    await this.storageService.renameSession(userId, sessionId, newName, true);

    return true;
  }

  /**
   * Get all sessions for the current user
   */
  public async getAllSessions(): Promise<ChatSession[]> {
    const userHistory = await this.getUserHistory();
    return userHistory.sessions.sort((a, b) => b.updatedAt - a.updatedAt);
  }

  /**
   * Clear all chat history for the current user
   */
  public async clearAllHistory(): Promise<void> {
    const userId = this.getCurrentUserId();
    await this.storageService.clearAllHistory(userId);
  }

  /**
   * Generate a smart name for a session based on the first user message
   */
  public generateSessionName(firstUserMessage: string): string {
    const words = firstUserMessage.trim().split(/\s+/);
    if (words.length <= 4) {
      return firstUserMessage;
    }
    return words.slice(0, 4).join(' ') + '...';
  }
}
