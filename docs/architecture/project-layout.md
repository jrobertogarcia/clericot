# Clericot Standard Project Layout & Module Architecture (2026)

This guide specifies the project directory structure, package boundaries, module anatomy, and dependency injection conventions for the **Clericot** enterprise Go codebase.

---

## 1. Directory Tree

```text
├── cmd/
│   ├── api/             # HTTP API entrypoint (main.go)
│   ├── worker/          # Background worker entrypoint (main.go - River, Asynq, Watermill relay)
│   └── clericot/        # Clericot developer CLI tool (Cobra + Bubble Tea v2)
│       ├── commands/    # make:module, migrate:*, db:seed commands
│       └── templates/   # Embedded Go text/template files for scaffolding
├── internal/
│   ├── config/          # caarlos0/env configuration (DATABASE_URL, ADMIN_DATABASE_URL, Redis, S3)
│   ├── modules/         # Modular feature packages (Domain-Driven boundaries)
│   │   ├── auth/
│   │   │   ├── entity.go     # Pure domain business entities & domain errors
│   │   │   ├── handler.go    # Huma / Chi HTTP request/response handlers
│   │   │   ├── service.go    # Domain business logic & transaction orchestration
│   │   │   ├── repository.go # Data access layer (maps sqlc & bob to entities)
│   │   │   ├── worker.go     # River & Asynq background job handlers
│   │   │   └── module.go     # Feature constructor wiring (NewModule)
│   │   └── orders/
│   └── platform/        # Reusable Core Platform Engines (Generic / Infra)
│       ├── app/         # 5-phase graceful shutdown coordinator, StreamHub & health probes (alexliesenfeld/health)
│       ├── auth/        # Hybrid AuthPrincipal (JWT via jwx + Redis cookie via SCS) & TokenService
│       ├── audit/       # River Outbox audit.event.v1 CloudEvents schema & dispatcher
│       ├── httperr/     # RFC 9457 Problem Details error transformer (httperr.Problem)
│       ├── database/    # pgxpool, RunInTx transaction manager, Goose embedded runner
│       ├── tenant/      # PostgreSQL RLS session interceptors (set_config) & lifecycle management
│       ├── events/      # Watermill event bus, PoisonQueue DLQ & Outbox relay worker (Redis/NATS/Kafka)
│       ├── storage/     # gocloud.dev/blob cloud storage engine (S3/R2/GCS/MinIO)
│       ├── notify/      # wneessen/go-mail & multi-channel dispatcher
│       ├── cache/       # Two-tier cache (L1 otter/v2 + L2 Redis + Singleflight + CachedEntry envelope)
│       ├── security/    # Argon2id, Google Tink envelope KMS, AES-256-GCM, redis_rate GCRA limiters
│       └── telemetry/   # OpenTelemetry tracing, metrics & slog bridge
├── sql/
│   ├── migrations/      # 0001_init.sql, 0002_rls.sql (Goose migrations)
│   └── queries/         # users.sql, orders.sql (sqlc query definitions)
├── tests/
│   ├── testsuite/       # Singleton TestMain harness (Postgres/Redis/MinIO testcontainers)
│   └── fixtures/        # Deterministic generics factories (gofakeit/v7) and seeders
├── docker-compose.yml   # Local development dependencies (Postgres, Redis, Mailpit, MinIO)
├── sqlc.yaml            # sqlc compiler configuration
├── Makefile             # Developer automation (generate, migrate, test)
└── go.mod
```

---

## 2. Structural Layer Responsibilities

### 2.1 `cmd/` — Application Entrypoints
* Each executable has its own subpackage under `cmd/`.
* `cmd/api/main.go`: Loads configuration, initializes platform engines (`internal/platform`), instantiates domain modules (`internal/modules`), attaches HTTP routes, and boots the HTTP server under the `Coordinator` lifecycle manager.
* `cmd/worker/main.go`: Boots the background processing daemon, subscribing to River outbox queues and Asynq task queues.
* `cmd/clericot/main.go`: Developer CLI tool (`clericot`) for scaffolding (`make:module`), seeders (`db:seed`), and embedded migration commands (`migrate:*`).

### 2.2 `internal/platform/` — Core Infrastructure Engines
* Contains infrastructure code that is domain-agnostic and reusable across modules.
* Platform packages must **never** import domain modules (`internal/modules/*`).
* Components include the transaction coordinator (`RunInTx`), multi-tenancy RLS interceptors, cloud storage engines, two-tier caching engines, RFC 9457 error transformers, health checkers (`alexliesenfeld/health`), audit CloudEvent staging, Tink envelope KMS, stream trackers (`StreamHub`), and telemetry bridges.

### 2.3 `internal/modules/` — Domain Modules
* Each domain area (e.g. `auth`, `orders`, `billing`) is isolated inside its own module.
* High cohesion, loose coupling: Modules communicate with other modules via exported Service interfaces or via the asynchronous Event Bus.
* Avoid circular dependencies between modules.

---

## 3. Module Anatomy & File Responsibilities

| File | Purpose & Responsibilities |
| :--- | :--- |
| `entity.go` | Pure domain entities, value objects, domain error sentinels, and validation methods. Zero persistence or HTTP transport dependencies. |
| `handler.go` | HTTP/REST endpoints defined using Huma v2 operations. Handles HTTP serialization, status codes, input validation, and maps domain errors to RFC 9457 via `httperr.Transform()`. Calls domain services. |
| `service.go` | Business logic rules, multi-repository transaction orchestration via `RunInTx`, and event publishing. Never interacts directly with raw SQL or `net/http`. |
| `repository.go`| Database access layer. Executes queries generated by `sqlc` or built by `bob`. Resolves active transactions via `TxManager.GetDB(ctx)` and maps database structs into clean `entity.go` models. |
| `worker.go` | Handlers for River outbox events or Asynq background jobs relevant to this domain module. |
| `module.go` | Constructor function (`NewModule(...)`) that wires repository, service, handlers, and registers routes with Huma/Chi. |

---

## 4. Explicit Constructor Dependency Injection

To avoid reflection magic, runtime panics, and opaque dependency containers (like `uber-go/dig` or `fx`), all modules and platform engines use explicit constructor functions:

```go
package auth

import (
    "github.com/danielgtaylor/huma/v2"
    "clericot/internal/platform/database"
    "clericot/internal/platform/security"
)

type Module struct {
    Handler *Handler
    Service *Service
    Repo    *Repository
}

func NewModule(api huma.API, txManager *database.TxManager, crypto *security.CryptoEngine) *Module {
    repo := NewRepository(txManager)
    service := NewService(repo, txManager, crypto)
    handler := NewHandler(service)

    // Register routes with Huma API contract
    RegisterRoutes(api, handler)

    return &Module{
        Handler: handler,
        Service: service,
        Repo:    repo,
    }
}
```
