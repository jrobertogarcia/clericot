package testharness

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/sqlcgen"
	"clericot/tests/testsuite"
)

// Harness provides unified fixtures and containers for testing enterprise modules.
// Deprecated: Prefer using tests/testsuite and tests/fixtures directly.
type Harness struct {
	AdminPool *pgxpool.Pool
	AppPool   *pgxpool.Pool
	TxManager *database.TxManager
	TokenSvc  *platformAuth.TokenService
	cleanupFn func()
}

// New constructs a test harness delegating to the centralized testsuite singletons.
func New(t *testing.T) *Harness {
	t.Helper()

	if testsuite.SharedAdminPool == nil || testsuite.SharedAppPool == nil {
		t.Fatal("testsuite is not initialized: ensure TestMain calls testsuite.Main(m)")
	}

	tokenSvc := platformAuth.NewTokenService("test-jwt-secret-key-32-characters-long", nil)

	return &Harness{
		AdminPool: testsuite.SharedAdminPool,
		AppPool:   testsuite.SharedAppPool,
		TxManager: testsuite.SharedTxManager,
		TokenSvc:  tokenSvc,
		cleanupFn: func() {},
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
	return testsuite.SeedTenant(ctx, name)
}

// SeedUser creates an active user under a tenant.
func (h *Harness) SeedUser(ctx context.Context, tenantID, email, name, role string) (*sqlcgen.Users, error) {
	return testsuite.SeedUser(ctx, tenantID, email, name, role)
}
