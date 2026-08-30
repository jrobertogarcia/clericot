# Clericot — Enterprise Go Virtual Framework & Architecture (2026)

[![Go Version](https://img.shields.io/badge/go-1.25%2B-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Clericot** is an enterprise-grade Virtual Framework for Go. Like its namesake—a refined cocktail combining distinct, high-quality fruits and wines into a harmonious blend—Clericot composes standard-compliant, zero-magic libraries on top of Go's standard `net/http` and PostgreSQL to achieve high developer velocity, full interoperability, and compile-time safety without monolithic framework lock-in.

---

## 1. Architectural Highlights

* **API-First & Headless Scope:** High-performance, contract-first backend engine powered by `Chi v5` and `Huma v2` (OpenAPI 3.1 & RFC 9457 Problem Details via `platform/httperr`), with non-blocking asynchronous health probes (`alexliesenfeld/health`).
* **Savepoint-Backed Transaction Management (`RunInTx`):** Atomic multi-repository workflows through a context-bound transaction coordinator with native `pgx/v5` savepoint nested transactions and detached rollback context (`context.WithoutCancel`) without leaking database primitives into domain service signatures.
* **Engine-Level Multi-Tenancy with PostgreSQL RLS:** Strict data isolation at the database engine level using PostgreSQL Row-Level Security (RLS) with `FORCE ROW LEVEL SECURITY` activated via transaction-scoped parameterized configuration (`SELECT set_config('app.current_tenant_id', $1, true)`) and active state filtering (`status = 'active'`).
* **Two-Tier Persistence:** Default to `sqlc` + `pgx/v5` for static OLTP queries, complemented by `stephenafamo/bob` for complex dynamic multi-predicate queries, mapped to pure domain entities in `repository.go`.
* **Strict 3-Tier Asynchronous Architecture:** Transactional outbox staging via `river` (with aggressive autovacuum tuning and `audit.event.v1` CloudEvents), high-throughput stateless job queues via `asynq`, and universal event routing via `watermill` (with Redis Streams default, PEL auto-reclamation, `SET NX` idempotency, and `middleware.PoisonQueue` DLQ routing).
* **Multi-Cloud Envelope Encryption:** Secure confidential secrets and sensitive PII at rest using `google/tink/v2/go` (`KmsEnvelopeAead`) backed by AWS KMS, GCP Cloud KMS, or HashiCorp Vault.
* **Two-Tier Hybrid Cache:** Sub-microsecond in-memory L1 cache (`otter/v2` W-TinyLFU adaptive eviction with 2-minute fallback TTL) paired with distributed L2 (`redis`), `singleflight` thundering herd protection, versioned payload validation, and Redis Pub/Sub cross-pod invalidation.
* **Synchronized 5-Phase Graceful Shutdown:** Coordinate a zero-downtime 25-second shutdown budget guaranteed to complete cleanly within standard container orchestrator grace periods.
* **Zero-State Integration Testing & Test Factories:** Real ephemeral containers using singleton `TestMain` Testcontainers, auto-rolling-back transactional test wrappers (`RunTestInTx`), and deterministic generics test factories (`brianvoe/gofakeit/v7`).

---

## 2. Directory Structure

```text
├── cmd/
│   ├── api/             # HTTP API entrypoint (main.go)
│   ├── worker/          # Background worker entrypoint (main.go - River, Asynq, Watermill relay)
│   └── clericot/        # Clericot developer CLI tool (Cobra + Bubble Tea v2)
├── internal/
│   ├── config/          # caarlos0/env configuration
│   ├── modules/         # Modular feature packages (auth, orders)
│   └── platform/        # Reusable Core Platform Engines
│       ├── app/         # 5-phase shutdown coordinator, StreamHub & health probes
│       ├── auth/        # Hybrid AuthPrincipal (JWT + Redis cookie session)
│       ├── audit/       # River Outbox audit.event.v1 CloudEvents schema & dispatcher
│       ├── httperr/     # RFC 9457 Problem Details error transformer
│       ├── database/    # pgxpool, RunInTx transaction manager, Goose embedded runner
│       ├── tenant/      # PostgreSQL RLS session interceptors (set_config)
│       ├── events/      # Watermill event bus, PoisonQueue DLQ & Outbox relay worker
│       ├── storage/     # gocloud.dev/blob cloud storage engine
│       ├── notify/      # wneessen/go-mail & multi-channel dispatcher
│       ├── cache/       # Two-tier cache (otter/v2 + Redis + Singleflight)
│       ├── security/    # Argon2id, Google Tink envelope KMS, AES-256-GCM, redis_rate
│       └── telemetry/   # OpenTelemetry tracing, metrics & slog bridge
├── sql/
│   ├── migrations/      # Goose SQL migrations
│   └── queries/         # sqlc query definitions
├── tests/
│   ├── testsuite/       # Singleton TestMain harness (Postgres/Redis/MinIO testcontainers)
│   └── fixtures/        # Deterministic generics factories (gofakeit/v7)
├── docker-compose.yml
├── sqlc.yaml
├── Makefile
└── go.mod
```

---

## 3. Documentation

* [Stack Standards](docs/architecture/stack-standards.md) — Master dependency matrix and technical rationale.
* [Project Layout](docs/architecture/project-layout.md) — Module anatomy and dependency wiring rules.
* [Golden Rules](docs/guidelines/golden-rules.md) — The 15 canonical engineering principles.

---

## 4. License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.
