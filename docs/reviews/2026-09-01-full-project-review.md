# Clericot — Full Project Review

**Verdict: REQUEST CHANGES**

Scope: entire repository at `main` (780d5f1). 13 review agents across 12 code slices plus a documentation-conformance sweep, applying the `/review-branch` rubric. 261 raw findings consolidated to 16 ranked issues. Critical findings were spot-verified against source; the most severe was verified by execution.

Raw per-slice agent reports are preserved verbatim in [`raw/`](raw/).

---

## How to read this

Two agents used "CRITICAL" on different axes: live exploitable defects (slices 1–8, 10, 11) versus "a critical bug could ship undetected here" (slice 9b, test quality). These are normalized below. A coverage gap is rated on the severity of what it fails to catch, and appears as *why the defect survived* rather than as a separate finding.

Severity follows the review rubric: **C** blocks release (severe bug, resource leak, architectural degradation), **I** blocks release (violates core design principles), **S** non-blocking improvement.

---

## Ranked Findings

### C1. The transactional outbox and audit tier is non-functional in both binaries

**Verified by execution.** Applied the repo's four migrations to a clean Postgres 16 and booted a real River v0.30 client:

```
STEP client-construct: OK
STEP InsertTx FAILED: ERROR: column "unique_key" of relation "river_job" does not exist (SQLSTATE 42703)
STEP Start   FAILED: ERROR: relation "river_queue" does not exist (SQLSTATE 42P01)
```

- `sql/migrations/0003_river_autovacuum.sql:3` hand-rolls `river_job` to attach autovacuum settings. It omits `unique_key`, `unique_states`, `attempted_by`, types `state` as `TEXT` instead of River's enum, and creates none of `river_migration`, `river_leader`, `river_queue`, `river_client`.
- `rivermigrate` is called nowhere in the repo.
- **`cmd/worker/main.go:103`** calls `riverClient.Start` then `os.Exit(1)` on error. The worker daemon cannot boot against any database this repo provisions.
- **`cmd/api/main.go:98`** passes `nil` as the River client. `orders/service.go:117` guards on `s.riverClient != nil` and `audit.StageAuditLog` (`audit/audit.go:54`) returns `nil` — success — when the client is nil. Every order commits with no outbox event and no audit record, returning 201.
- The fix is blocked too: River's `002_initial_schema.up.sql:11` uses bare `CREATE TABLE river_job(` with no `IF NOT EXISTS`, so adding `rivermigrate` later fails with "relation already exists".

So the API silently drops every event and audit record, and the worker cannot start. Rules 7 and 9 are unsatisfiable as the code stands.

**Compounding:** the consumer half of the event bus does not exist. `cmd/worker/main.go:80` discards the Watermill subscriber (`pub, _, err :=`), no `message.Router` is constructed anywhere, and `events.ConfigureSubscriberRouter` — the sole wiring point for `SET NX` idempotency, retry, and `PoisonQueue` DLQ routing — has zero callers. The system publishes to Redis Streams that nothing reads, trimmed at `Maxlen: 100_000`. Rule 7's idempotency mandate is vacuously true.

**Also:** broker construction failure is swallowed (`cmd/worker/main.go:79`, `if err == nil && pub != nil`), silently leaving `NopPublisher` in place; `CompletedJobRetentionPeriod` is never set, so River's 24h default applies instead of Rule 7's mandated 5 minutes; and there is no durable `audit_log` table anywhere — the compliance record's only home is a River row scheduled for deletion.

*Why tests missed it:* no test constructs a `river.Client`. `orders_test.go:27` and `api_test.go:45` both wire `nil`, so the suite exercises the degraded path exclusively. `audit_test.go:28` asserts `require.NoError` on `StageAuditLog(ctx, nil, nil, payload)` — codifying the silent drop as the contract.

---

### C2. Tenant isolation is not enforced in any shipped configuration

Five independent mechanisms each defeat it:

