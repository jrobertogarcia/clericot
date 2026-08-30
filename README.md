# Clericot: Production-Grade Go Enterprise Framework

[![CI Pipeline](https://github.com/jrobertogarcia/clericot/actions/workflows/ci.yml/badge.svg)](https://github.com/jrobertogarcia/clericot/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jrobertogarcia/clericot)](https://goreportcard.com/report/github.com/jrobertogarcia/clericot)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Clericot** is an enterprise-grade Go web framework and architecture template engineered with modern 2026 patterns: PostgreSQL Row-Level Security (RLS) multi-tenancy, River transactional outbox, Watermill Redis Streams event bus, Otter v2 L1/L2 caching, Google Tink KMS envelope encryption, Huma v2 OpenAPI 3.1 type-safe REST transport, and a deterministic 5-phase graceful shutdown lifecycle.

---

## 🏛️ Architecture Overview

```mermaid
graph TD
    Client[HTTP Client / Frontend] -->|REST / OpenAPI 3.1| Chi[Chi v5 Router]
    Chi -->|Type Validation & Routing| Huma[Huma v2 Engine]
    
    subgraph "Application Core (Domain Layer)"
        Huma --> AuthModule[Auth & User Module]
        Huma --> OrderModule[Orders Reference Module]
    end

    subgraph "Platform Foundation Tier"
        AuthModule --> TxManager[TxManager (Savepoints & Hooks)]
        OrderModule --> TxManager
        TxManager -->|Injects app.current_tenant_id| Postgres[(PostgreSQL 16 + RLS)]
        OrderModule -->|Atomically Enqueues| RiverOutbox[(River Outbox `river_job`)]
        AuthModule --> TokenService[JWX JOSE / JWT]
        TokenService -->|Revocation Check| Redis[(Redis 7 Cluster)]
        
        CacheEngine[Otter v2 L1 + Redis L2 Cache] --> Redis
        StorageEngine[GoCloud Blob Storage] --> MinIO[(S3 / GCS / MinIO)]
    end

    subgraph "Asynchronous Background Tier"
        RiverWorker[River Outbox Worker Daemon] -->|Dequeues Jobs| RiverOutbox
        RiverWorker -->|Publishes CloudEvents| Watermill[Watermill Redis Streams Broker]
        Watermill -->|At-Most-Once Idempotent Handlers| EventHandlers[Domain Consumers / Webhooks]
        AsynqWorker[Asynq Distributed Worker] -->|Background Tasks| Mailer[Email / SMS / Push]
    end
```

---

## 🚀 Key Features & Capabilities

- **PostgreSQL Native RLS Multi-Tenancy**: Kernel-level tenant data isolation enforced via `FORCE ROW LEVEL SECURITY` and transaction-scoped `set_config('app.current_tenant_id', ...)` hooks.
- **Transactional Outbox Engine**: Zero dual-write hazard. Domain events and compliance audit trails are committed atomically to PostgreSQL via River and relayed asynchronously.
- **Universal Event Broker**: Powered by Watermill with Redis Streams, stream auto-trimming (100k cap), PEL consumer group crash recovery, and poison queue dead-lettering.
- **L1/L2 High-Throughput Caching**: In-memory `otter/v2` (W-TinyLFU eviction) coupled with Redis L2, `singleflight` stampede suppression, and cross-node Pub/Sub invalidations.
- **Type-Safe REST Transport**: Chi v5 with Huma v2 generating live OpenAPI 3.1 specifications (`/openapi.json`) and interactive documentation (`/docs`).
- **5-Phase Deterministic Shutdown Coordinator**: Strictly budget-managed 25-second teardown cycle (Readiness Draining $\rightarrow$ Ingress/SSE Shutdown $\rightarrow$ Worker Drain $\rightarrow$ Telemetry Flush $\rightarrow$ Resource Closure).
- **Multi-Cloud Envelope Encryption**: Google Tink AEAD key management for sensitive PII and field-level encryption.
- **Developer Tooling & CLI**: Built-in scaffolding tool `clericot` for domain modules and Goose migrations.

---

## 🛠️ Getting Started

### Prerequisites
- Go `1.25+`
- Docker & Docker Compose
- `sqlc` & `golangci-lint`

### 1. Start Infrastructure
```bash
docker compose up -d
```

### 2. Run Database Migrations
```bash
go run cmd/clericot/main.go migrate up
```

### 3. Start API Server Daemon
```bash
go run cmd/api/main.go
```
The server will start on `http://localhost:8080`.
- Interactive API Docs: `http://localhost:8080/docs`
- OpenAPI Specification: `http://localhost:8080/openapi.json`
- Liveness Probe: `http://localhost:8080/livez`
- Readiness Probe: `http://localhost:8080/readyz`

### 4. Start Background Worker Daemon
```bash
go run cmd/worker/main.go
```

---

## 🧪 Testing

Run all unit and Testcontainers integration tests:
```bash
go test -v ./...
```

Run linter:
```bash
golangci-lint run ./...
```

---

## 📦 Developer CLI Scaffolding

Scaffold a new enterprise domain module (creates models, service, handlers, and sqlc queries):
```bash
go run cmd/clericot/main.go module create billing
```

Create a new timestamped migration:
```bash
go run cmd/clericot/main.go migrate create add_invoices_table
```

---

## 📜 License

This project is licensed under the [MIT License](LICENSE).

