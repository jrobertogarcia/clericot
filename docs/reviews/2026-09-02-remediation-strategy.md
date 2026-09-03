# Clericot Remediation Strategy

**Status:** proposed, awaiting the scope decision in [Move 0](#move-0-decide-the-v1-surface).
**Input:** [2026-09-01-full-project-review.md](2026-09-01-full-project-review.md) and the 13 per-slice reports in [`raw/`](raw/).

This document is the execution rationale for closing the review's findings. It does not restate them. The review is the evidence base; this is the work plan and the reasoning behind its shape.

---

## 1. The findings are three different problems, not one backlog

By the raw reports' own category tags, the 261 findings split almost evenly, and each third has different economics:

| Class | Tags | Count | Cost to close |
| :--- | :--- | --: | :--- |
| Specification divergence | DRIFT, SPEC, GAP | 78 (30%) | Mostly writing. Slice 11 supplies a per-row verdict. |
| Live or latent defects | BUG, SECURITY | 89 (34%) | Engineering, plus tests that do not exist yet. |
| Design and hygiene | DESIGN, CLARITY, PERF | 94 (36%) | Discretionary. Much of it sits in code with no internal caller. |

Treating this as a flat 261-item list is the main failure mode. A third of it is a documentation decision, and a further slice is in packages that cannot fail in production today.

---

## 2. Four root causes carry roughly half the findings

These are not themes. Each is a single decision replicated across the codebase, so each is one fix.

### A. A nil infrastructure dependency is encoded as runtime success

Same shape in ten places: `audit.StageAuditLog` returns nil with a nil client; `orders.Service` skips outbox staging when `riverClient == nil`; `TokenService.RevokeToken` and `IsTokenRevoked` report success with nil Redis; `NewSessionManager` falls back to an in-process memstore; `RateLimiter.Allow` drops both key and rate; `NotificationDispatcher.EnqueueEmail` no-ops; `PurgeTenantWorker` skips storage erasure; `NewPubSub` failure leaves `NopPublisher` in place; both mains discard `redis.ParseURL` errors.

One convention, that a missing infrastructure dependency is a construction error and never a runtime no-op, plus a `Config.Validate()`, closes parts of C1, C3, I4, I7 and a dozen Important findings.

### B. Authentication populates but never rejects

`TokenService.HTTPMiddleware` attaches a principal on success and does nothing on failure. From that single choice: C3 entirely, the `X-Tenant-ID`-trusted-when-nil half of C2, the anonymously-writable generated handler in C6, the absent `SecuritySchemes`, and every "no test for an unauthenticated request" finding.

### C. The dual-role database architecture was designed and never wired

`DatabaseConfig.AdminURL` is declared and read by nothing. From that: the superuser default DSN, the `DATABASE_URL` versus `DB_DATABASE_URL` name mismatch, `FORCE ROW LEVEL SECURITY` being inert because a superuser owns the tables, migrations running on the app pool in both binaries and in the CLI, and several documentation rows.

### D. River was never installed

Zero `rivermigrate` calls, plus a hand-rolled `river_job` in `0003`. From that: all of C1, the `nil` River client in `cmd/api`, the untestability of outbox and audit, and the absence of a durable audit table.

---

## 3. Three constraints that fix the ordering

**Existing tests assert the defects as correct.** `audit_test.go:28` requires `NoError` on the nil-client path. `TestAuthPrincipal_Methods` pins the `superadmin` wildcard as intended. `cache_test.go:73` asserts `TenantKey` returns `"global"`. `orders_test.go:27` and `api_test.go:45` both wire a nil River client. Every root-cause fix therefore breaks tests in the same change. Code and test changes cannot be sequenced separately without a broken build at every step.

**The instrument is broken, so nothing can be verified yet.** `tests/testsuite/suite.go:103` derives the `app_user` pool by `strings.Replace` on the admin DSN. A non-matching substring returns the input unchanged with no error, silently yielding a second superuser pool and turning every RLS test green for the wrong reason. Nothing asserts `current_user`. Redis has no per-test isolation, and `RunTestInSchema` never applies migrations into the schema it creates. Fixing the harness precedes the tenancy work because it is how that work would be proven.

**The schema work has hard internal ordering.** River's `002_initial_schema.up.sql` uses bare `CREATE TABLE river_job` with no `IF NOT EXISTS`, so adding `rivermigrate` against a database that already ran `0003` fails. The hand-rolled table must go first. Separately, `0002`'s `ALTER DEFAULT PRIVILEGES` runs before `0003` creates `river_job`, so River's table silently inherits `app_user` DML and needs an explicit revoke. And introducing a non-superuser owner role changes what `FORCE RLS` means, which is what makes the RLS assertions meaningful, so it lands before them.

---

## 4. This is a framework, which changes what "fixed" means

Six packages have no production caller: `platform/cache` (constructed in `cmd/api` but passed only to the shutdown coordinator), `platform/resilience`, `platform/notify`, `security/rate_limiter`, `security/tink`, and `auth.NewSessionManager`. Roughly 1,200 lines and on the order of 30 findings.

In an application these would be dead code. Here the callers are downstream adopters, so this code is the product. The framework framing raises their priority rather than lowering it: a defect in an uncalled function in an application is latent with zero blast radius, while the same defect in a framework ships to every adopting team, pre-endorsed by a stack-standards row that says it works.

**No package should be deleted.** But "ready" for a framework feature takes more than correctness:

1. Correct.
2. Reachable: a documented way to turn it on.
3. A reference consumer in-repo, which is both the copy-paste source and the only proof the API is usable.
4. Tested against real infrastructure, not a nil-dependency bypass.
5. The documented claim matches the behavior.
6. The scaffolder or the docs demonstrate the wiring.

Point 6 is specific to this project and is the one most easily missed. `clericot module create` currently emits handlers using `RunInTx` and `httperr` and nothing else: no cache usage, no rate limiting, no audit staging, no `Security` on operations, no migration carrying RLS. Even a perfect `platform/cache` would be invisible to a developer following the generated path. For this repo the scaffolder *is* the API surface, which is also why it is sequenced late: its templates should demonstrate the finished patterns rather than be written twice.

Every one of the six packages is currently at partial-1, and none has 3 or 4.

| Package | Verdict | The gap that is not a bug fix |
| :--- | :--- | :--- |
| `cache` | Keep, complete | Negative caching needs an API change (below) |
| `security/rate_limiter` | Keep, complete, urgent | Exposes no middleware at all |
| `notify` | Keep, complete | Asynq handler unregistered; no channel abstraction |
| `auth/session` | Scope decision | Cookie half of "hybrid auth" unbuilt; CSRF absent |
| `security/tink` | Keep, rename, then build KMS | Current state is actively hazardous |
| `resilience` | Keep, fix, shrink | The wrapper subtracts capability from failsafe-go |

Three of these are not what the one-line summaries suggest:

**`cache` negative caching is an API gap.** `cache.go:110` builds `CachedEntry[T]{Value: data}` and never sets `IsEmpty`, so the cache-penetration defense both architecture docs describe does not exist. It cannot be fixed by adding a line: the compute signature is `func() (T, error)`, which has no way to express "found nothing" as distinct from "errored". Closing it means changing the signature. That decision is far cheaper now, with zero callers, than after adopters have written fifty.

**`rate_limiter` is half a feature and it is blocking a live defect.** It exposes `Allow(ctx, key, rps) (bool, error)` and nothing else: no `func(http.Handler) http.Handler`, no key extractor, no 429 with `Retry-After`. Rule 9 mandates it in front of Argon2id, which runs at 64 MiB by 4 lanes on two unauthenticated endpoints. `rate_limiter.go:31` also makes the fallback one process-global bucket ignoring both `key` and `rps`, so per-key isolation collapses exactly when Redis is degraded. This is the one orphan that jumps the queue.

**`tink` is where leaving it alone is the dangerous option.** `NewTinkLocalEngine` mints a fresh in-memory AES-256-GCM keyset per call. There is no KMS client and `cfg.Auth.KMSKeyURI` has no reader, while the docs promise AWS, GCP, or Vault `KmsEnvelopeAead`. Using it as documented means every ciphertext becomes permanently undecryptable on the first pod restart. The AEAD abstraction and the AAD threading are the right foundation; the constructor name and the doc claim are not.

**`resilience` is the only place where less code is the better framework.** `WithBudget`, `WithMaxDuration`, `WithFailureRateThreshold`, and `WithFailureThresholdPeriod` all exist upstream and are unreachable through the 37-symbol surface, and failure-rate thresholding is the option that matters most for the package's stated purpose. On top of that, `resilience.go:286` builds on `failsafehttp.NewRetryPolicyBuilder()`, whose baked-in predicate retries 429, and `HandleIf` appends to the failure conditions rather than replacing them, so the project's own 429-ignoring predicate is OR'd with the library default instead of overriding it. Rule 12's central guarantee is inverted. A thinner preset layer plus access to the underlying builder is both less code and more capability.

---

## 5. Approaches considered

**Severity-first hotfix train.** Ship the six Criticals, then the ten Importants, then backlog the rest. Rejected: two Criticals cannot be verified until the harness is fixed, and several are asserted-as-correct by existing tests, so the small-and-fast framing is false. It also optimizes for reducing production exposure, and nothing is in production.

**Root-cause-first program.** Group by the four decisions in section 2. Correct as an execution engine, because it matches the actual coupling and lets each PR carry one thesis with its tests. But it reorders the work without reducing it, and would spend engineering effort on features that may not belong in v1.

**Clean-room rebuild against the review as spec.** Less absurd than it sounds: the repo is 8,650 lines and the review's "What is genuinely well built" section is a precise salvage list (`RunInTx`, the RLS policy SQL, the Bob search construction, the explicit-algorithm `jwt.Parse`, AES-256-GCM, the relay's stable-UUID choice). Rejected because the review's value is a verified map of what is broken and where; a rebuild discards that map, substitutes a fresh set of unknown bugs, re-derives the same four root causes anyway, and loses the migration and git provenance that adopters rely on.

