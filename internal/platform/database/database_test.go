package database_test

import (
	"context"
	"errors"
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
	"clericot/internal/platform/database"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

func startTestPostgres(t *testing.T) (*pgxpool.Pool, func()) {
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

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := database.NewPool(ctx, config.DatabaseConfig{
		URL:             connStr,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 5 * time.Minute,
	})
	require.NoError(t, err)

	// Configure and run embedded Goose migrations
	database.SetMigrationsFS(sql.MigrationsFS)
	err = database.MigrateUp(ctx, pool, "migrations")
	require.NoError(t, err)

	cleanup := func() {
		pool.Close()
		_ = pgContainer.Terminate(ctx)
	}

	return pool, cleanup
}

func TestDatabase_PoolAndMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping testcontainers integration test in short mode")
	}

	pool, cleanup := startTestPostgres(t)
	defer cleanup()

	ctx := context.Background()
	var exists bool
	err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users')").Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestTxManager_RunInTx_CommitAndRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping testcontainers integration test in short mode")
	}

	pool, cleanup := startTestPostgres(t)
	defer cleanup()

	ctx := context.Background()
	txManager := database.NewTxManager(pool)
	rootQueries := sqlcgen.New(pool)

	tenantID := "tenant-" + uuid.NewString()[:8]
	_, err := pool.Exec(ctx, "INSERT INTO tenants (id, name) VALUES ($1, $2)", tenantID, "Acme Corp")
	require.NoError(t, err)

	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	// 1. Test Successful Commit inside RunInTx
	userID1 := "usr-" + uuid.NewString()[:8]
	err = txManager.RunInTx(ctx, func(txCtx context.Context) error {
		db := txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)
		_, err := queries.CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userID1,
			TenantID:     tenantID,
			Email:        "alice@acme.com",
			Name:         "Alice",
			PasswordHash: "hashed-pw",
			Role:         "admin",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		return err
	})
	require.NoError(t, err)

	// Assert user was committed
	user, err := rootQueries.GetUserByID(ctx, sqlcgen.GetUserByIDParams{
		ID:       userID1,
		TenantID: tenantID,
	})
	require.NoError(t, err)
	assert.Equal(t, "Alice", user.Name)

	// 2. Test Rollback on Error
	userID2 := "usr-" + uuid.NewString()[:8]
	err = txManager.RunInTx(ctx, func(txCtx context.Context) error {
		db := txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)
		_, err := queries.CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userID2,
			TenantID:     tenantID,
			Email:        "bob@acme.com",
			Name:         "Bob",
			PasswordHash: "hashed-pw",
			Role:         "member",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		require.NoError(t, err)
		return errors.New("simulated business error")
	})
	assert.Error(t, err)

	// Assert user was rolled back
	_, err = rootQueries.GetUserByID(ctx, sqlcgen.GetUserByIDParams{
		ID:       userID2,
		TenantID: tenantID,
	})
	assert.Error(t, err)
}

func TestTxManager_RunInTx_SavepointNesting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping testcontainers integration test in short mode")
	}

	pool, cleanup := startTestPostgres(t)
	defer cleanup()

	ctx := context.Background()
	txManager := database.NewTxManager(pool)
	rootQueries := sqlcgen.New(pool)

	tenantID := "tenant-" + uuid.NewString()[:8]
	_, err := pool.Exec(ctx, "INSERT INTO tenants (id, name) VALUES ($1, $2)", tenantID, "Savepoint Corp")
	require.NoError(t, err)

	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	userIDPrimary := "usr-prim-" + uuid.NewString()[:8]
	userIDSecondary := "usr-sec-" + uuid.NewString()[:8]

	// Outer transaction
	err = txManager.RunInTx(ctx, func(txCtx context.Context) error {
		db := txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)

		// Create primary user in outer transaction
		_, err := queries.CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userIDPrimary,
			TenantID:     tenantID,
			Email:        "primary@corp.com",
			Name:         "Primary User",
			PasswordHash: "hash1",
			Role:         "admin",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		require.NoError(t, err)

		// Inner nested transaction (SAVEPOINT) that fails
		nestedErr := txManager.RunInTx(txCtx, func(nestedCtx context.Context) error {
			nestedDB := txManager.GetDB(nestedCtx)
			nestedQueries := sqlcgen.New(nestedDB)
			_, err := nestedQueries.CreateUser(nestedCtx, sqlcgen.CreateUserParams{
				ID:           userIDSecondary,
				TenantID:     tenantID,
				Email:        "secondary@corp.com",
				Name:         "Secondary User",
				PasswordHash: "hash2",
				Role:         "member",
				CreatedAt:    ts,
				UpdatedAt:    ts,
			})
			require.NoError(t, err)
			return errors.New("nested operation failed")
		})
		assert.Error(t, nestedErr)

		// Outer transaction recovers from savepoint error and continues
		return nil
	})
	require.NoError(t, err)

	// Primary user should exist
	user, err := rootQueries.GetUserByID(ctx, sqlcgen.GetUserByIDParams{
		ID:       userIDPrimary,
		TenantID: tenantID,
	})
	require.NoError(t, err)
	assert.Equal(t, "Primary User", user.Name)

	// Secondary user from aborted savepoint should NOT exist
	_, err = rootQueries.GetUserByID(ctx, sqlcgen.GetUserByIDParams{
		ID:       userIDSecondary,
		TenantID: tenantID,
	})
	assert.Error(t, err)
}
