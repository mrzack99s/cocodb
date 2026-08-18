package crypto_test

import (
	"bytes"
	"testing"

	"cocodb/internal/crypto"
)

func TestAES256GCMEncryption(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	pageData := make([]byte, 16384)
	copy(pageData, []byte("sensitive database page content at rest"))

	// Encrypt
	envelope, err := crypto.EncryptPage(pageData, key, 42)
	if err != nil {
		t.Fatalf("EncryptPage failed: %v", err)
	}

	if bytes.Equal(envelope, pageData) {
		t.Fatalf("ciphertext should not match plaintext")
	}

	// Decrypt with correct key & pageID
	decrypted, err := crypto.DecryptPage(envelope, key, 42)
	if err != nil {
		t.Fatalf("DecryptPage failed: %v", err)
	}

	if !bytes.Equal(decrypted, pageData) {
		t.Fatalf("decrypted data mismatch")
	}

	// Tampered ciphertext must fail
	tampered := make([]byte, len(envelope))
	copy(tampered, envelope)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = crypto.DecryptPage(tampered, key, 42)
	if err == nil {
		t.Fatalf("expected decryption failure on tampered ciphertext")
	}

	// Wrong pageID must fail (AAD protection)
	_, err = crypto.DecryptPage(envelope, key, 99)
	if err == nil {
		t.Fatalf("expected decryption failure on wrong page ID AAD")
	}
}
