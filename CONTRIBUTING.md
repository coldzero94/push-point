# Contributing

Push-Point is a single-user personal project, but the workflow below is enforced by CI and branch protection, so it applies to every change — including the author's.

## Setup

Requirements: Go 1.25+, [just](https://just.systems) (`brew install just`). Node 22+ and `just web-install` are needed only for the web frontend. The SQLite driver is CGO-free (`modernc.org/sqlite`), so there is no C toolchain and no container runtime to install.

`just setup` installs the Go dev tools at pinned versions (oapi-codegen, [air](https://github.com/air-verse/air), gotestsum, goimports) and the web dependencies; `just doctor` checks the environment and points at what is missing. [air](https://github.com/air-verse/air) gives `just dev` hot-reload — it rebuilds and restarts on a `.go`/`.sql` change, and without it `just dev` falls back to `go run`. [mprocs](https://github.com/pvolok/mprocs) (`brew install mprocs`, brew-managed like golangci-lint) powers `just dev-all`. Every optional tool degrades gracefully when absent.

```bash
just setup        # once — pinned Go dev tools + web dependencies
just doctor       # check the environment (required / recommended / optional tools)
just dev          # API + worker; scans upward from :8420 for a free port (hot-reload via air, colored logs)
just web-dev      # Vite dev server on :8421, proxied to the backend it detects
just dev-all      # both of the above in one split-screen TUI (mprocs)
just --list       # every recipe with its description
```

Operational topics — environment variables, launchd/systemd, Tailscale access from the phone, bookmark/Takeout import, backup and restore — are documented in [docs/v2/ko/07-DEPLOYMENT.md](docs/v2/ko/07-DEPLOYMENT.md).

Working in git worktrees (one per agent) needs no manual setup: `frontend/node_modules` is the only ignored path a fresh worktree cannot rebuild on demand, and `orca.yaml` restores it — cloned from the primary checkout when the lockfile matches, `just web-install` otherwise (Go modules come from the global cache and are only pre-warmed). Dev servers pick free ports per worktree, so several can run at once: `just dev` records its port and PID for the checkout, and `just web-dev` proxies to that backend rather than to whichever one answers first. The file is [Orca](https://www.onorca.dev)-specific but readable as the checklist for any worktree tool; `.orca/` holds per-user overrides and is ignored.

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
| `just setup` | One-time onboarding: `go install` the pinned Go dev tools (oapi-codegen v2.8.0, air, gotestsum, goimports) and run `web-install` |
| `just doctor` | Report required/recommended/optional tool presence and how to get what is missing (installs nothing) |
| `just dev` | Dev server (`PUSHPOINT_API_KEY=dev-key`); scans from `:8420` (override the base with `PUSHPOINT_PORT`) and prints the URL. Hot-reload via air when installed (else `go run`); forces `PUSHPOINT_LOG_FORMAT=text` and `PUSHPOINT_LOG_LEVEL=debug` for colored, verbose dev logs |
| `just dev-all` | `just dev` + `just web-dev` in one split-screen TUI (mprocs) — panels keep their own colors, web restarts with `r` while air reloads the backend |
| `just build` | `go build -o bin/pushpoint ./cmd/pushpoint` |
| `just release` | `web-build` then `go build -tags embed_frontend` — single binary with the SPA embedded |
| `just gen` | `api/openapi.yaml` → `backend/internal/api/gen/` (oapi-codegen v2.8.0, output committed) |
| `just gen-check` | Contract drift guard — fails if `git diff` remains after regeneration (CI) |
| `just enum-lint` | `openapi.yaml` enums vs migration `CHECK` constraints; exit 1 on mismatch |
| `just dict-lint` | `nlu/dictionary/` (tags.json/domains.json) vs the seed migrations; exit 1 on mismatch |
| `just test` | `go test ./...` |
| `just test-watch` | Re-run tests on change (`gotestsum --watch`) — handy for the M3 tagging/eval loop |
| `just db-reset` | Delete the dev SQLite DB under `backend/data/` (next `just dev` recreates it via migrations); thumbnails and the port record are left alone |
| `just lint` | `golangci-lint run` |
| `just fmt` | `gofmt` + `goimports` over `backend/` |
| `just bench` | Microbenchmarks: `go test -bench=. -benchmem ./...` (the p99 verdict belongs to `bench-http`) |
| `just bench-http` | Save API HTTP-path p99 gate — exit 1 when p99 >= 50 ms |
| `just test-crash` | Crash recovery: build → fixture server → save → `kill -9` → restart → assert every job reaches `done` |
| `just seed 100000` | Mixed Korean/English seed DB for benchmarks (fixed seed, default n=10000) |
| `just eval` | Tagging accuracy on the golden set — top-3 recall against the baseline (M3+) |
| `just web-install` | `npm ci` in `frontend/` |
| `just web-dev` | Vite dev server on `:8421`; proxies to the port `just dev` recorded in this checkout, else probes `/healthz` from `:8420` |
| `just web-gen` | `api/openapi.yaml` → `frontend/src/lib/api/schema.d.ts` (openapi-typescript pinned, output committed) |
| `just web-gen-check` | Web contract drift guard (CI) |
| `just web-build` | Production bundle → `frontend/dist/`, copied to `backend/internal/web/dist/` for `go:embed` |
| `just web-lint` | oxlint |
| `just web-test` | frontend unit tests (vitest) — the pure logic in `src/lib/`, no DOM |
| `just web-embed-test` | `web-build`, then the backend tests that only compile under `-tags embed_frontend` |
| `just streak-selftest` | checks the streak rule agrees between the web and `scripts/streak.sh` (shared fixture in `testdata/`) |

`scripts/` holds the shell harnesses these recipes call: `bench_http.sh`, `coldstart.sh` (exec → `/healthz` 200 in under 1 s), `test_crash.sh`, `lint_enums.sh`.

## Documentation changes

`docs/v2/` is the single source of truth; `docs/v1/` is an archive and is never modified. For the language policy see `CLAUDE.md`. Detailed rules live in `.claude/rules/` scoped by path.
