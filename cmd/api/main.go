package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"clericot/internal/config"
	"clericot/internal/modules/auth"
	"clericot/internal/modules/orders"
	"clericot/internal/platform/app"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/cache"
	"clericot/internal/platform/database"
	"clericot/internal/platform/router"
	"clericot/internal/platform/storage"
	"clericot/internal/platform/telemetry"
	"clericot/internal/platform/tenant"
	"clericot/sql"
)

func main() {
	// 1. Initialize Structured Logging
	logger := telemetry.InitLogger(os.Getenv("APP_LOG_LEVEL"))
	logger.Info("bootstrapping clericot api daemon")

	// 2. Load Strong-Typed Environment Config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 3. OpenTelemetry Tracer Provider
	tp, err := telemetry.InitTracer(ctx, cfg.App.Name, cfg.OTel.SamplingRatio)
	if err != nil {
		logger.Warn("failed to initialize otel tracer", "error", err)
	}

	// 4. Primary PostgreSQL Database Connection Pool
	dbPool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Error("failed to connect to database pool", "error", err)
		os.Exit(1)
	}

	// Run Embedded Goose Migrations
	database.SetMigrationsFS(sql.MigrationsFS)
	if err := database.MigrateUp(ctx, dbPool, "migrations"); err != nil {
		logger.Error("failed to execute database migrations", "error", err)
		os.Exit(1)
	}

	// 5. Redis Client
	var rdb *redis.Client
	if cfg.Redis.URL != "" {
		opt, err := redis.ParseURL(cfg.Redis.URL)
		if err == nil {
			rdb = redis.NewClient(opt)
		}
	}

	// 6. Cache & Storage Engines
	cacheEngine, err := cache.NewCacheEngine(rdb)
	if err != nil {
		logger.Warn("failed to initialize cache engine", "error", err)
	}

	var storageEngine *storage.StorageEngine
	if cfg.Storage.BucketURL != "" {
		storageEngine, err = storage.NewStorageEngine(ctx, cfg.Storage.BucketURL)
		if err != nil {
			logger.Warn("failed to initialize storage engine", "error", err)
		}
	}

	// 7. Platform Services
	txManager := database.NewTxManager(dbPool)
	tokenService := platformAuth.NewTokenService(cfg.Auth.JWTSecret, rdb)
	healthChecker := app.NewHealthChecker(dbPool, rdb)
	streamHub := app.NewStreamHub()

	// 8. Construct Chi Router & Mount Huma v2 OpenAPI Engine
	bundle := router.NewRouter(cfg, healthChecker, tokenService.HTTPMiddleware, tenant.Middleware)

	// 9. Instantiate Domain Modules via Explicit Constructor DI
	auth.NewModule(bundle.API, txManager, tokenService)
	orders.NewModule(bundle.API, txManager, nil)

	// 10. HTTP Server
	serverAddr := fmt.Sprintf(":%d", cfg.App.Port)
	srv := &http.Server{
		Addr:              serverAddr,
		Handler:           bundle.Mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// 11. Graceful Shutdown Coordinator
	coordinator := app.NewCoordinator(
		srv,
		streamHub,
		healthChecker,
		nil,
		nil,
		nil,
		tp,
		nil,
		storageEngine,
		cacheEngine,
		dbPool,
		rdb,
		logger,
	)

	// 12. Launch HTTP Listener
	go func() {
		logger.Info("http api server listening", "addr", serverAddr, "port", cfg.App.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed to listen", "error", err)
			os.Exit(1)
		}
	}()

	// 13. Wait for Termination Signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	coordinator.Shutdown(ctx)
}
