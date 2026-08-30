# Distilled Engineering Lessons & Edge Cases

Key operational and architectural lessons distilled from the Clericot implementation.

---

### 1. PostgreSQL RLS & Tenancy Isolation
* **Superusers Bypass RLS:** Roles with `SUPERUSER` or `BYPASSRLS` (`postgres`) silently bypass `FORCE ROW LEVEL SECURITY`. Application connection pools and integration tests **must** connect as an unprivileged role (`app_user`).
* **Permissive vs Restrictive Policies:** Restrictive policies without at least one permissive policy default to **DENY ALL**. Always define base access as `CREATE POLICY ... FOR ALL TO app_user`.

### 2. SQL Migrations & Goose
* **Procedural SQL Blocks:** Multi-line SQL blocks with internal semicolons (`DO $$ BEGIN ... END $$;`, functions, triggers) must be bounded with `-- +goose StatementBegin` and `-- +goose StatementEnd` to prevent parser segmentation.

### 3. CI/CD & Go Toolchain
* **Linter Toolchain Matching:** In CI targeting Go 1.25+, pre-compiled `golangci-lint` binaries built on Go 1.24 fail with AST parser mismatches. Install via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8` on the runner.

### 4. High-Throughput Caching & DB Guard
* **Cache Penetration Protection:** Cache missing/empty records (`CachedEntry.IsEmpty = true`) alongside `singleflight.Group` to prevent thundering herd database crashes on non-existent keys.
* **Testcontainer Connection Flakiness:** Container log readiness does not guarantee immediate socket bind. Connection initializers (`NewPool`) should include lightweight connection retry loops (5 attempts, 500ms).

### 5. Git & PR Lifecycle Protocol
* **Strict PR Auditing:** Never replace GitHub PR creation with local non-fast-forward branch merges. Always execute the full lifecycle: `git push` $\rightarrow$ `gh pr create` $\rightarrow$ `/review-branch` $\rightarrow$ `/review-pr` $\rightarrow$ `gh pr merge`.
