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

import { ChatStorageService } from './ChatStorageService';
import { ChatSession, UserChatHistory } from '../types/chat';
import { AuthError, NotFoundError } from './ChatErrors';

const mockBackendService = {
  isBackendAvailable: jest.fn(),
  getUserHistory: jest.fn(),
  saveUserHistory: jest.fn(),
  getSession: jest.fn(),
  createSession: jest.fn(),
  updateSession: jest.fn(),
  renameSession: jest.fn(),
  addMessage: jest.fn(),
  updateMessage: jest.fn(),
  deleteSession: jest.fn(),
  clearAllHistory: jest.fn(),
};

// Mock ChatBackendService to control backend availability
jest.mock('./ChatBackendService', () => ({
  ChatBackendService: {
    getInstance: jest.fn(() => mockBackendService),
  },
}));

const STORAGE_KEY = 'grafana-aichat-history';
const MIGRATION_FLAG_KEY = 'grafana-aichat-migrated';

/**
 * Helper to create a test session
 */
function createTestSession(id: string, name: string, userId: string): ChatSession {
  const now = Date.now();
  return {
    id,
    name,
    userId,
    messages: [],
    createdAt: now,
    updatedAt: now,
    isActive: true,
  };
}

/**
 * Helper to get storage service instance
 * Since ChatStorageService is a singleton, we need to reset it between tests
 */
function getStorageService(): ChatStorageService {
  // Clear the singleton instance by accessing the private property
  // This is a testing workaround for the singleton pattern
  (ChatStorageService as unknown as { instance: ChatStorageService | undefined }).instance = undefined;
  return ChatStorageService.getInstance();
}

