package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrTOTPNotEnrolled is returned when an operation needs a TOTP secret the
// user does not have (no enrollment has ever been started).
var ErrTOTPNotEnrolled = errors.New("two-factor authentication is not set up")

// TOTPEnrollment is the stored second-factor state for one user.
//
// Secret is the *encrypted* base32 TOTP secret exactly as it sits in the
// database; callers decrypt it before use. Enabled distinguishes a confirmed
// enrollment from a pending one — a pending enrollment has a secret but does
// not gate login yet.
type TOTPEnrollment struct {
	Secret  string
	Enabled bool
}

// ─────────────────────────────────────────────────────────────
// Enrollment
// ─────────────────────────────────────────────────────────────

// GetTOTPEnrollment returns the user's stored second-factor state.
// A user who has never started enrollment yields ErrTOTPNotEnrolled.
func (s *UserStore) GetTOTPEnrollment(userID int64) (*TOTPEnrollment, error) {
	var secret sql.NullString
	var enabled bool
	err := s.db.QueryRow(
		`SELECT totp_secret, totp_enabled FROM users WHERE id = $1 AND deleted_at IS NULL`,
		userID,
	).Scan(&secret, &enabled)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if !secret.Valid || secret.String == "" {
		return nil, ErrTOTPNotEnrolled
	}
	return &TOTPEnrollment{Secret: secret.String, Enabled: enabled}, nil
}

// SetPendingTOTPSecret stores a freshly generated (encrypted) secret without
// enabling it. Starting a new enrollment discards any previous pending secret
// — and, for a user re-enrolling, only takes effect once ConfirmTOTP runs, so
// an abandoned setup never locks anyone out of an already-working factor.
//
// It refuses to overwrite a secret that is already enabled: turning an active
// factor off is DisableTOTP's job, and requires proof of possession.
func (s *UserStore) SetPendingTOTPSecret(userID int64, encryptedSecret string) error {
	res, err := s.db.Exec(
		`UPDATE users SET totp_secret = $1, totp_enabled = FALSE
         WHERE id = $2 AND deleted_at IS NULL AND totp_enabled = FALSE`,
		encryptedSecret, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTOTPAlreadyEnabled
	}
	return nil
}

// ErrTOTPAlreadyEnabled is returned when a new enrollment is started for a
// user who already has a confirmed second factor.
var ErrTOTPAlreadyEnabled = errors.New("two-factor authentication is already enabled")

// ConfirmTOTP flips a pending enrollment to enabled and installs the user's
// recovery codes, atomically. Any codes from a previous enrollment are
// discarded so a stale printout can never be redeemed against a new factor.
func (s *UserStore) ConfirmTOTP(userID int64, codeHashes []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op if Commit() succeeds

	res, err := tx.Exec(
		`UPDATE users SET totp_enabled = TRUE, totp_enrolled_at = NOW()
         WHERE id = $1 AND deleted_at IS NULL AND totp_secret IS NOT NULL`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("enable totp: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTOTPNotEnrolled
	}

	if err := replaceRecoveryCodesTx(tx, userID, codeHashes); err != nil {
		return err
	}

	return tx.Commit()
}

// DisableTOTP clears the secret and every recovery code in one transaction,
// returning the user to password-only login.
func (s *UserStore) DisableTOTP(userID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`UPDATE users SET totp_secret = NULL, totp_enabled = FALSE, totp_enrolled_at = NULL
         WHERE id = $1`,
		userID,
	); err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete recovery codes: %w", err)
	}

	return tx.Commit()
}

// ─────────────────────────────────────────────────────────────
// Recovery codes
// ─────────────────────────────────────────────────────────────

// ReplaceRecoveryCodes swaps the user's entire set of recovery codes for a new
// one. Used when regenerating codes on an already-enabled factor.
func (s *UserStore) ReplaceRecoveryCodes(userID int64, codeHashes []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := replaceRecoveryCodesTx(tx, userID, codeHashes); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceRecoveryCodesTx(tx *sql.Tx, userID int64, codeHashes []string) error {
	if _, err := tx.Exec(`DELETE FROM recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("clear recovery codes: %w", err)
	}
	for _, h := range codeHashes {
		if _, err := tx.Exec(
			`INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1, $2)`,
			userID, h,
		); err != nil {
			return fmt.Errorf("insert recovery code: %w", err)
		}
	}
	return nil
}

// ConsumeRecoveryCode burns a single unused recovery code, reporting whether
// one matched. The UPDATE ... WHERE used_at IS NULL is the whole concurrency
// story: two simultaneous logins presenting the same code race on the row and
// exactly one of them sees a row affected.
func (s *UserStore) ConsumeRecoveryCode(userID int64, codeHash string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE recovery_codes SET used_at = NOW()
         WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`,
		userID, codeHash,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountUnusedRecoveryCodes reports how many codes the user has left, so the UI
// can nag before they run out.
func (s *UserStore) CountUnusedRecoveryCodes(userID int64) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM recovery_codes WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	).Scan(&n)
	return n, err
}