**Scope-first, then root-cause. Selected.**

---

## 6. Decision: scope-first, then root-cause

The determining factor is that nothing is deployed. There is no incident; the CORS policy is not being exploited because nothing serves traffic. The risk actually under our control is shipping to downstream teams, which is a future event on a date we choose. That makes "reduce exposure fastest" the wrong objective and "reach a trustworthy v1 with the least total work" the right one.

Scope-first is the only approach that removes work rather than resequencing it. Of the 78 DRIFT/SPEC/GAP findings, a large share close by amending a documentation row; slice 11 alone is 23 findings, nearly all documentation. Deciding that some capabilities are roadmap rather than v1 retires a substantial block of the queue for the cost of one careful writing pass, and does so before engineering time is spent on features that turn out not to be in scope.

### Move 0: decide the v1 surface

Walk the 33-row stack-standards matrix and mark each row **Delivered**, **In scope for v1**, or **Roadmap**. Slice 11 already supplies a per-row evidence verdict, so this is a decision pass over an existing table, not an investigation. Publish the honest matrix plus a roadmap section.

This is the highest-leverage step in the program because it defines what "done" means for everything after it. For each documented capability, both "works" and "documented as roadmap" are acceptable end states. "Documented as working, half-built" is the one that is not, because that is the state that produced the KMS data-loss trap and a rate limiter that Rule 9 says protects an endpoint it has never been attached to.

