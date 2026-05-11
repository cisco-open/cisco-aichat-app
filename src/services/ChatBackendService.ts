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
import { ChatMessage, ChatSession, UserChatHistory, SearchResult } from '../types/chat';
import { classifyBackendError } from './ChatErrors';

/**
 * Response wrapper for backend API calls
 */
interface BackendResponse<T> {
  success: boolean;
  data?: T;
  error?: string;
}

/**
 * Token statistics for a session
 * Matches backend response from GET /sessions/{id}/tokens
 */
export interface TokenStats {
  sessionId: string;
  totalTokens: number;
  contextLimit: number;
  contextUsage: number;  // percentage (0-100)
  messageCount: number;
  uncountedMsgs: number;
}

/**
 * Paginated messages response for lazy loading
 */
export interface MessagesPage {
  messages: ChatMessage[];
  hasMore: boolean;
  nextCursor: string | null;
}

/**
 * Service for interacting with the backend Chat API
 * Implements secure REST API calls with timeout and error handling
 * Follows the singleton pattern for consistent state management
 */
export class ChatBackendService {
  private static instance: ChatBackendService;
  private static readonly BASE_PATH = '/api/plugins/cisco-aichat-app/resources';

  private constructor() {}

  /**
   * Get singleton instance
   */
  public static getInstance(): ChatBackendService {
    if (!ChatBackendService.instance) {
      ChatBackendService.instance = new ChatBackendService();
    }
    return ChatBackendService.instance;
  }

  /**
   * Check if backend API is available
   */
  public async isBackendAvailable(): Promise<boolean> {
    try {
      const response = await this.requestRaw<{ status: string }>({
        url: `${ChatBackendService.BASE_PATH}/health`,
        method: 'GET',
      }, 'Failed to check backend availability');
      return response.status === 200;
    } catch (error: unknown) {
      const classified = classifyBackendError(error, 'Failed to check backend availability');
      if (classified.name === 'AuthError') {
        throw classified;
      }
      // /health returns 503 in degraded mode while the backend is still reachable.
      // Treat this as reachable to avoid diverging into browser localStorage state.
      if ((classified as any)?.name === 'ServerError' && (classified as any)?.status === 503) {
        return true;
      }
      console.log('Backend API not available:', classified);
      return false;
    }
  }

  private async requestRaw<T>(
    request: { url: string; method: 'GET' | 'POST' | 'PUT' | 'DELETE'; data?: unknown },
    operation: string
  ) {
    try {
      return await lastValueFrom(
        getBackendSrv().fetch<T>(request)
      );
    } catch (error: unknown) {
      throw classifyBackendError(error, operation);
    }
  }

  /**
   * Get user's chat history from backend
   */
  public async getUserHistory(): Promise<UserChatHistory> {
    const response = await this.requestRaw<BackendResponse<UserChatHistory>>({
      url: `${ChatBackendService.BASE_PATH}/history`,
      method: 'GET',
    }, 'Failed to fetch user history');

    if (!response.data.success || !response.data.data) {
      throw new Error(response.data.error || 'Failed to fetch user history');
    }

    return response.data.data;
  }

  /**
   * Save user's chat history to backend
   */
  public async saveUserHistory(history: UserChatHistory): Promise<void> {
    const response = await this.requestRaw<BackendResponse<void>>({
      url: `${ChatBackendService.BASE_PATH}/history`,
      method: 'POST',
      data: history,
    }, 'Failed to save user history');

    if (!response.data.success) {
      throw new Error(response.data.error || 'Failed to save user history');
    }
  }