1. **Default DSN is a superuser.** `internal/config/config.go:29` defaults `DB_DATABASE_URL` to `postgres://postgres:postgrespassword@...`. Superusers hold `BYPASSRLS`, so every policy in `0002_rls.sql` and `0004_orders.sql` — all scoped `TO app_user` — is inert. `distilled-lessons.md:8` documents this exact trap.
2. **The documented env var does not exist.** `config.go:13` sets `envPrefix:"DB_"`, so the real name is `DB_DATABASE_URL`. Both architecture docs say `DATABASE_URL`. There is no `required` tag and no validation, so setting the documented name silently falls back to the superuser default.
3. **`FORCE ROW LEVEL SECURITY` is inert as deployed.** `FORCE` exists to subject the *table owner* to RLS. The migration never creates a non-superuser owner, so whichever role runs goose owns the tables — and both DSNs default to `postgres`. The rule is satisfied textually, not substantively.
4. **The `tenants` table has no RLS at all**, while `0002_rls.sql:13` grants `app_user` blanket DML on every table in `public`. `sql/queries/tenants.sql` ships `GetTenantByID`, `UpdateTenantStatus`, and `CreateTenant` as unguarded generated methods. `DELETE FROM tenants WHERE id='victim'` cascades to that tenant's users, orders, and order items.
5. **`X-Tenant-ID` is trusted from unauthenticated requests.** `tenant/middleware.go:19` reads `if principal != nil && !principal.HasTenantAccess(headerTenant)` — a nil principal short-circuits the ACL check and the header becomes the RLS scope. Reachable on every route because `token.go:130` never rejects.

**Architectural note:** `app.current_tenant_id` is an unreserved GUC that any role can set, and `0002_rls.sql:6` hardcodes `CREATE ROLE app_user WITH LOGIN PASSWORD 'app_user_password'` in a committed migration. Anyone who reaches the Postgres port with that published credential can `set_config` to any tenant and read everything. RLS does not resist this — it authenticates the session variable, and the session variable is attacker-controlled once the role is.

*Why tests missed it:* the harness builds a genuine `app_user` pool (`suite.go:103`) and proves the SQL policies are correct. It never exercises the pool-construction path the binaries use.

---

### C3. Authentication can be bypassed, and the OpenAPI contract says every endpoint is public

- **`internal/config/config.go:50`** ships a working HS256 signing key as an `envDefault`: `dev-super-secret-jwt-signing-key-32b`. A deploy that omits `AUTH_JWT_SECRET` boots healthy and signs with a secret published in this repository. Anyone can mint a token with `role: "superadmin"` for any tenant. `principal.go:23` grants that role blanket cross-tenant access.
- **No operation anywhere declares `Security`, and no `SecuritySchemes` are registered.** Verified by grep. Generated clients send no `Authorization` header; gateway policies derived from the spec classify every route as public.
- **`token.go:125`** attaches a principal on success and does nothing on failure. Invalid, expired, and revoked tokens are indistinguishable from anonymous. The only gate is a hand-written `PrincipalFromContext(ctx) == nil` check inside each service.
- **`auth/service.go:47`** takes `tenant_id` from the body of the unauthenticated `POST /v1/auth/register` and uses it verbatim as the RLS scope. Knowing a tenant ID — which appears in every JWT and in `/v1/auth/me` responses — is sufficient to join that tenant as a member.
- **`token.go:103`** returns `nil` from `RevokeToken` when Redis is nil, reporting success while revoking nothing. `cmd/api/main.go:66` leaves `rdb` nil on any `redis.ParseURL` error, which it discards.
- **`token.go:80`** discards the `ok` flag from `tok.Get(...)` and applies `fmt.Sprint` to the result, so a missing claim becomes the four-character string `"<nil>"` rather than an error.

---

### C4. Path traversal in storage lets one tenant overwrite another's objects

`storage.go:49` builds the key as `path.Join("tenants", tenantID, "uploads", fmt.Sprintf("%s-%s", uuid, filename))` with no sanitization of the client-supplied `filename`. `path.Join` cleans rather than rejects. **Reproduced directly:**

```
../../../../evil.pdf                            -> tenants/evil.pdf
../../../../../tenants/victim/uploads/pwned.pdf -> tenants/victim/uploads/pwned.pdf
../../../../../etc/x                            -> etc/x
```

The escaped key is passed to `bucket.SignedURL(ctx, key, PUT)`, so the caller receives a signed URL authorizing that write.

Compounding: `TenantExtractor` is consulted **only** in `PresignedUpload` (`storage.go:44`). `PresignedDownload`, `ConfirmUpload`, `Read`, `Write`, `Delete`, and `DeletePrefix` all accept arbitrary caller-supplied keys with no tenant check, so the two-step confirm flow provides no authorization and cross-tenant read is a matter of submitting the right key.

---

### C5. CORS reflects any origin while allowing credentials

