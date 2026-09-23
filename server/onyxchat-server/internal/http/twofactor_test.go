package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ─────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────

func testCipher(t *testing.T) *SecretCipher {
	t.Helper()
	c, err := NewSecretCipher("test-key-material")
	if err != nil {
		t.Fatalf("NewSecretCipher: %v", err)
	}
	return c
}

func permissiveLimiter() *KeyedLimiter {
	return NewKeyedLimiter(rate.Limit(1000), 1000, time.Minute)
}

// enrollTOTP puts a user into the fully-enabled state and returns the plaintext
// secret plus the recovery codes handed to them at enrollment.
func enrollTOTP(t *testing.T, us *fakeUserStore, cipher *SecretCipher, userID int64, username string) (secret string, recovery []string) {
	t.Helper()

	key, err := newTOTPKey(username)
	if err != nil {
		t.Fatalf("newTOTPKey: %v", err)
	}
	enc, err := cipher.Encrypt(key.Secret())
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := us.SetPendingTOTPSecret(userID, enc); err != nil {
		t.Fatalf("SetPendingTOTPSecret: %v", err)
	}
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		t.Fatalf("generateRecoveryCodes: %v", err)
	}
	if err := us.ConfirmTOTP(userID, hashes); err != nil {
		t.Fatalf("ConfirmTOTP: %v", err)
	}
	return key.Secret(), codes
}

func currentCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	return code
}

// wrongCode returns a six-digit code guaranteed not to equal the valid one.
func wrongCode(valid string) string {
	n := int(valid[0]-'0'+1) % 10
	return fmt.Sprintf("%d%s", n, valid[1:])
}

func postJSON(t *testing.T, h http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodPost, path, mustMarshal(t, body)))
	return rr
}

// ─────────────────────────────────────────────────────────────
// Secret encryption at rest
// ─────────────────────────────────────────────────────────────

func TestSecretCipher_RoundTrip(t *testing.T) {
	c := testCipher(t)

	const secret = "JBSWY3DPEHPK3PXP"
	enc, err := c.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if strings.Contains(enc, secret) {
		t.Fatal("ciphertext contains the plaintext secret")
	}

	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != secret {
		t.Fatalf("round trip: got %q want %q", got, secret)
	}
}

func TestSecretCipher_NonceIsPerMessage(t *testing.T) {
	c := testCipher(t)
	a, _ := c.Encrypt("JBSWY3DPEHPK3PXP")
	b, _ := c.Encrypt("JBSWY3DPEHPK3PXP")
	if a == b {
		t.Fatal("encrypting the same secret twice produced identical ciphertext")
	}
}

func TestSecretCipher_WrongKeyFails(t *testing.T) {
	enc, err := testCipher(t).Encrypt("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	other, err := NewSecretCipher("a-different-key")
	if err != nil {
		t.Fatalf("NewSecretCipher: %v", err)
	}
	if _, err := other.Decrypt(enc); err == nil {
		t.Fatal("decryption with the wrong key succeeded")
	}
}

func TestSecretCipher_TamperedCiphertextFails(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("JBSWY3DPEHPK3PXP")

	// Flip a character in the middle of the payload.
	b := []byte(enc)
	mid := len(b) / 2
	if b[mid] == 'A' {
		b[mid] = 'B'
	} else {
		b[mid] = 'A'
	}
	if _, err := c.Decrypt(string(b)); err == nil {
		t.Fatal("GCM accepted a tampered ciphertext")
	}
}

// ─────────────────────────────────────────────────────────────
// Recovery codes
// ─────────────────────────────────────────────────────────────

func TestGenerateRecoveryCodes(t *testing.T) {
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		t.Fatalf("generateRecoveryCodes: %v", err)
	}
	if len(codes) != recoveryCodeCount || len(hashes) != recoveryCodeCount {
		t.Fatalf("got %d codes / %d hashes, want %d of each", len(codes), len(hashes), recoveryCodeCount)
	}

	seen := make(map[string]bool)
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("duplicate recovery code %q", c)
		}
		seen[c] = true

		if !looksLikeRecoveryCode(c) {
			t.Fatalf("code %q is not recognised as a recovery code", c)
		}
		if got := len(normalizeRecoveryCode(c)); got != recoveryCodeLen {
			t.Fatalf("code %q normalizes to %d chars, want %d", c, got, recoveryCodeLen)
		}
	}
}

