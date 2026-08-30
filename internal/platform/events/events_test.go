package events_test

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/platform/events"
)

type MockPublisher struct {
	PublishedEvents []*message.Message
}

func (m *MockPublisher) Publish(topic string, messages ...*message.Message) error {
	m.PublishedEvents = append(m.PublishedEvents, messages...)
	return nil
}

func (m *MockPublisher) Close() error {
	return nil
}

func TestDomainEvent_Serialization(t *testing.T) {
	event := events.DomainEvent{
		ID:        uuid.NewString(),
		Type:      "orders.created.v1",
		Source:    "orders.service",
		TenantID:  "tenant-123",
		Data:      json.RawMessage(`{"order_id":"ord_456"}`),
		Timestamp: time.Now().Unix(),
	}

	assert.Equal(t, "domain.event.v1", event.Kind())

	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	var decoded events.DomainEvent
	err = json.Unmarshal(encoded, &decoded)
	require.NoError(t, err)
	assert.Equal(t, event.ID, decoded.ID)
	assert.Equal(t, event.Type, decoded.Type)
}

func TestOutboxRelayWorker_Work(t *testing.T) {
	ctx := context.Background()
	mockPub := &MockPublisher{}
	worker := events.NewOutboxRelayWorker(mockPub)

	event := events.DomainEvent{
		ID:          "evt-123",
		Type:        "payments.settled.v1",
		Source:      "billing",
		TenantID:    "tenant-org",
		TraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		Data:        json.RawMessage(`{"amount":5000}`),
		Timestamp:   time.Now().Unix(),
	}

	job := &river.Job[events.DomainEvent]{
		Args: event,
	}

	err := worker.Work(ctx, job)
	require.NoError(t, err)
	require.Len(t, mockPub.PublishedEvents, 1)

	published := mockPub.PublishedEvents[0]
	assert.Equal(t, "evt-123", published.UUID)
	assert.Equal(t, "payments.settled.v1", published.Metadata.Get("event_type"))
	assert.Equal(t, "tenant-org", published.Metadata.Get("tenant_id"))
	assert.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", published.Metadata.Get("traceparent"))
}

func TestIdempotencyMiddleware_NoRedis(t *testing.T) {
	// Nil redis should pass through cleanly
	mw := events.IdempotencyMiddleware(nil, 1*time.Minute)

	var executionCount atomic.Int64
	handler := mw(func(msg *message.Message) ([]*message.Message, error) {
		executionCount.Add(1)
		return nil, nil
	})

	msg := message.NewMessage("test-uuid", []byte("payload"))
	_, err := handler(msg)
	require.NoError(t, err)
	assert.Equal(t, int64(1), executionCount.Load())
}