  /**
   * Get a specific session by ID
   */
  public async getSession(sessionId: string): Promise<ChatSession> {
    console.log('[ChatBackend] getSession: fetching', sessionId);
    const response = await this.requestRaw<BackendResponse<ChatSession>>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}`,
      method: 'GET',
    }, 'Failed to fetch session');

    console.log('[ChatBackend] getSession: response', response.status, 'success:', response.data.success, 'hasData:', !!response.data.data);
    if (!response.data.success || !response.data.data) {
      throw new Error(response.data.error || 'Failed to fetch session');
    }

    console.log('[ChatBackend] getSession: returning session with', response.data.data.messages?.length, 'messages');
    return response.data.data;
  }

  /**
   * Create a new chat session
   */
  public async createSession(session: ChatSession): Promise<ChatSession> {
    const response = await this.requestRaw<BackendResponse<ChatSession>>({
      url: `${ChatBackendService.BASE_PATH}/sessions`,
      method: 'POST',
      data: session,
    }, 'Failed to create session');

    if (!response.data.success || !response.data.data) {
      throw new Error(response.data.error || 'Failed to create session');
    }

    return response.data.data;
  }

  /**
   * Update an existing session
   */
  public async updateSession(sessionId: string, session: ChatSession): Promise<ChatSession> {
    const response = await this.requestRaw<BackendResponse<ChatSession>>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}`,
      method: 'PUT',
      data: session,
    }, 'Failed to update session');

    if (!response.data.success || !response.data.data) {
      throw new Error(response.data.error || 'Failed to update session');
    }

    return response.data.data;
  }

  /**
   * Delete a session
   */
  public async deleteSession(sessionId: string): Promise<void> {
    const response = await this.requestRaw<BackendResponse<void>>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}`,
      method: 'DELETE',
    }, 'Failed to delete session');

    if (!response.data.success) {
      throw new Error(response.data.error || 'Failed to delete session');
    }
  }

  /**
   * Add a message to a session
   */
  public async addMessage(sessionId: string, message: ChatMessage): Promise<void> {
    const response = await this.requestRaw<BackendResponse<void>>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}/messages`,
      method: 'POST',
      data: message,
    }, 'Failed to add message');

    if (!response.data.success) {
      throw new Error(response.data.error || 'Failed to add message');
    }
  }

  /**
   * Update a message in a session
   */
  public async updateMessage(sessionId: string, messageId: string, content: string): Promise<void> {
    const response = await this.requestRaw<BackendResponse<void>>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}/messages/${messageId}`,
      method: 'PUT',
      data: { content },
    }, 'Failed to update message');

    if (!response.data.success) {
      throw new Error(response.data.error || 'Failed to update message');
    }
  }

  /**
   * Rename a session
   */
  public async renameSession(sessionId: string, newName: string): Promise<void> {
    const response = await this.requestRaw<BackendResponse<void>>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}/rename`,
      method: 'PUT',
      data: { name: newName },
    }, 'Failed to rename session');

    if (!response.data.success) {
      throw new Error(response.data.error || 'Failed to rename session');
    }
  }

  /**
   * Clear all chat history for the current user
   */
  public async clearAllHistory(): Promise<void> {
    const response = await this.requestRaw<BackendResponse<void>>({
      url: `${ChatBackendService.BASE_PATH}/history`,
      method: 'DELETE',
    }, 'Failed to clear history');

    if (!response.data.success) {
      throw new Error(response.data.error || 'Failed to clear history');
    }
  }

  /**
   * Get token statistics for a session
   * Returns usage information for context window management
   */
  public async getSessionTokenStats(sessionId: string): Promise<TokenStats> {
    const response = await this.requestRaw<TokenStats>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}/tokens`,
      method: 'GET',
    }, 'Failed to fetch token stats');

    // Backend returns TokenStats directly (not wrapped in BackendResponse)
    const stats = response.data;
    if (!stats || typeof stats.totalTokens !== 'number') {
      throw new Error('Invalid token stats response');
    }

    return stats;
  }

  /**
   * Trigger compaction (summarization) for a session
   * This will summarize older messages to free up context space
   */
  public async triggerCompaction(sessionId: string): Promise<void> {
    const response = await this.requestRaw<BackendResponse<void>>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}/compact`,
      method: 'POST',
    }, 'Failed to trigger compaction');

    if (!response.data.success) {
      throw new Error(response.data.error || 'Failed to trigger compaction');
    }
  }

  /**
   * Get paginated messages for a session
   * Used for lazy loading / infinite scroll
   * @param sessionId - Session to fetch from
   * @param limit - Number of messages per page (default 50, max 100)
   * @param cursor - Optional cursor for pagination (message ID to start after)
   */
  public async getSessionMessagesPage(
    sessionId: string,
    limit = 50,
    cursor?: string
  ): Promise<MessagesPage> {
    // Build URL with pagination params
    const params = new URLSearchParams({ limit: String(limit) });
    if (cursor) {
      params.set('cursor', cursor);
    }

    const response = await this.requestRaw<MessagesPage>({
      url: `${ChatBackendService.BASE_PATH}/sessions/${sessionId}/messages?${params}`,
      method: 'GET',
    }, 'Failed to fetch messages');

    return response.data;
  }

  /**
   * Search messages across all sessions using full-text search
   * @param query - Search query string
   * @param limit - Max results (default 50, max 100)
   * @param offset - Pagination offset (default 0)
   * @returns Array of search results with highlighted content
   */
  public async searchMessages(query: string, limit = 50, offset = 0): Promise<SearchResult[]> {
    const response = await this.requestRaw<{ results: SearchResult[]; total: number }>({
      url: `${ChatBackendService.BASE_PATH}/search?q=${encodeURIComponent(query)}&limit=${limit}&offset=${offset}`,
      method: 'GET',
    }, 'Failed to search messages');
    return response.data.results || [];
  }
}

export default ChatBackendService;