func TestNormalizeRecoveryCode_IgnoresFormatting(t *testing.T) {
	codes, _, err := generateRecoveryCodes()
	if err != nil {
		t.Fatalf("generateRecoveryCodes: %v", err)
	}
	code := codes[0]

	variants := []string{
		code,
		strings.ToLower(code),
		strings.ReplaceAll(code, "-", ""),
		strings.ReplaceAll(code, "-", " "),
		" " + code + " ",
	}
	want := hashRecoveryCode(code)
	for _, v := range variants {
		if got := hashRecoveryCode(v); got != want {
			t.Fatalf("variant %q hashed differently from %q", v, code)
		}
	}
}

func TestLooksLikeRecoveryCode_RejectsTOTP(t *testing.T) {
	if looksLikeRecoveryCode("123456") {
		t.Fatal("a six-digit TOTP code was mistaken for a recovery code")
	}
}

// ─────────────────────────────────────────────────────────────
// Login — challenge step
// ─────────────────────────────────────────────────────────────

func TestLoginHandler_NoTOTP_ReturnsSessionDirectly(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")

	rr := postJSON(t, LoginHandler(us, jwtMgr, permissiveLimiter(), rdb, zap.NewNop()),
		"/login", map[string]string{"username": "alice", "password": "correct-horse"})

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp LoginResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Token == "" || resp.RefreshToken == "" {
		t.Fatal("expected a token pair for an account without a second factor")
	}
	if resp.MFARequired {
		t.Fatal("mfa_required set for an account without a second factor")
	}
}

func TestLoginHandler_TOTPEnabled_ReturnsChallengeNotSession(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	cipher := testCipher(t)
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")
	user, _ := us.GetUserByUsername("alice")
	enrollTOTP(t, us, cipher, user.ID, "alice")

	rr := postJSON(t, LoginHandler(us, jwtMgr, permissiveLimiter(), rdb, zap.NewNop()),
		"/login", map[string]string{"username": "alice", "password": "correct-horse"})

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp LoginResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if !resp.MFARequired || resp.MFAToken == "" {
		t.Fatalf("expected an MFA challenge, got %+v", resp)
	}
	// The whole point: the password alone must not yield a usable session.
	if resp.Token != "" || resp.RefreshToken != "" {
		t.Fatal("password step handed out a session despite 2FA being enabled")
	}
	if resp.Username != "" {
		t.Fatal("challenge response leaked account details")
	}
}

func TestLoginHandler_TOTPEnabled_WrongPasswordStillRejected(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	cipher := testCipher(t)
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")
	user, _ := us.GetUserByUsername("alice")
	enrollTOTP(t, us, cipher, user.ID, "alice")

	rr := postJSON(t, LoginHandler(us, jwtMgr, permissiveLimiter(), rdb, zap.NewNop()),
		"/login", map[string]string{"username": "alice", "password": "wrong"})

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

// ─────────────────────────────────────────────────────────────
// Login — second factor step
// ─────────────────────────────────────────────────────────────

// startChallenge runs the password step and returns the challenge token.
func startChallenge(t *testing.T, us *fakeUserStore, jwtMgr *JWTManager, rdb *redis.Client, username, password string) string {
	t.Helper()
	rr := postJSON(t, LoginHandler(us, jwtMgr, permissiveLimiter(), rdb, zap.NewNop()),
		"/login", map[string]string{"username": username, "password": password})
	var resp LoginResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.MFAToken == "" {
		t.Fatalf("no challenge token issued: %d %s", rr.Code, rr.Body.String())
	}
	return resp.MFAToken
}

func TestLoginTwoFactor_ValidCodeCompletesLogin(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	cipher := testCipher(t)
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")
	user, _ := us.GetUserByUsername("alice")
	secret, _ := enrollTOTP(t, us, cipher, user.ID, "alice")

	token := startChallenge(t, us, jwtMgr, rdb, "alice", "correct-horse")

	rr := postJSON(t, LoginTwoFactorHandler(us, jwtMgr, cipher, rdb, zap.NewNop()),
		"/login/2fa", map[string]string{"mfa_token": token, "code": currentCode(t, secret)})

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp LoginResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Token == "" || resp.RefreshToken == "" {
		t.Fatalf("expected a token pair, got %+v", resp)
	}
	if resp.Username != "alice" {
		t.Fatalf("username = %q, want alice", resp.Username)
	}
}

func TestLoginTwoFactor_WrongCodeRejected(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	cipher := testCipher(t)
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")
	user, _ := us.GetUserByUsername("alice")
	secret, _ := enrollTOTP(t, us, cipher, user.ID, "alice")

	token := startChallenge(t, us, jwtMgr, rdb, "alice", "correct-horse")

	rr := postJSON(t, LoginTwoFactorHandler(us, jwtMgr, cipher, rdb, zap.NewNop()),
		"/login/2fa", map[string]string{"mfa_token": token, "code": wrongCode(currentCode(t, secret))})

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body %s)", rr.Code, rr.Body.String())
	}
}

