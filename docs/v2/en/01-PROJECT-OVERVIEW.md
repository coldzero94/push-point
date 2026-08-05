# Push-Point - Project Overview

> Push-Point v2.1 — last updated: 2026-07-21

## Overview

Push-Point is a personal link archive that tags what you share, automatically. Save a YouTube video, a web article or a social post with one tap from the iOS share sheet, and in the background it collects metadata and a thumbnail, then classifies the link against a controlled tag dictionary and attaches the tags. Later you find it again by tag or by full-text search.

It is a single-user, self-hosted app. The server is one Go binary (`pushpoint`), the data lives in a single SQLite file, and nothing depends on an external API.

## v2 principles

**Product before infrastructure.** v1 was a design with Minikube + k8s + HPA, PostgreSQL, Redis, MinIO and the OpenAI API — for a product with zero users. Autoscaling is a problem you get *after* you have users. v2 inverts that order.

- The goal is "the app I use every day". What counts as finished is real use, not a deployment topology.
- Performance comes from **the quality of a single-process design**, not from distribution. Design choices like SQLite WAL, keyset pagination and an in-process worker pool are what satisfy the performance targets below.
- v1's k8s manifests are preserved in `deploy/k8s-future/`. Store/Queue/Tagger sit behind interfaces, so once there are users only the implementations need swapping. This is folding it up, not throwing it away.

## Core features

### 1. Saving

- The clients are **both the iOS app and the web app, equal full-feature clients**. They consume the same `api/openapi.yaml` contract, so saving, listing, search, tag filtering, detail, tag editing, deletion, retry and stats are all usable end to end on either side. **Which feature goes to which side is judged per feature, and the reasoning is recorded in the PR** — the three axes (① the archive goes to both, ② entry points and platform features go to whatever is best on each, ③ where the data lives decides) are in [13-CLIENT-PARITY.md](13-CLIENT-PARITY.md). The save entry point falls on axis ②, so it diverges: the 2-second entry through the iOS share sheet is an OS feature (the Share Extension), and the web saves from a URL field (plus an optional bookmarklet). The API they call and the result are identical.
- One tap in the iOS Share Extension. The extension **writes straight into the App Group's shared SQLite and finishes tagging and summarizing on the spot**, so a save completes with no server and in airplane mode. Real use does not wait for the app (M4) — daily saving starts with an iOS Shortcut the moment M2 ends.
- The save API (`POST /api/v1/links`) does two INSERTs synchronously — the link and the job — and nothing else, so it answers immediately at a 50ms-p99. Scraping, tagging and thumbnails are all asynchronous.
- Re-saving the same URL is caught by `url_hash`, and the existing item comes back instead of a duplicate.

### 2. Auto-tagging

- Rather than "generating" free-form tags, the problem is narrowed to **classification** against a controlled tag dictionary (30–50 tags, user-editable). It is the only way to get quality without an LLM.
- A two-stage pipeline: Phase A rule-based (domain heuristics + candidate-phrase extraction on top of particle-suffix normalization + TF-IDF scoring and dictionary matching, pure Go) → Phase B an ONNX embedding classifier (Korean sentence embeddings, cosine similarity), ensembled.
- v1's dependency on the OpenAI API is gone. 0 cost, a response in the hundreds of ms, and no data leaving the machine. This pipeline is the project's technical differentiator.
- Manual tag corrections are recorded as `tag_feedback` and feed the reranking correction in Phase B.

### 3. Search

- SQLite FTS5 (trigram tokenizer) full-text search over title / description / note / tags, ranked by bm25. A query shorter than three characters falls back to LIKE rather than being refused (no 400).
- Combines with tag filters and a date range (`from`/`to`). Target: under 30ms at 10k links.

### 4. Personal notes

- Every link can carry a personal note (`note`), and notes are searchable too.

## User scenario

The unit of use for this app is the following loop, repeated dozens of times a day.

```
1. 유튜브/브라우저에서 관심 콘텐츠 발견
   ↓
2. 공유 시트 → Push-Point 한 탭 → 기기 안에서 저장·태깅 완료 후 시트 닫힘 (2초 미만, 서버가 없어도 성립)
   ↓
3. 서버가 백그라운드에서 스크랩 → 태깅 → 썸네일 (저장 후 3초 내 완료)
   ↓
4. 나중에 앱에서 태그 필터 또는 검색으로 재발견
   ↓
5. 다시 읽거나, 메모 추가, 태그 수정
```

Step two is the whole point. If the friction of saving goes past two seconds it stops being an app you use every day. So nothing is waited on at save time, and everything else is handed to the job queue.

## Development targets

Performance targets (on a local M-series machine, verified at every milestone — the p99 verdict is `just bench-http`, microbenchmarks are `just bench`):

| Metric | Target |
|---|---|
| Save API p99 | < 50ms |
| Save → tagging complete (async) | < 3s |
| Search (FTS5, 10k links) | < 30ms |
| List-scroll API at 100k links | < 50ms |
| Cold start (binary launch → serving) | < 1s |

Tagging accuracy gate: top-3 accuracy is measured with `just eval` over a golden set of 174 built from real-use and imported links (dev 62 / test 84 / wild 28, offline evaluation from snapshots). The gate is a **relative condition against a baseline** — M5 entry: Phase A is significant on paired samples against a **constant predictor** (content-blind, test 0.721) — met; M5 exit: the ensemble makes 0 regressions and 5 or more improvements over Phase A (redefined 2026-07-26 — details in 02-TECH-SPEC.md). No unmeasured "it seems to work".

## Differentiators

- **0 cost**: no external API calls, no cloud infrastructure bill. It runs on my machine.
- **Privacy**: the links, the notes and the whole tagging process stay local. No data leaves the machine.
- **A server that works offline**: a single binary that runs without the internet (reaching whatever is being scraped aside), tagging included.
- **Performance backed by measurement**: the numbers in the table above are not declarations, they are gates that verification commands like `just bench-http` and `just eval` check at every milestone.

## Users

This app has one user: me. v1 aimed at curators, researchers, developers and designers, but a persona list for a product with zero users means nothing. v2's only success metric is whether I use it every day, and every design decision (single-user auth, a schema with no separate users table, self-hosting) follows from that. Whether it is a thing anyone else needs is something I judge after using it every day for 4 weeks straight.

## Explicit non-goals

What v2 does not do:

- **k8s / HPA / multi-node** — `deploy/k8s-future/` comes back once there are users
- **Signup / multi-user** — a single user, authenticated by one static API key
- **Depending on an external LLM API such as OpenAI** — the NLU pipeline is this project's identity
- **Android** — decided after iOS proves itself in real use

## Related documents

- Tech spec: [02-TECH-SPEC.md](02-TECH-SPEC.md)
- System architecture: [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md)
- Development plan: [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)
