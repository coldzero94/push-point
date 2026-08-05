# Push-Point Documentation

> Push-Point v2.1 — last updated: 2026-07-22

**Push-Point** is a personal link archive: share a YouTube video or a web article, tags get attached automatically, and you find it again by tag or by search.

v2 rests on four values.

- **Fast saves** — the save API's p99 is < 50ms. From the iOS share sheet, one tap and the save is done.
- **Auto-tagging without an LLM** — instead of an external API, a two-stage lightweight NLU pipeline: rule-based → ONNX embedding. 0 cost, an answer in the hundreds of ms. This is the technical differentiator of the project.
- **Privacy** — the links and notes you save never leave for an outside service. Every step of the processing ends locally.
- **A single binary** — the API server and the worker are one Go process. One `just dev` brings the whole thing up. Backup is copying the `data/` directory.

The repo is a monorepo of 5 workspaces: api (the API contract — `openapi.yaml` is the machine source of truth) / backend (the single Go binary) / nlu (offline NLU assets) / ios (SwiftUI) / frontend (the web SPA — a full-feature client on par with iOS).

## Table of contents

| Document | Contents |
|---|---|
| [00-README.md](00-README.md) | This document. Project introduction and the documentation index |
| [01-PROJECT-OVERVIEW.md](01-PROJECT-OVERVIEW.md) | Project goals, user scenarios, why the direction turned to v2 |
| [02-TECH-SPEC.md](02-TECH-SPEC.md) | Technology choices and the reasoning behind them (Go, SQLite, chi, the NLU pipeline) |
| [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md) | The single-process architecture, component layout, SQLite settings |
| [04-DATA-FLOW.md](04-DATA-FLOW.md) | The save → scrape → tag flow, how the job queue behaves, retries and crash recovery |
| [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md) | The whole SQLite schema (links, tags, jobs, FTS5), migrations |
| [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) | REST API specification, auth, error shape, cursor pagination |
| [07-DEPLOYMENT.md](07-DEPLOYMENT.md) | How to run and operate it, environment variables, backup, the `deploy/k8s-future/` preservation policy |
| [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md) | Milestones M1~M6 (six months) and the completion criteria for each stage |
| [09-PLAN-REVIEW.md](09-PLAN-REVIEW.md) | The plan review and its outcome (2026-07-20) — all 8 recommendations applied in v2.1 |
| [10-DESIGN-SYSTEM.md](10-DESIGN-SYSTEM.md) | The design system — tokens (color, type, spacing, motion), component specs, the accessibility bar, the web↔iOS mapping table |
| [11-WEB-UX-SPEC.md](11-WEB-UX-SPEC.md) | The web UX spec — layout, states and responsive behaviour for seven screens, contract field mapping, keyboard shortcuts, implementation order |
| [12-BACKLOG.md](12-BACKLOG.md) | The backlog — candidates to look at **after** the plan (08), the conditions for starting or dropping each, and the reasons behind the things that were considered and cut |
| [13-CLIENT-PARITY.md](13-CLIENT-PARITY.md) | Client parity rules — the procedure for deciding whether a new feature goes to iOS or the web, and the current decision table |
| [14-STATS-REDESIGN.md](14-STATS-REDESIGN.md) | The stats screen redesign plan — measure which claims actually hold at 1–3-saves-a-day, and remove the ones that do not |

## Quick start

```bash
git clone https://github.com/coldzero94/push-point.git
cd push-point
just dev
```

That is all of it. `just dev` brings up the single process (API + worker) with `PUSHPOINT_API_KEY=dev-key`, and the migrations apply themselves at startup.

Save a link:

```bash
curl -X POST http://localhost:8420/api/v1/links \
  -H "Authorization: Bearer dev-key" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "note": "나중에 보기"}'
# 201 {"id": 1, "status": "pending", "created_at": ...}
```

Read the list back (a few seconds later the title, tags and thumbnail are filled in):

```bash
curl -H "Authorization: Bearer dev-key" \
  "http://localhost:8420/api/v1/links?limit=20"
```

## Architecture overview

```
┌─────────────────────────────────────────────────┐
│                push-point (단일 바이너리)          │
│                                                 │
│  HTTP API ──▶ enqueue ──▶ jobs 테이블 (SQLite)   │
│     │                          │                │
│     │                    dispatcher (goroutine) │
│     │                          │                │
│     │              ┌───────────┴──────────┐     │
│     │              ▼                      ▼     │
│     │         scraper pool           tagger     │
│     │         (bounded N)         (NLU 파이프라인)│
│     │              │                      │     │
│     ▼              ▼                      ▼     │
│  SQLite (WAL) ◀── links / tags / FTS5 ◀──┘     │
│  data/thumbs/ ◀── 썸네일                        │
└─────────────────────────────────────────────────┘
        ▲                          ▲
   iOS Share Ext              iOS 앱 (목록/검색)
```

