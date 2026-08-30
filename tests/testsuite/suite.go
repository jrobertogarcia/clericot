package testsuite

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcRedis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"clericot/internal/config"
	"clericot/internal/platform/database"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

var (
	// SharedAdminPool connects as privileged postgres superuser for migrations and administrative setup.
	SharedAdminPool *pgxpool.Pool

	// SharedAppPool connects as unprivileged app_user with Row-Level Security strictly enforced.
	SharedAppPool *pgxpool.Pool

	// SharedTxManager coordinates context-bound transactions against SharedAppPool.
	SharedTxManager *database.TxManager

	// SharedRedis provides access to the singleton Redis testcontainer instance.
	SharedRedis *goredis.Client
)

// Main initializes singleton PostgreSQL and Redis testcontainers, runs migrations once,
// executes all tests in the package via m.Run(), and deterministically tears down all resources.
func Main(m *testing.M) {
	ctx := context.Background()

	var pgContainer *postgres.PostgresContainer
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		pgContainer, err = postgres.Run(ctx,
			"postgres:16-alpine",
			postgres.WithDatabase("testdb"),
			postgres.WithUsername("postgres"),
			postgres.WithPassword("postgrespassword"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(30*time.Second)),
		)
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	adminConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get postgres admin connection string: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		SharedAdminPool, err = database.NewPool(ctx, config.DatabaseConfig{
			URL:             adminConnStr,
			MaxConns:        10,
			MinConns:        2,
			MaxConnLifetime: 5 * time.Minute,
		})
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize admin pool: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	database.SetMigrationsFS(sql.MigrationsFS)
	if err := database.MigrateUp(ctx, SharedAdminPool, "migrations"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run database migrations: %v\n", err)
		SharedAdminPool.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	appConnStr := strings.Replace(adminConnStr, "postgres:postgrespassword", "app_user:app_user_password", 1)
	for attempt := 1; attempt <= 3; attempt++ {
		SharedAppPool, err = database.NewPool(ctx, config.DatabaseConfig{
			URL:             appConnStr,
			MaxConns:        10,
			MinConns:        2,
			MaxConnLifetime: 5 * time.Minute,
		})
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize app pool: %v\n", err)
		SharedAdminPool.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	SharedTxManager = database.NewTxManager(SharedAppPool)

	// Start Redis testcontainer with retry and 30s startup timeout
	var redisC *tcRedis.RedisContainer
	for attempt := 1; attempt <= 3; attempt++ {
		redisC, err = tcRedis.Run(ctx, "redis:7-alpine",
			testcontainers.WithWaitStrategy(
				wait.ForLog("* Ready to accept connections").
					WithStartupTimeout(30*time.Second)),
		)
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start redis container: %v\n", err)
		SharedAppPool.Close()
		SharedAdminPool.Close()
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	redisConnStr, err := redisC.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get redis connection string: %v\n", err)
		SharedAppPool.Close()
		SharedAdminPool.Close()
		_ = redisC.Terminate(ctx)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	redisOpts, err := goredis.ParseURL(redisConnStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse redis connection string: %v\n", err)
		SharedAppPool.Close()
		SharedAdminPool.Close()
		_ = redisC.Terminate(ctx)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	SharedRedis = goredis.NewClient(redisOpts)

	code := m.Run()

	if SharedRedis != nil {
		_ = SharedRedis.Close()
	}
	if SharedAppPool != nil {
		SharedAppPool.Close()
	}
	if SharedAdminPool != nil {
		SharedAdminPool.Close()
	}
	_ = redisC.Terminate(ctx)
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

// RunTestInTx executes the test callback inside an isolated transaction that is automatically
// rolled back upon function completion. If tenantID is non-empty, the PostgreSQL session is
// configured with the tenant identifier to enforce Row-Level Security.
func RunTestInTx(t *testing.T, tenantID string, fn func(ctx context.Context)) {
	t.Helper()
	if SharedAppPool == nil {
		t.Fatal("testsuite is not initialized: ensure TestMain calls testsuite.Main(m)")
	}

	ctx := context.Background()
	if tenantID != "" {
		ctx = tenant.WithTenant(ctx, tenantID)
	}

	tx, err := SharedAppPool.Begin(ctx)
	require.NoError(t, err, "failed to begin test transaction")

	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()

	if tenantID != "" {
		err := tenant.SetTenantSession(ctx, tx)
		require.NoError(t, err, "failed to set tenant session config")
	}

	txCtx := database.WithTx(ctx, tx)
	fn(txCtx)
}

// RunTestInSchema creates an isolated ephemeral schema for commit-dependent tests.
// The schema is dropped with CASCADE upon test completion.
func RunTestInSchema(t *testing.T, fn func(ctx context.Context, schema string)) {
	t.Helper()
	if SharedAdminPool == nil {
		t.Fatal("testsuite is not initialized: ensure TestMain calls testsuite.Main(m)")
	}

	schemaName := fmt.Sprintf("test_schema_%s", strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	ctx := context.Background()

	_, err := SharedAdminPool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s AUTHORIZATION app_user", schemaName))
	require.NoError(t, err, "failed to create test schema")

	_, err = SharedAdminPool.Exec(ctx, fmt.Sprintf("GRANT ALL ON SCHEMA %s TO app_user", schemaName))
	require.NoError(t, err, "failed to grant privileges on test schema")

	defer func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = SharedAdminPool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA %s CASCADE", schemaName))
	}()

	fn(ctx, schemaName)
}

// SeedTenant creates an active tenant in the database via the admin pool.
func SeedTenant(ctx context.Context, name string) (string, error) {
	if SharedAdminPool == nil {
		return "", fmt.Errorf("SharedAdminPool is not initialized")
	}
	tenantID := "tenant-" + uuid.NewString()[:8]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	queries := sqlcgen.New(SharedAdminPool)

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

// SeedUser creates an active user under a tenant via the admin pool.
func SeedUser(ctx context.Context, tenantID, email, name, role string) (*sqlcgen.Users, error) {
	if SharedAdminPool == nil {
		return nil, fmt.Errorf("SharedAdminPool is not initialized")
	}
	userID := "usr-" + uuid.NewString()[:8]
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	queries := sqlcgen.New(SharedAdminPool)
	user, err := queries.CreateUser(ctx, sqlcgen.CreateUserParams{
		ID:           userID,
		TenantID:     tenantID,
		Email:        email,
		PasswordHash: "hashed_test_password",
		Name:         name,
		Role:         role,
		CreatedAt:    ts,
		UpdatedAt:    ts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to seed user: %w", err)
	}
	return &user, nil
}