func TestLoginTwoFactor_CodeCannotBeReplayed(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	cipher := testCipher(t)
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")
	user, _ := us.GetUserByUsername("alice")
	secret, _ := enrollTOTP(t, us, cipher, user.ID, "alice")

	code := currentCode(t, secret)
	handler := LoginTwoFactorHandler(us, jwtMgr, cipher, rdb, zap.NewNop())

	first := postJSON(t, handler, "/login/2fa", map[string]string{
		"mfa_token": startChallenge(t, us, jwtMgr, rdb, "alice", "correct-horse"),
		"code":      code,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first use: status = %d, body = %s", first.Code, first.Body.String())
	}

	// Same code, fresh challenge — a captured code must not work twice.
	second := postJSON(t, handler, "/login/2fa", map[string]string{
		"mfa_token": startChallenge(t, us, jwtMgr, rdb, "alice", "correct-horse"),
		"code":      code,
	})
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("replayed code: status = %d, want 401 (body %s)", second.Code, second.Body.String())
	}
}

func TestLoginTwoFactor_AttemptsAreBudgetedPerChallenge(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	cipher := testCipher(t)
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")
	user, _ := us.GetUserByUsername("alice")
	secret, _ := enrollTOTP(t, us, cipher, user.ID, "alice")

	token := startChallenge(t, us, jwtMgr, rdb, "alice", "correct-horse")
	handler := LoginTwoFactorHandler(us, jwtMgr, cipher, rdb, zap.NewNop())
	bad := wrongCode(currentCode(t, secret))

	for i := 0; i < maxMFAAttempts; i++ {
		rr := postJSON(t, handler, "/login/2fa", map[string]string{"mfa_token": token, "code": bad})
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rr.Code)
		}
	}

	rr := postJSON(t, handler, "/login/2fa", map[string]string{"mfa_token": token, "code": bad})
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt %d: status = %d, want 429", maxMFAAttempts+1, rr.Code)
	}

	// The challenge is burned, so even the right code can't rescue it.
	rr = postJSON(t, handler, "/login/2fa", map[string]string{"mfa_token": token, "code": currentCode(t, secret)})
	if rr.Code == http.StatusOK {
		t.Fatal("an exhausted challenge still completed a login")
	}
}

func TestLoginTwoFactor_UnknownChallengeRejected(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	cipher := testCipher(t)

	rr := postJSON(t, LoginTwoFactorHandler(us, jwtMgr, cipher, rdb, zap.NewNop()),
		"/login/2fa", map[string]string{"mfa_token": "not-a-real-token", "code": "123456"})

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestLoginTwoFactor_RecoveryCodeWorksOnce(t *testing.T) {
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	rdb := newTestRDB(t)
	cipher := testCipher(t)
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")
	user, _ := us.GetUserByUsername("alice")
	_, recovery := enrollTOTP(t, us, cipher, user.ID, "alice")

	handler := LoginTwoFactorHandler(us, jwtMgr, cipher, rdb, zap.NewNop())

	first := postJSON(t, handler, "/login/2fa", map[string]string{
		"mfa_token": startChallenge(t, us, jwtMgr, rdb, "alice", "correct-horse"),
		"code":      recovery[0],
	})
	if first.Code != http.StatusOK {
		t.Fatalf("recovery code rejected: %d %s", first.Code, first.Body.String())
	}

	second := postJSON(t, handler, "/login/2fa", map[string]string{
		"mfa_token": startChallenge(t, us, jwtMgr, rdb, "alice", "correct-horse"),
		"code":      recovery[0],
	})
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("reused recovery code: status = %d, want 401", second.Code)
	}

	// A different code from the same batch is still good.
	third := postJSON(t, handler, "/login/2fa", map[string]string{
		"mfa_token": startChallenge(t, us, jwtMgr, rdb, "alice", "correct-horse"),
		"code":      recovery[1],
	})
	if third.Code != http.StatusOK {
		t.Fatalf("second recovery code rejected: %d %s", third.Code, third.Body.String())
	}
}

// ─────────────────────────────────────────────────────────────
// Enrollment management
// ─────────────────────────────────────────────────────────────

