-- Migration: 000002_add_summary_fields (down)
-- Remove summary fields from messages table

DROP INDEX IF EXISTS idx_aichat_messages_is_summary;

-- SQLite doesn't support DROP COLUMN in older versions
-- Modern SQLite (3.35.0+) supports it, but for compatibility we use table recreation
-- This is a destructive operation - data will be lost

CREATE TABLE aichat_messages_backup AS SELECT
    id, session_id, role, content, timestamp, token_count, is_pinned
FROM aichat_messages;

DROP TABLE aichat_messages;

CREATE TABLE aichat_messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    token_count INTEGER NOT NULL DEFAULT 0,
    is_pinned INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (session_id) REFERENCES aichat_sessions(id) ON DELETE CASCADE
);

INSERT INTO aichat_messages (id, session_id, role, content, timestamp, token_count, is_pinned)
SELECT id, session_id, role, content, timestamp, token_count, is_pinned FROM aichat_messages_backup;

DROP TABLE aichat_messages_backup;

-- Recreate indexes
CREATE INDEX IF NOT EXISTS idx_aichat_messages_session_id ON aichat_messages(session_id);
CREATE INDEX IF NOT EXISTS idx_aichat_messages_timestamp ON aichat_messages(timestamp);