**The data flow in short**:
1. The save API commits the `links` INSERT and the `scrape` job INSERT in one transaction and answers 201 immediately
2. The dispatcher hands the job to a worker goroutine, and the scraper pool fills in the metadata and the thumbnail
3. On a successful scrape a `tag` job is enqueued in turn and the NLU pipeline attaches the tags (save → tagging complete < 3s)
4. The client reads the result through the list/search API — the state transition is `pending → scraping → tagging → done`

For the details see [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md) and [04-DATA-FLOW.md](04-DATA-FLOW.md).

## v1 → v2 change summary

v2 has exactly one principle: **the product before the infrastructure.** Performance is won by the quality of a single-process design, not by distributing it.

| Area | v1 | v2 | Why |
|---|---|---|---|
| Deployment | Minikube + k8s + HPA | A single Go binary (one `just dev`) | Autoscaling with zero users is engineering in reverse. Removes local test friction |
| DB | PostgreSQL (k8s pod) | SQLite (WAL mode) + FTS5 | Fast enough at personal-app scale. Backup = copy the file |
| Message queue | Redis Streams | In-process worker pool (goroutine + SQLite jobs table) | One process needs no network queue. Restart durability is the jobs table's job |
| Object storage | MinIO | Local disk (`data/thumbs/`) | The S3 API is overkill for a few GB of thumbnails |
| AI tagging | OpenAI API | Lightweight NLU (rule-based → ONNX embedding, two stages) | 0 cost, response in the hundreds of ms, privacy. The technical differentiator of this project |
| Client | React Native (undecided) | iOS Share Extension first (SwiftUI) | Past a 2-second save it stops being an app you use every day |
| Auth | JWT + signup | Single user, 1 static API key | Multi-user is an explicit non-goal |

The v1 k8s manifests are not deleted — they move to `deploy/k8s-future/` and are preserved there. This is folding them up, not throwing them away: Store/Queue/Tagger sit behind interfaces, so once there are users only the implementations get swapped.

## Project structure

```
push-point/
├── api/                       # API 계약 (기계 원본)
│   ├── openapi.yaml           # OpenAPI 3.1 — 백엔드·클라이언트가 여기서 생성
│   └── README.md
├── backend/                   # Go 단일 바이너리 (API + worker + NLU 런타임 추론)
│   ├── cmd/pushpoint/main.go  # 단일 진입점
│   ├── internal/
│   │   ├── api/               # HTTP 핸들러 (chi)
│   │   │   └── gen/           # oapi-codegen 생성물 (just gen, 커밋 대상)
│   │   ├── config/            # PUSHPOINT_* 환경 변수 로딩
│   │   ├── store/             # Store 인터페이스 + sqlite 구현
│   │   ├── queue/             # Queue 인터페이스 + sqlite jobs 구현
│   │   ├── scraper/           # fetch + goquery 파싱, singleflight
│   │   ├── safedial/          # SSRF 가드 (사설 대역 다이얼 차단)
│   │   ├── tagger/            # M3: Tagger 인터페이스 + rules / onnx 구현
│   │   ├── thumbs/            # 썸네일 생성·저장
│   │   └── web/               # SPA 서빙 (embed_frontend 태그 전용)
│   ├── migrations/            # SQLite 마이그레이션 (golang-migrate, embed)
│   └── go.mod                 # module github.com/coby/push-point/backend
├── nlu/                       # NLU 오프라인 자산 (런타임 코드 아님)
│   ├── dictionary/            # 태그 사전 정의·시드 (커밋 대상)
│   ├── golden/                # 태깅 품질 golden set (JSONL, 커밋 대상)
│   └── models/                # M5: ONNX 변환 스크립트(Python)·모델 아티팩트
├── ios/                       # M4: SwiftUI 앱 + Share Extension
├── frontend/                  # 웹 SPA (Vite + React 19 + TS) — iOS와 대등한 full-feature 클라이언트
├── docs/
│   ├── README.md              # v1 ↔ v2 문서 인덱스·비교
│   ├── v1/                    # v1 기획서 아카이브 (수정 금지)
│   └── v2/                    # 현행 문서 (단일 진실 원천)
├── deploy/k8s-future/         # v1 k8s 매니페스트 보존 (미사용)
├── CLAUDE.md
└── justfile                   # dev / test / bench / lint / eval (backend 대상)
```

## Performance targets

Measured on a local M-series. Checked at every milestone with the verification commands — the p99 verdict is `just bench-http`, microbenchmarks are `just bench` (the verification matrix is in [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)).

| Metric | Target |
|---|---|
| Save API p99 | < 50ms |
| Save → tagging complete (async) | < 3s |
| Search (FTS5, 10k links) | < 30ms |
| List-scroll API at 100k links | < 50ms |
| Cold start (binary launch → serving) | < 1s |

## Explicit non-goals

These are the things v2 does not do. Not doing them is the decision.

- **k8s / HPA / multi-node** — if users appear, `deploy/k8s-future/` comes back
- **Signup / multi-user** — a single user, authenticated by one API key
- **Depending on an external LLM API such as OpenAI** — the NLU pipeline is this project's identity
- **Android** — judged after iOS has proved itself in real use
