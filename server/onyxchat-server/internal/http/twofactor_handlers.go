package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/cole/onyxchat-server/internal/store"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ─────────────────────────────────────────────────────────────
// Shared helpers
// ─────────────────────────────────────────────────────────────

// reauthenticate re-checks the caller's password. Enabling, disabling, or
// re-issuing recovery codes all change what it takes to get into the account,
// so none of them ride on the access token alone — a stolen token should not
// be enough to strip the second factor off.
func reauthenticate(userStore userStorer, username, password string) (*store.User, error) {
	if password == "" {
		return nil, errors.New("password required")
	}

	dbStart := time.Now()
	user, err := userStore.GetUserByUsername(username)
	ObserveDBQuery("user_get_by_username", dbStart)
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("password is incorrect")
	}
	return user, nil
}

// verifySecondFactor accepts either a TOTP code or an unused recovery code,
// reporting nothing about which one the input was mistaken for.
func verifySecondFactor(
	ctx context.Context,
	rdb *redis.Client,
	userStore userStorer,
	userID int64,
	secret string,
	code string,
) error {
	if looksLikeRecoveryCode(code) {
		dbStart := time.Now()
		ok, err := userStore.ConsumeRecoveryCode(userID, hashRecoveryCode(code))
		ObserveDBQuery("recovery_code_consume", dbStart)
		if err != nil {
			return err
		}
		if !ok {
			return errMFACodeInvalid
		}
		return nil
	}
	return verifyTOTP(ctx, rdb, userID, secret, code)
}

// decryptEnrollment loads and unseals the caller's stored secret.
func decryptEnrollment(userStore userStorer, cipher *SecretCipher, userID int64) (*store.TOTPEnrollment, string, error) {
	dbStart := time.Now()
	enr, err := userStore.GetTOTPEnrollment(userID)
	ObserveDBQuery("totp_get_enrollment", dbStart)
	if err != nil {
		return nil, "", err
	}
	secret, err := cipher.Decrypt(enr.Secret)
	if err != nil {
		return nil, "", err
	}
	return enr, secret, nil
}

// ─────────────────────────────────────────────────────────────
// GET /api/v1/users/me/2fa — status
// ─────────────────────────────────────────────────────────────

type twoFactorStatusResponse struct {
	Enabled                bool `json:"enabled"`
	PendingSetup           bool `json:"pending_setup"`
	RecoveryCodesRemaining int  `json:"recovery_codes_remaining"`
}

