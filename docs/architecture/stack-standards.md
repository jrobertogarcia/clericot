# Clericot Master Stack Standards & Specifications (2026)

This document defines the core technology standards, approved packages, and architectural tiers for **Clericot** enterprise Go applications.

---

## 1. Architectural Technology Matrix

| Architectural Tier | Selected Standard | Package Path | Specification & Core Configuration |
| :--- | :--- | :--- | :--- |
| **HTTP Routing** | `Chi v5` | `github.com/go-chi/chi/v5` | Standard `net/http` router with composable middleware and sub-routing. |
| **REST & OpenAPI** | `Huma v2` | `github.com/danielgtaylor/huma/v2` | Code-first typed operations, auto OpenAPI 3.1, JSON Schema validation, and `SecuritySchemes` integration. |
| **Error Handling** | `RFC 9457` Problem Details | `internal/platform/httperr` | Unified `httperr.Problem` implementing `huma.StatusError` & `huma.ErrorDetailer` returning `application/problem+json`. |
| **Health & Probes** | `Health` (Async Caching) | `github.com/alexliesenfeld/health`<br>`internal/platform/app` | Asynchronous non-blocking `/livez` and `/readyz` health checker with cached background status evaluation preventing DB probe storms. |
| **Streaming & Push** | `StreamHub` Tracker | `internal/platform/app` | Concurrent-safe `sync.Map` stream registry implementing `StreamCloser` (WebSocket `CloseNormalClosure` 1000 & SSE `event: close`). |
| **OLTP Persistence** | `sqlc` + `pgx/v5` | `github.com/sqlc-dev/sqlc`<br>`github.com/jackc/pgx/v5` | Compile-time SQL-to-Go generation using native binary `pgxpool` connections. |
| **Dynamic Queries** | `Bob` | `github.com/stephenafamo/bob` | Dialect-specific parameterized SQL builder for multi-predicate search queries preserving index scans. |
| **Transaction Manager**| `RunInTx` Pattern | `internal/platform/database` | Context-bound transaction coordinator with native `pgx/v5` savepoint nested transactions, detached rollback context (`context.WithoutCancel`), and RLS scoping. |
| **Multi-Tenancy** | PostgreSQL RLS Engine | `internal/platform/tenant` | Context-bound PostgreSQL Row-Level Security via parameterized `SELECT set_config('app.current_tenant_id', $1, true)` and `FORCE ROW LEVEL SECURITY` with active state filtering (`status = 'active'`). |
| **Database Roles** | Dual Role Configuration | `internal/config` | Strict role separation: `DATABASE_URL` connects as unprivileged `app_user` (RLS enforced); `ADMIN_DATABASE_URL` connects as `postgres` for migrations. |
| **Schema Migrations** | `Goose v3` | `github.com/pressly/goose/v3`<br>`github.com/jackc/pgx/v5/stdlib` | Versioned SQL/Go migrations embedded in binary (`embed.FS`) executed via `stdlib.OpenDBFromPool(pool)` with Postgres advisory locking. |
| **Transactional Outbox**| `River` | `github.com/riverqueue/river` | Postgres-native transactional outbox (`river.InsertTx`) with aggressive table autovacuum tuning and 5m retention. |
| **Distributed Event Bus**| `Watermill` | `github.com/ThreeDotsLabs/watermill` | Universal Pub/Sub with Redis Streams default (`watermill-redisstream`), PEL claiming, stream trimming (`Maxlen: 100k`), `SET NX` consumer idempotency, and dead-letter queue routing (`middleware.PoisonQueue`). |
| **Compliance Audit Trail**| Outbox CloudEvents | `internal/platform/audit` | River Outbox staged `audit.event.v1` CloudEvents capturing Actor ID, Tenant, IP, and state diffs for SOC 2 / HIPAA compliance. |
| **Stateless Task Queue**| `Asynq` | `github.com/hibiken/asynq` | High-throughput Redis task queue for stateless jobs, retries, archived DLQ state, unique key tracking, and scheduled executions. |
| **Object Storage** | `Go Cloud Blob` | `gocloud.dev/blob` | Cloud-agnostic bucket abstraction (S3, R2, GCS, Azure, MinIO, Local FS) + tenant-prefixed presigned URLs and two-step confirmation. |
| **Mailer & Notifications**| `go-mail` + `notify` | `github.com/wneessen/go-mail`<br>`internal/platform/notify` | Asynq-queued notification engine for Email (SMTP/SES/Resend), SMS (Twilio), Webhooks, and Mailpit. |
| **Authentication** | Hybrid `AuthPrincipal` | `internal/platform/auth` | Unified context principal supporting Bearer JWTs (`jwx`) and Redis cookie sessions (`scs`), declaring Huma OpenAPI `SecuritySchemes`. |
| **Session Management** | `SCS v2` | `github.com/alexedwards/scs/v2`<br>`github.com/alexedwards/scs/goredisstore/v2` | Redis-backed session middleware natively using `go-redis/v9` with immutable claims and CSRF protection. |
| **Token / Crypto / JWT**| `JWX v2` | `github.com/lestrrat-go/jwx/v2` | Strict JOSE/JWT parsing, signing, verification with `jwk.Cache` auto-rotation and instant Redis JTI revocation blocklist (`SETEX auth:revoked:<jti>`). |
| **Password Hashing** | `Argon2id` (RFC 9106) | `golang.org/x/crypto/argon2` | OWASP RFC 9106 parameters ($m=64\text{MB}, t=3, p=4$) with ingress rate limiting to prevent CPU/memory exhaustion. |
| **Envelope Encryption**| `Google Tink` | `github.com/google/tink/v2/go`<br>`internal/platform/security` | Multi-cloud envelope encryption (`KmsEnvelopeAead`) for secrets and sensitive PII at rest via AWS KMS, GCP KMS, or HashiCorp Vault. |
| **Data Encryption** | `AES-256-GCM` | `crypto/cipher` | Field-level symmetric encryption for sensitive PII and confidential secrets at rest. |
| **Authorization** | `SQL RBAC` / `Cerbos`| Native SQL / `cerbos` | Domain SQL tables for standard RBAC; stateless Cerbos sidecars for dynamic ABAC. |
| **Resilience (Local)** | `Failsafe-Go` | `github.com/failsafe-go/failsafe-go` | In-process composable policies (Retry, Circuit Breaker, Timeout, Bulkhead) configured to ignore HTTP 429 rate limit statuses. |
| **Resilience (Edge)** | `Redis Rate` | `github.com/go-redis/redis_rate/v10` | Distributed GCRA token bucket rate limiter over Redis with fail-open in-process fallback. |
| **Caching Tier** | Two-Tier Hybrid Cache | `internal/platform/cache` | Typed `CachedEntry[T]` envelope + contextual `cache.TenantKey(ctx, domain, id)` namespacing + L1 Memory (`otter/v2` W-TinyLFU, 2m fallback TTL) + L2 Redis + Singleflight + Redis Pub/Sub invalidation bus. |
| **Lifecycle Manager** | Phased Coordinator | `internal/platform/app` | Deterministic 5-phase graceful shutdown with a 25-second total budget synchronized with container orchestrator grace periods. |
| **Configuration** | `Env v11` | `github.com/caarlos0/env/v11` | Direct environment variable parsing into typed structs with struct-tag validation. |
| **Logging & Tracing** | `slog` + `OTel` | `log/slog`<br>`go.opentelemetry.io/contrib/bridges/otelslog` | Stdlib structured JSON logging bridged with OpenTelemetry distributed trace contexts (5% head-based sampling). |
| **Testing Harness** | `Testcontainers` | `github.com/testcontainers/testcontainers-go` | Singleton `TestMain` Postgres/Redis/MinIO test containers + zero-state `RunTestInTx` auto-rollback runner and ephemeral test schemas. |
| **Test Data Factories**| Generics + `gofakeit` | `github.com/brianvoe/gofakeit/v7`<br>`tests/fixtures` | Generics-first functional option factories generating deterministic synthetic test entities. |
| **Developer CLI** | `clericot` (Cobra + Bubble Tea v2) | `github.com/spf13/cobra`<br>`charm.land/bubbletea/v2` | Unified `clericot` CLI for module scaffolding (`make:module`), embedded Goose migrations (`migrate:*`), seeders (`db:seed`), and `air` hot reloading. |

