---
paths:
  - "backend/**"
---

# Backend rules (Go single binary)

## Stack (fixed)

- `net/http` + chi router, `log/slog` (JSON), config via `os.Getenv` (prefix `PUSHPOINT_`). **No gin, ent, viper, or zap.**
- DB: `modernc.org/sqlite` (CGO-free). Migrations use golang-migrate + `embed.FS`, applied automatically at startup.
- Migration files that are already committed and applied are immutable — do not edit, delete, or renumber them; schema changes always mean **adding a new migration file** (editing an existing one makes golang-migrate fail startup with a dirty version).
- Concurrency utilities come from `golang.org/x/sync` (semaphore/singleflight/errgroup).
- Tests: standard `testing` + `httptest`. No testcontainers (temp-file or in-memory SQLite is enough).

## Interface contracts (internal boundaries)

- The interface definition files (store.go, queue.go) for `Store`, `Queue`, and friends are internal contracts. **When you change an interface, fix every implementation and call site in the same change set** — no commit that changes only the interface. `cd backend && go build ./...` with zero errors is the completion condition for that change.
- Put a compile-time assertion at the top of each implementation file: `var _ Store = (*sqliteStore)(nil)` — mismatches then surface as build errors at the definition site even with no call sites.
- When working in parallel, interface definition files are **owned by exactly one worker** (same principle as the source/derivative pairs in the docs rules — splitting contract and implementation across assignees creates mismatches).

## SQLite invariants

- PRAGMA: `journal_mode=WAL; synchronous=NORMAL; busy_timeout=5000; foreign_keys=ON; cache_size=-64000`.
- Connections: **one writer + a reader pool**. All writes go through a transaction.
- The save API commits `INSERT links + INSERT jobs` in one transaction and returns 201 immediately — never put scraping or tagging on the synchronous path.
- FTS5 (links_fts, trigram) is synchronized by DELETE then INSERT in the **same transaction** as the link/tag write.
- Job claim uses the atomic `UPDATE ... WHERE id = (SELECT ... LIMIT 1) RETURNING` pattern. Recover `running → pending` at startup.
- Lists and search use keyset cursor pagination. **No OFFSET.**

## Performance gates (p99 verdict from `just bench-http`, microbenchmarks from `just bench`)

| Metric | Target |
|---|---|
| Save API p99 | < 50ms |
| Save → tagging complete (async) | < 3s |
| Search (FTS5, 10k links) | < 30ms |
| List scroll API at 100k rows | < 50ms |
| Cold start → serving | < 1s |

## Sources and generated output

To change the API, edit `api/openapi.yaml` (the machine source of truth) first, then regenerate `backend/internal/api/gen/` with `just gen` — keep `docs/v2/ko/06-API-SPECIFICATION.md` in sync as commentary. For the schema, `docs/v2/ko/05-DATA-SCHEMA.md` is the source — update it first, then make the code match.

- `backend/internal/api/gen/` is generated — **never edit it directly**. Even compile errors and type mismatches are fixed only by editing `api/openapi.yaml` and re-running `just gen`.
