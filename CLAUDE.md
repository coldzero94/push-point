# Push-Point

Personal link-saving and auto-tagging app — single user; the technical differentiator is a lightweight, LLM-free NLU.
Current status: M1 (schema, store/queue, full API, bench harness) and M2 (worker pool, site adapters, thumbnails, import) are merged; the web client ships alongside them. Next: M3 rule-based tagging + search eval.

## Workspace (monorepo)

- `api/` — machine source of truth for the API, `openapi.yaml` (OpenAPI 3.1) — generates backend and client code (`just gen`).
- `backend/` — Go single binary (API + worker + NLU runtime inference). All executable code lives here.
- `nlu/` — NLU offline **assets only**: dictionary/ (tag dictionary), golden/ (eval set), models/ (ONNX conversion). Runtime inference is `backend/internal/tagger` (Go); Python is allowed only under `nlu/models/`.
- `ios/` — M4: SwiftUI app + Share Extension. No code yet.
- `frontend/` — web app (Vite + React + TS, consumes the `api/openapi.yaml` contract, peer of iOS). **Parity is decided per feature, not assumed** — classify against `docs/v2/ko/13-CLIENT-PARITY.md` and record the axis in the PR. ("Only the share sheet differs" was the old claim; it had already stopped being true.)
- `docs/v2/` = single source of truth, `docs/v1/` = v1 archive (do not modify), comparison in `docs/README.md`.
- `deploy/k8s-future/` — preserved v1 k8s manifests (unused, do not modify).

## Commands (root justfile; Go recipes target backend/)