`router.go:32` sets `AllowedOrigins: []string{"https://*", "http://*"}` with `AllowCredentials: true`. In go-chi/cors v1.2.2, `allowedOriginsAll` is set only for the literal `"*"`; a wildcard *pattern* takes the branch that echoes the request's `Origin` verbatim alongside `Access-Control-Allow-Credentials: true`. Every origin matches. `X-CSRF-Token` is in `AllowedHeaders`, so an attacker page can read and replay a CSRF token.

Currently blunted only because `session.go:20` pins `SameSite=Strict` — a mitigation the router knows nothing about, and one that evaporates the moment a downstream app sets `SameSite=None` for the cross-origin SPA this policy exists to serve.

---

### C6. The `clericot` scaffolder replicates a cross-tenant write primitive and destroys existing modules

Highest blast radius in the repo: every defect here is inherited by every generated module.

- **`main.go:327`** — the generated handler reads `tenantID := input.Body.TenantID` from the untrusted request body and threads it to `RunInTx`, with no principal check and no `Security` on the operation. The hand-written `orders` module it is modeled on does the opposite (`orders/service.go:61` derives tenant solely from the principal). Every scaffolded module is cross-tenant writable by an anonymous caller.
- **`main.go:105`** — `scaffoldModule` does `os.MkdirAll` then six unconditional `os.WriteFile` calls. Verified: no `os.Stat`, no force flag, no existence check. `clericot module create orders` silently overwrites the ~700-line hand-written orders module; `orders_test.go` survives and stops compiling, so the failure presents as an opaque build error.
- **`main.go:104`** — `strings.ToUpper(name[:1])` under `cobra.ExactArgs(1)` panics on empty input, and there is no identifier or keyword validation: `module create user-profile` emits `package user-profile`, which is a syntax error.
- **`main.go:177`** — generated `Repository.Create` acquires the DB handle, discards it with `_ = db`, and returns the input entity with timestamps set. A smoke test returns 200 with a populated body and nothing is persisted.
- Generated River and Asynq handlers are never registered in `cmd/worker/main.go`, and `NewModule` is never called in `cmd/api`. The CLI reports unqualified success.

`cmd/clericot` has no test files.

---

### I1. Graceful shutdown does not honor its own budget

- **`coordinator.go:84`** — Phase 1 sleeps a hardcoded `100 * time.Millisecond` with the comment "Short pause in tests, up to 2s in production". There is no such branch. The listener closes ~100ms after readiness flips, while kube-proxy endpoint propagation typically takes 1–5s, producing connection-refused bursts on every rolling deploy.
- **`coordinator.go:92,102,136`** — every phase derives its deadline from the caller's context. The idiomatic `signal.NotifyContext` pattern passes an already-Done context on SIGTERM, collapsing all phases to zero: in-flight requests force-closed, River jobs abandoned, telemetry unflushed — then `"shutdown complete within budget"` is logged. `cmd/api` escapes only by accident of style.
- Phase 3's 10s budget applies only to River; `asynq.Shutdown()` and `watermillRouter.Close()` take no context and default to 8s and **30s** respectively, and `workerWg.Wait()` is unbounded. Phase 5 has no timeout at all, and `pgxpool.Close()` blocks until every checked-out connection returns.

Rule 13's 25-second total is a comment, never computed or enforced.

### I2. Observability is decorative

`telemetry.go:31` builds a `TracerProvider` with a sampler and resource but **no span processor or exporter** — every span is recorded and discarded. `otel.SetTextMapPropagator` is never called, so W3C `traceparent` is neither extracted nor injected and this service silently terminates every upstream caller's distributed trace. No `otelhttp` middleware, so no server spans exist to export. No `otelslog` bridge, so logs carry no trace correlation. `cfg.OTel.Endpoint` has no reader. Phase 4 of shutdown spends 3 seconds flushing an exporter that does not exist.

There is also no request or error logging anywhere: `httperr.Transform` (`problem.go:110`) discards the underlying error entirely — not logged, not stored, no `Unwrap` — so a 500 leaves no server-side record of its cause.

### I3. Rule 12's central guarantee is inverted: the retry policy retries 429

