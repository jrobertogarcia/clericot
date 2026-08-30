package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidCiphertext = errors.New("ciphertext too short or corrupted")
	ErrInvalidKeySize    = errors.New("key must be 32 bytes for AES-256")
)

// EncryptAES256GCM encrypts plaintext using AES-256-GCM with a random 12-byte nonce.
func EncryptAES256GCM(plaintext []byte, key []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAES256GCM decrypts AES-256-GCM base64 encoded ciphertext.
func DecryptAES256GCM(encodedCiphertext string, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrInvalidKeySize
	}

	data, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, ErrInvalidCiphertext
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt ciphertext: %w", err)
	}

	return plaintext, nil
}
