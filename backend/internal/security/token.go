package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

const (
	TokenEncryptionKeyEnv   = "SANDBOX_TOKEN_ENCRYPTION_KEY"
	TokenEncryptionKeyIDEnv = "SANDBOX_TOKEN_ENCRYPTION_KEY_ID"
	defaultTokenKeyID       = "v1"
)

// TokenCipher provides encryption/decryption for sandbox access tokens.
type TokenCipher struct {
	aead  cipher.AEAD
	keyID string
}

// NewTokenCipherFromEnv initializes token encryption from environment variables.
func NewTokenCipherFromEnv() (*TokenCipher, error) {
	rawKey := os.Getenv(TokenEncryptionKeyEnv)
	if rawKey == "" {
		return nil, fmt.Errorf("%s is required", TokenEncryptionKeyEnv)
	}

	key, err := parseAESKey(rawKey)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create gcm: %w", err)
	}

	keyID := os.Getenv(TokenEncryptionKeyIDEnv)
	if keyID == "" {
		keyID = defaultTokenKeyID
	}

	return &TokenCipher{aead: aead, keyID: keyID}, nil
}

// Encrypt encrypts token and returns base64 ciphertext, base64 nonce and key ID.
func (c *TokenCipher) Encrypt(token string) (ciphertext, nonce, keyID string, err error) {
	nonceBytes := make([]byte, c.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return "", "", "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertextBytes := c.aead.Seal(nil, nonceBytes, []byte(token), nil)
	return base64.StdEncoding.EncodeToString(ciphertextBytes),
		base64.StdEncoding.EncodeToString(nonceBytes),
		c.keyID,
		nil
}

// Decrypt decrypts token from base64 ciphertext and nonce.
func (c *TokenCipher) Decrypt(ciphertext, nonce string) (string, error) {
	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return "", fmt.Errorf("failed to decode nonce: %w", err)
	}
	plain, err := c.aead.Open(nil, nonceBytes, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt token: %w", err)
	}
	return string(plain), nil
}

// HashToken hashes token with SHA-256 and returns hex string.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// GenerateToken creates a random token encoded as hex.
func GenerateToken(size int) (string, error) {
	if size <= 0 {
		size = 32
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func parseAESKey(raw string) ([]byte, error) {
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && validAESKeyLen(len(decoded)) {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(raw); err == nil && validAESKeyLen(len(decoded)) {
		return decoded, nil
	}
	if validAESKeyLen(len(raw)) {
		return []byte(raw), nil
	}
	return nil, fmt.Errorf("invalid %s length: must be 16/24/32 bytes (raw/hex/base64)", TokenEncryptionKeyEnv)
}

func validAESKeyLen(n int) bool {
	return n == 16 || n == 24 || n == 32
}
