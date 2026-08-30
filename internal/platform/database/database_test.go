package database_test

import (
	"context"
	"errors"
	"os"
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

var (
	testPool      *pgxpool.Pool
	testTxManager *database.TxManager
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

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	testPool, err = database.NewPool(ctx, config.DatabaseConfig{
		URL:             connStr,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 5 * time.Minute,
	})
	if err != nil {
		panic(err)
	}

	database.SetMigrationsFS(sql.MigrationsFS)
	if err := database.MigrateUp(ctx, testPool, "migrations"); err != nil {
		panic(err)
	}

	testTxManager = database.NewTxManager(testPool)

	code := m.Run()

	testPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

func TestDatabase_PoolAndMigrations(t *testing.T) {
	ctx := context.Background()
	var exists bool
	err := testPool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users')").Scan(&exists)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestTxManager_RunInTx_CommitAndRollback(t *testing.T) {
	ctx := context.Background()
	rootQueries := sqlcgen.New(testPool)

	tenantID := "tenant-" + uuid.NewString()[:8]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	_, err := rootQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantID,
		Name:      "Acme Corp",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	// 1. Test Successful Commit inside RunInTx
	userID1 := "usr-" + uuid.NewString()[:8]
	err = testTxManager.RunInTx(ctx, func(txCtx context.Context) error {
		db := testTxManager.GetDB(txCtx)
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
	err = testTxManager.RunInTx(ctx, func(txCtx context.Context) error {
		db := testTxManager.GetDB(txCtx)
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
	ctx := context.Background()
	rootQueries := sqlcgen.New(testPool)

	tenantID := "tenant-" + uuid.NewString()[:8]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	_, err := rootQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantID,
		Name:      "Savepoint Corp",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	userIDPrimary := "usr-prim-" + uuid.NewString()[:8]
	userIDSecondary := "usr-sec-" + uuid.NewString()[:8]

	// Outer transaction
	err = testTxManager.RunInTx(ctx, func(txCtx context.Context) error {
		db := testTxManager.GetDB(txCtx)
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
		nestedErr := testTxManager.RunInTx(txCtx, func(nestedCtx context.Context) error {
			nestedDB := testTxManager.GetDB(nestedCtx)
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
