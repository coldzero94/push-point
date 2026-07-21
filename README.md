# Push-Point

[![ci](https://github.com/coldzero94/push-point/actions/workflows/ci.yml/badge.svg)](https://github.com/coldzero94/push-point/actions/workflows/ci.yml)

A self-hosted personal link archive that auto-tags what you save — one Go binary, no external AI API.

[Docs](docs/v2/00-README.md) · [API contract](api/openapi.yaml) · [Roadmap](docs/v2/08-DEVELOPMENT-PLAN.md) · [Contributing](CONTRIBUTING.md)

## What it is

Share a YouTube video or an article and Push-Point saves it instantly, then fills in title, thumbnail and tags in the background so you can find it again by tag or full-text search. The server is a single Go process on SQLite: `just dev` starts everything, and a backup is a copy of `data/`. Tagging is solved by a local NLU pipeline (rules first, ONNX embeddings later) instead of an external LLM API — zero cost per item, and nothing you save leaves the machine. The v1 Kubernetes stack (PostgreSQL, Redis, MinIO on Minikube) is folded away in `deploy/k8s-future/`, because at zero users that was infrastructure built ahead of the product — see [docs/README.md](docs/README.md) for the v1 ↔ v2 comparison.

## Features

- **Instant save** — `POST /api/v1/links` commits two INSERTs and returns 201; scraping, thumbnails and tagging run in a SQLite-backed job queue that survives `kill -9`. Measured save p99 is 0.98 ms against a 50 ms target.
- **Auto-tagging without an LLM** *(planned, M3)* — a rule-based tagger classifies against a controlled tag dictionary with Korean normalization; ONNX embeddings join as an ensemble in M5. No API key, no per-item cost.
- **Search** — FTS5 trigram full-text search with bm25 ranking, which matches Korean without a morphological analyzer; queries shorter than 3 characters fall back to LIKE instead of failing. Tag and status filters on top.
- **Web and iOS clients** — the React SPA is a first-class client today; the SwiftUI Share Extension lands in M4. Both generate their types from the same `api/openapi.yaml`.
- **Single binary** — Go with a CGO-free SQLite driver. Migrations are embedded, and `just release` bakes the web bundle in too. No container runtime, no external database.
- **Private by default** — one static API key, all data on local disk, reachable from the phone over Tailscale instead of the public internet.

## Quick start

Requirements: Go 1.25+ and [just](https://just.systems) (`brew install just`). Node 22+ only if you want the web UI.

```bash
just dev          # API + worker, prints the URL it took (default http://127.0.0.1:8420)
just web-install  # once — installs web dependencies
just web-dev      # web UI on :8421, proxied to the backend it finds
```

A busy port never blocks a run: `just dev` scans upward from 8420 for a free port and prints the one it picked, `just web-dev` probes `/healthz` to locate the running backend, and Vite moves off 8421 by itself if that port is taken.

Save a link:

```bash
curl -X POST http://localhost:8420/api/v1/links \
  -H "Authorization: Bearer dev-key" -H "Content-Type: application/json" \
  -d '{"url": "https://go.dev/blog/wal"}'
# 201 {"id":1,"status":"pending","created_at":1784937600}
# a worker fills in title and thumbnail within seconds: pending -> scraping -> done
```

For a production build, `just release` produces one binary at `backend/bin/pushpoint` with the web UI embedded. Environment variables, always-on setup (launchd/systemd), iPhone access over Tailscale, bookmark/Takeout import and backups are all in [docs/v2/07-DEPLOYMENT.md](docs/v2/07-DEPLOYMENT.md).

## Configuration

Everything is read from `PUSHPOINT_`-prefixed environment variables; there is no config file.

| Variable | Default | Purpose |
|---|---|---|
| `PUSHPOINT_API_KEY` | (required) | Bearer token for the API. `just dev` sets it to `dev-key` |
| `PUSHPOINT_ADDR` | `:8420` | HTTP listen address |
| `PUSHPOINT_DATA_DIR` | `./data` | SQLite database and thumbnails |
| `PUSHPOINT_SCRAPE_CONCURRENCY` | `8` | Maximum concurrent scraper workers |
| `PUSHPOINT_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

For real use, replace the API key with a long random string (`openssl rand -hex 32`). Full reference: [docs/v2/07-DEPLOYMENT.md](docs/v2/07-DEPLOYMENT.md).

## Status

M1 (schema, store/queue, full API, bench harness) and M2 (worker pool, site adapters, thumbnails, import, SSRF guard) are merged, and the web client shipped alongside them — daily use works today. Next up is M3, rule-based tagging plus search evaluation, then M4, the iOS Share Extension; M5 (ONNX ensemble) and M6 (widget, polish, write-up) follow.

Milestones, definitions of done and the per-milestone verification matrix live in [docs/v2/08-DEVELOPMENT-PLAN.md](docs/v2/08-DEVELOPMENT-PLAN.md).

## Documentation

Project docs are written in Korean and live in `docs/v2/`, the single source of truth. This README is the English entry point.

- [docs/v2/00-README.md](docs/v2/00-README.md) — table of contents and project intro
- [docs/v2/03-SYSTEM-ARCHITECTURE.md](docs/v2/03-SYSTEM-ARCHITECTURE.md) — single-process architecture, repository layout, package roles
- [docs/v2/06-API-SPECIFICATION.md](docs/v2/06-API-SPECIFICATION.md) — REST API, auth, cursor pagination (machine source: [api/openapi.yaml](api/openapi.yaml))
- [docs/v2/07-DEPLOYMENT.md](docs/v2/07-DEPLOYMENT.md) — running and operating the server, configuration, measured benchmarks
- [docs/v2/08-DEVELOPMENT-PLAN.md](docs/v2/08-DEVELOPMENT-PLAN.md) — milestones M1–M6 and completion criteria
- [docs/README.md](docs/README.md) — v1 ↔ v2 document index and comparison

## Contributing

`main` is protected: work on a branch, open a PR, and let CI pass. Run `just --list` to see the task runner recipes; the workflow, the merge gate and the definition of done are in [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Not licensed yet — this is a single-user personal project. Open an issue if you need terms for reuse.