`resilience.go:286` builds on `failsafehttp.NewRetryPolicyBuilder()`, whose baked-in predicate returns true for `StatusTooManyRequests`. `HandleIf` **appends** to the failure conditions (`AppliesToAny`), so the project's own `HTTPRetryPredicate` returning false for 429 cannot suppress the library default — they are OR'd. A throttled upstream receives 4× the load. If it sends `Retry-After: 300`, failsafe's `DelayFunc` overrides the configured 2s cap and parks the goroutine for five minutes per attempt.

The tests appear to cover this but exercise only the *circuit breaker* path; no test sends a 429 through a retry policy.

Also in this package: discarded retry attempts' response bodies are never drained or closed, pinning a connection per abandoned attempt (`NewHTTPClient` sets no `Timeout` and uses `DefaultTransport`, which caps nothing); `ErrOpen` is classified as retryable, so an open breaker costs ~700ms of sleep per request instead of failing fast.

### I4. Cache: three correctness defects and an inert defense

- **`cache.go:57`** — `TenantKey` substitutes the literal `"global"` when no tenant is in context, silently merging every tenant-less caller into one shared namespace. Worker contexts carry the tenant in the job payload, not the context. `cache_test.go:73` asserts this behavior as correct.
- **`cache.go:110`** — `IsEmpty` is read at two sites and **assigned `true` nowhere**. Verified by grep. The cache-penetration defense that both architecture docs describe does not exist, and `singleflight` collapses only same-key concurrency, so an enumeration scan of distinct missing IDs reaches the database unimpeded.
- **`cache.go:104`** — the singleflight closure captures the first caller's context; that caller disconnecting fails all waiters and suppresses the L2 write.
- **`cache.go:144`** — a failed `DEL` returns before publishing, so peers never hear the invalidation and serve stale data for the full TTL while the originating pod looks correct.

The whole engine has no production consumer — it is constructed in `cmd/api` and passed only to the shutdown coordinator.

### I5. Orders: integer defects corrupt persisted totals

- **`service.go:54`** — `totalCents += int64(it.Quantity) * it.UnitPriceCents` with no overflow guard, and `handler.go:17-18` declares `minimum` but no `maximum`. A crafted quantity/price pair wraps to a negative total that passes every check and persists (the column has no `CHECK`).
- **`repository.go:73`** — `int32(item.Quantity)` narrows silently while the header total is computed from the untruncated `int`, so `sum(items)` and `orders.total_cents` diverge permanently and the transaction commits cleanly.
- `CancelOrder` has no state machine — a `completed` or `paid` order can be cancelled — and its read-then-write has no lock, so concurrent cancels both succeed.
- `Limit`/`Offset` declare no `maximum`; the repository's default of 50 applies only when the value is non-positive, never as a ceiling.

### I6. Auth module: TOCTOU, enumeration, and a timing oracle

- `service.go:48` — the read-then-write duplicate check is not the real guard; the unique constraint is, and `repository.go:47` returns the raw `23505` unwrapped, so a concurrent double-submit returns 500 instead of 409.
- `service.go:105` — login returns immediately when the user is absent, skipping ~100ms of Argon2 work. A two-order-of-magnitude timing difference makes account existence trivially probeable, with no rate limiting in front of it.
- Email is never normalized; `Jane.Doe@x.com` and `jane.doe@x.com` become two accounts and lock the user out of one.
- Unknown or suspended tenants surface as 500s; `ErrTenantNotFound` is declared and mapped but returned by no code path.

### I7. Argon2id and rate limiting: an unauthenticated memory amplifier

`DefaultArgon2Params` is 64 MiB × t=3 × p=4 — correct per RFC 9106. It runs on unauthenticated `/v1/auth/register` and `/v1/auth/login` with **no limiter in front of it**: `security.NewRateLimiter` has zero production callers, and `register` hashes *before* validating the tenant. 200 concurrent requests allocate ~12.8 GB. Rules 9 and 12 both mandate the missing wiring.

`VerifyPassword` (`argon2id.go:75`) also feeds cost parameters decoded from the stored hash straight into `argon2.IDKey` with no bounds check — a hostile or truncated hash can request 1 GiB or panic on a zero key length.

### I8. RFC 9457 is claimed but never emitted

`httperr.Problem` implements `Error()` and `GetStatus()` only. Huma selects the problem media type solely via `ContentTypeFilter` (`huma.go:661`), which `Problem` does not implement — so domain errors ship as `application/json`. Huma's *own* validation errors use `ErrorModel`, which does implement it. The same endpoint therefore returns two incompatible error contracts, and since `huma.NewError` is never overridden, the published OpenAPI schema documents the one domain handlers never use.