### Phases

Each phase is one or two branches. Ordering is dictated by section 3, not by severity.

**Phase 0. Make the instruments trustworthy.** Small and blocking.
Harness: rebuild the app DSN by parsing rather than substring surgery, then hard-assert `current_user = 'app_user'` with `rolsuper` and `rolbypassrls` false at bootstrap; per-test Redis isolation; make `SeedTenant` and `SeedUser` honor the transaction in their context; fix or remove `RunTestInSchema`.
CI: a checked-in `.golangci.yml` (gosec, errcheck with `check-blank`, errorlint, bodyclose, gofmt), `govulncheck`, a `permissions:` block, SHA-pinned actions, Dependabot, and gofmt the three offending files.
Expect this phase to turn currently-green tests red. That is the deliverable: it converts unknown-unknowns into a visible list.

**Phase 1. Close the deployment-configuration holes.** Highest severity-to-effort ratio in the review, and almost no logic changes. Drop the `envDefault` on `JWTSecret`, `SessionSecret`, and both database URLs. Add `Config.Validate()` gating on `App.Env`. Wire `AdminURL` into every migration path, including the `migrate` subcommands. Reconcile the documented environment variable names. Bind compose ports to loopback and add `requirepass` to Redis. Broaden `.gitignore`. Remove or wire the five config fields that have no readers.

