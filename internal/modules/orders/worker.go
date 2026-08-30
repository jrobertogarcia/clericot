package orders

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"
)

// OrderCreatedJobArgs defines the River job arguments for when an order is created.
type OrderCreatedJobArgs struct {
	OrderID    string `json:"order_id"`
	TenantID   string `json:"tenant_id"`
	UserID     string `json:"user_id"`
	TotalCents int64  `json:"total_cents"`
}

// Kind implements river.JobArgs.
func (OrderCreatedJobArgs) Kind() string {
	return "orders.order_created.v1"
}

// OrderCreatedWorker processes order creation River background events.
type OrderCreatedWorker struct {
	river.WorkerDefaults[OrderCreatedJobArgs]
	logger *slog.Logger
}

// NewOrderCreatedWorker creates a new OrderCreatedWorker instance.
func NewOrderCreatedWorker(logger *slog.Logger) *OrderCreatedWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &OrderCreatedWorker{logger: logger}
}

// Work executes background processing for newly created orders.
func (w *OrderCreatedWorker) Work(ctx context.Context, job *river.Job[OrderCreatedJobArgs]) error {
	w.logger.Info("processing order created background event",
		"order_id", job.Args.OrderID,
		"tenant_id", job.Args.TenantID,
		"user_id", job.Args.UserID,
		"total_cents", job.Args.TotalCents,
	)
	return nil
}
