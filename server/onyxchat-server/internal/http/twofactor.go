package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
)

const (
	// totpIssuer is what authenticator apps label the entry with.
	totpIssuer = "OnyxChat"

	// totpPeriod / totpSkew: codes rotate every 30s and one step either side
	// is accepted, giving a 90s window that absorbs ordinary clock drift
	// without widening the guessing surface much.
	totpPeriod = 30
	totpSkew   = 1

	// mfaChallengeTTL bounds how long a half-finished login stays resumable.
	mfaChallengeTTL     = 5 * time.Minute
	mfaChallengePrefix  = "mfa:chal:"
	mfaAttemptsPrefix   = "mfa:att:"
	maxMFAAttempts      = 5
	totpReplayPrefix    = "mfa:used:"
	recoveryCodeCount   = 10
	recoveryCodeLen     = 16 // characters, 5 bits each → 80 bits of entropy
	recoveryCodeGroup   = 4  // formatted as xxxx-xxxx-xxxx-xxxx
	recoveryCodeAlfabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

var (
	// errMFACodeInvalid is the single error surfaced for every kind of bad
	// second factor, so the response can't be used to tell "wrong code" from
	// "already used" from "not a recovery code".
	errMFACodeInvalid = errors.New("invalid verification code")

	errMFAChallengeInvalid = errors.New("invalid or expired login challenge")
	errMFATooManyAttempts  = errors.New("too many incorrect codes")
)

// ─────────────────────────────────────────────────────────────
// Enrollment secrets
// ─────────────────────────────────────────────────────────────

// newTOTPKey generates a fresh secret bound to this account, and the
// otpauth:// URI plus QR PNG the client needs to enroll it.
func newTOTPKey(username string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: username,
		Period:      totpPeriod,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1, // what every authenticator app supports
	})
}

