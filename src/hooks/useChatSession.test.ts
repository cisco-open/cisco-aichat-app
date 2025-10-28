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

import { act, waitFor } from '@testing-library/react';
import { renderHookWithProviders } from '../testutil/providers';
import { useChatSession } from './useChatSession';
import { ChatSession, ChatMessage } from '../types/chat';

// Mock ChatHistoryService - define mock before jest.mock
const mockHistoryService = {
  getActiveSession: jest.fn(),
  getAllSessions: jest.fn(),
  createSession: jest.fn(),
  switchToSession: jest.fn(),
  deleteSession: jest.fn(),
  renameSession: jest.fn(),
  clearAllHistory: jest.fn(),
  addMessage: jest.fn(),
  updateMessage: jest.fn(),
};

jest.mock('../services/ChatHistoryService', () => {
  return {
    ChatHistoryService: {
      getInstance: jest.fn(),
    },
  };
});

// Import after mocking
import { ChatHistoryService } from '../services/ChatHistoryService';

// Setup mock implementation before tests
beforeAll(() => {
  (ChatHistoryService.getInstance as jest.Mock).mockResolvedValue(mockHistoryService);
});

// Test fixtures
function createMockSession(overrides: Partial<ChatSession> = {}): ChatSession {
  return {
    id: `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
    name: 'Test Chat',
    userId: 'test-user',
    messages: [],
    createdAt: Date.now(),
    updatedAt: Date.now(),
    isActive: true,
    ...overrides,
  };
}

function createMockMessage(overrides: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id: `msg_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
    role: 'user',
    content: 'Hello, AI!',
    timestamp: Date.now(),
    ...overrides,
  };
}

