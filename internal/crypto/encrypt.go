package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidKeySize = errors.New("coco/crypto: encryption key must be exactly 32 bytes (AES-256)")
	ErrDecryptFailed  = errors.New("coco/crypto: authentication or decryption failed (corrupted or wrong key)")
)

// EncryptPage encrypts a 16 KiB page slice using AES-256-GCM with associated data.
func EncryptPage(plain []byte, key []byte, pageID uint64) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Associated data binds PageID to prevent page swapping
	aad := make([]byte, 8)
	aad[0] = byte(pageID >> 56)
	aad[1] = byte(pageID >> 48)
	aad[2] = byte(pageID >> 40)
	aad[3] = byte(pageID >> 32)
	aad[4] = byte(pageID >> 24)
	aad[5] = byte(pageID >> 16)
	aad[6] = byte(pageID >> 8)
	aad[7] = byte(pageID)

	ciphertext := gcm.Seal(nil, nonce, plain, aad)

	// Envelope: Nonce (12 bytes) + Ciphertext + Tag
	envelope := make([]byte, len(nonce)+len(ciphertext))
	copy(envelope, nonce)
	copy(envelope[len(nonce):], ciphertext)

	return envelope, nil
}

// DecryptPage decrypts and authenticates a page envelope.
func DecryptPage(envelope []byte, key []byte, pageID uint64) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(envelope) < nonceSize {
		return nil, fmt.Errorf("%w: envelope too short", ErrDecryptFailed)
	}

	nonce := envelope[:nonceSize]
	ciphertext := envelope[nonceSize:]

	aad := make([]byte, 8)
	aad[0] = byte(pageID >> 56)
	aad[1] = byte(pageID >> 48)
	aad[2] = byte(pageID >> 40)
	aad[3] = byte(pageID >> 32)
	aad[4] = byte(pageID >> 24)
	aad[5] = byte(pageID >> 16)
	aad[6] = byte(pageID >> 8)
	aad[7] = byte(pageID)

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptFailed, err)
	}

	return plaintext, nil
}