// authed runs a protected 2FA handler as the given user.
func authed(t *testing.T, h http.HandlerFunc, method, path string, u *AuthUser, body any) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, mustMarshal(t, body))
	injectUser(h, u).ServeHTTP(rr, req)
	return rr
}

func setupUser(t *testing.T) (*fakeUserStore, *JWTManager, *redis.Client, *SecretCipher, *AuthUser) {
	t.Helper()
	us := newFakeUserStore()
	jwtMgr := NewJWTManager("test-secret")
	registerUser(t, us, jwtMgr, "alice", "correct-horse", "invite-1")
	u, _ := us.GetUserByUsername("alice")
	return us, jwtMgr, newTestRDB(t), testCipher(t), &AuthUser{ID: u.ID, Username: u.Username}
}

func TestTwoFactorSetup_RequiresCorrectPassword(t *testing.T) {
	us, _, _, cipher, au := setupUser(t)

	rr := authed(t, TwoFactorSetupHandler(us, cipher, zap.NewNop()),
		http.MethodPost, "/users/me/2fa/setup", au, map[string]string{"password": "wrong"})

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if _, err := us.GetTOTPEnrollment(au.ID); err == nil {
		t.Fatal("a secret was stored despite the wrong password")
	}
}

func TestTwoFactorSetupThenConfirm_EnablesAndReturnsRecoveryCodes(t *testing.T) {
	us, _, rdb, cipher, au := setupUser(t)

	setup := authed(t, TwoFactorSetupHandler(us, cipher, zap.NewNop()),
		http.MethodPost, "/users/me/2fa/setup", au, map[string]string{"password": "correct-horse"})
	if setup.Code != http.StatusOK {
		t.Fatalf("setup: status = %d, body = %s", setup.Code, setup.Body.String())
	}

	var sr twoFactorSetupResponse
	json.Unmarshal(setup.Body.Bytes(), &sr)
	if sr.Secret == "" {
		t.Fatal("setup returned no secret")
	}
	if !strings.HasPrefix(sr.OTPAuthURI, "otpauth://totp/") {
		t.Fatalf("otpauth_uri = %q", sr.OTPAuthURI)
	}
	if !strings.HasPrefix(sr.QRCodeImage, "data:image/png;base64,") {
		t.Fatal("setup returned no QR image")
	}

	// The secret is stored encrypted, not as handed to the client.
	enr, err := us.GetTOTPEnrollment(au.ID)
	if err != nil {
		t.Fatalf("GetTOTPEnrollment: %v", err)
	}
	if enr.Enabled {
		t.Fatal("setup alone enabled the factor — it must wait for confirmation")
	}
	if enr.Secret == sr.Secret {
		t.Fatal("TOTP secret was stored in plaintext")
	}

	// A pending enrollment must not affect login yet.
	user, _ := us.GetUserByUsername("alice")
	if user.TOTPEnabled {
		t.Fatal("pending enrollment already gates login")
	}

	confirm := authed(t, TwoFactorConfirmHandler(us, cipher, rdb, zap.NewNop()),
		http.MethodPost, "/users/me/2fa/confirm", au, map[string]string{"code": currentCode(t, sr.Secret)})
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm: status = %d, body = %s", confirm.Code, confirm.Body.String())
	}

	var cr recoveryCodesResponse
	json.Unmarshal(confirm.Body.Bytes(), &cr)
	if len(cr.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("got %d recovery codes, want %d", len(cr.RecoveryCodes), recoveryCodeCount)
	}

	user, _ = us.GetUserByUsername("alice")
	if !user.TOTPEnabled {
		t.Fatal("confirm did not enable the factor")
	}
}

func TestTwoFactorConfirm_WrongCodeLeavesItDisabled(t *testing.T) {
	us, _, rdb, cipher, au := setupUser(t)

	setup := authed(t, TwoFactorSetupHandler(us, cipher, zap.NewNop()),
		http.MethodPost, "/users/me/2fa/setup", au, map[string]string{"password": "correct-horse"})
	var sr twoFactorSetupResponse
	json.Unmarshal(setup.Body.Bytes(), &sr)

	rr := authed(t, TwoFactorConfirmHandler(us, cipher, rdb, zap.NewNop()),
		http.MethodPost, "/users/me/2fa/confirm", au,
		map[string]string{"code": wrongCode(currentCode(t, sr.Secret))})

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	user, _ := us.GetUserByUsername("alice")
	if user.TOTPEnabled {
		t.Fatal("a rejected code still enabled the factor")
	}
}

