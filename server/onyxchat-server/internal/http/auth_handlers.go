package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cole/onyxchat-server/internal/store"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// ─────────────────────────────────────────────────────────────
// Health check
// ─────────────────────────────────────────────────────────────

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ─────────────────────────────────────────────────────────────
// Register
// ─────────────────────────────────────────────────────────────

type RegisterRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code"`
}

type RegisterResponse struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

func RegisterHandler(userStore userStorer, jwtMgr *JWTManager, rdb *redis.Client, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Warn("[Register] invalid JSON", zap.Error(err))
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || req.Password == "" {
			writeJSONError(w, http.StatusBadRequest, "username and password required")
			return
		}

		if req.InviteCode == "" {
			writeJSONError(w, http.StatusForbidden, "invite code required")
			return
		}

		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Error("[Register] could not hash password", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "could not hash password")
			return
		}

		// Consume invite code and create user atomically — if user creation
		// fails (e.g. duplicate username) the invite code is not burned.
		dbStart := time.Now()
		user, err := userStore.RegisterWithInvite(req.InviteCode, req.Username, string(hashedBytes))
		ObserveDBQuery("user_register", dbStart)
		if err != nil {
			log.Warn("[Register] could not create user", zap.String("username", req.Username), zap.Error(err))
			switch {
			case errors.Is(err, store.ErrInvalidInviteCode):
				writeJSONError(w, http.StatusForbidden, "invalid or already used invite code")
			case errors.Is(err, store.ErrUsernameTaken):
				writeJSONError(w, http.StatusConflict, "username already taken")
			default:
				writeJSONError(w, http.StatusInternalServerError, "failed to create user")
			}
			return
		}

		token, rt, err := issueSession(r.Context(), jwtMgr, rdb, user)
		if err != nil {
			log.Error("[Register] failed to issue session", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		writeJSON(w, http.StatusOK, RegisterResponse{
			ID:           user.ID,
			Username:     user.Username,
			Token:        token,
			RefreshToken: rt,
		})
	}
}

// ─────────────────────────────────────────────────────────────
// Login
// ─────────────────────────────────────────────────────────────

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is either a completed session (id/username/token/refresh_token)
// or — for an account with a second factor — a challenge the client must answer
// at POST /api/v1/login/2fa. The two sets are mutually exclusive, so the
// password step leaks nothing about the account beyond "2FA is on" to someone
// who already has the correct password.
type LoginResponse struct {
	ID           int64  `json:"id,omitempty"`
	Username     string `json:"username,omitempty"`
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	MFARequired bool   `json:"mfa_required,omitempty"`
	MFAToken    string `json:"mfa_token,omitempty"`
}

// issueSession mints the access/refresh token pair that ends a successful
// login. Shared by password-only login, the second-factor step, and register
// so the three cannot drift apart.
func issueSession(
	ctx context.Context,
	jwtMgr *JWTManager,
	rdb *redis.Client,
	user *store.User,
) (token string, refreshToken string, err error) {
	token, err = jwtMgr.Generate(user)
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err = newRefreshToken()
	if err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	if err := storeRefreshToken(ctx, rdb, refreshToken, user.ID); err != nil {
		return "", "", fmt.Errorf("store refresh token: %w", err)
	}
	return token, refreshToken, nil
}

