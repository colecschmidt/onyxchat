-- TOTP two-factor authentication (opt-in, per user).
--
-- totp_secret holds the AES-GCM-encrypted base32 TOTP secret, never the raw
-- one — see internal/http/totp_cipher.go. A row with a secret but
-- totp_enabled = FALSE is a *pending* enrollment: the user has scanned the QR
-- but has not yet proven possession by submitting a valid code, so login is
-- unaffected until they do.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS totp_secret      TEXT        DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS totp_enabled     BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS totp_enrolled_at TIMESTAMPTZ DEFAULT NULL;

-- Single-use recovery codes, for when the authenticator device is lost.
-- Codes are high-entropy (80 bits) random strings, so a plain SHA-256 of the
-- normalized code is stored rather than bcrypt: there is nothing to brute
-- force, and it keeps verification to one indexed lookup instead of N bcrypt
-- comparisons on every login attempt.
CREATE TABLE IF NOT EXISTS recovery_codes (
    id         BIGSERIAL   PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT        NOT NULL,
    used_at    TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS recovery_codes_user_hash_uniq
    ON recovery_codes (user_id, code_hash);

CREATE INDEX IF NOT EXISTS idx_recovery_codes_unused
    ON recovery_codes (user_id)
    WHERE used_at IS NULL;
