DROP INDEX IF EXISTS idx_messages_unread;
ALTER TABLE messages DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE messages DROP COLUMN IF EXISTS read_at;
