-- Migration: 000001_create_tables
-- Create initial schema for chat session persistence

-- Sessions table stores chat conversations
CREATE TABLE IF NOT EXISTS aichat_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    is_active INTEGER NOT NULL DEFAULT 0
);

-- Index for querying sessions by user
CREATE INDEX IF NOT EXISTS idx_aichat_sessions_user_id ON aichat_sessions(user_id);

-- Index for sorting sessions by last update
CREATE INDEX IF NOT EXISTS idx_aichat_sessions_updated_at ON aichat_sessions(updated_at);

-- Messages table stores individual chat messages
CREATE TABLE IF NOT EXISTS aichat_messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    token_count INTEGER NOT NULL DEFAULT 0,
    is_pinned INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (session_id) REFERENCES aichat_sessions(id) ON DELETE CASCADE
);

-- Index for querying messages by session
CREATE INDEX IF NOT EXISTS idx_aichat_messages_session_id ON aichat_messages(session_id);

-- Index for sorting messages by timestamp
CREATE INDEX IF NOT EXISTS idx_aichat_messages_timestamp ON aichat_messages(timestamp);
