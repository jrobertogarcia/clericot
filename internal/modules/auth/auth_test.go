package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authModule "clericot/internal/modules/auth"
	platformAuth "clericot/internal/platform/auth"
	"clericot/tests/fixtures"
	"clericot/tests/testsuite"
)

var (
	testAuthMod *authModule.Module
	tokenSvc    *platformAuth.TokenService
)

func TestMain(m *testing.M) {
	testsuite.Main(m)
}

func TestAuthService_RegisterLoginAndGetMe(t *testing.T) {
	ctx := context.Background()
	tokenSvc = platformAuth.NewTokenService("super-secret-jwt-key-minimum-32-chars-long", nil)
	testAuthMod = authModule.NewModule(nil, testsuite.SharedTxManager, tokenSvc)

	// Create test tenant
	tenantID, err := testsuite.SeedTenant(ctx, "Auth Module Test Tenant")
	require.NoError(t, err)

	regDTO := fixtures.NewRegisterDTO(
		fixtures.WithRegisterTenantID(tenantID),
		fixtures.WithRegisterName("Decoupled User"),
	)

	// 1. Register User
	authToken, user, err := testAuthMod.Service.Register(ctx, regDTO)
	require.NoError(t, err)
	require.NotNil(t, authToken)
	require.NotEmpty(t, authToken.AccessToken)
	assert.True(t, authToken.ExpiresAt.After(time.Now()))
	assert.Equal(t, regDTO.Email, user.Email)
	assert.Equal(t, authModule.RoleMember, user.Role)

	// 2. Duplicate Registration Rejection
	_, _, err = testAuthMod.Service.Register(ctx, fixtures.NewRegisterDTO(
		fixtures.WithRegisterTenantID(tenantID),
		fixtures.WithRegisterEmail(regDTO.Email),
		fixtures.WithRegisterName("Duplicate"),
	))
	assert.ErrorIs(t, err, authModule.ErrUserAlreadyExists)

	// 3. Token Validation & GetMe
	principal, err := tokenSvc.ValidateToken(ctx, authToken.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, regDTO.Email, principal.Email)
	assert.Equal(t, tenantID, principal.TenantID)

	authedCtx := platformAuth.WithPrincipal(ctx, principal)
	me, err := testAuthMod.Service.GetMe(authedCtx)
	require.NoError(t, err)
	assert.Equal(t, "Decoupled User", me.Name)
	assert.Equal(t, regDTO.Email, me.Email)

	// 4. Login with Valid Credentials
	loginToken, loggedUser, err := testAuthMod.Service.Login(ctx, authModule.LoginDTO{
		TenantID: tenantID,
		Email:    regDTO.Email,
		Password: regDTO.Password,
	})
	require.NoError(t, err)
	require.NotNil(t, loginToken)
	assert.Equal(t, user.ID, loggedUser.ID)

	// 5. Login with Invalid Password
	_, _, err = testAuthMod.Service.Login(ctx, authModule.LoginDTO{
		TenantID: tenantID,
		Email:    regDTO.Email,
		Password: "InvalidPassword999",
	})
	assert.ErrorIs(t, err, authModule.ErrInvalidCredentials)
}
