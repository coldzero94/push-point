# Plan Review

> Push-Point v2.1 — last updated: 2026-07-21
> Status: **applied (2026-07-20)** — all 8 recommendations landed in docs/v2 and the justfile (formerly the Makefile). **Added: the web frontend is formally in scope (2026-07-21, the fifth section below)**.

## 1. How the review was run

The whole v2 plan (docs/v2) was read adversarially through 5 lenses: does the NLU approach hold up, does the Go stack survive a fact-check, can any of it be verified locally, is mobile capture trustworthy in daily use, and is the schedule ordered correctly. Of the 34 findings, the twenty-one that made factual claims were cross-checked against primary sources — official documentation, GitHub READMEs, and experiments run on this machine.

## 2. Overall verdict

**The architectural core passes.** A single binary + SQLite (WAL) + FTS5 + a jobs-table queue + classification against a curated dictionary all came through verification intact. Measured on modernc.org/sqlite (CGO_ENABLED=0): save transaction p99 129µs, keyset listing over 100k rows p99 92µs, trigram search over 10k rows ~13ms — FTS5 trigram and `UPDATE...RETURNING` both confirmed working. The performance gates have tens of times the headroom they need.

**The worst defect is not technical, it is ordering.** Real use starts when M4 ends (the 14th week), yet the M3 golden set demands "100 real saved links" — at M3 that data cannot exist. It also contradicts the project's own risk mitigation, which says the answer to losing interest is to put real use first.

## 3. The 8 recommendations (in priority order)

### ① Start real use earlier — highest priority