func TwoFactorStatusHandler(userStore userStorer, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		resp := twoFactorStatusResponse{}

		dbStart := time.Now()
		enr, err := userStore.GetTOTPEnrollment(user.ID)
		ObserveDBQuery("totp_get_enrollment", dbStart)
		switch {
		case errors.Is(err, store.ErrTOTPNotEnrolled):
			// Never enrolled — the zero-value response is correct.
		case err != nil:
			log.Error("[2FA] failed to read enrollment", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to read two-factor status")
			return
		default:
			resp.Enabled = enr.Enabled
			resp.PendingSetup = !enr.Enabled
		}

		if resp.Enabled {
			dbStart = time.Now()
			n, err := userStore.CountUnusedRecoveryCodes(user.ID)
			ObserveDBQuery("recovery_codes_count", dbStart)
			if err != nil {
				log.Error("[2FA] failed to count recovery codes", zap.Int64("user_id", user.ID), zap.Error(err))
				writeJSONError(w, http.StatusInternalServerError, "failed to read two-factor status")
				return
			}
			resp.RecoveryCodesRemaining = n
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// ─────────────────────────────────────────────────────────────
// POST /api/v1/users/me/2fa/setup — begin enrollment
// ─────────────────────────────────────────────────────────────

type twoFactorSetupRequest struct {
	Password string `json:"password"`
}

type twoFactorSetupResponse struct {
	Secret      string `json:"secret"`      // base32, for manual entry
	OTPAuthURI  string `json:"otpauth_uri"` // deep link for mobile authenticators
	QRCodeImage string `json:"qr_code"`     // PNG data URI
}

func TwoFactorSetupHandler(userStore userStorer, cipher *SecretCipher, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req twoFactorSetupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if _, err := reauthenticate(userStore, user.Username, req.Password); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "password is incorrect")
			return
		}

		key, err := newTOTPKey(user.Username)
		if err != nil {
			log.Error("[2FA] failed to generate secret", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to start two-factor setup")
			return
		}

		encrypted, err := cipher.Encrypt(key.Secret())
		if err != nil {
			log.Error("[2FA] failed to encrypt secret", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to start two-factor setup")
			return
		}

		dbStart := time.Now()
		err = userStore.SetPendingTOTPSecret(user.ID, encrypted)
		ObserveDBQuery("totp_set_pending", dbStart)
		if err != nil {
			if errors.Is(err, store.ErrTOTPAlreadyEnabled) {
				writeJSONError(w, http.StatusConflict, "two-factor authentication is already enabled")
				return
			}
			log.Error("[2FA] failed to store pending secret", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to start two-factor setup")
			return
		}

		qr, err := qrDataURI(key)
		if err != nil {
			log.Error("[2FA] failed to render QR code", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to start two-factor setup")
			return
		}

		writeJSON(w, http.StatusOK, twoFactorSetupResponse{
			Secret:      key.Secret(),
			OTPAuthURI:  key.URL(),
			QRCodeImage: qr,
		})
	}
}

// ─────────────────────────────────────────────────────────────
// POST /api/v1/users/me/2fa/confirm — finish enrollment
// ─────────────────────────────────────────────────────────────

type twoFactorCodeRequest struct {
	Code string `json:"code"`
}

type recoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

func TwoFactorConfirmHandler(userStore userStorer, cipher *SecretCipher, rdb *redis.Client, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req twoFactorCodeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		enr, secret, err := decryptEnrollment(userStore, cipher, user.ID)
		if err != nil {
			if errors.Is(err, store.ErrTOTPNotEnrolled) {
				writeJSONError(w, http.StatusBadRequest, "no pending two-factor setup — start setup first")
				return
			}
			log.Error("[2FA] failed to load enrollment", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to confirm two-factor setup")
			return
		}
		if enr.Enabled {
			writeJSONError(w, http.StatusConflict, "two-factor authentication is already enabled")
			return
		}

		// Only a real TOTP code proves the authenticator was set up — recovery
		// codes don't exist yet at this point in the flow.
		if err := verifyTOTP(r.Context(), rdb, user.ID, secret, req.Code); err != nil {
			if errors.Is(err, errMFACodeInvalid) {
				writeJSONError(w, http.StatusUnauthorized, "invalid verification code")
				return
			}
			log.Error("[2FA] replay guard failed", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to confirm two-factor setup")
			return
		}

		codes, hashes, err := generateRecoveryCodes()
		if err != nil {
			log.Error("[2FA] failed to generate recovery codes", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to confirm two-factor setup")
			return
		}

		dbStart := time.Now()
		err = userStore.ConfirmTOTP(user.ID, hashes)
		ObserveDBQuery("totp_confirm", dbStart)
		if err != nil {
			log.Error("[2FA] failed to enable", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to confirm two-factor setup")
			return
		}

		log.Info("[2FA] enabled", zap.Int64("user_id", user.ID), zap.String("username", user.Username))
		writeJSON(w, http.StatusOK, recoveryCodesResponse{RecoveryCodes: codes})
	}
}

// ─────────────────────────────────────────────────────────────
// DELETE /api/v1/users/me/2fa — disable
// ─────────────────────────────────────────────────────────────

type twoFactorDisableRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func TwoFactorDisableHandler(userStore userStorer, cipher *SecretCipher, rdb *redis.Client, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req twoFactorDisableRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if _, err := reauthenticate(userStore, user.Username, req.Password); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "password is incorrect")
			return
		}

		enr, secret, err := decryptEnrollment(userStore, cipher, user.ID)
		if err != nil {
			if errors.Is(err, store.ErrTOTPNotEnrolled) {
				writeJSONError(w, http.StatusBadRequest, "two-factor authentication is not enabled")
				return
			}
			log.Error("[2FA] failed to load enrollment", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to disable two-factor authentication")
			return
		}

		// A pending (unconfirmed) enrollment is just a dangling secret — let it
		// be cleared without a code, since the user may never have scanned it.
		if enr.Enabled {
			if err := verifySecondFactor(r.Context(), rdb, userStore, user.ID, secret, req.Code); err != nil {
				if errors.Is(err, errMFACodeInvalid) {
					writeJSONError(w, http.StatusUnauthorized, "invalid verification code")
					return
				}
				log.Error("[2FA] verification failed", zap.Int64("user_id", user.ID), zap.Error(err))
				writeJSONError(w, http.StatusInternalServerError, "failed to disable two-factor authentication")
				return
			}
		}

		dbStart := time.Now()
		err = userStore.DisableTOTP(user.ID)
		ObserveDBQuery("totp_disable", dbStart)
		if err != nil {
			log.Error("[2FA] failed to disable", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to disable two-factor authentication")
			return
		}

		log.Info("[2FA] disabled", zap.Int64("user_id", user.ID), zap.String("username", user.Username))
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─────────────────────────────────────────────────────────────
// POST /api/v1/users/me/2fa/recovery-codes — reissue
// ─────────────────────────────────────────────────────────────

func TwoFactorRecoveryCodesHandler(userStore userStorer, cipher *SecretCipher, rdb *redis.Client, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req twoFactorDisableRequest // same shape: password + code
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if _, err := reauthenticate(userStore, user.Username, req.Password); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "password is incorrect")
			return
		}

		enr, secret, err := decryptEnrollment(userStore, cipher, user.ID)
		if err != nil {
			if errors.Is(err, store.ErrTOTPNotEnrolled) {
				writeJSONError(w, http.StatusBadRequest, "two-factor authentication is not enabled")
				return
			}
			log.Error("[2FA] failed to load enrollment", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to regenerate recovery codes")
			return
		}
		if !enr.Enabled {
			writeJSONError(w, http.StatusBadRequest, "two-factor authentication is not enabled")
			return
		}

		if err := verifySecondFactor(r.Context(), rdb, userStore, user.ID, secret, req.Code); err != nil {
			if errors.Is(err, errMFACodeInvalid) {
				writeJSONError(w, http.StatusUnauthorized, "invalid verification code")
				return
			}
			log.Error("[2FA] verification failed", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to regenerate recovery codes")
			return
		}

		codes, hashes, err := generateRecoveryCodes()
		if err != nil {
			log.Error("[2FA] failed to generate recovery codes", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to regenerate recovery codes")
			return
		}

		dbStart := time.Now()
		err = userStore.ReplaceRecoveryCodes(user.ID, hashes)
		ObserveDBQuery("recovery_codes_replace", dbStart)
		if err != nil {
			log.Error("[2FA] failed to store recovery codes", zap.Int64("user_id", user.ID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to regenerate recovery codes")
			return
		}

		writeJSON(w, http.StatusOK, recoveryCodesResponse{RecoveryCodes: codes})
	}
}
