-- Migration: 000002_add_summary_fields (down)
-- Remove summary fields from messages table (PostgreSQL)

DROP INDEX IF EXISTS idx_aichat_messages_is_summary;

ALTER TABLE aichat_messages DROP COLUMN IF EXISTS summary_depth;
ALTER TABLE aichat_messages DROP COLUMN IF EXISTS summarized_ids;
ALTER TABLE aichat_messages DROP COLUMN IF EXISTS is_summary;
