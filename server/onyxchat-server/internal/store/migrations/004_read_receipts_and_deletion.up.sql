ALTER TABLE messages ADD COLUMN IF NOT EXISTS read_at    TIMESTAMPTZ DEFAULT NULL;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;

-- Speeds up the "mark unread messages as read" update.
CREATE INDEX IF NOT EXISTS idx_messages_unread
    ON messages (recipient_id, sender_id)
    WHERE read_at IS NULL AND deleted_at IS NULL;
