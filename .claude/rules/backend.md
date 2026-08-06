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

## Performance gates

Five targets, and until 2026-08-06 only two of them had a command. A target nobody measures is
not a target. Last reading is in the table — re-measure rather than trusting it.

| Metric | Target | Command | Last measured (2026-08-06) |
|---|---|---|---|
| Save API p99 | < 50ms | `just bench-http` | **1.2ms** ✅ (32KB capture payload: 4.3ms) |
| Cold start → serving | < 1s | `scripts/coldstart.sh` | **428ms** ✅ |
| List scroll API at 100k rows | < 50ms | `just bench-read` | **2.6ms** ✅ |
| Search (FTS5, 10k links) | < 30ms | `just bench-read` | **33ms** ❌ — and 335ms at 100k, i.e. linear in corpus size |
| Save → tagging complete (async) | < 3s | **still none** | never measured |

The search miss is not the coverage expression. A CPU profile under search load puts 88% inside
SQLite's VDBE with FTS5 posting-list iteration on top (`_fts5MultiIterNext` 15.7% cumulative);
`instr` + `lower` together are 8.5%. Trigram phrase matching is the cost, so the fix is a
tokenizer/index decision, not a query rewrite — and it has to be weighed against `just eval-search`,
because changing the tokenizer changes what gets found.

Per-query at 10k, showing the spread: `성능` 1.0ms (2 runes, below the trigram floor, LIKE fallback)
· `database` 2.7ms · `golang` 13.5ms · `kubernetes` 18.9ms · `데이터베이스` 24.6ms · `쿠버네티스` 31.9ms.

Note the perf gates are **local only** — CI runs none of them.

## Sources and generated output

To change the API, edit `api/openapi.yaml` (the machine source of truth) first, then regenerate `backend/internal/api/gen/` with `just gen` — keep `docs/v2/ko/06-API-SPECIFICATION.md` in sync as commentary. For the schema, `docs/v2/ko/05-DATA-SCHEMA.md` is the source — update it first, then make the code match.

- `backend/internal/api/gen/` is generated — **never edit it directly**. Even compile errors and type mismatches are fixed only by editing `api/openapi.yaml` and re-running `just gen`.
