package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/platform/app"
)

func TestHealthChecker_LiveAndReady(t *testing.T) {
	checker := app.NewHealthChecker(nil, nil)

	// 1. Live probe
	liveReq := httptest.NewRequest("GET", "/livez", nil)
	liveRec := httptest.NewRecorder()
	checker.LiveHandler().ServeHTTP(liveRec, liveReq)
	assert.Equal(t, http.StatusOK, liveRec.Code)
	assert.Contains(t, liveRec.Body.String(), "UP")

	// 2. Ready probe when ready
	readyReq := httptest.NewRequest("GET", "/readyz", nil)
	readyRec := httptest.NewRecorder()
	checker.ReadyHandler().ServeHTTP(readyRec, readyReq)
	assert.Equal(t, http.StatusOK, readyRec.Code)

	// 3. Ready probe when draining (Phase 1 shutdown)
	checker.SetReady(false)
	drainRec := httptest.NewRecorder()
	checker.ReadyHandler().ServeHTTP(drainRec, readyReq)
	assert.Equal(t, http.StatusServiceUnavailable, drainRec.Code)
	assert.Contains(t, drainRec.Body.String(), "DRAINING")
}

func TestStreamHub_Lifecycle(t *testing.T) {
	hub := app.NewStreamHub()

	var closedA, closedB atomic.Bool

	hub.Register("conn-1", func() {
		closedA.Store(true)
	})
	hub.Register("conn-2", func() {
		closedB.Store(true)
	})

	hub.Unregister("conn-2") // Normal disconnect
	hub.CloseActiveStreams() // Shutdown teardown

	assert.True(t, closedA.Load())
	assert.False(t, closedB.Load())
}

func TestCoordinator_Shutdown(t *testing.T) {
	ctx := context.Background()

	server := &http.Server{
		Addr: ":0",
	}

	streamHub := app.NewStreamHub()
	healthChecker := app.NewHealthChecker(nil, nil)

	coord := app.NewCoordinator(
		server,
		streamHub,
		healthChecker,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	// Must complete cleanly without timeout
	done := make(chan struct{})
	go func() {
		coord.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Succeeded cleanly
	case <-time.After(5 * time.Second):
		require.Fail(t, "shutdown took longer than expected")
	}
}
