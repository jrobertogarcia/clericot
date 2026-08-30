package app

import "sync"

// StreamCloser is the interface for tearing down streaming connections.
type StreamCloser interface {
	CloseActiveStreams()
}

// StreamHub tracks active WebSocket and SSE client connections with concurrent-safe lifecycle management.
type StreamHub struct {
	connections sync.Map // map[string]func()
}

func NewStreamHub() *StreamHub {
	return &StreamHub{}
}

// Register registers an active streaming connection and its closer function.
func (h *StreamHub) Register(id string, closeFn func()) {
	h.connections.Store(id, closeFn)
}

// Unregister removes a connection from tracking upon normal completion.
func (h *StreamHub) Unregister(id string) {
	h.connections.Delete(id)
}

// CloseActiveStreams broadcasts clean close frames to all active streaming connections.
func (h *StreamHub) CloseActiveStreams() {
	h.connections.Range(func(key, value any) bool {
		if closeFn, ok := value.(func()); ok {
			closeFn() // Sends WebSocket CloseNormalClosure (1000) or SSE event: close
		}
		h.connections.Delete(key)
		return true
	})
}
