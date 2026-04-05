-- Make messages.iv nullable so unencrypted messages (iv = '') don't require a NOT NULL value.
-- db/init.sql created this column as NOT NULL DEFAULT '', which caused INSERT failures
-- when nullableString("") passed SQL NULL for plaintext messages.
ALTER TABLE messages ALTER COLUMN iv DROP NOT NULL;