func LoginHandler(userStore userStorer, jwtMgr *JWTManager, idLimiter *KeyedLimiter, rdb *redis.Client, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || req.Password == "" {
			writeJSONError(w, http.StatusBadRequest, "username and password required")
			return
		}

		// rate limit by identifier BEFORE lookup/hash work
		if !idLimiter.Allow(strings.ToLower(req.Username)) {
			writeJSONError(w, http.StatusTooManyRequests, "too many requests")
			return
		}

		dbStart := time.Now()
		user, err := userStore.GetUserByUsername(req.Username)
		ObserveDBQuery("user_get_by_username", dbStart)
		if err != nil {
			if err == store.ErrUserNotFound {
				writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
			} else {
				log.Error("[Login] database error", zap.String("username", req.Username), zap.Error(err))
				writeJSONError(w, http.StatusInternalServerError, "internal server error")
			}
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}

		// Password is correct but it isn't the whole story — hand back an
		// opaque challenge instead of a session and wait for the code.
		if user.TOTPEnabled {
			mfaToken, err := newMFAChallenge(r.Context(), rdb, mfaChallenge{
				UserID:   user.ID,
				Username: user.Username,
			})
			if err != nil {
				log.Error("[Login] failed to create MFA challenge", zap.Int64("user_id", user.ID), zap.Error(err))
				writeJSONError(w, http.StatusInternalServerError, "failed to start two-factor verification")
				return
			}
			writeJSON(w, http.StatusOK, LoginResponse{MFARequired: true, MFAToken: mfaToken})
			return
		}

		token, rt, err := issueSession(r.Context(), jwtMgr, rdb, user)
		if err != nil {
			log.Error("[Login] failed to issue session", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		writeJSON(w, http.StatusOK, LoginResponse{
			ID:           user.ID,
			Username:     user.Username,
			Token:        token,
			RefreshToken: rt,
		})
	}
}

// ─────────────────────────────────────────────────────────────
// Login — second factor
// ─────────────────────────────────────────────────────────────

type loginTwoFactorRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

// LoginTwoFactorHandler completes a login that stopped at the challenge.
// It takes either a TOTP code or an unused recovery code; the challenge token
// is consumed on success and after too many wrong guesses, so a stolen one
// buys at most a handful of tries at a six-digit code inside five minutes.
func LoginTwoFactorHandler(
	userStore userStorer,
	jwtMgr *JWTManager,
	cipher *SecretCipher,
	rdb *redis.Client,
	log *zap.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginTwoFactorRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		challenge, err := lookupMFAChallenge(r.Context(), rdb, req.MFAToken)
		if err != nil {
			switch {
			case errors.Is(err, errMFAChallengeInvalid):
				writeJSONError(w, http.StatusUnauthorized, "invalid or expired login challenge")
			case errors.Is(err, errMFATooManyAttempts):
				writeJSONError(w, http.StatusTooManyRequests, "too many incorrect codes — please sign in again")
			default:
				log.Error("[Login2FA] challenge lookup failed", zap.Error(err))
				writeJSONError(w, http.StatusInternalServerError, "failed to verify code")
			}
			return
		}

		enr, secret, err := decryptEnrollment(userStore, cipher, challenge.UserID)
		if err != nil {
			// The factor was removed between the two steps, or the stored
			// secret can't be read. Either way this login can't complete.
			log.Error("[Login2FA] failed to load enrollment",
				zap.Int64("user_id", challenge.UserID), zap.Error(err))
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired login challenge")
			return
		}
		if !enr.Enabled {
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired login challenge")
			return
		}

		if err := verifySecondFactor(r.Context(), rdb, userStore, challenge.UserID, secret, req.Code); err != nil {
			if errors.Is(err, errMFACodeInvalid) {
				writeJSONError(w, http.StatusUnauthorized, "invalid verification code")
				return
			}
			log.Error("[Login2FA] verification failed",
				zap.Int64("user_id", challenge.UserID), zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to verify code")
			return
		}

		dbStart := time.Now()
		user, err := userStore.GetUserByID(challenge.UserID)
		ObserveDBQuery("user_get_by_id", dbStart)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "account not found")
			return
		}

		// One challenge, one session.
		_ = discardMFAChallenge(r.Context(), rdb, req.MFAToken)

		token, rt, err := issueSession(r.Context(), jwtMgr, rdb, user)
		if err != nil {
			log.Error("[Login2FA] failed to issue session", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		writeJSON(w, http.StatusOK, LoginResponse{
			ID:           user.ID,
			Username:     user.Username,
			Token:        token,
			RefreshToken: rt,
		})
	}
}

// ─────────────────────────────────────────────────────────────
// Refresh
// ─────────────────────────────────────────────────────────────

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshHandler exchanges a valid refresh token for a new access token + rotated refresh token.
func RefreshHandler(userStore userStorer, jwtMgr *JWTManager, rdb *redis.Client, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.RefreshToken == "" {
			writeJSONError(w, http.StatusUnauthorized, "refresh_token required")
			return
		}

		userID, err := lookupRefreshToken(r.Context(), rdb, req.RefreshToken)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}

		dbStart := time.Now()
		user, err := userStore.GetUserByID(userID)
		ObserveDBQuery("user_get_by_id", dbStart)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "account not found")
			return
		}

		// Rotate: delete old, issue new
		_ = deleteRefreshToken(r.Context(), rdb, req.RefreshToken)

		newRT, err := newRefreshToken()
		if err != nil {
			log.Error("[Refresh] failed to generate refresh token", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}
		if err := storeRefreshToken(r.Context(), rdb, newRT, user.ID); err != nil {
			log.Error("[Refresh] failed to store refresh token", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		token, err := jwtMgr.Generate(user)
		if err != nil {
			log.Error("[Refresh] failed to generate access token", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}

		writeJSON(w, http.StatusOK, refreshResponse{Token: token, RefreshToken: newRT})
	}
}

// ─────────────────────────────────────────────────────────────
// Logout
// ─────────────────────────────────────────────────────────────

// LogoutHandler revokes the supplied refresh token so it can't be used again.
func LogoutHandler(rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req refreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.RefreshToken != "" {
			_ = deleteRefreshToken(r.Context(), rdb, req.RefreshToken)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// ─────────────────────────────────────────────────────────────
// List users
// ─────────────────────────────────────────────────────────────

type UserResponse struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func ListUsersHandler(userStore userStorer, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var users []*store.User
		var err error

		dbStart := time.Now()
		if q := strings.TrimSpace(r.URL.Query().Get("search")); q != "" {
			users, err = userStore.SearchUsers(q)
			ObserveDBQuery("user_search", dbStart)
		} else {
			users, err = userStore.ListUsers()
			ObserveDBQuery("user_list", dbStart)
		}
		if err != nil {
			log.Error("[ListUsers] failed to list users", zap.Error(err))
			writeJSONError(w, http.StatusInternalServerError, "failed to list users")
			return
		}

		resp := make([]UserResponse, 0, len(users))
		for _, u := range users {
			resp = append(resp, UserResponse{ID: u.ID, Username: u.Username})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