### I9. The test suite proves substantially less than its green status suggests

- **`suite.go:244,266`** — `SeedTenant`/`SeedUser` accept a `ctx` but execute on `SharedAdminPool`, so seeded rows commit and `RunTestInTx` cannot roll them back. The signature implies participation in the test transaction; the body ignores it.
- **`suite.go:103`** — the `app_user` pool is derived by `strings.Replace` on the admin DSN. A non-matching substring returns the input unchanged with no error, silently yielding a second superuser pool and turning every RLS test green for the wrong reason. Nothing asserts `current_user`.
- Redis has no per-test isolation — no `FLUSHDB`, no key namespacing, no per-test DB index — so revocation blocklists, idempotency guards, and rate-limiter counters leak across tests.
- **`RunTestInSchema` is documented as the escape hatch for commit-dependent tests but never applies migrations into the schema it creates**, so any real use fails with "relation does not exist".
- Every `TokenService` in the suite is built with `nil` Redis; `IdempotencyMiddleware` is tested only on its nil-Redis bypass; the rate limiter test makes one call against a 100-token burst; `notify_test.go` and `audit_test.go` assert that nil-dependency no-ops return success, **codifying the defects above as the contract** so any fix breaks a test.
- The RLS test asserts reads only — the `WITH CHECK` write boundary and the unset-tenant case are never exercised.

### I10. Documentation systematically over-promises

The conformance sweep found **3 of 33 technology-matrix rows DELIVERED as described**; the rest are PARTIAL or ABSENT. Documented and entirely absent: Bubble Tea v2, `air`, `db:seed`, `make:module`/`migrate:*` command names, Cerbos, MinIO in the harness, NATS/Kafka drivers, SMS/Twilio and webhook channels, `otelslog`, `jwk.Cache` rotation, CSRF protection, `KmsEnvelopeAead`, S3/GCS/Azure blob drivers, migration advisory locking, `EXPLAIN ANALYZE` tooling, `cmd/clericot/{commands,templates}/`.

Two that matter most:

- **`project-layout.md:106`** — the canonical DI example does not compile: `security.CryptoEngine` does not exist and `RegisterRoutes` is called in package form where both real modules use the method form. It is the block a developer copies first.
- **`stack-standards.md:22` + `:24`** are mutually exclusive: 5-minute River retention deletes the audit record that the same document designates as the SOC 2 / HIPAA trail, and the relay's only sink is a Redis stream trimmed at 100k. The compliance record has no durable terminus by specification, not just by omission.

Full rule-by-rule and row-by-row verdicts: [`raw/slice-11-cross-cutting-conformance.md`](raw/slice-11-cross-cutting-conformance.md).

### S1. `internal/platform/resilience` is 453 LoC of speculative generality

~40 exported symbols, zero production callers, and 15 of 23 functional options never called even by its own tests. It is a 1:1 pass-through over failsafe-go's builders that *removes* capability — `WithBudget`, `WithMaxDuration`, `WithFailureRateThreshold`, and `WithFailureThresholdPeriod` all exist upstream and are unreachable through the wrapper, and failure-rate thresholding is the option that actually matters for the package's stated purpose. Three exported config structs are unreachable API: no exported function accepts one. An equivalent surface is ~90 LoC.

### S2. Assorted

Three files are not gofmt-clean (`config.go`, `auth/entity.go`, `events/factory.go`) and CI has no format gate. No `.golangci.yml`, so lint runs defaults — `gosec` would flag the committed credentials, `errcheck` with `check-blank` would flag `_ = audit.StageAuditLog`. No `govulncheck`, no Dependabot, no coverage gate, no `permissions:` block in CI, and third-party actions pinned to mutable tags. `docker-compose.yml` publishes Postgres, Redis (no `requirepass`), and MinIO on `0.0.0.0` with default credentials. `make clean` destroys the developer's database; `migrate-up` is in `.PHONY` with no rule.

---

## What is genuinely well built

Worth preserving explicitly, because the defect list is long and these are the parts that are right:

