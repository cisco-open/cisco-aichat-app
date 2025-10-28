-- Migration: 000003_add_fts_search
-- Add full-text search support using FTS5 external content table

-- Create FTS5 virtual table with external content from aichat_messages
-- External content mode: FTS table doesn't store content, only index
-- content_rowid maps FTS rowid to aichat_messages.rowid
CREATE VIRTUAL TABLE aichat_messages_fts USING fts5(
    content,
    content=aichat_messages,
    content_rowid=rowid
);

-- Trigger: After INSERT - sync new messages to FTS index
CREATE TRIGGER aichat_messages_fts_ai AFTER INSERT ON aichat_messages BEGIN
    INSERT INTO aichat_messages_fts(rowid, content) VALUES (new.rowid, new.content);
END;

-- Trigger: After DELETE - remove deleted messages from FTS index
CREATE TRIGGER aichat_messages_fts_ad AFTER DELETE ON aichat_messages BEGIN
    INSERT INTO aichat_messages_fts(aichat_messages_fts, rowid, content)
    VALUES('delete', old.rowid, old.content);
END;

-- Trigger: After UPDATE - update FTS index when content changes
CREATE TRIGGER aichat_messages_fts_au AFTER UPDATE ON aichat_messages BEGIN
    INSERT INTO aichat_messages_fts(aichat_messages_fts, rowid, content)
    VALUES('delete', old.rowid, old.content);
    INSERT INTO aichat_messages_fts(rowid, content) VALUES (new.rowid, new.content);
END;

-- Rebuild index for any existing data
INSERT INTO aichat_messages_fts(aichat_messages_fts) VALUES('rebuild');
