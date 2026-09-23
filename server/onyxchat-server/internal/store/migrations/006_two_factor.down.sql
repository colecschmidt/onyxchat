DROP TABLE IF EXISTS recovery_codes;

ALTER TABLE users
    DROP COLUMN IF EXISTS totp_secret,
    DROP COLUMN IF EXISTS totp_enabled,
    DROP COLUMN IF EXISTS totp_enrolled_at;
