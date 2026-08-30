package auth_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"clericot/internal/config"
	authModule "clericot/internal/modules/auth"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

var (
	testAdminPool *pgxpool.Pool
	testAppPool   *pgxpool.Pool
	testAuthMod   *authModule.Module
	tokenSvc      *platformAuth.TokenService
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgrespassword"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		panic(err)
	}

	adminConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	testAdminPool, err = database.NewPool(ctx, config.DatabaseConfig{
		URL:             adminConnStr,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 5 * time.Minute,
	})
	if err != nil {
		panic(err)
	}

	database.SetMigrationsFS(sql.MigrationsFS)
	if err := database.MigrateUp(ctx, testAdminPool, "migrations"); err != nil {
		panic(err)
	}

	appConnStr := strings.Replace(adminConnStr, "postgres:postgrespassword", "app_user:app_user_password", 1)
	testAppPool, err = database.NewPool(ctx, config.DatabaseConfig{
		URL:             appConnStr,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 5 * time.Minute,
	})
	if err != nil {
		panic(err)
	}

	txManager := database.NewTxManager(testAppPool)
	tokenSvc = platformAuth.NewTokenService("super-secret-jwt-key-minimum-32-chars-long", nil)
	testAuthMod = authModule.NewModule(nil, txManager, tokenSvc)

	code := m.Run()

	testAppPool.Close()
	testAdminPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

func TestAuthService_RegisterLoginAndGetMe(t *testing.T) {
	ctx := context.Background()
	adminQueries := sqlcgen.New(testAdminPool)

	// Create test tenant
	tenantID := "tenant-auth-" + uuid.NewString()[:8]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	_, err := adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantID,
		Name:      "Auth Module Test Tenant",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	email := "user-" + uuid.NewString()[:8] + "@auth-module.com"
	password := "SecureP@ssword2026!"

	// 1. Register User
	authToken, user, err := testAuthMod.Service.Register(ctx, authModule.RegisterDTO{
		TenantID: tenantID,
		Email:    email,
		Password: password,
		Name:     "Decoupled User",
	})
	require.NoError(t, err)
	require.NotNil(t, authToken)
	require.NotEmpty(t, authToken.AccessToken)
	assert.True(t, authToken.ExpiresAt.After(time.Now()))
	assert.Equal(t, email, user.Email)
	assert.Equal(t, authModule.RoleMember, user.Role)

	// 2. Duplicate Registration Rejection
	_, _, err = testAuthMod.Service.Register(ctx, authModule.RegisterDTO{
		TenantID: tenantID,
		Email:    email,
		Password: password,
		Name:     "Duplicate",
	})
	assert.ErrorIs(t, err, authModule.ErrUserAlreadyExists)

	// 3. Token Validation & GetMe
	principal, err := tokenSvc.ValidateToken(ctx, authToken.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, email, principal.Email)
	assert.Equal(t, tenantID, principal.TenantID)

	authedCtx := platformAuth.WithPrincipal(ctx, principal)
	me, err := testAuthMod.Service.GetMe(authedCtx)
	require.NoError(t, err)
	assert.Equal(t, "Decoupled User", me.Name)
	assert.Equal(t, email, me.Email)

	// 4. Login with Valid Credentials
	loginToken, loggedUser, err := testAuthMod.Service.Login(ctx, authModule.LoginDTO{
		TenantID: tenantID,
		Email:    email,
		Password: password,
	})
	require.NoError(t, err)
	require.NotNil(t, loginToken)
	assert.Equal(t, user.ID, loggedUser.ID)

	// 5. Login with Invalid Password
	_, _, err = testAuthMod.Service.Login(ctx, authModule.LoginDTO{
		TenantID: tenantID,
		Email:    email,
		Password: "InvalidPassword999",
	})
	assert.ErrorIs(t, err, authModule.ErrInvalidCredentials)
}