- `just dev` — run locally (PUSHPOINT_API_KEY=dev-key)
- `just test` — full test suite (`cd backend && go test ./...`)
- `just bench` — microbenchmarks (not the p99 verdict — that is bench-http)
- `just bench-http` — p99 < 50ms gate on the save API HTTP path; exits 1 when exceeded (M1+)
- `just eval` — top-3 tagging accuracy on nlu/golden/ (M3+)
- `just eval-search` — search hit@1 / MRR@10 on nlu/golden/search.jsonl
- `just gen` — api/openapi.yaml → backend/internal/api/gen/ (oapi-codegen pinned to v2.8.0, generated output committed)
- `just web-dev` — Vite dev server on :8421 (proxies /api, /thumbs, /healthz → :8420, so relative-path code matches the prod embed)
- `just web-build` — build frontend/dist/ (dist/ is not committed)
- `just release` — web build + single binary with the SPA embedded (`backend/bin/pushpoint`, `-tags embed_frontend`)
- `just web-gen` — api/openapi.yaml → frontend/src/lib/api/schema.d.ts (openapi-typescript pinned, generated output committed)
- `just web-test` — frontend unit tests (vitest, pure logic in `src/lib/`). `just web-embed-test` is the different thing: Go tests of the `embed_frontend` SPA path
- `just streak-selftest` — the streak rule agrees between web and `scripts/streak.sh` (shared fixture `testdata/streak-cases.json`)
- `just icons` — `design/icon/mark.svg` → the 8 icon files across iOS/web/extension. Generated output is committed and has **no CI drift gate** (CI has no macOS/Chrome), so run it and commit the result in the same change
- `just ios-api-gen` — api/openapi.yaml → ios/PushPoint/Generated/ (swift-openapi-generator CLI, generated output committed — the contract's third consumer). `just ios-stamp-check` is the CI-side gate: it compares a committed hash of the spec instead of regenerating, so it runs without macOS or Swift
- `just flow [file]` — Maestro flow against the booted simulator's real data (default `maestro/smoke.yaml`)
- `just ios-uitest` — XCUITest on the simulator with its own seeded fixtures
- `just ios-test` — iOS unit tests (`PushPointTests` — the cover-hash goldens shared with web)
- `just ios-api-gen-check` — iOS generated-output drift (the contract's third consumer)
- `just ios-bind-check` — **the gomobile binding vs the backend it was built from.** `ios-build` depends on it. `ios/Frameworks/` is a gitignored local artifact that `git pull` does not refresh, and a stale one shipped 30 of 42 dictionary tags for two days
- `just save-timing` — M4 DoD verdict: was the share save under 2s (exits 1 if not)
- For the remaining recipes (build/gen-check/web-gen-check/test-crash/seed/lint/fmt), run `just` to list them

## Core rules

- **`docs/v2/` ships in both languages — `ko/` and `en/`, one file each.** Korean is the source of truth (the author writes there first); the English twin travels in the same change, and `just docs-parity` fails when their structure, tables, code blocks or numbers drift. It cannot check that the prose means the same thing, so that stays a human job. Code, identifiers and technical terms are English on both sides. The root `README.md` is English because it is the public GitHub landing page, and agent-facing files (`CLAUDE.md`, `.claude/rules/`) are English too. This policy lives here; other files point at it rather than restating it.
- Commit messages follow Conventional Commits (`feat:`/`fix:`/`docs:`/`chore:` etc.); the subject is a single English line.
- The task runner is just (adopted after the 2026-07-20 evaluation — re-evaluation triggers: starting frontend work, a collaborator joining). The API contract stack is hand-written OpenAPI 3.1 + oapi-codegen pinned to v2.8.0 + swift-openapi-generator (settled in the 2026-07-20 review; background in docs/v2/ko/09-PLAN-REVIEW.md and .claude/rules/api.md).
- Design sources of truth: schema = `docs/v2/ko/05-DATA-SCHEMA.md`, API = `api/openapi.yaml` (`docs/v2/ko/06-API-SPECIFICATION.md` is commentary), plan = `docs/v2/ko/08-DEVELOPMENT-PLAN.md`. To change a design, edit the source first and let the rest follow (for the API, regenerate with `just gen`).
- No unmeasured "seems to work" — back performance and quality claims with numbers from `just bench-http` (p99 gate), `just bench`, or `just eval`.
- **Definition of done**: declare implementation work complete only after `just fmt`, `just lint`, `just test`, and `just gen-check` all pass (plus `just web-gen-check`, `just web-test`, and `just web-build` for frontend changes; `just ios-bind-check`, `just ios-api-gen-check`, `just ios-test`, and `just ios-uitest` for iOS or contract changes — and `just ios-bind` first whenever `backend/` moved), and present the commands you ran and their output as evidence (no success claims without output).
- **UI changes need a screen, not a build.** A successful build is not evidence that a screen is right — every UI failure this project shipped compiled cleanly. Look at the screen before calling UI work done — **this applies to the web too**: Maestro has a `chromium` device, so "the web screen cannot be inspected here" is not a reason to stop at typecheck. Driving finds what is visible; it has never found an error path or a race, so keep `just ios-uitest` and `just web-test` for those. Details in `.claude/rules/ui-verification.md`.
- **Verify the artifact, not its proxy** — the repeated failure here is not missing verification but verifying something that *resembles* what a person meets: matching column names instead of contents, `POST /markdown` instead of the README renderer, pattern parameters instead of the drawing, a key set instead of the literal. Seven of those in one day (2026-08-04~05). `.claude/rules/verification.md` lists this repo's seams and three corollaries — compare outputs not inputs, break a new gate before trusting it, and suspect the wiring when a measurement improves.
- **Sweep rule**: for edits spanning many files, do not assign targets from memory — first build the target list with `grep -l`/glob, save it to a file, and work it off as a checklist. When done, re-run the same search and confirm zero remaining.
- Mention the v1 stack (PostgreSQL/Redis/MinIO/OpenAI/k8s/Gin/Ent) only in "v1 vs v2" context. It must not appear in descriptions of the current architecture.
- The 8 recommendations from the plan review (2026-07-20) are already applied — see `docs/v2/ko/09-PLAN-REVIEW.md` for background and rationale.
- The web frontend is officially in scope as of 2026-07-21 — the non-goal is retired and it is promoted to a full-feature client on par with iOS. Background in `docs/v2/ko/09-PLAN-REVIEW.md`, detailed rules in `.claude/rules/frontend.md`.
- **Code review gate**: when a unit of implementation work (milestone or feature) is finished, run `/pr-review-toolkit:review-pr` before committing. Fix high/medium findings before commit and push, and record the reason for anything deliberately deferred.
- **Merge rule**: never push directly to main (enforced by the GitHub ruleset `main-protection` — PR required, `ci` check required, force-push and deletion blocked). Flow: branch → commit and push → PR → green CI + code review gate → merge. If CI breaks on main, fix it before anything else.
- Per-area detailed rules are split into `.claude/rules/` by path scope (backend, nlu, ios, frontend, docs, api) plus `ui-verification.md` (how to check a screen without asking a human — Maestro / AXe / XCUITest, and which to reach for).
