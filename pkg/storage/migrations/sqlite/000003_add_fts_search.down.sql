-- Migration: 000003_add_fts_search (rollback)
-- Remove full-text search support

-- Drop triggers first (must be done before dropping FTS table)
DROP TRIGGER IF EXISTS aichat_messages_fts_ai;
DROP TRIGGER IF EXISTS aichat_messages_fts_ad;
DROP TRIGGER IF EXISTS aichat_messages_fts_au;

-- Drop FTS virtual table
DROP TABLE IF EXISTS aichat_messages_fts;
