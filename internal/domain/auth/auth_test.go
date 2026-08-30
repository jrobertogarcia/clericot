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
	domainAuth "clericot/internal/domain/auth"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

var (
	testAdminPool *pgxpool.Pool
	testAppPool   *pgxpool.Pool
	testAuthSvc   *domainAuth.AuthService
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
	testAuthSvc = domainAuth.NewAuthService(txManager, tokenSvc)

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
		Name:      "Auth Tenant Org",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	email := "developer@auth-org.com"
	password := "SecureP@ssword2026!"

	// 1. Register User
	token, expiresAt, err := testAuthSvc.Register(ctx, tenantID, email, password, "Developer Dev")
	require.NoError(t, err)
	require.NotEmpty(t, token)
	assert.True(t, expiresAt.After(time.Now()))

	// 2. Validate Token and Retrieve User
	principal, err := tokenSvc.ValidateToken(ctx, token)
	require.NoError(t, err)
	assert.Equal(t, email, principal.Email)
	assert.Equal(t, tenantID, principal.TenantID)

	authedCtx := platformAuth.WithPrincipal(ctx, principal)
	user, err := testAuthSvc.GetMe(authedCtx)
	require.NoError(t, err)
	assert.Equal(t, "Developer Dev", user.Name)
	assert.Equal(t, email, user.Email)

	// 3. Login with Credentials
	loginToken, _, err := testAuthSvc.Login(ctx, tenantID, email, password)
	require.NoError(t, err)
	require.NotEmpty(t, loginToken)

	// 4. Login with Invalid Password
	_, _, err = testAuthSvc.Login(ctx, tenantID, email, "WrongPassword")
	assert.Error(t, err)
}
