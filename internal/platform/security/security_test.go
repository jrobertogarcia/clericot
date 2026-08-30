package security_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/platform/security"
)

func TestArgon2id_HashAndVerify(t *testing.T) {
	password := "SecretP@ssw0rd!2026"

	// Use lightweight test parameters for fast execution
	testParams := &security.Argon2Params{
		Memory:      16 * 1024,
		Iterations:  1,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}

	hash, err := security.HashPassword(password, testParams)
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	// Verify valid password
	match, err := security.VerifyPassword(password, hash)
	require.NoError(t, err)
	assert.True(t, match)

	// Verify invalid password
	match, err = security.VerifyPassword("WrongP@ssw0rd", hash)
	require.NoError(t, err)
	assert.False(t, match)
}

func TestAES256GCM_EncryptDecrypt(t *testing.T) {
	key, err := security.GenerateRandomBytes(32)
	require.NoError(t, err)

	plaintext := []byte("confidential PII data")

	ciphertext, err := security.EncryptAES256GCM(plaintext, key)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)

	decrypted, err := security.DecryptAES256GCM(ciphertext, key)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	// Test decryption failure with wrong key
	wrongKey, _ := security.GenerateRandomBytes(32)
	_, err = security.DecryptAES256GCM(ciphertext, wrongKey)
	assert.Error(t, err)
}

func TestTinkEngine_LocalAEAD(t *testing.T) {
	ctx := context.Background()
	engine, err := security.NewTinkLocalEngine()
	require.NoError(t, err)

	plaintext := []byte("top secret envelope data")
	aad := []byte("tenant-org-123")

	ciphertext, err := engine.Encrypt(ctx, plaintext, aad)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)

	decrypted, err := engine.Decrypt(ctx, ciphertext, aad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	// Test mismatching AAD
	_, err = engine.Decrypt(ctx, ciphertext, []byte("wrong-tenant"))
	assert.Error(t, err)
}

func TestRateLimiter_FailOpenWhenNoRedis(t *testing.T) {
	ctx := context.Background()
	// Pass nil redis client to simulate redis down -> rate limiter must fail open
	limiter := security.NewRateLimiter(nil, nil)

	allowed, err := limiter.Allow(ctx, "ip:127.0.0.1", 10)
	require.NoError(t, err)
	assert.True(t, allowed)
}