---

## 2. Architectural Rationale & Tier Breakdown

### 2.1 Transport, Routing & Health Contracts
* **Chi v5**: Chosen for strict compliance with standard `net/http` signatures, 100% interoperability with Go middlewares, and zero-magic sub-routing capabilities.
* **Huma v2**: Provides a type-safe contract-first layer on top of Chi. Generates OpenAPI 3.1 specifications directly from Go request/response structs, automatically enforcing JSON Schema validation, `SecuritySchemes`, and RFC 9457 Problem Details. Raw SSE and WebSockets connect directly to Chi sub-routers.
* **RFC 9457 Problem Details (`platform/httperr`)**: Maps domain sentinel errors (`domain.ErrNotFound`, `domain.ErrConflict`, `domain.ErrValidation`, etc.) via `httperr.Problem` (implementing `huma.StatusError` & `huma.ErrorDetailer`) into `application/problem+json` error responses without leaking transport dependencies into domain logic.
* **Asynchronous Health Probes (`platform/app/health`)**: Employs `alexliesenfeld/health` to expose non-blocking `/livez` (shallow liveness) and `/readyz` (deep dependency checks). Probes are evaluated asynchronously in the background and cached, protecting downstream database and cache connection pools from probe storms.
* **Streaming Connection Tracker (`platform/app.StreamHub`)**: Tracks active WebSocket and SSE client connections via a concurrent `sync.Map` implementing `StreamCloser`. Broadcasts clean close frames (`websocket.CloseNormalClosure` 1000 / SSE `event: close`) during Phase 2 of shutdown.

