package tenant

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"clericot/internal/platform/database"
	"clericot/internal/platform/storage"
)

type tenantKey struct{}

func init() {
	// Register the tenant session setter hook with the database package
	database.TenantSetter = SetTenantSession
	// Register the tenant context extractor hook with the storage package
	storage.TenantExtractor = FromContext
}

// WithTenant stores the active tenant identifier in context.
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantKey{}, tenantID)
}

// FromContext extracts the active tenant identifier from context.
func FromContext(ctx context.Context) string {
	if tid, ok := ctx.Value(tenantKey{}).(string); ok {
		return tid
	}
	return ""
}

// SetTenantSession executes parameterized set_config inside an active transaction.
func SetTenantSession(ctx context.Context, tx pgx.Tx) error {
	tenantID := FromContext(ctx)
	if tenantID == "" {
		return nil // Single-tenant, migration, or system worker context
	}
	_, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", tenantID)
	if err != nil {
		return fmt.Errorf("failed to set tenant session: %w", err)
	}
	return nil
}
