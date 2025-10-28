-- Migration: 000003_add_fts_search (rollback)
-- Remove full-text search support

-- Drop GIN index first
DROP INDEX IF EXISTS idx_aichat_messages_search;

-- Drop tsvector column
ALTER TABLE aichat_messages
DROP COLUMN IF EXISTS content_search;