- **`RunInTx`** is the strongest code in the repo. Savepoint nesting via `parentTx.Begin`, detached rollback under `context.WithoutCancel` with a bounded 2s timeout applied to both root and nested paths, driven by an explicit `committed` flag rather than error-sniffing. The `DBTX` interface keeps `pgx.Tx` out of repository signatures while `GetTx` provides the narrow escape hatch River needs. Connection-leak-on-cancellation is a defect most codebases ship.
- **The RLS policy SQL itself is correct** — a permissive `FOR ALL` base policy with both `USING` and `WITH CHECK`, parameterized transaction-local `set_config`, and a predicate that fails closed when the GUC is unset. The `DO $$` block is correctly bounded by `StatementBegin/End`. The problems are in the deployment role and the missing `tenants` policy, not the policy logic.
- **The Bob dynamic search** is genuinely injection-free: every value flows through `psql.Arg`, every identifier is a compile-time literal, and the mandatory tenant predicate is applied before any optional filter, so no filter combination can drop tenant scoping.
- **Argon2 placement** — hashing runs deliberately *outside* the transaction on register and *after* commit on login, keeping ~100ms of 64 MB work off a pooled connection. Non-obvious and correct.
- **`jwt.Parse` with an explicit `jwt.WithKey(jwa.HS256, ...)`** builds the verifier from the option rather than the token header, making `alg: none` and algorithm-confusion attacks structurally impossible.
- **AES-256-GCM** is textbook: fresh 12-byte nonce per call from `crypto/rand`, prepended via `gcm.Seal(nonce, nonce, ...)`, key-size check before `NewCipher`, length check before slicing.
- **`relay.go:27`** uses the stable `job.Args.ID` as the Watermill message UUID rather than generating a fresh one, so River-side retries republish an identical dedup key. Whoever wrote this understood the outbox contract.
- **Shutdown phase *ordering*** is correct — streams before HTTP (hijacked connections never go idle), workers after HTTP, telemetry before pools, DB last. The ordering is right; only the budgets are unenforced.
- **`resilience_test.go`** is the model the rest of the suite should follow: a 12-case predicate table plus a real `httptest.Server` proving ten 429s leave the breaker closed while three 500s open it, with request counts proving the fourth never reached the server.

---

## Method and accounting

**Verification.** Two findings were confirmed by execution rather than reading: the River schema incompatibility (booted a real client against a container provisioned by the repo's migrations) and the storage path traversal (ran `path.Join` against the actual key-construction expression). All other critical findings were checked against source by direct grep or read. No finding is reported on an agent's word alone.

**Independence.** Agents were deliberately not told what had already been found. Four prior observations were independently rediscovered — the nil River client, the unwired subscriber router, the absent Bubble Tea/Cerbos/`db:seed`, and the hand-rolled `river_job` table — reaching them from the Go side and the SQL side separately. Findings that would otherwise have been missed: the path traversal, the 429 retry inversion, the inert `IsEmpty`, the `TenantKey` "global" collision, `FORCE RLS` being inert under a superuser owner, and the `DATABASE_URL` / `DB_DATABASE_URL` mismatch.

**Compression.** 261 raw findings → 16 ranked issues. The largest merges: the outbox tier (11 findings across 5 slices → C1), tenant isolation (9 across 5 → C2), and doc drift (33 rows and 15 rules → I10). Nothing was dropped for being duplicative; duplicates across slices raised confidence rather than count.

**Severity normalization.** Slice 9b's seven CRITICALs were coverage gaps, not live defects; they are folded into I9 and into the *why tests missed it* notes, rated on the severity of what they fail to catch. Two agents rated the nil-River-client issue CRITICAL and one IMPORTANT; it is merged into C1 at CRITICAL on the strength of the execution evidence.

**Verdict.** Per the rubric, any Critical or Important issue mandates REQUEST CHANGES. There are 6 Critical and 10 Important.

---

## Slice inventory

| Slice | Scope | Findings |
| :-- | :-- | --: |
| 1 | Transaction & tenancy core | 16 |
| 2 | Auth & crypto | 17 |
| 3 | Async & outbox | 25 |
| 4 | Domain: auth module | 19 |
| 5 | Domain: orders module | 18 |
| 6 | Transport & lifecycle | 24 |
| 7a | Cache & storage | 18 |
| 7b | Resilience, notify & telemetry | 22 |
| 8 | SQL & schema | 20 |
| 9a | Test infrastructure | 13 |
| 9b | Test suite quality | 26 |
| 10 | Tooling & supply chain | 20 |
| 11 | Cross-cutting conformance | 23 |
| | **Total** | **261** |

Raw reports carry duplicates across slices and un-normalized severities by design; they are the evidence base, not a work queue.