describe('ChatStorageService', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockBackendService.isBackendAvailable.mockResolvedValue(false);
    // Clear localStorage before each test
    localStorage.clear();
    // Clear migration flag
    localStorage.removeItem(MIGRATION_FLAG_KEY);
  });

  afterEach(() => {
    localStorage.clear();
  });

  describe('getInstance', () => {
    it('returns the same instance on multiple calls', () => {
      // Reset singleton first
      (ChatStorageService as unknown as { instance: ChatStorageService | undefined }).instance = undefined;
      const instance1 = ChatStorageService.getInstance();
      const instance2 = ChatStorageService.getInstance();
      expect(instance1).toBe(instance2);
    });
  });

  describe('getUserHistory', () => {
    it('returns empty history when no sessions exist', async () => {
      const service = getStorageService();
      const userId = 'test-user';

      const history = await service.getUserHistory(userId);

      expect(history.userId).toBe(userId);
      expect(history.sessions).toEqual([]);
      expect(history.activeSessionId).toBeNull();
    });

    it('returns stored history from localStorage', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Test Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };

      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      const history = await service.getUserHistory(userId);

      expect(history.userId).toBe(userId);
      expect(history.sessions).toHaveLength(1);
      expect(history.sessions[0].id).toBe('session-1');
      expect(history.activeSessionId).toBe('session-1');
    });

    it('handles corrupted JSON in localStorage gracefully', async () => {
      localStorage.setItem(STORAGE_KEY, 'not-valid-json');

      const service = getStorageService();
      const userId = 'test-user';
      const history = await service.getUserHistory(userId);

      expect(history.userId).toBe(userId);
      expect(history.sessions).toEqual([]);
    });

    it('does not fallback to localStorage on AuthError', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Test Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.getUserHistory.mockRejectedValue(new AuthError('Authentication required', 401, 'AUTH_REQUIRED'));

      const service = getStorageService();
      await expect(service.getUserHistory(userId)).rejects.toBeInstanceOf(AuthError);
    });

    it('prefers backend history over stale local history during migration', async () => {
      const userId = 'test-user';
      const localSession = createTestSession('local-session', 'Stale Local Chat', userId);
      const backendSession = createTestSession('backend-session', 'Canonical Backend Chat', userId);

      const localHistory: UserChatHistory = {
        userId,
        sessions: [localSession],
        activeSessionId: localSession.id,
      };
      const backendHistory: UserChatHistory = {
        userId,
        sessions: [backendSession],
        activeSessionId: backendSession.id,
      };

      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: localHistory }));
      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.getUserHistory.mockResolvedValue(backendHistory);

      const service = getStorageService();
      const history = await service.getUserHistory(userId);

      expect(history.sessions).toHaveLength(1);
      expect(history.sessions[0].id).toBe('backend-session');
      expect(mockBackendService.saveUserHistory).not.toHaveBeenCalled();

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions[0].id).toBe('backend-session');
      expect(localStorage.getItem(MIGRATION_FLAG_KEY)).toBe('true');
    });
  });

  describe('createSession', () => {
    it('creates session and stores in localStorage', async () => {
      const service = getStorageService();
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Test Chat', userId);

      const createdSession = await service.createSession(userId, session);

      expect(createdSession.id).toBe('session-1');
      expect(createdSession.name).toBe('Test Chat');

      // Verify session was stored in localStorage
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId]).toBeDefined();
      expect(stored[userId].sessions).toHaveLength(1);
      expect(stored[userId].activeSessionId).toBe('session-1');
    });

    it('creates session with optimistic update', async () => {
      const service = getStorageService();
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Test Chat', userId);

      const createdSession = await service.createSession(userId, session, true);

      expect(createdSession.id).toBe('session-1');

      // Verify session was stored immediately (optimistic)
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions[0].id).toBe('session-1');
    });
  });

  describe('getSession', () => {
    it('returns session by ID', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Test Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      const retrieved = await service.getSession(userId, 'session-1');

      expect(retrieved).not.toBeNull();
      expect(retrieved?.id).toBe('session-1');
      expect(retrieved?.name).toBe('Test Chat');
    });

    it('returns null for non-existent session', async () => {
      const service = getStorageService();
      const userId = 'test-user';

      const retrieved = await service.getSession(userId, 'non-existent');

      expect(retrieved).toBeNull();
    });
  });

  describe('updateSession', () => {
    it('updates session name and messages', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Test Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      const updatedSession = {
        ...session,
        name: 'Updated Chat Name',
        messages: [
          {
            id: 'msg-1',
            role: 'user' as const,
            content: 'Hello',
            timestamp: Date.now(),
          },
        ],
      };

      await service.updateSession(userId, updatedSession);

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions[0].name).toBe('Updated Chat Name');
      expect(stored[userId].sessions[0].messages).toHaveLength(1);
    });

    it('updates session with optimistic update', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Test Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      const updatedSession = { ...session, name: 'Updated Name' };

      await service.updateSession(userId, updatedSession, true);

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions[0].name).toBe('Updated Name');
    });

    it('rolls back optimistic update and throws when backend update fails', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Original Name', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.updateSession.mockRejectedValue(new Error('Rate limit exceeded'));

      const service = getStorageService();
      const updatedSession = { ...session, name: 'Updated Name' };

      await expect(service.updateSession(userId, updatedSession, true)).rejects.toThrow('Rate limit exceeded');

      // retryOnce should attempt backend mutation twice
      expect(mockBackendService.updateSession).toHaveBeenCalledTimes(2);

      // Optimistic change must be rolled back after backend failure
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions[0].name).toBe('Original Name');
    });
  });

  describe('renameSession', () => {
    it('renames session in localStorage mirror', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Original Name', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      await service.renameSession(userId, 'session-1', 'Renamed Session');

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions[0].name).toBe('Renamed Session');
    });

    it('rolls back optimistic rename and throws when backend rename fails', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Original Name', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.renameSession.mockRejectedValue(new Error('Rate limit exceeded'));

      const service = getStorageService();
      await expect(service.renameSession(userId, 'session-1', 'Renamed Session', true)).rejects.toThrow('Rate limit exceeded');

      // retryOnce should attempt backend mutation twice
      expect(mockBackendService.renameSession).toHaveBeenCalledTimes(2);

      // Optimistic change must be rolled back after backend failure
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions[0].name).toBe('Original Name');
    });
  });

  describe('deleteSession', () => {
    it('removes session from storage', async () => {
      const userId = 'test-user';
      const session1 = createTestSession('session-1', 'Chat 1', userId);
      const session2 = createTestSession('session-2', 'Chat 2', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session1, session2],
        activeSessionId: session1.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      await service.deleteSession(userId, 'session-1');

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions).toHaveLength(1);
      expect(stored[userId].sessions[0].id).toBe('session-2');
    });

    it('updates activeSessionId when deleting active session', async () => {
      const userId = 'test-user';
      const session1 = createTestSession('session-1', 'Chat 1', userId);
      const session2 = createTestSession('session-2', 'Chat 2', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session1, session2],
        activeSessionId: 'session-1',
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      await service.deleteSession(userId, 'session-1');

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].activeSessionId).toBe('session-2');
    });

    it('sets activeSessionId to null when deleting last session', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: 'session-1',
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      await service.deleteSession(userId, 'session-1');

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions).toHaveLength(0);
      expect(stored[userId].activeSessionId).toBeNull();
    });

    it('rolls back optimistic delete and throws when backend delete fails', async () => {
      const userId = 'test-user';
      const session1 = createTestSession('session-1', 'Chat 1', userId);
      const session2 = createTestSession('session-2', 'Chat 2', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session1, session2],
        activeSessionId: session1.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.deleteSession.mockRejectedValue(new Error('Rate limit exceeded'));

      const service = getStorageService();
      await expect(service.deleteSession(userId, 'session-1', true)).rejects.toThrow('Rate limit exceeded');

      // retryOnce should attempt backend mutation twice
      expect(mockBackendService.deleteSession).toHaveBeenCalledTimes(2);

      // Optimistic delete must be rolled back after backend failure
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions).toHaveLength(2);
      expect(stored[userId].sessions.map((s: ChatSession) => s.id)).toContain('session-1');
      expect(stored[userId].activeSessionId).toBe('session-1');
    });

    it('treats backend Session not found as already-deleted and keeps local state clean', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Chat 1', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.deleteSession.mockRejectedValue(new NotFoundError('Session not found', 404, 'NOT_FOUND'));

      const service = getStorageService();
      await expect(service.deleteSession(userId, 'session-1', true)).resolves.toBeUndefined();

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions).toHaveLength(0);
      expect(stored[userId].activeSessionId).toBeNull();
      expect(mockBackendService.deleteSession).toHaveBeenCalledTimes(2);
    });
  });

  describe('updateMessage', () => {
    it('rolls back optimistic message update and throws when backend update fails', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [{
          ...session,
          messages: [
            {
              id: 'msg-1',
              role: 'assistant',
              content: 'Original content',
              timestamp: Date.now(),
            },
          ],
        }],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.updateMessage.mockRejectedValue(new Error('Rate limit exceeded'));

      const service = getStorageService();
      await expect(
        service.updateMessage(userId, 'session-1', 'msg-1', 'Updated content', undefined, true)
      ).rejects.toThrow('Rate limit exceeded');

      // retryOnce should attempt backend mutation twice
      expect(mockBackendService.updateMessage).toHaveBeenCalledTimes(2);

      // Optimistic change must be rolled back after backend failure
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions[0].messages[0].content).toBe('Original content');
    });

    it('purges local session when backend reports Session not found during message update', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [{
          ...session,
          messages: [
            {
              id: 'msg-1',
              role: 'assistant',
              content: 'Original content',
              timestamp: Date.now(),
            },
          ],
        }],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.updateMessage
        .mockRejectedValueOnce(new NotFoundError('Session not found', 404, 'NOT_FOUND'))
        .mockRejectedValueOnce(new NotFoundError('Session not found', 404, 'NOT_FOUND'))
        .mockResolvedValueOnce(undefined);
      mockBackendService.createSession.mockResolvedValue(session);

      const service = getStorageService();
      await expect(
        service.updateMessage(userId, 'session-1', 'msg-1', 'Updated content', undefined, true)
      ).resolves.toBeUndefined();

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions).toHaveLength(1);
      expect(mockBackendService.createSession).toHaveBeenCalledTimes(1);
      expect(mockBackendService.updateMessage).toHaveBeenCalledTimes(3);
    });

    it('does not purge local session on Message not found during update', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [{
          ...session,
          messages: [
            {
              id: 'msg-1',
              role: 'assistant',
              content: 'Original content',
              timestamp: Date.now(),
            },
          ],
        }],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.updateMessage.mockRejectedValue(new NotFoundError('Message not found', 404, 'NOT_FOUND'));

      const service = getStorageService();
      await expect(
        service.updateMessage(userId, 'session-1', 'msg-1', 'Updated content', undefined, true)
      ).rejects.toBeInstanceOf(NotFoundError);

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions).toHaveLength(1);
      expect(stored[userId].sessions[0].id).toBe('session-1');
    });
  });

  describe('addMessage', () => {
    it('purges local session when backend reports Session not found during message add', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      mockBackendService.isBackendAvailable.mockResolvedValue(true);
      mockBackendService.addMessage
        .mockRejectedValueOnce(new NotFoundError('Session not found', 404, 'NOT_FOUND'))
        .mockRejectedValueOnce(new NotFoundError('Session not found', 404, 'NOT_FOUND'))
        .mockResolvedValueOnce(undefined);
      mockBackendService.createSession.mockResolvedValue(session);

      const service = getStorageService();
      await expect(
        service.addMessage(userId, 'session-1', { id: 'msg-1', role: 'user', content: 'hello', timestamp: Date.now() }, true)
      ).resolves.toBeUndefined();

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions).toHaveLength(1);
      expect(mockBackendService.createSession).toHaveBeenCalledTimes(1);
      expect(mockBackendService.addMessage).toHaveBeenCalledTimes(3);
    });
  });

  describe('clearAllHistory', () => {
    it('clears all sessions for user', async () => {
      const userId = 'test-user';
      const session1 = createTestSession('session-1', 'Chat 1', userId);
      const session2 = createTestSession('session-2', 'Chat 2', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session1, session2],
        activeSessionId: session1.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();
      await service.clearAllHistory(userId);

      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}');
      expect(stored[userId].sessions).toHaveLength(0);
      expect(stored[userId].activeSessionId).toBeNull();
    });
  });

  describe('getConnectionStatus', () => {
    it('returns localStorage status when backend unavailable', async () => {
      const service = getStorageService();
      const status = await service.getConnectionStatus();

      expect(status.connected).toBe(false);
      expect(status.message).toContain('local storage');
    });
  });

  describe('caching behavior', () => {
    it('returns cached data on subsequent calls', async () => {
      const userId = 'test-user';
      const session = createTestSession('session-1', 'Test Chat', userId);
      const storedHistory: UserChatHistory = {
        userId,
        sessions: [session],
        activeSessionId: session.id,
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: storedHistory }));

      const service = getStorageService();

      // First call should read from localStorage
      const history1 = await service.getUserHistory(userId);
      expect(history1.sessions).toHaveLength(1);

      // Modify localStorage directly (simulating external change)
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ [userId]: { ...storedHistory, sessions: [] } }));

      // Second call should return cached data (not the modified localStorage)
      const history2 = await service.getUserHistory(userId);
      expect(history2.sessions).toHaveLength(1); // Still 1 because cached
    });
  });
});