// qrDataURI renders the enrollment QR code as an inline PNG data URI. Doing
// this server-side keeps the web client free of a QR dependency.
func qrDataURI(key *otp.Key) (string, error) {
	img, err := key.Image(220, 220)
	if err != nil {
		return "", fmt.Errorf("render qr: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode qr png: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// ─────────────────────────────────────────────────────────────
// Code verification
// ─────────────────────────────────────────────────────────────

// verifyTOTP checks a 6-digit code against the user's secret and then burns
// it. TOTP codes stay valid for the whole 90s window, so without the burn an
// attacker who observes one (shoulder-surfing, a phished form, a replayed
// request) could reuse it; the Redis SETNX makes each code good exactly once
// per user, across every server task.
func verifyTOTP(ctx context.Context, rdb *redis.Client, userID int64, secret, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errMFACodeInvalid
	}

	ok, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      totpSkew,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil || !ok {
		return errMFACodeInvalid
	}

	// Window length: the current step plus `skew` steps either side.
	ttl := time.Duration(totpPeriod*(2*totpSkew+1)) * time.Second
	key := fmt.Sprintf("%s%d:%s", totpReplayPrefix, userID, code)

	defer ObserveRedisOp("mfa_replay_setnx", time.Now())
	fresh, err := rdb.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		// Fail closed: if the replay guard is unavailable we can't promise the
		// code is unused, and a second factor that silently degrades is worse
		// than one that asks the user to try again.
		return fmt.Errorf("totp replay guard unavailable: %w", err)
	}
	if !fresh {
		return errMFACodeInvalid
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// Recovery codes
// ─────────────────────────────────────────────────────────────

// generateRecoveryCodes returns a fresh set of display-formatted codes along
// with the hashes to persist. The plaintext is returned to the caller exactly
// once — it is never recoverable afterwards.
func generateRecoveryCodes() (codes []string, hashes []string, err error) {
	codes = make([]string, 0, recoveryCodeCount)
	hashes = make([]string, 0, recoveryCodeCount)

	for i := 0; i < recoveryCodeCount; i++ {
		buf := make([]byte, recoveryCodeLen)
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		var sb strings.Builder
		for j, b := range buf {
			if j > 0 && j%recoveryCodeGroup == 0 {
				sb.WriteByte('-')
			}
			// len(alphabet) is 32, which divides 256 evenly — no modulo bias.
			sb.WriteByte(recoveryCodeAlfabet[int(b)%len(recoveryCodeAlfabet)])
		}
		code := sb.String()
		codes = append(codes, code)
		hashes = append(hashes, hashRecoveryCode(code))
	}
	return codes, hashes, nil
}

// normalizeRecoveryCode makes lookup insensitive to the formatting dashes and
// to case, so a user can type the code however they copied it down.
func normalizeRecoveryCode(code string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(code) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// hashRecoveryCode is a plain SHA-256 rather than bcrypt: recovery codes carry
// 80 bits of entropy from crypto/rand, so there is no dictionary to stretch
// against, and a single digest keeps verification to one indexed lookup.
func hashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

// looksLikeRecoveryCode distinguishes the two things a user may type into the
// same box. A TOTP code is six digits; anything else of the right length is
// tried as a recovery code.
func looksLikeRecoveryCode(input string) bool {
	return len(normalizeRecoveryCode(input)) == recoveryCodeLen
}

// ─────────────────────────────────────────────────────────────
// Login challenges
// ─────────────────────────────────────────────────────────────

// mfaChallenge is the server-side state of a login that has passed the
// password step and is waiting on a second factor.
type mfaChallenge struct {
	UserID   int64  `json:"uid"`
	Username string `json:"uname"`
}

// newMFAChallenge issues an opaque, single-use token standing in for "this
// password was already verified". The password is never re-sent for step two,
// and the token is worthless without the code.
func newMFAChallenge(ctx context.Context, rdb *redis.Client, user mfaChallenge) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)

	payload, err := json.Marshal(user)
	if err != nil {
		return "", err
	}

	defer ObserveRedisOp("mfa_challenge_set", time.Now())
	if err := rdb.Set(ctx, mfaChallengePrefix+token, payload, mfaChallengeTTL).Err(); err != nil {
		return "", err
	}
	return token, nil
}

// lookupMFAChallenge resolves a challenge token and counts the attempt against
// it. Budgeting attempts per challenge (rather than per username) means an
// attacker who guesses wrong five times has to go back and re-prove the
// password before getting five more tries at a six-digit code.
func lookupMFAChallenge(ctx context.Context, rdb *redis.Client, token string) (*mfaChallenge, error) {
	if token == "" {
		return nil, errMFAChallengeInvalid
	}

	dbStart := time.Now()
	payload, err := rdb.Get(ctx, mfaChallengePrefix+token).Bytes()
	ObserveRedisOp("mfa_challenge_get", dbStart)
	if err == redis.Nil {
		return nil, errMFAChallengeInvalid
	}
	if err != nil {
		return nil, err
	}

	attempts, err := rdb.Incr(ctx, mfaAttemptsPrefix+token).Result()
	if err != nil {
		return nil, err
	}
	if attempts == 1 {
		// First attempt sets the counter's lifetime to the challenge's own.
		_ = rdb.Expire(ctx, mfaAttemptsPrefix+token, mfaChallengeTTL).Err()
	}
	if attempts > maxMFAAttempts {
		_ = discardMFAChallenge(ctx, rdb, token)
		return nil, errMFATooManyAttempts
	}

	var c mfaChallenge
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, errMFAChallengeInvalid
	}
	return &c, nil
}

// discardMFAChallenge retires a challenge and its attempt counter, on success
// or on exhaustion.
func discardMFAChallenge(ctx context.Context, rdb *redis.Client, token string) error {
	defer ObserveRedisOp("mfa_challenge_del", time.Now())
	return rdb.Del(ctx, mfaChallengePrefix+token, mfaAttemptsPrefix+token).Err()
}
