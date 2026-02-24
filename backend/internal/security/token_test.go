package security

import (
	"strings"
	"testing"
)

func TestTokenCipherEncryptDecryptRoundTrip(t *testing.T) {
	t.Setenv(TokenEncryptionKeyEnv, "0123456789abcdef0123456789abcdef")
	t.Setenv(TokenEncryptionKeyIDEnv, "k1")

	cipher, err := NewTokenCipherFromEnv()
	if err != nil {
		t.Fatalf("NewTokenCipherFromEnv() error = %v", err)
	}

	plain := "token-abc-123"
	ciphertext, nonce, keyID, err := cipher.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if ciphertext == "" || nonce == "" {
		t.Fatalf("Encrypt() returned empty ciphertext/nonce")
	}
	if strings.Contains(ciphertext, plain) {
		t.Fatalf("ciphertext should not contain plaintext")
	}
	if keyID != "k1" {
		t.Fatalf("unexpected keyID: got %q want %q", keyID, "k1")
	}

	decrypted, err := cipher.Decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted != plain {
		t.Fatalf("Decrypt() mismatch: got %q want %q", decrypted, plain)
	}
}

func TestNewTokenCipherFromEnvRequiresKey(t *testing.T) {
	t.Setenv(TokenEncryptionKeyEnv, "")
	_, err := NewTokenCipherFromEnv()
	if err == nil {
		t.Fatalf("expected error when %s is missing", TokenEncryptionKeyEnv)
	}
}

func TestHashTokenStable(t *testing.T) {
	const token = "same-token"
	if HashToken(token) != HashToken(token) {
		t.Fatalf("HashToken should be deterministic")
	}
	if HashToken(token) == HashToken("another-token") {
		t.Fatalf("different tokens should have different hashes")
	}
}