### 2.2 Persistence & Database Engines
* **sqlc + pgx/v5**: Generates type-safe Go code from plain SQL queries. Uses native binary PostgreSQL protocols via `pgxpool`, avoiding the reflection and ORM bloat of GORM or Ent.
* **Bob SQL Builder**: Complements `sqlc` for dynamic multi-predicate queries (e.g. faceted search with optional filters) where `sqlc.narg` would generate generic query plans that bypass PostgreSQL index scans. Repositories map both tools directly to clean domain entities.
* **Goose v3 + stdlib Bridge**: Handles versioned database migrations embedded directly into binary artifacts with `embed.FS` and executed via `stdlib.OpenDBFromPool(pool)`, supporting transactional migrations and PostgreSQL advisory locking.
* **Dual Database Role Architecture**: Isolates unprivileged `app_user` (enforcing PostgreSQL RLS) for standard API/worker connection pools (`DATABASE_URL`) from privileged migration roles (`ADMIN_DATABASE_URL`).

### 2.3 Multi-Tenancy & Transaction Management
* **Parameterized `set_config` & `FORCE RLS`**: Eliminates SQL injection and connection pool contamination by setting `SELECT set_config('app.current_tenant_id', $1, true)` scoped strictly to active transactions. `FORCE ROW LEVEL SECURITY` guarantees table owners cannot accidentally bypass tenant boundaries.
* **Tenant Lifecycle & State Filter**: Policies enforce `status = 'active'`, enabling instant workspace suspension without data modification. Automated Asynq pipelines execute GDPR data minimization and S3 tenant key purges.
* **Savepoint Pseudo-Transactions & Detached Rollback**: `TxManager.RunInTx` leverages `tx.Begin(ctx)` in `pgx/v5` to support savepoints for nested operations, allowing graceful error recovery without aborting the parent transaction. Deferred rollbacks execute with `context.WithoutCancel` (with 2s timeout) to guarantee cleanup over the wire even during client cancellations.

