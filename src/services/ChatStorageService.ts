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

import { ChatSession, UserChatHistory, ChatHistoryStorage, ChatMessage } from '../types/chat';
import { ChatBackendService } from './ChatBackendService';
import { isAuthError, isNetworkError, isNotFoundError } from './ChatErrors';

const STORAGE_KEY = 'cisco-aichat-history';

/**
 * Retry helper - attempts an async operation once more after a delay on failure
 */
const retryOnce = async <T>(fn: () => Promise<T>, delayMs = 500): Promise<T> => {
  try {
    return await fn();
  } catch (firstError) {
    await new Promise((r) => setTimeout(r, delayMs));
    return await fn(); // Second attempt - let it throw if it fails
  }
};
const MIGRATION_FLAG_KEY = 'cisco-aichat-migrated';
const CACHE_DURATION = 5000; // 5 seconds cache

/**
 * Interface for cached data
 */
interface CachedData<T> {
  data: T;
  timestamp: number;
}

/**
 * Storage service that implements backend-first with localStorage fallback
 * Provides caching layer for performance and automatic migration from localStorage to backend
 */
export class ChatStorageService {
  private static instance: ChatStorageService;
  private backendService: ChatBackendService;
  private backendAvailable: boolean | null = null;
  private cache: Map<string, CachedData<any>> = new Map();
  private migrationCompleted = false;

  private recordEvent(event: 'auth_expired' | 'fallback_used', operation: string): void {
    console.info('[ChatStorageEvent]', { event, operation, timestamp: Date.now() });
  }

  private constructor() {
    this.backendService = ChatBackendService.getInstance();
    this.checkMigrationStatus();
  }

  /**
   * Get singleton instance
   */
  public static getInstance(): ChatStorageService {
    if (!ChatStorageService.instance) {
      ChatStorageService.instance = new ChatStorageService();
    }
    return ChatStorageService.instance;
  }

  /**
   * Check if migration from localStorage to backend has been completed
   */
  private checkMigrationStatus(): void {
    try {
      const flag = localStorage.getItem(MIGRATION_FLAG_KEY);
      this.migrationCompleted = flag === 'true';
    } catch (error) {
      console.error('Error checking migration status:', error);
      this.migrationCompleted = false;
    }
  }

  /**
   * Mark migration as completed
   */
  private setMigrationCompleted(): void {
    try {
      localStorage.setItem(MIGRATION_FLAG_KEY, 'true');
      this.migrationCompleted = true;
    } catch (error) {
      console.error('Error setting migration flag:', error);
    }
  }

  /**
   * Check if backend is available (with caching for success only)
   * Note: Only caches positive results to allow recovery from transient failures
   */
  public async isBackendAvailable(): Promise<boolean> {
    // Only return cached value if it's true (backend confirmed available)
    // This allows retry if backend was previously unavailable
    if (this.backendAvailable === true) {
      return true;
    }

    try {
      const isAvailable = await this.backendService.isBackendAvailable();
      if (isAvailable) {
        // Cache successful result
        this.backendAvailable = true;
      }
      // Don't cache false - allow retry on next operation
      return isAvailable;
    } catch (error) {
      if (isAuthError(error)) {
        this.recordEvent('auth_expired', 'isBackendAvailable');
        throw error;
      }
      console.error('[ChatStorage] Error checking backend availability:', error);
      // Don't cache - allow retry on next operation
      return false;
    }
  }

  /**
   * Get data from cache if valid
   */
  private getCached<T>(key: string): T | null {
    const cached = this.cache.get(key);
    if (!cached) {
      return null;
    }

    const age = Date.now() - cached.timestamp;
    if (age > CACHE_DURATION) {
      this.cache.delete(key);
      return null;
    }

    return cached.data as T;
  }

  /**
   * Set data in cache
   */
  private setCache<T>(key: string, data: T): void {
    this.cache.set(key, {
      data,
      timestamp: Date.now(),
    });
  }

  /**
   * Clear specific cache entry
   */
  private clearCache(key: string): void {
    this.cache.delete(key);
  }

