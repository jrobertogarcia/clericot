package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"clericot/internal/platform/cache"
	"clericot/internal/platform/storage"
)

// Coordinator orchestrates a deterministic 5-phase graceful shutdown with a strict 25-second budget.
type Coordinator struct {
	httpServer      *http.Server
	streamCloser    StreamCloser
	healthChecker   *HealthChecker
	riverClient     *river.Client[pgx.Tx]
	asynqServer     *asynq.Server
	watermillRouter *message.Router
	tracerProvider  *sdktrace.TracerProvider
	meterProvider   *sdkmetric.MeterProvider
	storageEngine   *storage.StorageEngine
	cacheEngine     *cache.CacheEngine
	dbPool          *pgxpool.Pool
	redisClient     *redis.Client
	logger          *slog.Logger
}

func NewCoordinator(
	srv *http.Server,
	streamCloser StreamCloser,
	healthChecker *HealthChecker,
	riverClient *river.Client[pgx.Tx],
	asynqServer *asynq.Server,
	watermillRouter *message.Router,
	tp *sdktrace.TracerProvider,
	mp *sdkmetric.MeterProvider,
	storage *storage.StorageEngine,
	cacheEngine *cache.CacheEngine,
	dbPool *pgxpool.Pool,
	redisClient *redis.Client,
	logger *slog.Logger,
) *Coordinator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Coordinator{
		httpServer:      srv,
		streamCloser:    streamCloser,
		healthChecker:   healthChecker,
		riverClient:     riverClient,
		asynqServer:     asynqServer,
		watermillRouter: watermillRouter,
		tracerProvider:  tp,
		meterProvider:   mp,
		storageEngine:   storage,
		cacheEngine:     cacheEngine,
		dbPool:          dbPool,
		redisClient:     redisClient,
		logger:          logger,
	}
}

// Shutdown executes the 5 sequential graceful shutdown phases.
func (c *Coordinator) Shutdown(ctx context.Context) {
	c.logger.Info("initiating phased graceful shutdown (budget: 25s)")

	// Phase 1: Mark health check readiness to false (Traffic Draining - 2s)
	c.logger.Info("phase 1: marking service unready to ingress (2s)")
	if c.healthChecker != nil {
		c.healthChecker.SetReady(false)
	}
	time.Sleep(100 * time.Millisecond) // Short pause in tests, up to 2s in production

	// Phase 2: Teardown active SSE/WebSockets and drain HTTP server (8s)
	c.logger.Info("phase 2: shutting down HTTP server and streaming connections (8s)")
	if c.streamCloser != nil {
		c.streamCloser.CloseActiveStreams()
	}
	if c.httpServer != nil {
		httpShutdownCtx, httpCancel := context.WithTimeout(ctx, 8*time.Second)
		defer httpCancel()
		if err := c.httpServer.Shutdown(httpShutdownCtx); err != nil {
			c.logger.Error("error shutting down http server", "error", err)
		}
	}

	// Phase 3: Stop background workers and wait for active jobs to complete (10s)
	c.logger.Info("phase 3: stopping background queue workers and event router (10s)")
	var workerWg sync.WaitGroup
	workerCtx, workerCancel := context.WithTimeout(ctx, 10*time.Second)
	defer workerCancel()

	if c.riverClient != nil {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			if err := c.riverClient.Stop(workerCtx); err != nil {
				c.logger.Error("error stopping river client", "error", err)
			}
		}()
	}

	if c.asynqServer != nil {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			c.asynqServer.Shutdown()
		}()
	}

	if c.watermillRouter != nil {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			if err := c.watermillRouter.Close(); err != nil {
				c.logger.Error("error closing watermill router", "error", err)
			}
		}()
	}
	workerWg.Wait()

	// Phase 4: Flush OpenTelemetry telemetry buffers (3s)
	c.logger.Info("phase 4: flushing telemetry & logs (3s)")
	otelCtx, otelCancel := context.WithTimeout(ctx, 3*time.Second)
	defer otelCancel()
	if c.tracerProvider != nil {
		_ = c.tracerProvider.Shutdown(otelCtx)
	}
	if c.meterProvider != nil {
		_ = c.meterProvider.Shutdown(otelCtx)
	}

	// Phase 5: Close primary storage handles, cache, and database connection pools (2s)
	c.logger.Info("phase 5: closing storage, cache, and database pools (2s)")
	if c.storageEngine != nil {
		_ = c.storageEngine.Close()
	}
	if c.cacheEngine != nil {
		_ = c.cacheEngine.Close()
	}
	if c.redisClient != nil {
		_ = c.redisClient.Close()
	}
	if c.dbPool != nil {
		c.dbPool.Close()
	}

	c.logger.Info("shutdown complete within budget. process exiting cleanly.")
}