**Phase 2. Make auth enforcing (root cause B).** A rejecting `RequireAuth` middleware, registered Huma `SecuritySchemes`, and per-operation `Security`. Tenant middleware denies on a nil principal. Claim extraction fails closed instead of coercing a missing claim to `"<nil>"`. Case-insensitive Bearer. An explicit fail-open or fail-closed policy on the revocation check rather than a silent swallow. Constrain `superadmin` to a constant plus a `CHECK` constraint. Wire the rate limiter in front of the auth routes, which is what closes I7's live unauthenticated memory amplifier. Tests for unauthenticated, invalid, expired, wrong-secret, and revoked, none of which exist today.

**Phase 3. Make tenant isolation real (root cause C).** A non-superuser owner role owning the tenant-scoped tables. RLS on `tenants` plus revoking `UPDATE` and `DELETE` from `app_user`. Default privileges extended to sequences. Fail-closed symmetric `Down` migrations. Qualify the correlated `EXISTS`, and decide on hoisting the `status = 'active'` check out of the row predicate, which is a real performance fix rather than a cosmetic one. Move role creation out of the migration sequence. Tests for the `WITH CHECK` write boundary, the unset-tenant case, the `orders` and `order_items` policies, and `pg_class.relforcerowsecurity`.

**Phase 4a. Install River.** `rivermigrate` before goose; `0003` reduced to a guarded `ALTER`; the destructive `Down` replaced with `RESET`; a real River client wired in `cmd/api`; `CompletedJobRetentionPeriod` set; broker construction failure made fatal.

**Phase 4b. Build the consumer half.** Construct a `message.Router`, call `ConfigureSubscriberRouter`, reorder PoisonQueue ahead of Retry, make the idempotency marker two-phase, derive consumer groups per logical subscriber, set `DisableIndefiniteInitialBlock`, and give the DLQ its own untrimmed stream. Add a durable `audit_logs` table and a consumer that persists before the outbox row is reaped. This phase contains the one genuine design decision in the program: the compliance record currently has no durable terminus by specification, not merely by omission.

**Phase 5. Nil-dependency sweep (root cause A).** Deliberately after 2 through 4, which already touch most instances. This pass applies the convention uniformly and documents it once.

**Phase 6. The scaffolder.** Highest blast radius in the repo and deliberately last among the code phases, because its templates must emit the patterns phases 2 through 4 establish. Name validation, overwrite protection, atomic writes, principal-derived tenancy, `Security` on generated operations, loudly-failing repository stubs, a next-steps block, a companion migration carrying RLS boilerplate, and the first tests `cmd/clericot` has ever had.

**Phase 7. Feature readiness.** Bring the surviving packages from section 4 to the six-point bar. Scope is whatever Move 0 kept.

**Phase 8. Documentation truth-up.** Last, because only now is the full truth known. Slice 11 supplies a per-row fix-code-or-amend-doc verdict; adopt it as the default and escalate only disagreements.

### Parallel tracks

Independent of the four root causes and of each other. They can run concurrently in worktrees at any point: the orders integer defects (I5), the auth module TOCTOU, enumeration and timing findings (I6), and the observability wiring (I2: span exporter, `TextMapPropagator`, `otelhttp`, `otelslog`, request logging, and preserving the underlying error in `httperr.Transform`).

### Size

Roughly 20 to 22 pull requests. Worth stating up front rather than discovering at PR 15.

---

## 7. Verification discipline

The central lesson of the review is that a green suite proved nothing. So the rule for every phase is **reproduce before fixing**: write the test from the finding's stated failure scenario, watch it fail, then fix.

This is cheaper than it sounds. Nearly every finding is written as concrete inputs producing a concrete wrong outcome, and many name the file and line of the test that currently passes despite the defect. The review is already a test backlog.

Two standing rules follow from section 3: the harness fix lands first, and every root-cause fix carries its test changes in the same commit.

---

## 8. Tracking and process

### The ledger

The review states that the raw reports are "the evidence base, not a work queue." Somebody has to build the queue, and it should be a tracked artifact rather than something reconstructed per session. Maintain `2026-09-01-remediation-ledger.md` mapping each raw finding to a phase, an owning PR, and a status.

Without it the 245 findings merged into the 16 ranked items will silently drop, including real ones that appear nowhere in the ranked list by name: the correlated `EXISTS` security qual, the missing sequence grants, the single hardcoded consumer group, PoisonQueue registered inside Retry, and the `0003` `Down` that drops a table it did not create.

### Working with the agent-workflows lifecycle

