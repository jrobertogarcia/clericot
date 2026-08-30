package audit_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"clericot/internal/platform/audit"
)

func TestStageAuditLog_NilClients(t *testing.T) {
	ctx := context.Background()

	payload := audit.AuditPayload{
		ActorID:   "usr-1",
		TenantID:  "tenant-42",
		Action:    "orders.status_updated",
		Resource:  "orders/ord_999",
		ClientIP:  "127.0.0.1",
		UserAgent: "Mozilla/5.0",
		Diff:      json.RawMessage(`{"status":{"before":"PENDING","after":"COMPLETED"}}`),
		Timestamp: time.Now().UTC(),
	}

	err := audit.StageAuditLog(ctx, nil, nil, payload)
	require.NoError(t, err)
}
