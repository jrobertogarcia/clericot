package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"clericot/internal/config"
	"clericot/internal/modules/auth"
	"clericot/internal/modules/orders"
	"clericot/internal/platform/app"
	"clericot/internal/platform/database"
	"clericot/internal/platform/events"
	"clericot/internal/platform/storage"
	"clericot/internal/platform/telemetry"
	"clericot/internal/platform/tenant"
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

	// 3. Cloud Blob Storage Engine
	var storageEngine *storage.StorageEngine
	if cfg.Storage.BucketURL != "" {
		storageEngine, err = storage.NewStorageEngine(ctx, cfg.Storage.BucketURL)
		if err != nil {
			logger.Warn("failed to initialize storage engine", "error", err)
		}
	}

	// 4. Platform Services & TxManager
	txManager := database.NewTxManager(dbPool)
	tenantPurgeWorker := tenant.NewPurgeTenantWorker(txManager, storageEngine)

	// 5. Event Bus Publisher
	var publisher message.Publisher = &events.NopPublisher{}
	pub, _, err := events.NewPubSub(events.BrokerConfig{Driver: cfg.Events.Driver}, rdb, nil)
	if err == nil && pub != nil {
		publisher = pub
		defer pub.Close()
	}

	// 6. Configure River Outbox Workers
	workers := river.NewWorkers()
	river.AddWorker(workers, events.NewOutboxRelayWorker(publisher))
	river.AddWorker(workers, auth.NewUserRegisteredWorker(logger))
	river.AddWorker(workers, orders.NewOrderCreatedWorker(logger))

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

	// 7. Configure Asynq Background Task Server
	var asynqServer *asynq.Server
	if cfg.Redis.URL != "" {
		redisOpt, err := asynq.ParseRedisURI(cfg.Redis.URL)
		if err != nil {
			logger.Warn("failed to parse redis url for asynq", "error", err)
		} else {
			asynqServer = asynq.NewServer(
				redisOpt,
				asynq.Config{
					Concurrency: 10,
					Queues: map[string]int{
						"default": 10,
					},
				},
			)

			asynqMux := asynq.NewServeMux()
			asynqMux.HandleFunc(tenant.TypeTenantPurge, tenantPurgeWorker.ProcessTask)
			asynqMux.HandleFunc(auth.TypeUserWelcomeEmail, auth.ProcessWelcomeEmailTask)

			go func() {
				if err := asynqServer.Run(asynqMux); err != nil {
					logger.Error("asynq worker server encountered error", "error", err)
				}
			}()
			logger.Info("asynq task worker started listening for jobs")
		}
	}

	// 8. Graceful Shutdown Coordinator
	coordinator := app.NewCoordinator(
		nil,
		nil,
		nil,
		riverClient,
		asynqServer,
		nil,
		nil,
		nil,
		storageEngine,
		nil,
		dbPool,
		rdb,
		logger,
	)

	// 9. Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	coordinator.Shutdown(ctx)
}
