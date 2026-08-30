package tenant_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	"clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

var (
	testAdminPool *pgxpool.Pool
	testAppPool   *pgxpool.Pool
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

	code := m.Run()

	testAppPool.Close()
	testAdminPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

func TestTenantContext(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, tenant.FromContext(ctx))

	ctx = tenant.WithTenant(ctx, "tenant-123")
	assert.Equal(t, "tenant-123", tenant.FromContext(ctx))
}

func TestTenantMiddleware(t *testing.T) {
	handler := tenant.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tid := tenant.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(tid))
	}))

	// 1. Valid principal with default tenant
	req := httptest.NewRequest("GET", "/test", nil)
	principal := &auth.AuthPrincipal{
		ID:       "usr-1",
		TenantID: "tenant-default",
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "tenant-default", rec.Body.String())

	// 2. Principal switching to allowed secondary tenant via header
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-ID", "tenant-allowed")
	principalWithTenants := &auth.AuthPrincipal{
		ID:       "usr-1",
		TenantID: "tenant-default",
		Tenants:  []string{"tenant-default", "tenant-allowed"},
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), principalWithTenants))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "tenant-allowed", rec.Body.String())

	// 3. Principal attempting unauthorized tenant access
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-ID", "tenant-forbidden")
	req = req.WithContext(auth.WithPrincipal(req.Context(), principalWithTenants))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestPostgresRLS_TenantIsolation(t *testing.T) {
	ctx := context.Background()
	adminQueries := sqlcgen.New(testAdminPool)
	appTxManager := database.NewTxManager(testAppPool)

	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	// 1. Create Active Tenant A
	tenantA := "tenant-a-" + uuid.NewString()[:6]
	_, err := adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantA,
		Name:      "Org A",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	// 2. Create Active Tenant B
	tenantB := "tenant-b-" + uuid.NewString()[:6]
	_, err = adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantB,
		Name:      "Org B",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
		})
	require.NoError(t, err)

	// 3. Create Suspended Tenant C
	tenantC := "tenant-c-" + uuid.NewString()[:6]
	_, err = adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantC,
		Name:      "Org C",
		Status:    "suspended",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	// Seed user for Tenant A
	userA := "usr-a-" + uuid.NewString()[:6]
	err = appTxManager.RunInTx(tenant.WithTenant(ctx, tenantA), func(txCtx context.Context) error {
		db := appTxManager.GetDB(txCtx)
		_, err := sqlcgen.New(db).CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userA,
			TenantID:     tenantA,
			Email:        "alice@orga.com",
			Name:         "Alice OrgA",
			PasswordHash: "pw",
			Role:         "admin",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		return err
	})
	require.NoError(t, err)

	// Seed user for Tenant B
	userB := "usr-b-" + uuid.NewString()[:6]
	err = appTxManager.RunInTx(tenant.WithTenant(ctx, tenantB), func(txCtx context.Context) error {
		db := appTxManager.GetDB(txCtx)
		_, err := sqlcgen.New(db).CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userB,
			TenantID:     tenantB,
			Email:        "bob@orgb.com",
			Name:         "Bob OrgB",
			PasswordHash: "pw",
			Role:         "member",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		return err
	})
	require.NoError(t, err)

	// Verify Tenant A context only sees Alice
	err = appTxManager.RunInTx(tenant.WithTenant(ctx, tenantA), func(txCtx context.Context) error {
		db := appTxManager.GetDB(txCtx)
		var userNames []string
		rows, err := db.Query(txCtx, "SELECT name FROM users")
		require.NoError(t, err)
		defer rows.Close()

		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			userNames = append(userNames, name)
		}

		assert.Len(t, userNames, 1)
		assert.Contains(t, userNames, "Alice OrgA")
		assert.NotContains(t, userNames, "Bob OrgB")
		return nil
	})
	require.NoError(t, err)

	// Verify Tenant B context only sees Bob
	err = appTxManager.RunInTx(tenant.WithTenant(ctx, tenantB), func(txCtx context.Context) error {
		db := appTxManager.GetDB(txCtx)
		var userNames []string
		rows, err := db.Query(txCtx, "SELECT name FROM users")
		require.NoError(t, err)
		defer rows.Close()

		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			userNames = append(userNames, name)
		}

		assert.Len(t, userNames, 1)
		assert.Contains(t, userNames, "Bob OrgB")
		assert.NotContains(t, userNames, "Alice OrgA")
		return nil
	})
	require.NoError(t, err)

	// Verify Suspended Tenant C returns zero rows
	err = appTxManager.RunInTx(tenant.WithTenant(ctx, tenantC), func(txCtx context.Context) error {
		db := appTxManager.GetDB(txCtx)
		var count int
		err := db.QueryRow(txCtx, "SELECT COUNT(*) FROM users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
		return nil
	})
	require.NoError(t, err)
}