  /**
   * Clear all cache
   */
  private clearAllCache(): void {
    this.cache.clear();
  }

  /**
   * Get data from localStorage
   */
  private getFromLocalStorage(): ChatHistoryStorage {
    try {
      const data = localStorage.getItem(STORAGE_KEY);
      return data ? JSON.parse(data) : {};
    } catch (error) {
      console.error('Failed to load from localStorage:', error);
      return {};
    }
  }

  /**
   * Save data to localStorage
   */
  private saveToLocalStorage(data: ChatHistoryStorage): void {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
    } catch (error) {
      console.error('Failed to save to localStorage:', error);
    }
  }

  /**
   * Clone local storage data for safe optimistic rollback
   */
  private cloneLocalData(data: ChatHistoryStorage): ChatHistoryStorage {
    return JSON.parse(JSON.stringify(data));
  }

  /**
   * Mark backend status as unknown so next request re-checks health
   */
  private markBackendStatusUnknown(): void {
    this.backendAvailable = null;
  }

  /**
   * Fallback to browser storage only for transport-level failures.
   * Server-side errors should surface to the caller to avoid split-brain data.
   */
  private shouldFallbackToLocal(error: unknown): boolean {
    return isNetworkError(error);
  }

  private isSessionNotFoundError(error: unknown): boolean {
    if (!isNotFoundError(error)) {
      return false;
    }
    const message = error instanceof Error ? error.message.toLowerCase() : String(error).toLowerCase();
    return message.includes('session not found');
  }

  /**
   * Keep local mirror aligned with canonical backend history for this user.
   */
  private syncLocalMirrorWithBackend(userId: string, history: UserChatHistory): void {
    const localData = this.getFromLocalStorage();
    localData[userId] = history;
    if (history.userId && history.userId !== userId) {
      localData[history.userId] = history;
    }
    this.saveToLocalStorage(localData);
  }

  /**
   * Apply session create to localStorage mirror
   */
  private applyLocalCreateSession(userId: string, session: ChatSession): void {
    const localData = this.getFromLocalStorage();
    const userHistory = localData[userId] || { userId, sessions: [], activeSessionId: null };
    userHistory.sessions.push(session);
    userHistory.activeSessionId = session.id;
    localData[userId] = userHistory;
    this.saveToLocalStorage(localData);
  }

  /**
   * Apply session update to localStorage mirror
   */
  private applyLocalUpdateSession(userId: string, session: ChatSession): void {
    const localData = this.getFromLocalStorage();
    const userHistory = localData[userId] || { userId, sessions: [], activeSessionId: null };
    const index = userHistory.sessions.findIndex((s) => s.id === session.id);

    if (index !== -1) {
      userHistory.sessions[index] = session;
    } else {
      userHistory.sessions.push(session);
    }

    if (!userHistory.activeSessionId) {
      userHistory.activeSessionId = session.id;
    }

    localData[userId] = userHistory;
    this.saveToLocalStorage(localData);
  }

  /**
   * Apply session rename to localStorage mirror
   */
  private applyLocalRenameSession(userId: string, sessionId: string, newName: string): void {
    const localData = this.getFromLocalStorage();
    const userHistory = localData[userId];
    if (!userHistory) {
      return;
    }

    const sessionIndex = userHistory.sessions.findIndex((s) => s.id === sessionId);
    if (sessionIndex === -1) {
      return;
    }

    userHistory.sessions[sessionIndex].name = newName;
    userHistory.sessions[sessionIndex].updatedAt = Date.now();
    localData[userId] = userHistory;
    this.saveToLocalStorage(localData);
  }

  /**
   * Apply message append to localStorage mirror
   */
  private applyLocalAddMessage(userId: string, sessionId: string, message: ChatMessage): void {
    const localData = this.getFromLocalStorage();
    const userHistory = localData[userId];
    if (!userHistory) {
      return;
    }

    const sessionIndex = userHistory.sessions.findIndex((s) => s.id === sessionId);
    if (sessionIndex === -1) {
      return;
    }

    const existingIndex = userHistory.sessions[sessionIndex].messages.findIndex((m) => m.id === message.id);
    if (existingIndex !== -1) {
      userHistory.sessions[sessionIndex].messages[existingIndex] = message;
    } else {
      userHistory.sessions[sessionIndex].messages.push(message);
    }
    userHistory.sessions[sessionIndex].updatedAt = message.timestamp;
    userHistory.activeSessionId = sessionId;

    localData[userId] = userHistory;
    this.saveToLocalStorage(localData);
  }

  /**
   * Apply message content update to localStorage mirror
   */
  private applyLocalUpdateMessage(
    userId: string,
    sessionId: string,
    messageId: string,
    content: string,
    toolExecutions?: Array<{ toolName: string; status: 'success' | 'error'; errorMessage?: string }>
  ): void {
    const localData = this.getFromLocalStorage();
    const userHistory = localData[userId];
    if (!userHistory) {
      return;
    }

    const sessionIndex = userHistory.sessions.findIndex((s) => s.id === sessionId);
    if (sessionIndex === -1) {
      return;
    }

    const messageIndex = userHistory.sessions[sessionIndex].messages.findIndex((m) => m.id === messageId);
    if (messageIndex === -1) {
      return;
    }

    userHistory.sessions[sessionIndex].messages[messageIndex].content = content;
    if (toolExecutions) {
      userHistory.sessions[sessionIndex].messages[messageIndex].toolExecutions = toolExecutions;
    }
    userHistory.sessions[sessionIndex].updatedAt = Date.now();

    localData[userId] = userHistory;
    this.saveToLocalStorage(localData);
  }

  /**
   * Apply session delete to localStorage mirror
   */
  private applyLocalDeleteSession(userId: string, sessionId: string): void {
    const localData = this.getFromLocalStorage();
    const userHistory = localData[userId];
    if (!userHistory) {
      return;
    }

    userHistory.sessions = userHistory.sessions.filter((s) => s.id !== sessionId);
    if (userHistory.activeSessionId === sessionId) {
      userHistory.activeSessionId = userHistory.sessions.length > 0 ? userHistory.sessions[0].id : null;
    }

    localData[userId] = userHistory;
    this.saveToLocalStorage(localData);
  }

  /**
   * Apply clear-all to localStorage mirror
   */
  private applyLocalClearAllHistory(userId: string): void {
    const localData = this.getFromLocalStorage();
    localData[userId] = { userId, sessions: [], activeSessionId: null };
    this.saveToLocalStorage(localData);
  }

  /**
   * Get a session snapshot from local mirror storage.
   */
  private getLocalSessionSnapshot(userId: string, sessionId: string): ChatSession | null {
    const localData = this.getFromLocalStorage();
    const userHistory = localData[userId];
    if (!userHistory) {
      return null;
    }
    return userHistory.sessions.find((s) => s.id === sessionId) || null;
  }

  /**
   * Best-effort repair when backend lost a session that still exists in local mirror.
   * Returns true if session was recreated in backend.
   */
  private async recoverMissingBackendSession(userId: string, sessionId: string): Promise<boolean> {
    const localSession = this.getLocalSessionSnapshot(userId, sessionId);
    if (!localSession) {
      return false;
    }

    try {
      await retryOnce(() => this.backendService.createSession(localSession));
      this.applyLocalUpdateSession(userId, localSession);
      this.clearCache(`session-${sessionId}`);
      this.clearCache(`user-history-${userId}`);
      return true;
    } catch (error) {
      if (isAuthError(error)) {
        throw error;
      }
      console.warn('[ChatStorage] Failed to recover missing backend session', { userId, sessionId, error });
      return false;
    }
  }

  /**
   * Migrate data from localStorage to backend
   */
  public async migrateToBackend(userId: string): Promise<boolean> {
    if (this.migrationCompleted) {
      console.log('Migration already completed, skipping');
      return true;
    }

    const isAvailable = await this.isBackendAvailable();
    if (!isAvailable) {
      console.log('Backend not available, cannot migrate');
      return false;
    }

    try {
      // Always trust backend data if it already exists to avoid rehydrating stale local sessions.
      const backendHistory = await this.backendService.getUserHistory();
      if (backendHistory.sessions.length > 0) {
        this.syncLocalMirrorWithBackend(userId, backendHistory);
        this.setMigrationCompleted();
        return true;
      }

      const localData = this.getFromLocalStorage();
      const userHistory = localData[userId];

      if (!userHistory || userHistory.sessions.length === 0) {
        console.log('No local data to migrate');
        this.syncLocalMirrorWithBackend(userId, backendHistory);
        this.setMigrationCompleted();
        return true;
      }

      console.log(`Migrating ${userHistory.sessions.length} sessions to backend`);
      await this.backendService.saveUserHistory(userHistory);

      this.syncLocalMirrorWithBackend(userId, userHistory);
      this.setMigrationCompleted();
      console.log('Migration completed successfully');
      return true;
    } catch (error) {
      if (isAuthError(error)) {
        this.recordEvent('auth_expired', 'migrateToBackend');
        throw error;
      }
      console.error('Migration failed:', error);
      return false;
    }
  }

  /**
   * Get user history (backend-first with localStorage fallback)
   */
  public async getUserHistory(userId: string): Promise<UserChatHistory> {
    // Check cache first
    const cacheKey = `user-history-${userId}`;
    const cached = this.getCached<UserChatHistory>(cacheKey);
    if (cached) {
      return cached;
    }

    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      try {
        // Try to migrate if not done yet
        if (!this.migrationCompleted) {
          await this.migrateToBackend(userId);
        }

        // Get from backend
        const history = await this.backendService.getUserHistory();
        this.syncLocalMirrorWithBackend(userId, history);
        this.setCache(cacheKey, history);
        return history;
      } catch (error) {
        if (isAuthError(error)) {
          this.recordEvent('auth_expired', 'getUserHistory');
          throw error;
        }
        if (!this.shouldFallbackToLocal(error)) {
          throw error;
        }
        console.error('Error fetching from backend, falling back to localStorage:', error);
      }
    }

    // Fallback to localStorage
    this.recordEvent('fallback_used', 'getUserHistory');
    const localData = this.getFromLocalStorage();
    const userHistory = localData[userId] || {
      userId,
      sessions: [],
      activeSessionId: null,
    };
    this.setCache(cacheKey, userHistory);
    return userHistory;
  }

  /**
   * Save user history (backend-first with localStorage fallback)
   */
  public async saveUserHistory(history: UserChatHistory): Promise<void> {
    const cacheKey = `user-history-${history.userId}`;
    this.clearCache(cacheKey);

    console.log('[ChatStorage] saveUserHistory called with', history.sessions.length, 'sessions');

    const isAvailable = await this.isBackendAvailable();
    console.log('[ChatStorage] isBackendAvailable:', isAvailable);

    if (isAvailable) {
      try {
        console.log('[ChatStorage] Syncing to backend...');
        await this.backendService.saveUserHistory(history);
        console.log('[ChatStorage] Backend sync successful');
        this.setCache(cacheKey, history);

        // Also update localStorage for offline fallback
        const localData = this.getFromLocalStorage();
        localData[history.userId] = history;
        this.saveToLocalStorage(localData);
        return;
      } catch (error) {
        if (isAuthError(error)) {
          this.recordEvent('auth_expired', 'saveUserHistory');
          throw error;
        }
        if (!this.shouldFallbackToLocal(error)) {
          throw error;
        }
        console.error('[ChatStorage] Error saving to backend, falling back to localStorage:', error);
      }
    }

    // Fallback to localStorage
    this.recordEvent('fallback_used', 'saveUserHistory');
    console.log('[ChatStorage] Using localStorage fallback');
    const localData = this.getFromLocalStorage();
    localData[history.userId] = history;
    this.saveToLocalStorage(localData);
    this.setCache(cacheKey, history);
  }

  /**
   * Get a specific session (backend-first with localStorage fallback)
   */
  public async getSession(userId: string, sessionId: string): Promise<ChatSession | null> {
    const cacheKey = `session-${sessionId}`;
    const cached = this.getCached<ChatSession>(cacheKey);
    if (cached) {
      console.log('[ChatStorage] getSession: returning cached session', sessionId, 'messages:', cached.messages?.length);
      return cached;
    }

    const isAvailable = await this.isBackendAvailable();
    console.log('[ChatStorage] getSession: backend available:', isAvailable, 'sessionId:', sessionId);

    if (isAvailable) {
      try {
        const session = await this.backendService.getSession(sessionId);
        console.log('[ChatStorage] getSession: backend returned session with', session.messages?.length, 'messages');
        this.setCache(cacheKey, session);
        return session;
      } catch (error) {
        if (isAuthError(error)) {
          this.recordEvent('auth_expired', 'getSession');
          throw error;
        }
        if (isNotFoundError(error)) {
          // Session no longer exists in backend; purge stale local mirror.
          this.applyLocalDeleteSession(userId, sessionId);
          return null;
        }
        if (!this.shouldFallbackToLocal(error)) {
          throw error;
        }
        console.error('[ChatStorage] Error fetching session from backend, falling back to localStorage:', error);
      }
    }

    // Fallback to localStorage - WARNING: sessions from getUserHistory have empty messages
    this.recordEvent('fallback_used', 'getSession');
    console.warn('[ChatStorage] getSession: using localStorage fallback (messages will be empty!)');
    const userHistory = await this.getUserHistory(userId);
    const session = userHistory.sessions.find((s) => s.id === sessionId) || null;
    if (session) {
      console.log('[ChatStorage] getSession: fallback found session with', session.messages?.length, 'messages');
      this.setCache(cacheKey, session);
    }
    return session;
  }

  /**
   * Create a new session (with optimistic update support)
   */
  public async createSession(userId: string, session: ChatSession, optimistic = false): Promise<ChatSession> {
    const cacheKey = `user-history-${userId}`;
    this.clearCache(cacheKey);
    this.clearCache(`session-${session.id}`);

    const optimisticSnapshot = optimistic ? this.cloneLocalData(this.getFromLocalStorage()) : null;

    // Optimistic update to localStorage first
    if (optimistic) {
      this.applyLocalCreateSession(userId, session);
    }

    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      try {
        const createdSession = await retryOnce(() => this.backendService.createSession(session));

        // Keep local mirror in sync with canonical backend response
        this.applyLocalUpdateSession(userId, createdSession);
        this.setCache(`session-${createdSession.id}`, createdSession);
        return createdSession;
      } catch (error) {
        this.markBackendStatusUnknown();
        if (optimisticSnapshot) {
          this.saveToLocalStorage(optimisticSnapshot);
        }
        console.error('Error creating session in backend after retry:', error);
        throw error;
      }
    }

    // Fallback to localStorage (if not already done optimistically)
    this.recordEvent('fallback_used', 'createSession');
    if (!optimistic) {
      this.applyLocalCreateSession(userId, session);
    }

    this.setCache(`session-${session.id}`, session);
    return session;
  }

  /**
   * Update a session (with optimistic update support)
   */
  public async updateSession(userId: string, session: ChatSession, optimistic = false): Promise<void> {
    const cacheKey = `user-history-${userId}`;
    this.clearCache(cacheKey);
    this.clearCache(`session-${session.id}`);

    const optimisticSnapshot = optimistic ? this.cloneLocalData(this.getFromLocalStorage()) : null;

    // Optimistic update to localStorage first
    if (optimistic) {
      this.applyLocalUpdateSession(userId, session);
    }

    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      try {
        const updatedSession = await retryOnce(() => this.backendService.updateSession(session.id, session));

        // Keep local mirror in sync with canonical backend response
        this.applyLocalUpdateSession(userId, updatedSession);
        this.setCache(`session-${updatedSession.id}`, updatedSession);
        return;
      } catch (error) {
        if (this.isSessionNotFoundError(error)) {
          this.applyLocalDeleteSession(userId, session.id);
          throw error;
        }
        this.markBackendStatusUnknown();
        if (optimisticSnapshot) {
          this.saveToLocalStorage(optimisticSnapshot);
        }
        console.error('Error updating session in backend:', error);
        throw error;
      }
    }

    // Fallback to localStorage only when backend is unavailable
    this.recordEvent('fallback_used', 'updateSession');
    if (!optimistic) {
      this.applyLocalUpdateSession(userId, session);
    }

    this.setCache(`session-${session.id}`, session);
  }

  /**
   * Rename a session (with optimistic update support)
   */
  public async renameSession(userId: string, sessionId: string, newName: string, optimistic = false): Promise<void> {
    const cacheKey = `user-history-${userId}`;
    this.clearCache(cacheKey);
    this.clearCache(`session-${sessionId}`);

    const optimisticSnapshot = optimistic ? this.cloneLocalData(this.getFromLocalStorage()) : null;

    if (optimistic) {
      this.applyLocalRenameSession(userId, sessionId, newName);
    }

    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      try {
        await retryOnce(() => this.backendService.renameSession(sessionId, newName));

        // Keep local mirror in sync after successful backend rename.
        this.applyLocalRenameSession(userId, sessionId, newName);
        return;
      } catch (error) {
        if (this.isSessionNotFoundError(error)) {
          this.applyLocalDeleteSession(userId, sessionId);
          throw error;
        }
        this.markBackendStatusUnknown();
        if (optimisticSnapshot) {
          this.saveToLocalStorage(optimisticSnapshot);
        }
        console.error('Error renaming session in backend:', error);
        throw error;
      }
    }

    // Fallback to localStorage only when backend is unavailable
    this.recordEvent('fallback_used', 'renameSession');
    if (!optimistic) {
      this.applyLocalRenameSession(userId, sessionId, newName);
    }
  }

  /**
   * Add a message to a session (with optimistic update support)
   */
  public async addMessage(userId: string, sessionId: string, message: ChatMessage, optimistic = false): Promise<void> {
    const cacheKey = `user-history-${userId}`;
    this.clearCache(cacheKey);
    this.clearCache(`session-${sessionId}`);

    const optimisticSnapshot = optimistic ? this.cloneLocalData(this.getFromLocalStorage()) : null;

    if (optimistic) {
      this.applyLocalAddMessage(userId, sessionId, message);
    }

    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      try {
        await retryOnce(() => this.backendService.addMessage(sessionId, message));

        // Keep local mirror in sync for offline mode as well.
        this.applyLocalAddMessage(userId, sessionId, message);
        return;
      } catch (error) {
        if (this.isSessionNotFoundError(error)) {
          // Attempt to repair backend session from local mirror, then retry add once.
          const recovered = await this.recoverMissingBackendSession(userId, sessionId);
          if (recovered) {
            await retryOnce(() => this.backendService.addMessage(sessionId, message));
            this.applyLocalAddMessage(userId, sessionId, message);
            return;
          }
          // Could not recover; drop stale local session so next UI refresh picks a valid one.
          this.applyLocalDeleteSession(userId, sessionId);
          throw error;
        }
        this.markBackendStatusUnknown();
        if (optimisticSnapshot) {
          this.saveToLocalStorage(optimisticSnapshot);
        }
        console.error('Error adding message in backend:', error);
        throw error;
      }
    }

    // Fallback to localStorage only when backend is unavailable
    this.recordEvent('fallback_used', 'addMessage');
    if (!optimistic) {
      this.applyLocalAddMessage(userId, sessionId, message);
    }
  }

  /**
   * Update a message in a session (with optimistic update support)
   */
  public async updateMessage(
    userId: string,
    sessionId: string,
    messageId: string,
    content: string,
    toolExecutions?: Array<{ toolName: string; status: 'success' | 'error'; errorMessage?: string }>,
    optimistic = false
  ): Promise<void> {
    const cacheKey = `user-history-${userId}`;
    this.clearCache(cacheKey);
    this.clearCache(`session-${sessionId}`);

    const optimisticSnapshot = optimistic ? this.cloneLocalData(this.getFromLocalStorage()) : null;

    if (optimistic) {
      this.applyLocalUpdateMessage(userId, sessionId, messageId, content, toolExecutions);
    }

    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      try {
        await retryOnce(() => this.backendService.updateMessage(sessionId, messageId, content));

        // Keep local mirror in sync after successful backend update.
        this.applyLocalUpdateMessage(userId, sessionId, messageId, content, toolExecutions);
        return;
      } catch (error) {
        if (this.isSessionNotFoundError(error)) {
          // Attempt to repair backend session from local mirror, then retry update once.
          const recovered = await this.recoverMissingBackendSession(userId, sessionId);
          if (recovered) {
            await retryOnce(() => this.backendService.updateMessage(sessionId, messageId, content));
            this.applyLocalUpdateMessage(userId, sessionId, messageId, content, toolExecutions);
            return;
          }
          this.applyLocalDeleteSession(userId, sessionId);
          throw error;
        }
        this.markBackendStatusUnknown();
        if (optimisticSnapshot) {
          this.saveToLocalStorage(optimisticSnapshot);
        }
        console.error('Error updating message in backend:', error);
        throw error;
      }
    }

    // Fallback to localStorage only when backend is unavailable
    this.recordEvent('fallback_used', 'updateMessage');
    if (!optimistic) {
      this.applyLocalUpdateMessage(userId, sessionId, messageId, content, toolExecutions);
    }
  }

  /**
   * Delete a session (with optimistic update support)
   */
  public async deleteSession(userId: string, sessionId: string, optimistic = false): Promise<void> {
    const cacheKey = `user-history-${userId}`;
    this.clearCache(cacheKey);
    this.clearCache(`session-${sessionId}`);

    const optimisticSnapshot = optimistic ? this.cloneLocalData(this.getFromLocalStorage()) : null;

    // Optimistic update to localStorage first
    if (optimistic) {
      this.applyLocalDeleteSession(userId, sessionId);
    }

    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      try {
        await retryOnce(() => this.backendService.deleteSession(sessionId));

        // Keep local mirror in sync after successful backend delete
        this.applyLocalDeleteSession(userId, sessionId);
        return;
      } catch (error) {
        if (this.isSessionNotFoundError(error)) {
          // Backend already deleted this session (stale client state). Treat as success.
          this.applyLocalDeleteSession(userId, sessionId);
          return;
        }
        this.markBackendStatusUnknown();
        if (optimisticSnapshot) {
          this.saveToLocalStorage(optimisticSnapshot);
        }
        console.error('Error deleting session from backend:', error);
        throw error;
      }
    }

    // Fallback to localStorage only when backend is unavailable
    this.recordEvent('fallback_used', 'deleteSession');
    if (!optimistic) {
      this.applyLocalDeleteSession(userId, sessionId);
    }
  }

  /**
   * Clear all history (with optimistic update support)
   */
  public async clearAllHistory(userId: string, optimistic = false): Promise<void> {
    this.clearAllCache();

    const optimisticSnapshot = optimistic ? this.cloneLocalData(this.getFromLocalStorage()) : null;

    // Optimistic update to localStorage first
    if (optimistic) {
      this.applyLocalClearAllHistory(userId);
    }

    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      try {
        await retryOnce(() => this.backendService.clearAllHistory());

        // Keep local mirror in sync after successful backend clear
        this.applyLocalClearAllHistory(userId);
        return;
      } catch (error) {
        this.markBackendStatusUnknown();
        if (optimisticSnapshot) {
          this.saveToLocalStorage(optimisticSnapshot);
        }
        console.error('Error clearing history in backend:', error);
        throw error;
      }
    }

    // Fallback to localStorage only when backend is unavailable
    this.recordEvent('fallback_used', 'clearAllHistory');
    if (!optimistic) {
      this.applyLocalClearAllHistory(userId);
    }
  }

  /**
   * Get backend connection status for UI display
   */
  public async getConnectionStatus(): Promise<{
    connected: boolean;
    message: string;
  }> {
    const isAvailable = await this.isBackendAvailable();

    if (isAvailable) {
      return {
        connected: true,
        message: 'Connected to backend storage',
      };
    }

    return {
      connected: false,
      message: 'Using local storage (backend unavailable)',
    };
  }
}

export default ChatStorageService;
