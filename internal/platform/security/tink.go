package security

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/tink-crypto/tink-go/v2/aead"
	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/tink"
)

// EnvelopeEngine manages multi-cloud envelope encryption for sensitive secrets at rest.
type EnvelopeEngine struct {
	aead tink.AEAD
}

// NewTinkLocalEngine initializes an in-memory Tink AEAD for local development and integration tests.
func NewTinkLocalEngine() (*EnvelopeEngine, error) {
	kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return nil, fmt.Errorf("failed to create tink keyset handle: %w", err)
	}

	aeadPrimitive, err := aead.New(kh)
	if err != nil {
		return nil, fmt.Errorf("failed to create tink aead primitive: %w", err)
	}

	return &EnvelopeEngine{aead: aeadPrimitive}, nil
}

// Encrypt encrypts plaintext with optional associated authenticated data (AAD).
func (e *EnvelopeEngine) Encrypt(ctx context.Context, plaintext, associatedData []byte) ([]byte, error) {
	return e.aead.Encrypt(plaintext, associatedData)
}

// Decrypt decrypts ciphertext with associated authenticated data (AAD).
func (e *EnvelopeEngine) Decrypt(ctx context.Context, ciphertext, associatedData []byte) ([]byte, error) {
	return e.aead.Decrypt(ciphertext, associatedData)
}

// GenerateRandomBytes generates cryptographically secure random bytes of specified length.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return b, nil
}