The `agent-workflows` framework produced this review, and its rubric is load-bearing: review checklist item 3 (schema updates safe for concurrent old and new versions) produced slice 8's rolling-deploy RLS finding; item 4 (structured context without exposing PII) produced slice 3's worker-log email finding; item 6 (read-then-write races) produced slice 4's registration TOCTOU. The fresh-thread review boundary is why the findings carry per-item failure scenarios rather than generic advice.

Its gap for this program is that it generates findings and has no mechanism for retiring them. Every skill assumes one branch, one change, one review: `phase-breakdown` divides a single change, `review-branch` diffs one branch against its target, `check-alignment` verifies one implementation against one plan. Nothing tracks state across twenty branches, and `check-alignment` verifies that the plan was followed, never that the defect stopped reproducing.

Three adaptations for this program:

- **Add the ledger** (above). It is the missing cross-branch state.
- **Add a closure gate.** Hang the reproduce-before-fixing rule off `plan-testing` so each phase proves the finding is closed, not merely that the plan was executed.
- **Tier the ceremony.** Full lifecycle (`plan-implementation`, `delegate-plan`, `check-alignment`, `prepare-handover`, fresh-thread `review-branch`) for the four root-cause phases and the scaffolder, where a mistake propagates. Short loop (`plan-implementation`, implement, `validate-commit`, `create-pr`) for mechanical work: gofmt, `.gitignore`, compose port binding, documentation rows.

Two smaller notes. `validate-commit`'s guidance that local validation is optional when the project relies on remote CI inverts here, because CI is one of the things being fixed; local validation matters more until Phase 0 lands. And `phase-breakdown` should not be re-run for this program: the breakdown above was produced with grounding a fresh run would not match.

One suggested rubric improvement, derived from this exercise: the review guidelines have no reachability dimension, which is why roughly 30 findings on packages with no internal caller ranked alongside findings on every request path. A blast-radius axis beside severity, distinguishing Live (reachable in a shipped binary), Shipped-API (exported to adopters with no internal caller), and Internal, would separate them automatically. In an application, Shipped-API collapses to dead code; in a framework it is the product, and the rubric currently cannot express the difference.

---

## 9. Assumptions and open decisions

Stated so that a reader can tell which parts of the plan move if an answer differs.

1. **Clericot is a framework and template, not a single application.** Assumed from the documentation. If it is an application, Phase 6 drops to near-last priority and most of section 4 becomes deletion rather than completion.
2. **No deployed databases run these migrations.** Assumed. If any do, Phases 3 and 4a become forward-only `0005` and later migrations rather than edits to `0002` and `0003`, and the River table swap needs a data-preserving path.
3. **Documentation-versus-code default follows slice 11's per-row verdict:** amend the document where the code fix is a project (`jwk.Cache`, `KmsEnvelopeAead`, Bubble Tea, NATS and Kafka, Twilio), fix the code where it is cheap (config validation, MinIO in the harness, `EVENTS_DRIVER=nop`, the unregistered email handler).
4. **Open: `auth/session` plus CSRF, and `security/tink` plus KMS.** These are the two largest genuine builds in section 4 and the two where the roadmap option is most defensible. Move 0 should settle them.

---

## Appendix: claims verified directly

Checked against the working tree before this plan was written, rather than taken from the review:

| Claim | Result |
| :--- | :--- |
| `rivermigrate` is called nowhere | Zero occurrences in any `.go` file |
| No operation declares `Security`; no `SecuritySchemes` registered | Zero declarations; the only matches are three comments |
| `DatabaseConfig.AdminURL` has no readers | Declared at `internal/config/config.go:30`, zero other references |
| `CachedEntry.IsEmpty` is never set true | Read at `cache.go:79` and `:95`, assigned nowhere |
| The Watermill consumer half does not exist | `ConfigureSubscriberRouter` defined at `events/middleware.go:38` with zero callers; `message.NewRouter` zero occurrences |
| Three files are not gofmt-clean | `internal/config/config.go`, `internal/modules/auth/entity.go`, `internal/platform/events/factory.go` |
| Six packages have no production caller | `resilience`, `notify`, `security/rate_limiter`, `security/tink` imported only by their own tests; `auth.NewSessionManager` zero callers; `cache` reaches only `cmd/api` and the shutdown coordinator |
| `notify`'s Asynq handler is unregistered | `TypeEmailNotification` defined at `notify.go:13`; `cmd/worker` registers only `tenant.TypeTenantPurge` and `auth.TypeUserWelcomeEmail` |
| Seven test packages each start their own containers | Seven callers of `testsuite.Main` |
