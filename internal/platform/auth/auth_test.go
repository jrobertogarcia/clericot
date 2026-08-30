package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/platform/auth"
)

func TestAuthPrincipal_Methods(t *testing.T) {
	principal := &auth.AuthPrincipal{
		ID:       "usr-1",
		TenantID: "tenant-a",
		Email:    "user@a.com",
		Role:     "admin",
		Tenants:  []string{"tenant-a", "tenant-b"},
	}

	assert.True(t, principal.HasTenantAccess("tenant-a"))
	assert.True(t, principal.HasTenantAccess("tenant-b"))
	assert.False(t, principal.HasTenantAccess("tenant-c"))
	assert.True(t, principal.HasRole("admin"))
	assert.False(t, principal.HasRole("superadmin"))

	superadmin := &auth.AuthPrincipal{
		ID:       "usr-root",
		TenantID: "system",
		Role:     "superadmin",
	}
	assert.True(t, superadmin.HasTenantAccess("any-tenant"))
	assert.True(t, superadmin.HasRole("any-role"))
}

func TestTokenService_GenerateAndValidate(t *testing.T) {
	ctx := context.Background()
	secret := "test-secret-key-must-be-long-enough-32b"
	tokenSvc := auth.NewTokenService(secret, nil)

	principal := &auth.AuthPrincipal{
		ID:       "usr-123",
		TenantID: "tenant-corp",
		Email:    "user@corp.com",
		Role:     "member",
		Tenants:  []string{"tenant-corp", "tenant-secondary"},
	}

	tokenStr, err := tokenSvc.GenerateToken(principal, 15*time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, tokenStr)

	parsed, err := tokenSvc.ValidateToken(ctx, tokenStr)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	assert.Equal(t, principal.ID, parsed.ID)
	assert.Equal(t, principal.TenantID, parsed.TenantID)
	assert.Equal(t, principal.Email, parsed.Email)
	assert.Equal(t, principal.Role, parsed.Role)
	assert.True(t, parsed.HasTenantAccess("tenant-secondary"))
}

func TestTokenService_ValidateInvalidToken(t *testing.T) {
	ctx := context.Background()
	secret := "test-secret-key-must-be-long-enough-32b"
	tokenSvc := auth.NewTokenService(secret, nil)

	_, err := tokenSvc.ValidateToken(ctx, "invalid.jwt.string")
	assert.ErrorIs(t, err, auth.ErrInvalidToken)
}
