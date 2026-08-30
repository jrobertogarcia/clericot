package events

import "encoding/json"

// DomainEvent represents a standardized CloudEvent-compliant payload.
type DomainEvent struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Source      string          `json:"source"`
	TenantID    string          `json:"tenant_id,omitempty"`
	TraceParent string          `json:"traceparent,omitempty"`
	Data        json.RawMessage `json:"data"`
	Timestamp   int64           `json:"timestamp"`
}

// Kind implements river.JobArgs for River worker dispatching.
func (DomainEvent) Kind() string {
	return "domain.event.v1"
}