func TestTwoFactorSetup_RefusedWhenAlreadyEnabled(t *testing.T) {
	us, _, _, cipher, au := setupUser(t)
	enrollTOTP(t, us, cipher, au.ID, au.Username)

	rr := authed(t, TwoFactorSetupHandler(us, cipher, zap.NewNop()),
		http.MethodPost, "/users/me/2fa/setup", au, map[string]string{"password": "correct-horse"})

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}

func TestTwoFactorDisable_NeedsPasswordAndCode(t *testing.T) {
	us, _, rdb, cipher, au := setupUser(t)
	secret, _ := enrollTOTP(t, us, cipher, au.ID, au.Username)
	h := TwoFactorDisableHandler(us, cipher, rdb, zap.NewNop())

	// Right password, no code.
	rr := authed(t, h, http.MethodDelete, "/users/me/2fa", au,
		map[string]string{"password": "correct-horse", "code": ""})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("missing code: status = %d, want 401", rr.Code)
	}

	// Right code, wrong password.
	rr = authed(t, h, http.MethodDelete, "/users/me/2fa", au,
		map[string]string{"password": "nope", "code": currentCode(t, secret)})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: status = %d, want 401", rr.Code)
	}

	user, _ := us.GetUserByUsername("alice")
	if !user.TOTPEnabled {
		t.Fatal("the factor was removed by a rejected request")
	}

	// Both correct.
	rr = authed(t, h, http.MethodDelete, "/users/me/2fa", au,
		map[string]string{"password": "correct-horse", "code": currentCode(t, secret)})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	user, _ = us.GetUserByUsername("alice")
	if user.TOTPEnabled {
		t.Fatal("the factor is still enabled after a successful disable")
	}
	if _, err := us.GetTOTPEnrollment(au.ID); err == nil {
		t.Fatal("the secret survived disabling")
	}
	if n, _ := us.CountUnusedRecoveryCodes(au.ID); n != 0 {
		t.Fatalf("%d recovery codes survived disabling", n)
	}
}

func TestTwoFactorDisable_AcceptsRecoveryCode(t *testing.T) {
	us, _, rdb, cipher, au := setupUser(t)
	_, recovery := enrollTOTP(t, us, cipher, au.ID, au.Username)

	rr := authed(t, TwoFactorDisableHandler(us, cipher, rdb, zap.NewNop()),
		http.MethodDelete, "/users/me/2fa", au,
		map[string]string{"password": "correct-horse", "code": recovery[0]})

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestTwoFactorStatus(t *testing.T) {
	us, _, _, cipher, au := setupUser(t)
	h := TwoFactorStatusHandler(us, zap.NewNop())

	rr := authed(t, h, http.MethodGet, "/users/me/2fa", au, nil)
	var before twoFactorStatusResponse
	json.Unmarshal(rr.Body.Bytes(), &before)
	if before.Enabled || before.PendingSetup {
		t.Fatalf("fresh account reports %+v", before)
	}

	enrollTOTP(t, us, cipher, au.ID, au.Username)

	rr = authed(t, h, http.MethodGet, "/users/me/2fa", au, nil)
	var after twoFactorStatusResponse
	json.Unmarshal(rr.Body.Bytes(), &after)
	if !after.Enabled {
		t.Fatal("status does not report the enabled factor")
	}
	if after.RecoveryCodesRemaining != recoveryCodeCount {
		t.Fatalf("recovery_codes_remaining = %d, want %d", after.RecoveryCodesRemaining, recoveryCodeCount)
	}
}

func TestTwoFactorRecoveryCodes_RegenerateInvalidatesOldSet(t *testing.T) {
	us, _, rdb, cipher, au := setupUser(t)
	secret, oldCodes := enrollTOTP(t, us, cipher, au.ID, au.Username)

	rr := authed(t, TwoFactorRecoveryCodesHandler(us, cipher, rdb, zap.NewNop()),
		http.MethodPost, "/users/me/2fa/recovery-codes", au,
		map[string]string{"password": "correct-horse", "code": currentCode(t, secret)})

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp recoveryCodesResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("got %d codes, want %d", len(resp.RecoveryCodes), recoveryCodeCount)
	}

	// Old codes must be dead.
	ok, err := us.ConsumeRecoveryCode(au.ID, hashRecoveryCode(oldCodes[0]))
	if err != nil {
		t.Fatalf("ConsumeRecoveryCode: %v", err)
	}
	if ok {
		t.Fatal("a code from the previous set is still redeemable")
	}
}
