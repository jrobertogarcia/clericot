package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"clericot/internal/platform/events"
	"clericot/internal/platform/tenant"
)

// AuditPayload captures immutable change state for regulatory compliance (SOC 2, HIPAA).
type AuditPayload struct {
	ActorID   string          `json:"actor_id"`
	TenantID  string          `json:"tenant_id"`
	Action    string          `json:"action"`   // e.g. "orders.order_created"
	Resource  string          `json:"resource"` // e.g. "orders/ord_123"
	ClientIP  string          `json:"client_ip,omitempty"`
	UserAgent string          `json:"user_agent,omitempty"`
	Diff      json.RawMessage `json:"diff,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// StageAuditLog inserts an immutable audit log entry into the River Outbox atomically inside an active transaction.
func StageAuditLog(
	ctx context.Context,
	riverClient *river.Client[pgx.Tx],
	tx pgx.Tx,
	payload AuditPayload,
) error {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	tenantID := payload.TenantID
	if tenantID == "" {
		tenantID = tenant.FromContext(ctx)
	}

	event := events.DomainEvent{
		ID:        uuid.NewString(),
		Type:      "audit.event.v1",
		Source:    "compliance.audit",
		TenantID:  tenantID,
		Data:      rawPayload,
		Timestamp: time.Now().Unix(),
	}

	if riverClient != nil && tx != nil {
		_, err = riverClient.InsertTx(ctx, tx, event, nil)
		return err
	}

	return nil
}