describe('useChatSession', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    sessionStorage.setItem('aichat_welcome_choice_made', 'true');
    // Default mock implementations
    mockHistoryService.getActiveSession.mockResolvedValue(null);
    mockHistoryService.getAllSessions.mockResolvedValue([]);
  });

  afterEach(() => {
    sessionStorage.clear();
  });

  describe('initial state', () => {
    it('loads sessions on mount', async () => {
      const mockSession = createMockSession({ name: 'Initial Chat' });
      mockHistoryService.getActiveSession.mockResolvedValue(mockSession);
      mockHistoryService.getAllSessions.mockResolvedValue([mockSession]);

      const { result } = renderHookWithProviders(() => useChatSession());

      // Initially loading
      expect(result.current.isLoading).toBe(true);

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.currentSession).toEqual(mockSession);
      expect(result.current.allSessions).toHaveLength(1);
      expect(result.current.error).toBeNull();
    });

    it('handles load error gracefully', async () => {
      mockHistoryService.getActiveSession.mockRejectedValue(new Error('Storage error'));
      mockHistoryService.getAllSessions.mockRejectedValue(new Error('Storage error'));

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.error).toBe('Failed to load chat history');
      expect(result.current.currentSession).toBeNull();
    });

    it('starts with no session when none exists', async () => {
      mockHistoryService.getActiveSession.mockResolvedValue(null);
      mockHistoryService.getAllSessions.mockResolvedValue([]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.currentSession).toBeNull();
      expect(result.current.allSessions).toHaveLength(0);
      expect(result.current.messages).toHaveLength(0);
    });
  });

  describe('createNewSession', () => {
    it('creates session with given name', async () => {
      const newSession = createMockSession({ name: 'My Custom Chat' });
      mockHistoryService.createSession.mockResolvedValue(newSession);
      mockHistoryService.getAllSessions.mockResolvedValue([newSession]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        await result.current.createNewSession('My Custom Chat');
      });

      expect(mockHistoryService.createSession).toHaveBeenCalledWith('My Custom Chat');
      expect(result.current.currentSession?.name).toBe('My Custom Chat');
      expect(result.current.error).toBeNull();
    });

    it('creates session without name (default name)', async () => {
      const newSession = createMockSession({ name: 'Chat 1' });
      mockHistoryService.createSession.mockResolvedValue(newSession);
      mockHistoryService.getAllSessions.mockResolvedValue([newSession]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        await result.current.createNewSession();
      });

      expect(mockHistoryService.createSession).toHaveBeenCalledWith(undefined);
      expect(result.current.currentSession).not.toBeNull();
    });

    it('sets isCreatingSession during creation', async () => {
      let resolveCreate: (value: ChatSession) => void;
      const createPromise = new Promise<ChatSession>((resolve) => {
        resolveCreate = resolve;
      });
      mockHistoryService.createSession.mockReturnValue(createPromise);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      // Start creation
      let createPromiseFromHook: Promise<void>;
      act(() => {
        createPromiseFromHook = result.current.createNewSession('Test');
      });

      // Should be creating
      expect(result.current.isCreatingSession).toBe(true);

      // Resolve the promise
      const newSession = createMockSession({ name: 'Test' });
      mockHistoryService.getAllSessions.mockResolvedValue([newSession]);

      await act(async () => {
        resolveCreate!(newSession);
        await createPromiseFromHook;
      });

      expect(result.current.isCreatingSession).toBe(false);
    });

    it('handles creation error', async () => {
      mockHistoryService.createSession.mockRejectedValue(new Error('Create failed'));

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        await result.current.createNewSession('Test');
      });

      expect(result.current.error).toBe('Failed to create new session');
    });
  });

  describe('switchSession', () => {
    it('switches to different session', async () => {
      const session1 = createMockSession({ id: 'session-1', name: 'Chat 1' });
      const session2 = createMockSession({ id: 'session-2', name: 'Chat 2' });

      mockHistoryService.getActiveSession.mockResolvedValue(session1);
      mockHistoryService.getAllSessions.mockResolvedValue([session1, session2]);
      mockHistoryService.switchToSession.mockResolvedValue(session2);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.currentSession?.id).toBe('session-1');

      await act(async () => {
        await result.current.switchSession('session-2');
      });

      expect(mockHistoryService.switchToSession).toHaveBeenCalledWith('session-2');
      expect(result.current.currentSession?.id).toBe('session-2');
      expect(result.current.error).toBeNull();
    });

    it('handles session not found', async () => {
      mockHistoryService.switchToSession.mockResolvedValue(null);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        await result.current.switchSession('nonexistent');
      });

      expect(result.current.error).toBe('Session not found');
    });

    it('sets isSwitchingSession during switch', async () => {
      let resolveSwitch: (value: ChatSession | null) => void;
      const switchPromise = new Promise<ChatSession | null>((resolve) => {
        resolveSwitch = resolve;
      });
      mockHistoryService.switchToSession.mockReturnValue(switchPromise);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      let switchPromiseFromHook: Promise<void>;
      act(() => {
        switchPromiseFromHook = result.current.switchSession('session-2');
      });

      expect(result.current.isSwitchingSession).toBe(true);

      await act(async () => {
        resolveSwitch!(createMockSession({ id: 'session-2' }));
        await switchPromiseFromHook;
      });

      expect(result.current.isSwitchingSession).toBe(false);
    });
  });

  describe('deleteSession', () => {
    it('removes session and updates list', async () => {
      const session1 = createMockSession({ id: 'session-1', name: 'Chat 1' });
      const session2 = createMockSession({ id: 'session-2', name: 'Chat 2' });

      mockHistoryService.getActiveSession.mockResolvedValue(session1);
      mockHistoryService.getAllSessions.mockResolvedValue([session1, session2]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.allSessions).toHaveLength(2);

      mockHistoryService.deleteSession.mockResolvedValue(true);
      mockHistoryService.getAllSessions.mockResolvedValue([session2]);
      mockHistoryService.getActiveSession.mockResolvedValue(session2);

      await act(async () => {
        await result.current.deleteSession('session-1');
      });

      expect(mockHistoryService.deleteSession).toHaveBeenCalledWith('session-1');
      expect(result.current.allSessions).toHaveLength(1);
      expect(result.current.currentSession?.id).toBe('session-2');
      expect(result.current.error).toBeNull();
    });

    it('handles delete failure', async () => {
      const session = createMockSession({ id: 'session-1' });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);
      mockHistoryService.deleteSession.mockResolvedValue(false);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      await act(async () => {
        await result.current.deleteSession('session-1');
      });

      expect(result.current.error).toBe('Failed to delete session');
    });

    it('clears current session when all deleted', async () => {
      const session = createMockSession({ id: 'session-1' });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      mockHistoryService.deleteSession.mockResolvedValue(true);
      mockHistoryService.getAllSessions.mockResolvedValue([]);

      await act(async () => {
        await result.current.deleteSession('session-1');
      });

      expect(result.current.currentSession).toBeNull();
      expect(result.current.allSessions).toHaveLength(0);
    });
  });

  describe('renameSession', () => {
    it('renames session successfully', async () => {
      const session = createMockSession({ id: 'session-1', name: 'Old Name' });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      const renamedSession = { ...session, name: 'New Name', updatedAt: Date.now() };
      mockHistoryService.renameSession.mockResolvedValue(true);
      mockHistoryService.getAllSessions.mockResolvedValue([renamedSession]);

      await act(async () => {
        await result.current.renameSession('session-1', 'New Name');
      });

      expect(mockHistoryService.renameSession).toHaveBeenCalledWith('session-1', 'New Name');
      // Optimistic update applies immediately
      expect(result.current.currentSession?.name).toBe('New Name');
    });

    it('reverts optimistic update on failure', async () => {
      const session = createMockSession({ id: 'session-1', name: 'Original Name' });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      mockHistoryService.renameSession.mockResolvedValue(false);
      // After failure, getAllSessions returns original
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      await act(async () => {
        await result.current.renameSession('session-1', 'New Name');
      });

      expect(result.current.error).toBe('Failed to rename session');
      expect(result.current.currentSession?.name).toBe('Original Name');
    });
  });

  describe('clearAllHistory', () => {
    it('clears all sessions', async () => {
      const session = createMockSession();
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      mockHistoryService.clearAllHistory.mockResolvedValue(undefined);

      await act(async () => {
        await result.current.clearAllHistory();
      });

      expect(mockHistoryService.clearAllHistory).toHaveBeenCalled();
      expect(result.current.currentSession).toBeNull();
      expect(result.current.allSessions).toHaveLength(0);
    });
  });

  describe('addMessage', () => {
    it('adds message to current session', async () => {
      const session = createMockSession({ id: 'session-1', messages: [] });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.messages).toHaveLength(0);

      const newMessage = createMockMessage({ content: 'Hello!' });
      mockHistoryService.addMessage.mockResolvedValue(undefined);
      const sessionWithMessage = {
        ...session,
        messages: [newMessage],
        updatedAt: Date.now(),
      };
      mockHistoryService.getAllSessions.mockResolvedValue([sessionWithMessage]);

      await act(async () => {
        await result.current.addMessage(newMessage);
      });

      expect(mockHistoryService.addMessage).toHaveBeenCalledWith(newMessage, 'session-1');
      // Optimistic update adds message immediately
      expect(result.current.messages).toHaveLength(1);
      expect(result.current.messages[0].content).toBe('Hello!');
    });

    it('handles add message error', async () => {
      const session = createMockSession({ id: 'session-1', messages: [] });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      const newMessage = createMockMessage({ content: 'Hello!' });
      mockHistoryService.addMessage.mockRejectedValue(new Error('Save failed'));
      // After error revert, getActiveSession returns original
      mockHistoryService.getActiveSession.mockResolvedValue(session);

      await act(async () => {
        await result.current.addMessage(newMessage);
      });

      expect(result.current.error).toBe('Failed to save message');
      // After revert, messages should be empty
      expect(result.current.messages).toHaveLength(0);
    });
  });

  describe('updateMessage', () => {
    it('updates message content in current session', async () => {
      const existingMessage = createMockMessage({ id: 'msg-1', content: 'Original' });
      const session = createMockSession({
        id: 'session-1',
        messages: [existingMessage],
      });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.messages[0].content).toBe('Original');

      const updatedSession = {
        ...session,
        messages: [{ ...existingMessage, content: 'Updated content' }],
      };
      mockHistoryService.updateMessage.mockResolvedValue(undefined);
      mockHistoryService.getAllSessions.mockResolvedValue([updatedSession]);

      await act(async () => {
        await result.current.updateMessage('msg-1', 'Updated content');
      });

      expect(mockHistoryService.updateMessage).toHaveBeenCalledWith('msg-1', 'Updated content', undefined, 'session-1');
      // Optimistic update applies immediately
      expect(result.current.messages[0].content).toBe('Updated content');
    });

    it('updates message with tool executions', async () => {
      const existingMessage = createMockMessage({ id: 'msg-1', content: 'Original', role: 'assistant' });
      const session = createMockSession({
        id: 'session-1',
        messages: [existingMessage],
      });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      const toolExecutions = [{ toolName: 'search', status: 'success' as const }];
      mockHistoryService.updateMessage.mockResolvedValue(undefined);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      await act(async () => {
        await result.current.updateMessage('msg-1', 'Updated', toolExecutions);
      });

      expect(mockHistoryService.updateMessage).toHaveBeenCalledWith('msg-1', 'Updated', toolExecutions, 'session-1');
      expect(result.current.messages[0].toolExecutions).toEqual(toolExecutions);
    });
  });

  describe('messages computed property', () => {
    it('returns empty array when no current session', async () => {
      mockHistoryService.getActiveSession.mockResolvedValue(null);
      mockHistoryService.getAllSessions.mockResolvedValue([]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.messages).toEqual([]);
    });

    it('returns messages from current session', async () => {
      const messages = [
        createMockMessage({ id: 'msg-1', content: 'Hello', role: 'user' }),
        createMockMessage({ id: 'msg-2', content: 'Hi there!', role: 'assistant' }),
      ];
      const session = createMockSession({ messages });
      mockHistoryService.getActiveSession.mockResolvedValue(session);
      mockHistoryService.getAllSessions.mockResolvedValue([session]);

      const { result } = renderHookWithProviders(() => useChatSession());

      await waitFor(() => {
        expect(result.current.isLoading).toBe(false);
      });

      expect(result.current.messages).toHaveLength(2);
      expect(result.current.messages[0].content).toBe('Hello');
      expect(result.current.messages[1].content).toBe('Hi there!');
    });
  });
});
