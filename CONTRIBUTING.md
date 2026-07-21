# Contributing

Push-Point is a single-user personal project, but the workflow below is enforced by CI and branch protection, so it applies to every change — including the author's.

## Setup

Requirements: Go 1.25+, [just](https://just.systems) (`brew install just`). Node 22+ and `just web-install` are needed only for the web frontend. The SQLite driver is CGO-free (`modernc.org/sqlite`), so there is no C toolchain and no container runtime to install.

```bash
just dev          # API + worker; scans upward from :8420 for a free port
just web-dev      # Vite dev server on :8421, proxied to the backend it detects
just --list       # every recipe with its description
```

Operational topics — environment variables, launchd/systemd, Tailscale access from the phone, bookmark/Takeout import, backup and restore — are documented in [docs/v2/07-DEPLOYMENT.md](docs/v2/07-DEPLOYMENT.md).

## Workflow

Direct pushes to `main` are blocked by the GitHub ruleset `main-protection`: a pull request is required, the `ci` status check must pass, and force-push and branch deletion are blocked.

1. Branch, commit, push, open a PR. Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `chore:`) with a single-line English subject.
2. CI must be green (see below).
3. Code review gate: every implementation change is reviewed before commit; high and medium findings are fixed, or deferred with the reason recorded.
4. Merge. If CI breaks on `main`, fixing it takes priority over everything else.

## CI

`.github/workflows/ci.yml` runs two jobs, and both must pass to merge.

- **ci** (backend) — gofmt check, `go build`, `go vet`, race-enabled `go test`, contract drift (`just gen-check`), and enum consistency between `api/openapi.yaml` and the migration `CHECK` constraints.
- **web** (frontend + embed) — `npm ci`, contract drift (`just web-gen-check`), oxlint, `tsc --noEmit`, `just web-build`, then the release-only path: `go build -tags embed_frontend` and the embed tests, which only compile once `dist/` exists.

## Definition of done

Implementation work is done when `just fmt`, `just lint`, `just test` and `just gen-check` pass — plus `just web-gen-check` and `just web-build` for frontend changes — and the commands and their output are shown as evidence. Performance and quality claims need numbers from `just bench-http`, `just bench` or `just eval`; "seems faster" does not count.

Generated output is committed and must not be hand-edited: `backend/internal/api/gen/` (oapi-codegen, pinned to v2.8.0) and `frontend/src/lib/api/schema.d.ts` (openapi-typescript, pinned). Change `api/openapi.yaml` first, then regenerate with `just gen` / `just web-gen` and commit both in the same change. `frontend/dist/` and `backend/internal/web/dist/` are build artifacts and stay uncommitted.

## Recipe reference

All Go recipes run inside `backend/`. Recipes for milestones that have not landed yet print a notice instead of failing, except the web gate recipes, which fail when their prerequisites are missing.

| Recipe | What it does |
|---|---|
| `just` | List recipes (default) |
| `just dev` | Dev server (`PUSHPOINT_API_KEY=dev-key`); scans from `:8420` (override the base with `PUSHPOINT_PORT`) and prints the URL |
| `just build` | `go build -o bin/pushpoint ./cmd/pushpoint` |
| `just release` | `web-build` then `go build -tags embed_frontend` — single binary with the SPA embedded |
| `just gen` | `api/openapi.yaml` → `backend/internal/api/gen/` (oapi-codegen v2.8.0, output committed) |
| `just gen-check` | Contract drift guard — fails if `git diff` remains after regeneration (CI) |
| `just enum-lint` | `openapi.yaml` enums vs migration `CHECK` constraints; exit 1 on mismatch |
| `just test` | `go test ./...` |
| `just lint` | `golangci-lint run` |
| `just fmt` | `gofmt` + `goimports` over `backend/` |
| `just bench` | Microbenchmarks: `go test -bench=. -benchmem ./...` (the p99 verdict belongs to `bench-http`) |
| `just bench-http` | Save API HTTP-path p99 gate — exit 1 when p99 >= 50 ms |
| `just test-crash` | Crash recovery: build → fixture server → save → `kill -9` → restart → assert every job reaches `done` |
| `just seed 100000` | Mixed Korean/English seed DB for benchmarks (fixed seed, default n=10000) |
| `just eval` | Tagging accuracy on the golden set — top-3 recall against the baseline (M3+) |
| `just web-install` | `npm ci` in `frontend/` |
| `just web-dev` | Vite dev server on `:8421`; probes `/healthz` from `:8420` to find the backend to proxy to |
| `just web-gen` | `api/openapi.yaml` → `frontend/src/lib/api/schema.d.ts` (openapi-typescript pinned, output committed) |
| `just web-gen-check` | Web contract drift guard (CI) |
| `just web-build` | Production bundle → `frontend/dist/`, copied to `backend/internal/web/dist/` for `go:embed` |
| `just web-lint` | oxlint |
| `just web-test` | `web-build`, then the backend tests that only compile under `-tags embed_frontend` |

`scripts/` holds the shell harnesses these recipes call: `bench_http.sh`, `coldstart.sh` (exec → `/healthz` 200 in under 1 s), `test_crash.sh`, `lint_enums.sh`.

## Documentation changes

`docs/v2/` is the single source of truth; `docs/v1/` is an archive and is never modified. For the language policy see `CLAUDE.md`. Detailed rules live in `.claude/rules/` scoped by path.
