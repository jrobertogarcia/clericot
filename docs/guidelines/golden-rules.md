# The 15 Golden Rules of Clericot (2026)

These fifteen canonical rules guide architectural decisions, engineering workflows, and system reliability across the **Clericot** framework and applications.

---

### Rule 1: Avoid Monolithic Meta-Frameworks
Compose standard-compliant, modular packages (`chi`, `sqlc`, `slog`, `caarlos0/env`) on top of standard `net/http` to eliminate framework lock-in and preserve long-term maintainability.

### Rule 2: Contract-First APIs with RFC 9457
Use `huma/v2` and `platform/httperr` so documentation, validation, serialization, and error contracts (`application/problem+json`) are strictly derived from Go types at compile time.

### Rule 3: Guard the Primary PostgreSQL Database
Never overload PostgreSQL with per-request session writes or high-churn background tasks. Configure aggressive autovacuum parameters on outbox tables and isolate connection pools.

### Rule 4: Isolate Multi-Tenancy at the Engine Level
Enforce PostgreSQL Row-Level Security (RLS) with `FORCE ROW LEVEL SECURITY` and parameterized `SELECT set_config('app.current_tenant_id', $1, true)` scoped strictly to transactions with composite `(tenant_id, ...)` indexes.

### Rule 5: Decouple Transactions & Guarantee Safe Rollback
Use context-bound `RunInTx` coordinators with `pgx/v5` savepoint nested transactions without leaking `pgx.Tx` into domain signatures. Pass `context.WithoutCancel` to deferred rollbacks and never share transaction contexts across goroutines.

### Rule 6: Never Use `sqlc.narg` for Large-Table Search
Verify query execution plans with `EXPLAIN ANALYZE`. Use `stephenafamo/bob` for dynamic multi-predicate filters to preserve PostgreSQL index scans on large tables ($>50\text{k}$ rows), mapping output directly to pure domain entities.

### Rule 7: Tier Queues, Mandate Idempotency & Quarantine Poison Pills
Reserve `river` for ACID-critical transactional outbox jobs (with 5-minute retention); relay events to `watermill` (with `middleware.PoisonQueue` routing persistent failures to DLQ streams), and route high-throughput stateless jobs to Redis `asynq`. All event consumers must implement idempotency guards (`SET NX`).

### Rule 8: Abstract Cloud Storage with Lifecycle Controls
Use `gocloud.dev/blob` with tenant-prefixed presigned URLs, a two-step confirmation flow (`POST /confirm`), and bucket lifecycle expiration rules to prevent server bandwidth saturation and orphan storage bloat.

### Rule 9: Centralize Notifications, Audit State & Throttle Ingress
Unify notifications through Asynq, stage immutable compliance audit CloudEvents (`audit.event.v1`) atomically inside `RunInTx` via River Outbox, and enforce edge rate limiting in front of **Argon2id** password hashing.

### Rule 10: Two-Tier Caching with Short Fallback TTLs
Combine in-memory L1 (`otter/v2` with W-TinyLFU adaptive eviction and hard 2-minute fallback TTL), distributed L2 (`redis`), `singleflight.Group` thundering herd protection, and Redis Pub/Sub invalidation with versioned payloads.

### Rule 11: Standardize on Modern Cryptography & Immutable Sessions
Hash passwords with **Argon2id**, manage multi-cloud envelope encryption via **Google Tink** (`KmsEnvelopeAead`), sign tokens with **JWX** (with Redis revocation blocklists), and maintain immutable identity claims in **SCS** session cookies.

### Rule 12: Enforce Filtered Two-Layer Resilience
Combine in-process `failsafe-go` execution policies (explicitly configured to ignore HTTP 429 status codes) with edge `redis_rate` distributed rate limiters featuring fail-open in-process fallback.

### Rule 13: Synchronize Shutdown Budgets & Cache Health Probes
Execute deterministic 5-phase shutdown within a strict **25-second total budget** (beating Kubernetes 30-second `terminationGracePeriodSeconds`), tear down streaming connections, and serve asynchronous cached `/livez` and `/readyz` health probes (`alexliesenfeld/health`).

### Rule 14: Maintain Zero-State Test Isolation & Deterministic Factories
Use singleton `TestMain` ephemeral test containers, execute integration tests inside auto-rolling-back transactions (`RunTestInTx`), generate deterministic test entities via generics-first functional factories (`gofakeit/v7`), and assert business state rather than non-transactional database sequence counters.

### Rule 15: Prioritize Compile-Time Verification & Explicit Wiring
Wire services using explicit constructor functions (`NewModule(...)`), decouple domain entities from persistence structs, and provide a unified scaffolding CLI (`clericot` via `Cobra` + `Bubble Tea v2` via `charm.land/bubbletea/v2`) without magical reflection.
