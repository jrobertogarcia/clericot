package events

import (
	"context"
	"encoding/json"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/riverqueue/river"
)

// OutboxRelayWorker consumes River jobs and forwards them to the Watermill universal event bus.
type OutboxRelayWorker struct {
	river.WorkerDefaults[DomainEvent]
	publisher message.Publisher
}

func NewOutboxRelayWorker(pub message.Publisher) *OutboxRelayWorker {
	return &OutboxRelayWorker{publisher: pub}
}

func (w *OutboxRelayWorker) Work(ctx context.Context, job *river.Job[DomainEvent]) error {
	payload, err := json.Marshal(job.Args)
	if err != nil {
		return err
	}

	msg := message.NewMessage(job.Args.ID, payload)
	msg.Metadata.Set("event_type", job.Args.Type)
	msg.Metadata.Set("source", job.Args.Source)
	if job.Args.TenantID != "" {
		msg.Metadata.Set("tenant_id", job.Args.TenantID)
	}
	if job.Args.TraceParent != "" {
		msg.Metadata.Set("traceparent", job.Args.TraceParent)
	}

	return w.publisher.Publish(job.Args.Type, msg)
}
