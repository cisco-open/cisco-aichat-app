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

import React, { useState, useMemo, useCallback, useEffect } from 'react';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import {
  Button,
  IconButton,
  Input,
  ConfirmModal,
  useStyles2,
  Tooltip,
  Icon
} from '@grafana/ui';
import { ChatSession, SearchResult } from '../types/chat';
import { InlineEditableText } from './InlineEditableText';
import { SearchResultsList } from './SearchResultsList';
import { ChatBackendService } from '../services/ChatBackendService';

const getStyles = (theme: GrafanaTheme2) => ({
  sidebar: css`
    width: 320px;
    height: 100%;
    background: ${theme.colors.background.primary};
    border-right: 1px solid ${theme.colors.border.medium};
    display: flex;
    flex-direction: column;
    overflow: hidden;
  `,
  header: css`
    padding: ${theme.spacing(2)};
    border-bottom: 1px solid ${theme.colors.border.medium};
    display: flex;
    justify-content: space-between;
    align-items: center;
  `,
  title: css`
    font-weight: ${theme.typography.fontWeightMedium};
    color: ${theme.colors.text.primary};
    margin: 0;
  `,
  searchContainer: css`
    padding: ${theme.spacing(1.5)} ${theme.spacing(2)};
    border-bottom: 1px solid ${theme.colors.border.weak};
  `,
  searchInput: css`
    width: 100%;
  `,
  sessionsList: css`
    flex: 1;
    overflow-y: auto;
    padding: ${theme.spacing(1)};
  `,
  groupContainer: css`
    margin-bottom: ${theme.spacing(2)};
  `,
  groupHeader: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: ${theme.spacing(1)} ${theme.spacing(1.5)};
    margin-bottom: ${theme.spacing(0.5)};
    font-size: ${theme.typography.size.sm};
    font-weight: ${theme.typography.fontWeightMedium};
    color: ${theme.colors.text.secondary};
    text-transform: uppercase;
    letter-spacing: 0.5px;
  `,
  groupCount: css`
    font-size: ${theme.typography.size.xs};
    color: ${theme.colors.text.disabled};
    font-weight: ${theme.typography.fontWeightRegular};
    background: ${theme.colors.background.secondary};
    padding: ${theme.spacing(0.25)} ${theme.spacing(0.75)};
    border-radius: ${theme.shape.radius.pill};
  `,
  sessionItem: css`
    display: flex;
    align-items: center;
    padding: ${theme.spacing(1.5)};
    margin-bottom: ${theme.spacing(0.5)};
    border-radius: ${theme.shape.borderRadius()};
    cursor: pointer;
    transition: background-color 0.2s;
    border: 1px solid transparent;

    &:hover {
      background: ${theme.colors.background.secondary};
    }
  `,
  sessionItemActive: css`
    background: ${theme.colors.primary.main}20;
    border: 1px solid ${theme.colors.primary.main};
  `,
  sessionContent: css`
    flex: 1;
    min-width: 0;
    margin-right: ${theme.spacing(1)};
  `,
  sessionName: css`
    font-size: ${theme.typography.size.sm};
    font-weight: ${theme.typography.fontWeightMedium};
    color: ${theme.colors.text.primary};
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    margin-bottom: ${theme.spacing(0.5)};
  `,
  sessionPreview: css`
    font-size: ${theme.typography.size.xs};
    color: ${theme.colors.text.secondary};
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    line-height: 1.3;
    margin-bottom: ${theme.spacing(0.25)};
  `,
  sessionMeta: css`
    font-size: ${theme.typography.size.xs};
    color: ${theme.colors.text.disabled};
    display: flex;
    align-items: center;
    gap: ${theme.spacing(1)};
  `,
  sessionActions: css`
    display: flex;
    gap: ${theme.spacing(0.5)};
    opacity: 0;
    transition: opacity 0.2s;

    .session-item:hover & {
      opacity: 1;
    }
  `,
  actionButton: css`
    width: 24px;
    height: 24px;
    padding: 0;
  `,
  footer: css`
    padding: ${theme.spacing(2)};
    border-top: 1px solid ${theme.colors.border.medium};
  `,
  emptyState: css`
    padding: ${theme.spacing(3)};
    text-align: center;
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.size.sm};
  `,
  noResults: css`
    padding: ${theme.spacing(3)};
    text-align: center;
    color: ${theme.colors.text.secondary};
    font-size: ${theme.typography.size.sm};
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: ${theme.spacing(1)};
  `,
  newSessionButton: css`
    width: 100%;
    margin-bottom: ${theme.spacing(1)};
  `,
});

interface ChatSidebarProps {
  sessions: ChatSession[];
  currentSessionId: string | null;
  onSessionSelect: (sessionId: string) => Promise<void>;
  onSessionCreate: (name?: string) => Promise<void>;
  onSessionDelete: (sessionId: string) => Promise<void>;
  onSessionRename: (sessionId: string, newName: string) => Promise<void>;
  onClearHistory: () => Promise<void>;
  onCollapse: () => void;
  isCreatingSession?: boolean;
  isSwitchingSession?: boolean;
  isDeletingSession?: boolean;
  isRenamingSession?: boolean;
  isClearingHistory?: boolean;
}

// Date grouping categories
type DateGroup = 'today' | 'yesterday' | 'last7days' | 'older';

interface GroupedSessions {
  today: ChatSession[];
  yesterday: ChatSession[];
  last7days: ChatSession[];
  older: ChatSession[];
}

const GROUP_LABELS: Record<DateGroup, string> = {
  today: 'Today',
  yesterday: 'Yesterday',
  last7days: 'Last 7 Days',
  older: 'Older'
};

export function ChatSidebar({
  sessions,
  currentSessionId,
  onSessionSelect,
  onSessionCreate,
  onSessionDelete,
  onSessionRename,
  onClearHistory,
  onCollapse,
  isCreatingSession = false,
  isSwitchingSession = false,
  isDeletingSession = false,
  isRenamingSession = false,
  isClearingHistory = false
}: ChatSidebarProps) {
  const s = useStyles2(getStyles);
  const [editingSessionId, setEditingSessionId] = useState<string | null>(null);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [clearModalOpen, setClearModalOpen] = useState(false);
  const [selectedSession, setSelectedSession] = useState<ChatSession | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('');
  // Backend search state
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);

  // Debounce search query (300ms delay)
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchQuery(searchQuery);
    }, 300);

    return () => clearTimeout(timer);
  }, [searchQuery]);

  // Perform backend search when debounced query changes
  useEffect(() => {
    const performSearch = async () => {
      if (!debouncedSearchQuery.trim()) {
        setSearchResults([]);
        setSearchError(null);
        return;
      }

      setIsSearching(true);
      setSearchError(null);

      try {
        const backendService = ChatBackendService.getInstance();
        const results = await backendService.searchMessages(debouncedSearchQuery);
        setSearchResults(results);
      } catch (error: any) {
        console.error('Backend search failed:', error);
        setSearchError(error.message || 'Search failed');
        setSearchResults([]);
      } finally {
        setIsSearching(false);
      }
    };

    performSearch();
  }, [debouncedSearchQuery]);

  // Get date group for a session
  const getDateGroup = useCallback((timestamp: number): DateGroup => {
    const now = new Date();
    const date = new Date(timestamp);
    const diffMs = now.getTime() - date.getTime();
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffDays === 0) {
      return 'today';
    } else if (diffDays === 1) {
      return 'yesterday';
    } else if (diffDays < 7) {
      return 'last7days';
    } else {
      return 'older';
    }
  }, []);

  // Get conversation preview from first user message
  const getSessionPreview = useCallback((session: ChatSession): string => {
    const firstUserMessage = session.messages.find(msg => msg.role === 'user');
    if (!firstUserMessage) {
      return 'No messages yet';
    }
    // Truncate to 60 characters
    const preview = firstUserMessage.content.trim();
    return preview.length > 60 ? preview.substring(0, 60) + '...' : preview;
  }, []);

  // Filter and group sessions
  const { filteredSessions, groupedSessions } = useMemo(() => {
    console.log('[ChatSidebar] Filtering sessions with query:', debouncedSearchQuery);
    console.log('[ChatSidebar] Total sessions:', sessions.length);

    // Filter sessions based on search query
    const filtered = sessions.filter(session => {
      if (!debouncedSearchQuery.trim()) {
        return true;
      }

      const query = debouncedSearchQuery.toLowerCase();
      const nameMatch = session.name.toLowerCase().includes(query);
      const messageMatch = session.messages.some(msg =>
        msg.content.toLowerCase().includes(query)
      );

      const isMatch = nameMatch || messageMatch;
      if (isMatch) {
        console.log('[ChatSidebar] Match found:', session.name, { nameMatch, messageMatch });
      }

      return isMatch;
    });

    console.log('[ChatSidebar] Filtered sessions:', filtered.length);

    // Group sessions by date
    const grouped: GroupedSessions = {
      today: [],
      yesterday: [],
      last7days: [],
      older: []
    };

    filtered.forEach(session => {
      const group = getDateGroup(session.updatedAt);
      grouped[group].push(session);
    });

    // Sort each group by updatedAt (most recent first)
    Object.keys(grouped).forEach(key => {
      grouped[key as DateGroup].sort((a, b) => b.updatedAt - a.updatedAt);
    });

    console.log('[ChatSidebar] Grouped sessions:', {
      today: grouped.today.length,
      yesterday: grouped.yesterday.length,
      last7days: grouped.last7days.length,
      older: grouped.older.length
    });

    return { filteredSessions: filtered, groupedSessions: grouped };
  }, [sessions, debouncedSearchQuery, getDateGroup]);

  const formatDate = (timestamp: number) => {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffDays === 0) {
      return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    } else if (diffDays === 1) {
      return 'Yesterday';
    } else if (diffDays < 7) {
      return date.toLocaleDateString([], { weekday: 'short' });
    } else {
      return date.toLocaleDateString([], { month: 'short', day: 'numeric' });
    }
  };

  const handleDelete = (session: ChatSession) => {
    setSelectedSession(session);
    setDeleteModalOpen(true);
  };

  const confirmDelete = async () => {
    if (selectedSession) {
      await onSessionDelete(selectedSession.id);
    }
    setDeleteModalOpen(false);
    setSelectedSession(null);
  };

  const confirmClearHistory = async () => {
    await onClearHistory();
    setClearModalOpen(false);
  };

  // Handle click on search result - navigate to session
  const handleSearchResultClick = async (sessionId: string, messageId: string) => {
    // Load session
    await onSessionSelect(sessionId);
    // Clear search to show full session
    setSearchQuery('');
    setSearchResults([]);
    // Note: Scroll to messageId can be enhanced later
    // For now, just navigate to session
    console.log('Navigate to message:', messageId);
  };

  // Render a session item
  const renderSessionItem = (session: ChatSession) => {
    const preview = getSessionPreview(session);
    const isEditing = editingSessionId === session.id;

    return (
      <div
        key={session.id}
        className={`session-item ${s.sessionItem} ${
          session.id === currentSessionId ? s.sessionItemActive : ''
        }`}
        onClick={() => {
          // Prevent session selection while editing
          if (isEditing) {
            return;
          }
          onSessionSelect(session.id);
        }}
      >
        <div className={s.sessionContent}>
          {isEditing ? (
            <InlineEditableText
              value={session.name}
              onSave={async (newName) => {
                await onSessionRename(session.id, newName);
                setEditingSessionId(null);
              }}
              placeholder="Session name"
            />
          ) : (
            <div
              className={s.sessionName}
              onDoubleClick={(e) => {
                e.stopPropagation();
                setEditingSessionId(session.id);
              }}
            >
              {session.name}
            </div>
          )}
          <div className={s.sessionPreview}>{preview}</div>
          <div className={s.sessionMeta}>
            <span>{session.messages.length} messages</span>
            <span>•</span>
            <span>{formatDate(session.updatedAt)}</span>
          </div>
        </div>
        <div className={s.sessionActions}>
          <Tooltip content="Rename">
            <IconButton
              name="edit"
              size="sm"
              aria-label="Rename session"
              className={s.actionButton}
              onClick={(e) => {
                e.stopPropagation();
                setEditingSessionId(session.id);
              }}
            />
          </Tooltip>
          <Tooltip content="Delete">
            <IconButton
              name="trash-alt"
              size="sm"
              aria-label="Delete session"
              className={s.actionButton}
              onClick={(e) => {
                e.stopPropagation();
                handleDelete(session);
              }}
            />
          </Tooltip>
        </div>
      </div>
    );
  };

  // Render a group of sessions
  const renderGroup = (groupKey: DateGroup) => {
    const groupSessions = groupedSessions[groupKey];
    if (groupSessions.length === 0) {
      return null;
    }

    return (
      <div key={groupKey} className={s.groupContainer}>
        <div className={s.groupHeader}>
          <span>{GROUP_LABELS[groupKey]}</span>
          <span className={s.groupCount}>{groupSessions.length}</span>
        </div>
        {groupSessions.map(renderSessionItem)}
      </div>
    );
  };

  return (
    <div className={s.sidebar}>
      <div className={s.header}>
        <h3 className={s.title}>Chat History</h3>
        <Tooltip content="Hide chat history">
          <IconButton
            name="times"
            size="md"
            aria-label="Hide chat history"
            onClick={onCollapse}
          />
        </Tooltip>
      </div>

      {/* Search Bar */}
      <div className={s.searchContainer}>
        <Input
          className={s.searchInput}
          prefix={<Icon name="search" />}
          placeholder="Search conversations..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.currentTarget.value)}
          suffix={
            searchQuery && (
              <IconButton
                name="times"
                size="sm"
                aria-label="Clear search"
                onClick={() => setSearchQuery('')}
                tooltip="Clear search"
              />
            )
          }
        />
      </div>

      {/* Sessions List with Grouping or Search Results */}
      <div className={s.sessionsList}>
        {/* Backend search results (when query is active) */}
        {debouncedSearchQuery.trim() ? (
          searchError ? (
            <div className={s.noResults}>
              <Icon name="exclamation-triangle" size="xl" />
              <div>Search error</div>
              <div style={{ fontSize: '12px', marginTop: '4px' }}>
                {searchError}
              </div>
            </div>
          ) : (
            <SearchResultsList
              results={searchResults}
              isLoading={isSearching}
              onResultClick={handleSearchResultClick}
            />
          )
        ) : sessions.length === 0 ? (
          <div className={s.emptyState}>
            No chat sessions yet.<br />
            Start a new conversation!
          </div>
        ) : filteredSessions.length === 0 ? (
          <div className={s.noResults}>
            <Icon name="search" size="xl" />
            <div>No conversations found</div>
            <div style={{ fontSize: '12px', marginTop: '4px' }}>
              Try a different search term
            </div>
          </div>
        ) : (
          <>
            {renderGroup('today')}
            {renderGroup('yesterday')}
            {renderGroup('last7days')}
            {renderGroup('older')}
          </>
        )}
      </div>

      <div className={s.footer}>
        <Button
          variant="secondary"
          size="sm"
          className={s.newSessionButton}
          onClick={() => onSessionCreate()}
          disabled={isCreatingSession}
        >
          {isCreatingSession ? 'Creating...' : 'New Chat'}
        </Button>
        {sessions.length > 0 && (
          <Button
            variant="destructive"
            size="sm"
            fill="outline"
            onClick={() => setClearModalOpen(true)}
            disabled={isClearingHistory}
          >
            {isClearingHistory ? 'Clearing...' : 'Clear All History'}
          </Button>
        )}
      </div>

      {/* Delete Confirmation Modal */}
      <ConfirmModal
        isOpen={deleteModalOpen}
        title="Delete Chat Session"
        body={`Are you sure you want to delete "${selectedSession?.name}"? This action cannot be undone.`}
        confirmText="Delete"
        onConfirm={confirmDelete}
        onDismiss={() => setDeleteModalOpen(false)}
      />

      {/* Clear History Confirmation Modal */}
      <ConfirmModal
        isOpen={clearModalOpen}
        title="Clear All Chat History"
        body="Are you sure you want to delete all chat sessions? This action cannot be undone."
        confirmText="Clear All"
        onConfirm={confirmClearHistory}
        onDismiss={() => setClearModalOpen(false)}
      />
    </div>
  );
}
