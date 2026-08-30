package app

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/alexliesenfeld/health"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// HealthChecker manages cached background diagnostic checks for Kubernetes /livez and /readyz probes.
type HealthChecker struct {
	checker health.Checker
	isReady atomic.Bool
}

func NewHealthChecker(dbPool *pgxpool.Pool, rdb *redis.Client) *HealthChecker {
	h := &HealthChecker{}
	h.isReady.Store(true)

	var checkOptions []health.CheckerOption

	if dbPool != nil {
		checkOptions = append(checkOptions, health.WithPeriodicCheck(10*time.Second, 1*time.Second, health.Check{
			Name: "postgres",
			Check: func(ctx context.Context) error {
				return dbPool.Ping(ctx)
			},
		}))
	}

	if rdb != nil {
		checkOptions = append(checkOptions, health.WithPeriodicCheck(10*time.Second, 1*time.Second, health.Check{
			Name: "redis",
			Check: func(ctx context.Context) error {
				return rdb.Ping(ctx).Err()
			},
		}))
	}

	h.checker = health.NewChecker(checkOptions...)
	return h
}

// SetReady sets the readiness status (used during Phase 1 of graceful shutdown).
func (h *HealthChecker) SetReady(ready bool) {
	h.isReady.Store(ready)
}

// LiveHandler serves shallow liveness probe (never fails on DB outage to prevent restart storms).
func (h *HealthChecker) LiveHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	})
}

// ReadyHandler serves deep readiness probe with cached background health evaluations.
func (h *HealthChecker) ReadyHandler() http.Handler {
	baseHandler := health.NewHandler(h.checker)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.isReady.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"DRAINING"}`))
			return
		}

		baseHandler.ServeHTTP(w, r)
	})
}
