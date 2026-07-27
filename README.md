# Push-Point

[![ci](https://github.com/coldzero94/push-point/actions/workflows/ci.yml/badge.svg)](https://github.com/coldzero94/push-point/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![SQLite](https://img.shields.io/badge/SQLite-CGO--free-003B57?logo=sqlite&logoColor=white)](https://modernc.org/sqlite)
[![no LLM API](https://img.shields.io/badge/tagging-no%20LLM%20API-2ea44f)](nlu/golden/README.md)

**Save a link in one tap. Find it again months later.** A self-hosted personal archive that reads what you save and tags it — one Go binary, one SQLite file, no external AI API and no per-item cost.

[Docs](docs/v2/00-README.md) · [API contract](api/openapi.yaml) · [Tagging evaluation](nlu/golden/README.md) · [Roadmap](docs/v2/08-DEVELOPMENT-PLAN.md) · [Contributing](CONTRIBUTING.md)

---

## Why

Read-it-later apps forget things for you. The link goes in, and finding it later means remembering it existed.

Push-Point tags every save so the archive stays searchable — and it does that **without sending anything to an LLM**. Not because an API call is hard, but because a personal archive shouldn't have a per-item cost, a rate limit, an outage, or a third party reading it. Tagging is a rule engine over a controlled dictionary, and its quality is measured rather than asserted.

Everything runs in one process on your own machine. A backup is `cp -r data/`.

## What it does

```
┌── iOS Share Sheet ──┐
│   Web URL bar       │──▶  POST /api/v1/links  ──▶  201 in under 1 ms
└── Browser extension ┘                                    │
                                                           ▼
                                       SQLite job queue (survives kill -9)
                                                           │
                            ┌──────────────┬───────────────┴──────────────┐
                            ▼              ▼                              ▼
                        scrape          tag (rules, local)            thumbnail
                     og / adapters      42-tag dictionary            640px JPEG
                            │              │                              │
                            └──────────────┴───────────────┬──────────────┘
                                                           ▼
                                            FTS5 trigram search · tag filter
```

## Measured, not asserted

Every number below has a command that reproduces it. No figure enters this README without one.

| | Target | Measured (2026-07-27) | Command |
|---|---|---|---|
| Save API p99 | < 50 ms | **1.22 ms** (p50 0.27 / p95 0.36, n=2000) | `just bench-http` |
| Save p99, client-capture path | < 50 ms | **4.41 ms** (n=500) | `just bench-http` |
| Cold start → serving | < 1 s | **405 ms** | `scripts/coldstart.sh` |
| Tagging, frozen test set | — | **0.905** top-3 recall (84 links) | `just eval` |
| Tagging, open-web set | — | **0.821** (28 links) | `just eval` |

That last row is the one that matters. A tagger evaluated only on developer blogs scores well and tells you nothing, so there is a second set built deliberately from the rest of the web — commerce, communities, app stores, video, wikis. It scores lower, on purpose: **it is the number that is allowed to be disappointing.**

## Features

- **Instant save** — `POST /api/v1/links` commits two INSERTs and returns 201. Scraping, tagging and thumbnails run in a SQLite-backed queue that recovers from `kill -9`; nothing slow ever sits on the request path.
- **Auto-tagging without an LLM** — a rule engine over a 42-tag controlled dictionary. Korean is handled by NFC normalization, particle stripping, and word-boundary matching at *both* ends of a compound, because Korean compounds are head-final and `대박식당` has to reach `식당`. Ties break on evidence volume rather than alphabetical order.
- **Search that works in Korean** — FTS5 trigram with bm25 ranking, no morphological analyzer required. Queries under 3 characters fall back to LIKE instead of returning nothing.
- **Three clients, one contract** — a React SPA, a SwiftUI app with a Share Extension, and a browser extension. All three generate their types from `api/openapi.yaml`; CI blocks drift in every one.
- **Pages a server can't fetch** — bot walls and login walls are captured from the browser that is already rendering them, not by pretending to be a browser. A blocked link fails honestly with an actionable message instead of storing the wall's text as content.
- **Single binary** — Go with a CGO-free SQLite driver. Migrations are embedded; `just release` bakes the web bundle in too. No container runtime, no external database, no Redis.
- **Private by default** — one static API key, all data on local disk, reachable from a phone over Tailscale rather than the public internet.

## Quick start

Requirements: Go 1.25+ and [just](https://just.systems) (`brew install just`). Node 22+ only for the web UI. Optional: [air](https://github.com/air-verse/air) for hot reload.

```bash
just dev          # API + worker (default http://127.0.0.1:8420)
just web-install  # once
just web-dev      # web UI on :8421, proxied to the backend it finds
```

Save a link:

```bash
curl -X POST http://localhost:8420/api/v1/links \
  -H "Authorization: Bearer dev-key" -H "Content-Type: application/json" \
  -d '{"url": "https://go.dev/blog/wal"}'
# 201 {"id":1,"status":"pending","created_at":1784937600}
# pending → scraping → done, with title, tags and thumbnail filled in
```

A busy port never blocks a run: `just dev` scans upward from 8420 and prints the port it took, `just web-dev` proxies to the backend in the same checkout, and Vite moves off 8421 by itself. Several worktrees can run side by side.

`just release` produces one binary at `backend/bin/pushpoint` with the web UI embedded. Always-on setup (launchd/systemd), iPhone access over Tailscale, bookmark import and backups are in [07-DEPLOYMENT.md](docs/v2/07-DEPLOYMENT.md).

## Configuration

Read from `PUSHPOINT_`-prefixed environment variables; there is no config file.

| Variable | Default | Purpose |
|---|---|---|
| `PUSHPOINT_API_KEY` | (required) | Bearer token. `just dev` sets it to `dev-key` |
| `PUSHPOINT_ADDR` | `:8420` | HTTP listen address |
| `PUSHPOINT_DATA_DIR` | `./data` | SQLite database and thumbnails |
| `PUSHPOINT_SCRAPE_CONCURRENCY` | `8` | Maximum concurrent scraper workers |
| `PUSHPOINT_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `PUSHPOINT_LOG_FORMAT` | `auto` | `text` / `json` / `auto` |

For real use, replace the key with `openssl rand -hex 32`.

## How tagging is evaluated

The evaluation is the interesting part, and it is documented in full at [nlu/golden/README.md](nlu/golden/README.md).

Three sets, reported separately and never averaged together:

| Set | Links | Role |
|---|---|---|
| `dev` | 77 | Tuning. Rules, thresholds and dictionary changes are measured here first |
| `test` | 84 | **Frozen.** The only set a milestone gate may read |
| `wild` | 28 | The open web outside developer blogs. Graded like `dev`, never a gate |

Snapshots are captured through the production scrape path, so evaluation input is byte-identical to runtime input — no train/serve skew — and `just eval` makes zero network calls, which keeps results reproducible years later.

The harness reports what a single recall number cannot: how many misses are recoverable by re-ranking versus structurally unreachable, how many links were decided by a tie at the cut, and how much of the loss belongs to the scraper rather than the tagger. A dictionary typo that would silently depress recall fails CI instead.

## Status

**Working today:** save from iOS, web or browser extension; scraping with per-site adapters; rule-based tagging; full-text search; thumbnails; bookmark import; Google Sheets export.

**In progress:** M5 — the ONNX embedding ensemble was rescoped after measurement showed the original exit gate was unreachable, and it now runs as a disposable offline spike with explicit kill criteria before any integration.

**Not started:** M6 (widget, performance polish, write-up), search-quality evaluation.

Milestones and definitions of done are in [08-DEVELOPMENT-PLAN.md](docs/v2/08-DEVELOPMENT-PLAN.md).

## Documentation

Project docs are written in Korean and live in `docs/v2/`, the single source of truth. This README is the English entry point.

- [00-README.md](docs/v2/00-README.md) — table of contents and project intro
- [03-SYSTEM-ARCHITECTURE.md](docs/v2/03-SYSTEM-ARCHITECTURE.md) — single-process architecture, package roles
- [06-API-SPECIFICATION.md](docs/v2/06-API-SPECIFICATION.md) — REST API, auth, cursor pagination
- [07-DEPLOYMENT.md](docs/v2/07-DEPLOYMENT.md) — running, operating, measured benchmarks
- [nlu/golden/README.md](nlu/golden/README.md) — tagging evaluation protocol and every measurement to date
- [docs/README.md](docs/README.md) — v1 ↔ v2 comparison

> The v1 Kubernetes stack (PostgreSQL, Redis, MinIO on Minikube) is folded away in `deploy/k8s-future/` — at zero users that was infrastructure built ahead of the product.

## Contributing

`main` is protected: branch, open a PR, let CI pass. `just --list` shows the recipes; the workflow, merge gate and definition of done are in [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Not licensed yet — a single-user personal project. Open an issue if you need terms for reuse.
