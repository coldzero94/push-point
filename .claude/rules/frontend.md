---
paths:
  - "frontend/**"
---

# Frontend workspace rules (web app)

The web app is a first-class, full-feature client on par with iOS (adopted 2026-07-21). It consumes the same `api/openapi.yaml`, so save, list, search, tag filtering, detail, tag editing, delete, retry, and stats are all implemented end-to-end on the web.
- **Parity is decided per feature, not assumed.** "Only the share sheet differs" was the original claim and it had already stopped being true — stats shipped on iOS alone, dictionary CRUD on web alone. Before adding a feature to either client, classify it against [`docs/v2/ko/13-CLIENT-PARITY.md`](../../docs/v2/ko/13-CLIENT-PARITY.md): ① archive work (both or neither), ② entry and platform affordances (each at its best, parity would be worse), ③ decided by where the data lives (state the condition). Record the axis in the PR.

## Stack (fixed — §16)

- **Vite + React 19 + TypeScript, pure SPA** (no SSR — single-user local app).
- **Tailwind CSS v4** (CSS-first `@theme`) + **shadcn/ui** — Radix primitives are **explicitly pinned** (shadcn's default moved to Base UI in 2026-07, but we stay on Radix for a11y maturity) + lucide-react.
- **TanStack Router** (typed search params — `?q ?tag ?status cursor` are URL state) + **TanStack Query v5** (useInfiniteQuery).
- Contract type generation: **openapi-typescript** (pinned) + **openapi-fetch** (bearer injected via an onRequest middleware — symmetric with the iOS ClientMiddleware). **Do not adopt openapi-react-query** (maintenance mode + useInfiniteQuery typing defects that hit exactly the list and search screens) — wrap TanStack `useInfiniteQuery` directly over openapi-fetch (2 hooks, 0 hand-written API types).
- No global state library (server state = Query, URL state = Router, local UI = useState). No form library (useState + Zod). Dark mode is prefers-color-scheme + a localStorage toggle + `dark:`.

## Contract alignment (contract-first — the third consumer of openapi.yaml)

- `frontend/src/lib/api/schema.d.ts` is a contract artifact **generated from `api/openapi.yaml` and committed** — **never hand-write it**, never edit it directly. Even API type mismatches are fixed only by editing openapi.yaml and re-running `just web-gen`.
- After an API change, re-run `just web-gen` (`openapi-typescript`, version-pinned — no `@latest`) and commit `schema.d.ts` with it. `just web-gen-check` (web-gen followed by `git diff --exit-code`) blocks drift in CI — symmetric with the backend `gen-check`.

## Paths, auth, deployment

- **Use origin-relative paths only** — no absolute URLs or hard-coded hosts (i.e. nothing like `http://localhost:8420/...`); this is not a mandate for document-relative (`./`) paths. Paths are origin-based and start with `/`: call only `/api/v1/...`, `/thumbs/...`, and `/healthz`, and leave Vite `base` at `'/'` (with `'./'`, a `/links/123` deep link resolves assets to `/links/assets/...`, the SPA fallback returns index.html, and MIME rejection breaks boot). This is what makes the same code work in dev (Vite proxy :8421 → Go :8420) and prod (same-origin embed).
- **Auth**: the API key is entered on the settings screen, stored in localStorage, and attached as `Authorization: Bearer` (parity with iOS). Do not add new auth exemptions, and do not ask for server-side loopback bypasses or relaxed auth (api.md rule — the only exemptions are healthz and thumbs).
- **`dist/` is not committed** (build artifact — CI produces it with `just web-build`). Production serves `dist/` via `//go:embed all:dist` + `http.FileServerFS`, but the embed-serving code sits behind the `embed_frontend` build tag — without dist/ it fails to compile, so backend-only `just build` and CI stay green without the tag, and only releases run `web-build && go build -tags embed_frontend`.
- Display fallback follows the same discipline as iOS: when the server returns an empty string for `title` (no og or title tag), show `domain` instead (then `url`) — preventing empty cells is the client's responsibility.

## Tests

- `just web-test` runs **vitest** over the pure logic in `src/lib/` (node environment, no DOM). Added 2026-07-28; before that `frontend/` had no runner at all and `just web-test` was a Go recipe that executed zero TypeScript. Component testing (jsdom/testing-library) is still an open decision.
- **TZ is pinned to `America/Los_Angeles` in `vitest.config.ts` on purpose.** A mutation reverting the local-time date parse in `rhythm.ts` passes in KST, because east of Greenwich the UTC-parse trap cannot fire. A west-of-Greenwich zone is what gives the suite the power to catch it.
- Rules implemented in more than one language keep a **shared fixture**, not a claim: `testdata/streak-cases.json` is read by both `rhythm.test.ts` and `scripts/streak.sh --self-test` (`just streak-selftest`). The sentence it replaced — "verified against four cases by hand" — could not be re-run and was wrong about two neighbouring rules.

## The spacing and radius scales are explicit — off-scale classes do nothing

`tailwind.css` resets `--spacing-*` and `--radius-*` to `initial` and defines exactly
**2 4 6 8 12 16 20 24 32 40 56 80** (number = pixels), plus named radii
(chip/control/thumb/card/panel/sheet/bar). Anything else — `h-64`, `gap-1`, `rounded-xs` —
compiles to **a class with no CSS**: no lint error, no type error, and nothing on screen.
A skeleton written `h-64` reserved no height at all and nobody noticed until the page was
opened. Dimensions outside the 12 steps go in `--size-*` and are referenced as `h-(--size-name)`.

## Looking at the screen

Maestro's `chromium` device drives the real app — see `.claude/rules/ui-verification.md`.
Serve it with `just release` + the binary so the SPA and API share an origin, not a mock.
