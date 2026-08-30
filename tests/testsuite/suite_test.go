package testsuite_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/sqlcgen"
	"clericot/tests/testsuite"
)

func TestMain(m *testing.M) {
	testsuite.Main(m)
}

func TestRunTestInTx_AutoRollback(t *testing.T) {
	ctx := context.Background()

	tenantID, err := testsuite.SeedTenant(ctx, "Auto-Rollback Test Tenant")
	require.NoError(t, err)

	ephemeralUserID := "usr-ephem-" + uuid.NewString()[:8]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	testsuite.RunTestInTx(t, tenantID, func(txCtx context.Context) {
		db := testsuite.SharedTxManager.GetDB(txCtx)
		queries := sqlcgen.New(db)

		created, err := queries.CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           ephemeralUserID,
			TenantID:     tenantID,
			Email:        "ephemeral@tenant.com",
			Name:         "Ephemeral User",
			PasswordHash: "pw",
			Role:         "member",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		require.NoError(t, err)
		assert.Equal(t, ephemeralUserID, created.ID)

		// Verify readable inside active transaction
		fetched, err := queries.GetUserByID(txCtx, sqlcgen.GetUserByIDParams{
			ID:       ephemeralUserID,
			TenantID: tenantID,
		})
		require.NoError(t, err)
		assert.Equal(t, "Ephemeral User", fetched.Name)
	})

	// Outside transaction: Verify row was completely rolled back
	adminQueries := sqlcgen.New(testsuite.SharedAdminPool)
	_, err = adminQueries.GetUserByID(ctx, sqlcgen.GetUserByIDParams{
		ID:       ephemeralUserID,
		TenantID: tenantID,
	})
	assert.Error(t, err, "user should have been rolled back and not exist in database")
}

func TestRunTestInTx_TenantIsolation(t *testing.T) {
	ctx := context.Background()

	tenantA, err := testsuite.SeedTenant(ctx, "Tenant Alpha")
	require.NoError(t, err)

	tenantB, err := testsuite.SeedTenant(ctx, "Tenant Beta")
	require.NoError(t, err)

	userA, err := testsuite.SeedUser(ctx, tenantA, "alice@alpha.com", "Alice Alpha", "admin")
	require.NoError(t, err)

	userB, err := testsuite.SeedUser(ctx, tenantB, "bob@beta.com", "Bob Beta", "member")
	require.NoError(t, err)

	// Run within Tenant A transaction context
	testsuite.RunTestInTx(t, tenantA, func(txCtx context.Context) {
		db := testsuite.SharedTxManager.GetDB(txCtx)

		rows, err := db.Query(txCtx, "SELECT id, name FROM users")
		require.NoError(t, err)
		defer rows.Close()

		var names []string
		for rows.Next() {
			var id, name string
			require.NoError(t, rows.Scan(&id, &name))
			names = append(names, name)
		}

		assert.Contains(t, names, userA.Name)
		assert.NotContains(t, names, userB.Name)
	})

	// Run within Tenant B transaction context
	testsuite.RunTestInTx(t, tenantB, func(txCtx context.Context) {
		db := testsuite.SharedTxManager.GetDB(txCtx)

		rows, err := db.Query(txCtx, "SELECT id, name FROM users")
		require.NoError(t, err)
		defer rows.Close()

		var names []string
		for rows.Next() {
			var id, name string
			require.NoError(t, rows.Scan(&id, &name))
			names = append(names, name)
		}

		assert.Contains(t, names, userB.Name)
		assert.NotContains(t, names, userA.Name)
	})
}

func TestRunTestInSchema_EphemeralIsolation(t *testing.T) {
	var createdSchema string

	testsuite.RunTestInSchema(t, func(ctx context.Context, schema string) {
		createdSchema = schema
		require.NotEmpty(t, schema)

		// Create a test table inside the ephemeral schema
		createTableSQL := fmt.Sprintf("CREATE TABLE %s.ephemeral_data (id text primary key, value text)", schema)
		_, err := testsuite.SharedAdminPool.Exec(ctx, createTableSQL)
		require.NoError(t, err)

		// Insert test row
		insertSQL := fmt.Sprintf("INSERT INTO %s.ephemeral_data (id, value) VALUES ('1', 'test-data')", schema)
		_, err = testsuite.SharedAdminPool.Exec(ctx, insertSQL)
		require.NoError(t, err)

		// Verify schema existence
		var count int
		err = testsuite.SharedAdminPool.QueryRow(ctx, "SELECT count(*) FROM information_schema.schemata WHERE schema_name = $1", schema).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	// Outside callback: Verify schema was dropped via cascade
	ctx := context.Background()
	var count int
	err := testsuite.SharedAdminPool.QueryRow(ctx, "SELECT count(*) FROM information_schema.schemata WHERE schema_name = $1", createdSchema).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "ephemeral schema must be cleaned up after test")
}

func TestSharedRedis_Operations(t *testing.T) {
	ctx := context.Background()
	require.NotNil(t, testsuite.SharedRedis)

	key := "test:key:" + uuid.NewString()[:8]
	err := testsuite.SharedRedis.Set(ctx, key, "hello-redis", 1*time.Minute).Err()
	require.NoError(t, err)

	val, err := testsuite.SharedRedis.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, "hello-redis", val)
}
