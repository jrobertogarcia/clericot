package testharness

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"clericot/internal/config"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

// Harness provides unified fixtures and containers for testing enterprise modules.
type Harness struct {
	AdminPool *pgxpool.Pool
	AppPool   *pgxpool.Pool
	TxManager *database.TxManager
	TokenSvc  *platformAuth.TokenService
	cleanupFn func()
}

// New constructs a test harness with migrated PostgreSQL container.
func New(t *testing.T) *Harness {
	t.Helper()
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
	require.NoError(t, err)

	adminConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	adminPool, err := database.NewPool(ctx, config.DatabaseConfig{
		URL:             adminConnStr,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 5 * time.Minute,
	})
	require.NoError(t, err)

	database.SetMigrationsFS(sql.MigrationsFS)
	err = database.MigrateUp(ctx, adminPool, "migrations")
	require.NoError(t, err)

	appConnStr := strings.Replace(adminConnStr, "postgres:postgrespassword", "app_user:app_user_password", 1)
	appPool, err := database.NewPool(ctx, config.DatabaseConfig{
		URL:             appConnStr,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 5 * time.Minute,
	})
	require.NoError(t, err)

	txManager := database.NewTxManager(appPool)
	tokenSvc := platformAuth.NewTokenService("test-jwt-secret-key-32-characters-long", nil)

	return &Harness{
		AdminPool: adminPool,
		AppPool:   appPool,
		TxManager: txManager,
		TokenSvc:  tokenSvc,
		cleanupFn: func() {
			appPool.Close()
			adminPool.Close()
			_ = pgContainer.Terminate(ctx)
		},
	}
}

// Close terminates containers and pools.
func (h *Harness) Close() {
	if h.cleanupFn != nil {
		h.cleanupFn()
	}
}

// SeedTenant creates an active tenant in the database.
func (h *Harness) SeedTenant(ctx context.Context, name string) (string, error) {
	tenantID := "tenant-" + uuid.NewString()[:8]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	queries := sqlcgen.New(h.AdminPool)

	_, err := queries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantID,
		Name:      name,
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	if err != nil {
		return "", fmt.Errorf("failed to seed tenant: %w", err)
	}
	return tenantID, nil
}

// SeedUser creates an active user under a tenant.
func (h *Harness) SeedUser(ctx context.Context, tenantID, email, name, role string) (*sqlcgen.Users, error) {
	userID := "usr-" + uuid.NewString()[:8]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	var user sqlcgen.Users
	err := h.TxManager.RunInTx(tenant.WithTenant(ctx, tenantID), func(txCtx context.Context) error {
		db := h.TxManager.GetDB(txCtx)
		u, err := sqlcgen.New(db).CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userID,
			TenantID:     tenantID,
			Email:        email,
			PasswordHash: "pw",
			Name:         name,
			Role:         role,
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		if err != nil {
			return err
		}
		user = u
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to seed user: %w", err)
	}
	return &user, nil
}
