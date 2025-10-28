-- Migration: 000001_create_tables (down)
-- Drop tables in reverse order due to foreign key constraints

DROP TABLE IF EXISTS aichat_messages;
DROP TABLE IF EXISTS aichat_sessions;
