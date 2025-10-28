-- Migration: 000003_add_fts_search
-- Add full-text search support using tsvector generated column with GIN index

-- Add generated tsvector column for full-text search
-- GENERATED ALWAYS AS ensures automatic sync when content changes
ALTER TABLE aichat_messages
ADD COLUMN content_search tsvector
GENERATED ALWAYS AS (to_tsvector('english', content)) STORED;

-- Create GIN index for efficient full-text search queries
CREATE INDEX idx_aichat_messages_search
ON aichat_messages USING GIN(content_search);