### 2.4 Asynchronous Processing, Messaging & Audit
* **Strict 3-Tier Queue Boundary**:
  1. **River (Tier 1):** Reserved for Postgres-native ACID transactional outbox staging (`river.InsertTx`) inside domain transactions. Maintained with table-level autovacuum parameters and short 5-minute retention.
  2. **Asynq (Tier 2):** Redis-backed queue for high-frequency stateless tasks (emails, webhooks, PDF generation, cron, GDPR erasure) to prevent PostgreSQL WAL bloat and table churn.
  3. **Watermill (Tier 3):** Universal message broker abstraction connecting the River outbox relay to Redis Streams (default) or NATS JetStream/Kafka for cross-service event distribution. Features Pending Entries List (PEL) reclamation, approximate stream trimming (`Maxlen: 100k`), mandatory consumer idempotency guards (`SET NX`), and dead-letter queue routing (`middleware.PoisonQueue`).
* **Compliance Audit Trail (`platform/audit`)**: Staging `audit.event.v1` CloudEvents inside the exact same `RunInTx` database transaction records immutable actor, tenant, IP, and state diffs for SOC 2, HIPAA, and ISO 27001 compliance.

### 2.5 Hybrid Authentication, Envelope KMS & Security
* **Hybrid `AuthPrincipal`**: Supports both Bearer JWTs (via `jwx/v2` with `jwk.Cache` auto-rotation) for mobile/CLI/external APIs and secure cookie sessions (via `SCS` + Redis) for web SPAs, mapped into a single authenticated principal in the request context and registered with Huma OpenAPI `SecuritySchemes`.
* **Instant Token Revocation**: Token verification checks Redis JTI existence (`SETEX auth:revoked:<jti> ttl 1`), enabling instant revocation of compromised tokens without sacrificing stateless performance.
* **Argon2id (RFC 9106) & Google Tink Envelope Encryption**: OWASP-recommended password hashing ($m=64\text{MB}, t=3, p=4$) protected by ingress rate limiters. Google Tink (`KmsEnvelopeAead`) provides multi-cloud envelope encryption for sensitive secrets at rest using AWS KMS, GCP KMS, or HashiCorp Vault.

### 2.6 Two-Tier Caching & Resilience
* **Hybrid Cache Engine**: Combines typed `CachedEntry[T]` envelopes (with explicit empty flags defending against cache penetration), contextual `cache.TenantKey(ctx, domain, id)` namespacing, in-memory L1 cache (`maypok86/otter/v2` with W-TinyLFU adaptive eviction and 2-minute fallback TTL) for sub-microsecond reads, Redis L2 for distributed consistency, `singleflight.Group` for thundering herd prevention, and Redis Pub/Sub for cross-pod cache invalidation.
* **Two-Layer Resilience**: In-process execution policies (`failsafe-go` configured to ignore HTTP 429 status codes) paired with edge distributed GCRA token bucket rate limiting (`redis_rate` with local fail-open fallback).

### 2.7 Observability & Testing Harness
* **slog + OpenTelemetry**: Stdlib structured logging correlated with OpenTelemetry distributed trace contexts via W3C TraceContext headers, using 5% probabilistic head-based sampling in production.
* **High-Performance Testcontainers**: Singleton `TestMain` test container instances running migrations once, with individual subtests running in sub-milliseconds via `RunTestInTx` auto-rollback transactions and ephemeral schemas for commit-dependent tests.
* **Generics Test Factories**: Type-safe functional option factories paired with `gofakeit/v7` generate deterministic synthetic test entities without reflection overhead.

