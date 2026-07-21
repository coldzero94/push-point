# Push-Point

[![ci](https://github.com/coldzero94/push-point/actions/workflows/ci.yml/badge.svg)](https://github.com/coldzero94/push-point/actions/workflows/ci.yml)

A personal link archive that auto-tags everything you save — shipped as a single Go binary.

## Why

Product before infrastructure. Instead of bolting autoscaling onto a service with zero users, Push-Point gets its performance from the design quality of a single process that starts with one `just dev`. Auto-tagging is solved without any external LLM API — a lightweight NLU pipeline (rule-based + ONNX embeddings) is the technical differentiator of this project. The v1 Kubernetes setup is preserved in `deploy/k8s-future/`, ready to return if real users ever demand it.

## Status

**M1 (core) is implemented** — a single Go binary with:

- Link save / list / detail / tags / search / stats API
- SQLite (WAL) + FTS5 trigram full-text search
- Durable in-process job queue: atomic claim, crash recovery
- Contract-first codegen: `api/openapi.yaml` → oapi-codegen v2.8.0 strict-server

The scraper lands in M2 and NLU tagging in M3, so saved links currently stay in `pending` status. Roadmap:

| Milestone | Scope |
|---|---|
| M2 Scraper | Worker pool + site-adapter parsing + thumbnails + retry/crash recovery + bookmark/Takeout import — daily real use begins |
| M3 Tagging A + Search | Rule-based tagger (Korean normalization) + tag dictionary + FTS5/LIKE search + eval harness |
| M4 iOS | SwiftUI Share Extension with local queue + list view + Tailscale on-device use |
| M5 Tagging B | Go tokenizer + ONNX embedding bake-off + ensemble + tag feedback loop |
| M6 Polish | iOS widget + performance tuning + public technical write-up |

## Quick start

Requirements: Go 1.25+, [just](https://just.systems) (`brew install just`).

```bash
just dev
# cd backend && PUSHPOINT_API_KEY=dev-key go run ./cmd/pushpoint
# cold start < 1s, serving at http://localhost:8420
```

Save a link:

```bash
curl -X POST http://localhost:8420/api/v1/links \
  -H "Authorization: Bearer dev-key" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article", "note": "read later"}'
# 201 {"id": 1, "status": "pending", "created_at": 1784937600}
# scraping/tagging run asynchronously once M2/M3 land; for now links stay "pending"
```

List links:

```bash
curl "http://localhost:8420/api/v1/links?limit=20" \
  -H "Authorization: Bearer dev-key"
# keyset cursor pagination — pass the response's next_cursor as ?cursor=
```

Search:

```bash
curl "http://localhost:8420/api/v1/search?q=kubernetes" \
  -H "Authorization: Bearer dev-key"
# FTS5 trigram full-text search (query >= 3 chars), bm25 ranking
```

## justfile recipes

The task runner is [just](https://just.systems) — 14 recipes, all Go recipes run inside `backend/`.

| Recipe | Description |
|---|---|
| `just` | List recipes (default) |
| `just dev` | Run the local dev server (`PUSHPOINT_API_KEY=dev-key go run ./cmd/pushpoint`) |
| `just build` | `go build -o bin/pushpoint ./cmd/pushpoint` |
| `just gen` | Generate `backend/internal/api/gen/` from `api/openapi.yaml` (oapi-codegen v2.8.0 pinned, output is committed) |
| `just gen-check` | Drift guard — fails if `git diff` remains after regeneration (runs in CI) |
| `just enum-lint` | Check `openapi.yaml` enums ↔ migration CHECK constraints match (exit 1 on mismatch) |
| `just test` | `go test ./...` |
| `just bench` | Microbenchmarks: `go test -bench=. -benchmem ./...` (p99 verdict belongs to bench-http) |
| `just bench-http` | Save API HTTP-path p99 gate — exit 1 if p99 >= 50ms |
| `just test-crash` | Crash recovery check — save → kill -9 → restart → assert all jobs done (M2+) |
| `just seed 100000` | Generate a mixed Korean/English seed DB for benchmarks (fixed seed, default n=10000) |
| `just eval` | Tagging accuracy on the golden set — top-3 Recall vs baseline (M3+) |
| `just lint` | `golangci-lint run` |
| `just fmt` | `gofmt` / `goimports` |

## Architecture

```
┌─────────────────────────────────────────────────┐
│            push-point (single binary)           │
│                                                 │
│  HTTP API ──▶ enqueue ──▶ jobs table (SQLite)   │
│     │                          │                │
│     │                    dispatcher (goroutine) │
│     │                          │                │
│     │              ┌───────────┴──────────┐     │
│     │              ▼                      ▼     │
│     │         scraper pool           tagger     │
│     │         (bounded N)        (NLU pipeline) │
│     │              │                      │     │
│     ▼              ▼                      ▼     │
│  SQLite (WAL) ◀── links / tags / FTS5 ◀──┘     │
│  data/thumbs/ ◀── thumbnails                    │
└─────────────────────────────────────────────────┘
        ▲                          ▲
   iOS Share Ext          iOS app (list/search)
```

Core flow:

1. The save API commits `INSERT links` + `INSERT jobs(scrape)` in one transaction and returns 201 immediately (p99 < 50ms).
2. The dispatcher goroutine atomically claims jobs from the jobs table and hands them to workers — no job is lost across restarts.
3. The scraper pool (M2) parses metadata, then chains a tag job (plus a thumb job when og:image exists) in the success transaction.
4. The tagger (M3) classifies against a controlled tag dictionary and marks the link `done` — save to tags-complete within 3s.

## Performance

Measured on Apple Silicon (2026-07-20) via `just bench-http` and `scripts/coldstart.sh`. Per-milestone verification matrix: [docs/v2/08-DEVELOPMENT-PLAN.md](docs/v2/08-DEVELOPMENT-PLAN.md).

| Metric | Target | Measured |
|---|---|---|
| Save API p99 | < 50ms | p50 0.244ms / p95 0.35ms / p99 0.981ms |
| Save → tags complete (async) | < 3s | — (M3) |
| Search (FTS5, 10k links) | < 30ms | — |
| List scroll API at 100k links | < 50ms | — |
| Cold start (exec → serving) | < 1s | 314–684ms |

## Project structure

```
push-point/
├── api/                       # API contract (machine source of truth)
│   ├── openapi.yaml           # OpenAPI 3.1 — backend and clients generate from here
│   └── README.md
├── backend/                   # Go single binary (API + worker + NLU runtime inference)
│   ├── cmd/pushpoint/main.go  # single entry point
│   ├── internal/
│   │   ├── api/               # HTTP handlers (chi)
│   │   │   └── gen/           # oapi-codegen output (just gen, committed)
│   │   ├── store/             # Store interface + sqlite implementation
│   │   ├── queue/             # Queue interface + sqlite jobs implementation
│   │   ├── scraper/           # fetch + goquery parsing, singleflight
│   │   ├── tagger/            # Tagger interface + rules / onnx implementations
│   │   └── thumbs/            # thumbnail generation and storage
│   ├── migrations/            # SQLite migrations (golang-migrate, embedded)
│   └── go.mod                 # module github.com/coby/push-point/backend
├── nlu/                       # NLU offline assets (not runtime code)
│   ├── dictionary/            # tag dictionary definitions and seeds (committed)
│   ├── golden/                # tagging-quality golden set (JSONL, committed)
│   └── models/                # M5: ONNX conversion scripts (Python) + model artifacts
├── ios/                       # M4: SwiftUI app + Share Extension
├── frontend/                  # web front end — explicit non-goal (revisit after M6), placeholder only
├── docs/
│   ├── README.md              # v1 ↔ v2 doc index and comparison
│   ├── v1/                    # v1 planning archive (do not modify)
│   └── v2/                    # current docs (single source of truth)
├── deploy/k8s-future/         # v1 k8s manifests preserved (unused)
├── CLAUDE.md
└── justfile                   # 14 task-runner recipes — dev / test / bench / eval etc.
```

Workspaces: `api/` (contract source), `backend/` (all runtime code), `nlu/` (offline assets only — backend reads artifacts), `ios/` (M4), `frontend/` (reserved, explicit non-goal).

## Development workflow

Direct pushes to `main` are blocked by the GitHub ruleset `main-protection` (PR required, `ci` status check required, force-push and deletion blocked). The flow:

1. Branch → commit → push → open a PR.
2. CI must pass: gofmt check, build, `go vet`, race-enabled tests, `just gen-check` (contract drift), enum-lint (`openapi.yaml` ↔ migration CHECK constraints).
3. Code review gate: every implementation change is reviewed via Claude Code before commit; high/medium findings are fixed or explicitly waived with a reason.
4. Merge. If CI breaks on `main`, fixing it takes priority over everything else.

## Docs

Current documentation lives in `docs/v2/` — the single source of truth, written in Korean (internal docs stay Korean; this README is the English public entry point). `docs/v1/` is the archived v1 plan; [docs/README.md](docs/README.md) compares v1 ↔ v2. Start with [docs/v2/00-README.md](docs/v2/00-README.md) for the table of contents, then 01 (overview) through 08 (milestones M1–M6 and completion criteria) and 09 (plan review).

## About the v1 infra

v1 ran PostgreSQL, Redis Streams, and MinIO on Minikube with HPA scaling. At zero users that setup was reverse-engineering — all local-testing friction, no benefit — so v2 switched to a single binary on SQLite (WAL) + an in-process worker pool + local disk. The k8s manifests were not deleted: they are preserved in `deploy/k8s-future/`. This is folding, not discarding — Store/Queue/Tagger sit behind interfaces, so when real users make a distributed setup necessary, only the implementations get swapped.
