package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"clericot/internal/config"
	"clericot/internal/platform/database"
	"clericot/internal/platform/events"
	"clericot/internal/platform/telemetry"
	"clericot/sql"
)

func main() {
	logger := telemetry.InitLogger(os.Getenv("APP_LOG_LEVEL"))
	logger.Info("bootstrapping clericot worker daemon")

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Primary PostgreSQL Database Connection Pool
	dbPool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		logger.Error("failed to connect to database pool", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// Run Embedded Goose Migrations
	database.SetMigrationsFS(sql.MigrationsFS)
	if err := database.MigrateUp(ctx, dbPool, "migrations"); err != nil {
		logger.Error("failed to execute database migrations", "error", err)
		os.Exit(1)
	}

	// 2. Redis Client
	var rdb *redis.Client
	if cfg.Redis.URL != "" {
		opt, err := redis.ParseURL(cfg.Redis.URL)
		if err == nil {
			rdb = redis.NewClient(opt)
			defer rdb.Close()
		}
	}

	// 3. Event Bus Publisher
	var publisher message.Publisher = &events.NopPublisher{}
	pub, _, err := events.NewPubSub(events.BrokerConfig{Driver: cfg.Events.Driver}, rdb, nil)
	if err == nil && pub != nil {
		publisher = pub
		defer pub.Close()
	}

	// 4. Configure River Workers
	workers := river.NewWorkers()
	river.AddWorker(workers, events.NewOutboxRelayWorker(publisher))

	riverClient, err := river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Workers: workers,
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
		},
	})
	if err != nil {
		logger.Error("failed to create river client", "error", err)
		os.Exit(1)
	}

	if err := riverClient.Start(ctx); err != nil {
		logger.Error("failed to start river client", "error", err)
		os.Exit(1)
	}
	logger.Info("river outbox worker started listening for jobs")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down worker daemon")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()

	if err := riverClient.Stop(stopCtx); err != nil {
		logger.Error("error stopping river worker", "error", err)
	}
	logger.Info("worker daemon exited cleanly")
}
