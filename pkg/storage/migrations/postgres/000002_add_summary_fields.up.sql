-- Migration: 000002_add_summary_fields
-- Add fields to support message summarization for context window management (PostgreSQL)

-- Add summary flag to mark summary messages
ALTER TABLE aichat_messages ADD COLUMN is_summary BOOLEAN NOT NULL DEFAULT FALSE;

-- Add array of message IDs that this summary replaces
ALTER TABLE aichat_messages ADD COLUMN summarized_ids TEXT[];

-- Add depth tracking to prevent infinite summarization recursion
-- 0 = original message, 1 = first summary, 2+ = meta-summary
ALTER TABLE aichat_messages ADD COLUMN summary_depth INTEGER NOT NULL DEFAULT 0;

-- Index for filtering summary messages
CREATE INDEX IF NOT EXISTS idx_aichat_messages_is_summary ON aichat_messages(is_summary);
