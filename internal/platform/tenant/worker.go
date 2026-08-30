package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"clericot/internal/platform/database"
	"clericot/internal/platform/storage"
)

const (
	// TypeTenantPurge represents the Asynq task type for tenant GDPR cascading purges.
	TypeTenantPurge = "tenant:purge"
)

// PurgeTenantPayload defines the JSON payload for a tenant purge background task.
type PurgeTenantPayload struct {
	TenantID string `json:"tenant_id"`
}

// PurgeTenantWorker handles two-phase cascading GDPR tenant deletion and PII scrubbing.
type PurgeTenantWorker struct {
	txManager *database.TxManager
	storage   *storage.StorageEngine
}

// NewPurgeTenantWorker constructs a new PurgeTenantWorker instance.
func NewPurgeTenantWorker(txManager *database.TxManager, storage *storage.StorageEngine) *PurgeTenantWorker {
	return &PurgeTenantWorker{
		txManager: txManager,
		storage:   storage,
	}
}

// EnqueuePurgeTenant enqueues an Asynq task to asynchronously purge and anonymize a tenant.
func EnqueuePurgeTenant(ctx context.Context, client *asynq.Client, tenantID string) error {
	if client == nil {
		return errors.New("asynq client is nil")
	}
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}

	payload, err := json.Marshal(PurgeTenantPayload{TenantID: tenantID})
	if err != nil {
		return fmt.Errorf("failed to marshal tenant purge payload: %w", err)
	}

	task := asynq.NewTask(TypeTenantPurge, payload)
	_, err = client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue tenant purge task: %w", err)
	}

	return nil
}

// ProcessTask executes the two-phase GDPR purge workflow for a tenant.
// Phase 1: In RunInTx, anonymize user records (scrub PII, hash, email) and mark tenant status deleted with deleted_at.
// Phase 2: Purge tenant-scoped blob storage under tenants/<tenant_id>/.
func (w *PurgeTenantWorker) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload PurgeTenantPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal tenant purge payload: %w", err)
	}

	if payload.TenantID == "" {
		return errors.New("tenant_id is missing from task payload")
	}

	tenantID := payload.TenantID

	// Phase 1: Database Transaction - Anonymize User Records & Mark Tenant Deleted
	now := time.Now().UTC()
	ts := pgtype.Timestamptz{Time: now, Valid: true}

	err := w.txManager.RunInTx(WithTenant(ctx, tenantID), func(txCtx context.Context) error {
		db := w.txManager.GetDB(txCtx)

		// Scrub user PII (anonymize email, name, password hash)
		scrubUsersQuery := `
			UPDATE users
			SET email = 'anonymized_' || id || '@deleted.local',
			    name = 'Anonymized User',
			    password_hash = 'REDACTED',
			    updated_at = $1
			WHERE tenant_id = $2;
		`
		if _, err := db.Exec(txCtx, scrubUsersQuery, ts, tenantID); err != nil {
			return fmt.Errorf("failed to scrub user PII: %w", err)
		}

		// Update tenant status to 'deleted' and set deleted_at timestamp
		deleteTenantQuery := `
			UPDATE tenants
			SET status = 'deleted',
			    deleted_at = $1,
			    updated_at = $1
			WHERE id = $2;
		`
		if _, err := db.Exec(txCtx, deleteTenantQuery, ts, tenantID); err != nil {
			return fmt.Errorf("failed to mark tenant deleted: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("phase 1 failed (database purge): %w", err)
	}

	// Phase 2: Storage Prefix Deletion
	if w.storage != nil {
		prefix := fmt.Sprintf("tenants/%s/", tenantID)
		if err := w.storage.DeletePrefix(ctx, prefix); err != nil {
			return fmt.Errorf("phase 2 failed (storage purge prefix %s): %w", prefix, err)
		}
	}

	return nil
}
