package http

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// totpKeyInfo domain-separates the TOTP encryption key from whatever else the
// same key material is used for. Changing it invalidates every stored secret.
const totpKeyInfo = "onyxchat-totp-secret-encryption-v1"

// SecretCipher encrypts TOTP secrets for storage.
//
// A TOTP secret is a bearer credential: anyone holding it can mint valid codes
// forever, so unlike a password it cannot be hashed — the server has to be
// able to read it back. Encrypting it at rest means a database-only compromise
// (a leaked snapshot, an RDS read path, SQL injection) does not by itself hand
// the attacker everyone's second factor, since the key lives in the process
// environment rather than in Postgres.
type SecretCipher struct {
	aead cipher.AEAD
}

// NewSecretCipher derives an AES-256-GCM key from arbitrary-length key
// material via HKDF, so callers can pass a passphrase-style secret without
// worrying about length.
func NewSecretCipher(keyMaterial string) (*SecretCipher, error) {
	if keyMaterial == "" {
		return nil, errors.New("NewSecretCipher: key material must not be empty")
	}

	key, err := hkdf.Key(sha256.New, []byte(keyMaterial), nil, totpKeyInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("derive totp key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &SecretCipher{aead: aead}, nil
}

// Encrypt returns base64(nonce || ciphertext || tag).
func (c *SecretCipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. A tampered or key-mismatched value fails the GCM
// tag check and returns an error rather than garbage.
func (c *SecretCipher) Decrypt(encoded string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode stored secret: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("stored secret is truncated")
	}
	plaintext, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt stored secret: %w", err)
	}
	return string(plaintext), nil
}