An iOS Shortcut (share sheet → Get Contents of URL POST, custom headers supported — confirmed against Apple's own documentation) makes **phone capture possible from right after M1, with no app at all**. Add to the M1 DoD: "1 save from the phone succeeds through the Shortcut". Add to M2: bulk import of browser bookmarks / YouTube Takeout (300~500 items). That pulls the real-use anchor forward to the 5th week and dissolves the golden-set and corpus_df cold-start problem in the same move. The M3 60% gate moves from "blocks entry into M4" to "a condition for entering M5".

→ Applied in: 08-DEVELOPMENT-PLAN.md · 07-DEPLOYMENT.md (2026-07-20)

### ② Specify Phase A's Korean handling

As written, "TF-IDF + a RAKE variant" never says what unit matching happens on: with exact token matching, "쿠버네티스를" ≠ the dictionary's "쿠버네티스" (the particle problem); with substring matching, two-character aliases (`ai`, `ml`) fire on words like "said". RAKE's stopword-as-delimiter design fails on an agglutinative language by construction. → Apply the same particle-suffix stripping normalization (a list of 20~30) to both corpus_df accumulation and dictionary matching, match Hangul entries by prefix after normalization, and require word boundaries for Latin entries of fewer than three characters. Write it down at the algorithm level in 02's 7th section.

→ Applied in: 02-TECH-SPEC.md · 08-DEVELOPMENT-PLAN.md (2026-07-20)

### ③ Make M5 realistic — tokenizer and deployment shape

yalue/onnxruntime_go **requires cgo and loads the onnxruntime shared library (.dylib) at runtime** (confirmed in the README) — the "CGO-free single static binary" claim breaks from M5 onward, so the wording in 02/07/08 has to be corrected. The task of building a Go tokenizer whose token IDs agree with HF's is missing from M5 entirely — add it in the 1st week (sugarme/tokenizer or hugot, with a golden test for token agreement against Python). Add the multilingual-e5-small-ko family to the model candidates (ONNX already published, 384-dim) — KoSimCSE and ko-sroberta are 110M (base class), and KoSimCSE has no official ONNX.

→ Applied in: 02-TECH-SPEC.md · 07-DEPLOYMENT.md · 08-DEVELOPMENT-PLAN.md (2026-07-20)

### ④ Settle the evaluation protocol

"top-3 accuracy" has no mathematical definition, tuning and the verdict run on the same 100 links (overfitting), and if eval re-scrapes live it is non-deterministic. → Take the golden set offline as JSONL that carries the scrape snapshot (`{url, snapshot:{title,description,...}, expected_tags}`), split it dev 50 / test 50 (the verdict comes only from the frozen test), and always measure a "domain heuristics only" baseline alongside it so the gate becomes a relative condition (baseline +15pp, for instance).

→ Applied in: 02-TECH-SPEC.md · 08-DEVELOPMENT-PLAN.md · nlu/golden/README.md (2026-07-20)

### ⑤ Verification matrix — turn the DoD into commands

`go test -bench` reports a mean, so no command that renders the "p99 < 50ms" verdict currently exists. → `just bench-http` (measures p99 on the HTTP path, exits 1 when the threshold is crossed), `scripts/coldstart.sh`, `just test-crash` (start the binary → kill -9 → restart → assert recovery, with a fixture HTTP server removing the external dependency), `just seed 100000` (a seed generator for benchmarks). Add a "milestone × verification command" table to 08-DEVELOPMENT-PLAN.md.

→ Applied in: 08-DEVELOPMENT-PLAN.md · justfile (formerly Makefile) (2026-07-20)

### ⑥ Share Extension capture reliability

As designed, a save is simply lost whenever the server is unreachable (Mac asleep, VPN dropped). → Write to a local App Group queue first → POST with a 2~3s timeout → on failure the entry stays queued and the main app / BGTaskScheduler drains it. url_hash is idempotent, so retrying is safe. "Close immediately after the POST" is forbidden, because it is exactly the in-flight loss path (confirmed against Apple's lifecycle documentation). Rewrite the M4 DoD as "even with the server off, a share save always succeeds within two seconds (queued), 0 losses".

→ Applied in: 02-TECH-SPEC.md · 04-DATA-FLOW.md · 06-API-SPECIFICATION.md · 08-DEVELOPMENT-PLAN.md (2026-07-20)

### ⑦ Fill out the 07 operations document

- Sleep: launchd KeepAlive has nothing to do with sleep. A sleeping Mac cannot be woken over Tailscale → `pmset sleep 0`, auto-login, `pmset autorestart 1`.
- ATS: hitting the IP directly (`http://100.x.y.z`) works under the exemption, while a MagicDNS name over plaintext HTTP is blocked → either use the IP only, or `tailscale cert` HTTPS. Tailscale on iOS needs VPN On-Demand (Always).
- State the Apple Developer Program ($99/year) as a prerequisite for M4 — App Groups and Keychain Sharing do work on a free account (confirmed in the official table; the opposing claim raised during the review was rebutted), but **free provisioning expires after 7 days**, which cannot coexist with "an app I use every day".
- Exempt `/thumbs/` from Bearer auth (AsyncImage does not support custom headers, and Tailscale is already the network boundary).
- Scraper adapters: X has no og meta → branch to `publish.twitter.com/oembed`; Naver blogs get rewritten to `m.blog.naver.com`; Instagram gets a rule that tolerates absent meta (all three reproduced by hand).
- Two-character search queries: instead of rejecting q<3 with 400, fall back to LIKE (measured: a 100k-row full scan in 37ms).

→ Applied in: 02-TECH-SPEC.md · 04-DATA-FLOW.md · 06-API-SPECIFICATION.md · 07-DEPLOYMENT.md (2026-07-20)

### ⑧ Schedule structure

Write down the assumed hours per week, and declare the gap between the 22 weeks of milestones and six months — four weeks — as an explicit buffer, with a rule for spending it. Cut order when things slip: Live Activity (demoted past M6) → widgets → all of M5 (run on Phase A's 60%) → the M4 search screen. The anchor held under every scenario = the week real use starts. M1's "backend rework" is corrected to writing it new (applied 2026-07-20), and M6's overcrowding is relieved (notes for the technical write-up accumulate from M5 onward).

→ Applied in: 08-DEVELOPMENT-PLAN.md (2026-07-20)

## 4. Fact-check results, summarized

| Claim | Verdict |
|---|---|
| onnxruntime_go needs cgo and a shared library (no single static binary) | confirmed |
| the tokenizer task is missing from M5 (HF token agreement required) | confirmed |
| a free Apple account cannot use App Groups / Keychain Sharing | **rebutted** (the official table says it can) |
| free provisioning expires after 7 days → cannot be used daily | confirmed |
| an iOS Shortcut can POST a capture with headers attached | confirmed |
| ATS: direct IP is exempt, a hostname over plaintext is blocked | confirmed |
| a sleeping Mac cannot be woken over Tailscale | confirmed |
| X / Instagram / Naver blogs yield no og meta from a static fetch | confirmed (measured) |
| YouTube oEmbed carries no description (thin input for the tagger) | confirmed |
| modernc.org/sqlite: FTS5 trigram and RETURNING work, with performance headroom | confirmed (measured) |
| `go test -bench` cannot measure p99 | confirmed |

## 5. The web frontend formally in scope (2026-07-21)

**Decision**: the web frontend is promoted from an explicit non-goal to **a first-class client on par with iOS**. Both clients consume the same `api/openapi.yaml` contract, so their features are the same; the only iOS-specific part of saving is the two-second entry through the share sheet (the web has a URL input box). This is not a division of labour — the same features, reached a different way.

**2 re-evaluation triggers fired** (the ones we had already agreed to document when work began):
- CLAUDE.md's task-runner re-evaluation trigger, "starting frontend work" — fired. just stays (web tasks go into the justfile as `web-*` recipes; no tool swap).
- `.claude/rules/api.md`'s TypeSpec re-evaluation trigger, "when the web pulls in a Node toolchain" — fired. **Verdict: the hand-written openapi.yaml 3.1 stays, settled.** TypeSpec's condition (40+ operations) is unmet (currently 15), and for a spec this simple and stable TypeSpec's build step is marginal gain, so it stays as it is.

**Stack** (vetted in separate research): Vite + React 19 + TS SPA, Tailwind v4 + shadcn (Radix pinned), TanStack Router + Query v5, openapi-typescript (schema.d.ts committed) + openapi-fetch (Bearer via onRequest). openapi-react-query was not adopted — 2025-12 maintenance mode plus a useInfiniteQuery typing defect — so TanStack's useInfiniteQuery is wrapped directly (0 hand-written types). Deployment stays a single binary through Go `//go:embed` (the embed_frontend tag).

**Contract alignment**: `api/openapi.yaml` is the single source of types for all three consumers — backend (oapi-codegen), iOS (swift-openapi-generator), web (openapi-typescript). The web runs under the same discipline as the backend, through `just web-gen` / `web-gen-check`.

→ Applied in: 01-PROJECT-OVERVIEW.md · 08-DEVELOPMENT-PLAN.md (M-Web) · CLAUDE.md · .claude/rules/frontend.md · frontend/README.md · justfile · .github/workflows/ci.yml (2026-07-21)
