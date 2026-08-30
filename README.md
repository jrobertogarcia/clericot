# Clericot

[![CI](https://github.com/jrobertogarcia/clericot/actions/workflows/ci.yml/badge.svg)](https://github.com/jrobertogarcia/clericot/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.25%2B-blue.svg)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Clericot is an enterprise Go web framework featuring PostgreSQL Row-Level Security (RLS) multi-tenancy, River transactional outbox, Watermill Redis Streams, Otter v2 / Redis two-tier caching, and Huma v2 OpenAPI 3.1 REST transport.

## Prerequisites

- Go `1.25+`
- Docker and Docker Compose
- `golangci-lint` (for static analysis)

## Quickstart

### 1. Start Infrastructure

Start local dependencies (PostgreSQL 16, Redis 7, MinIO, Mailpit):

```bash
docker compose up -d
```

### 2. Run Database Migrations

Apply pending Goose database migrations:

```bash
go run cmd/clericot/main.go migrate up
```

### 3. Start API Server

```bash
go run cmd/api/main.go
```

The API server listens on `http://localhost:8080`.

### 4. Start Background Worker

Run River outbox workers and Asynq task processors:

```bash
go run cmd/worker/main.go
```

## Local Endpoints

| Endpoint | Path | Description |
| :--- | :--- | :--- |
| Interactive API Docs | [http://localhost:8080/docs](http://localhost:8080/docs) | Huma v2 OpenAPI documentation interface |
| OpenAPI 3.1 Spec | [http://localhost:8080/openapi.json](http://localhost:8080/openapi.json) | Raw OpenAPI 3.1 JSON schema |
| Liveness Probe | [http://localhost:8080/livez](http://localhost:8080/livez) | Non-blocking shallow liveness check |
| Readiness Probe | [http://localhost:8080/readyz](http://localhost:8080/readyz) | Dependency health check (PostgreSQL, Redis) |

## Testing & Quality

Run tests with the race detector enabled:

```bash
go test -v -race ./...
```

Run static analysis:

```bash
golangci-lint run ./...
```

## Developer CLI

Clericot includes a built-in CLI for domain scaffolding and schema migrations:

```bash
# Scaffold a new domain module with canonical 6-file layout
go run cmd/clericot/main.go module create billing

# Create a new timestamped migration file
go run cmd/clericot/main.go migrate create add_invoices_table

# Apply all pending migrations
go run cmd/clericot/main.go migrate up
```

## Core Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `APP_PORT` | `8080` | HTTP API server listener port |
| `APP_ENV` | `development` | Application runtime environment (`development`, `production`, `test`) |
| `APP_LOG_LEVEL` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |
| `DB_DATABASE_URL` | `postgres://postgres:postgrespassword@localhost:5432/clericot?sslmode=disable` | Primary PostgreSQL connection string |
| `DB_ADMIN_DATABASE_URL` | `postgres://postgres:postgrespassword@localhost:5432/clericot?sslmode=disable` | Privileged migration connection string |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis instance connection string |
| `STORAGE_BUCKET_URL` | `file:///tmp/clericot-storage` | Object storage bucket URL (`s3://...`, `file://...`) |
| `AUTH_JWT_SECRET` | `dev-super-secret-jwt-signing-key-32b` | Secret key for JWT signing and verification |
| `AUTH_SESSION_SECRET` | `dev-super-secret-session-key-32b` | Secret key for session cookie encryption |
| `EVENTS_DRIVER` | `redis` | Event bus driver backend (`redis`, `nop`) |

## Documentation

For architecture standards and engineering guidelines:

- [Project Layout & Architecture](docs/architecture/project-layout.md): Directory structure, module boundaries, and dependency injection conventions.
- [Master Stack Standards](docs/architecture/stack-standards.md): Technology matrix, library choices, and architectural rationale.
- [The 15 Golden Rules](docs/guidelines/golden-rules.md): Architectural rules for multi-tenancy, transactional outbox, and resilience.
- [Distilled Lessons & Edge Cases](docs/guidelines/distilled-lessons.md): Practical operational guidance and edge-case handling.

## License

This project is licensed under the [MIT License](LICENSE).
