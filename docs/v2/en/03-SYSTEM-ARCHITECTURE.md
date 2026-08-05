# System architecture

> Push-Point v2 — last updated: 2026-07-21

## 1. The whole picture

v2 is a single-binary architecture: the API server and the workers run inside one Go process (`backend/cmd/pushpoint/main.go`). Where v1 brought up API Server / Worker / PostgreSQL / Redis / MinIO as separate k8s pods, every v2 component converges onto goroutines and a single SQLite file. The monorepo (backend / nlu / ios / frontend) is a repository layout and nothing more — the unit of execution is still the one binary that backend builds.

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

### The core flow

One principle sits at the centre of the design. **The save API only enqueues and hands back an immediate 201-Created. All the heavy work is asynchronous.**

1. The iOS Share Extension calls `POST /api/v1/links`.
2. The handler commits `INSERT INTO links` + `INSERT INTO jobs(kind='scrape')` in a single transaction, notifies the dispatcher over an in-process channel, and immediately returns `201 {id, status:"pending", created_at}`. Two INSERTs are the whole of the synchronous stretch — the basis for a sub-50ms p99.
3. The dispatcher claims a job from the jobs table and hands it to the scraper pool.
4. The scrape-success transaction chain-enqueues a `tag` job and, if there is an og:image, a `thumb` job.
5. Once the tagger attaches tags through the NLU pipeline, the link's status becomes `done`. From save to tags-attached, the goal is a sub-3-second turnaround.

> **M2 interim (no tagger)**: step 4 onward is the steady state, the one where a tagger is registered (M3 and later). The tagger arrives in M3 (Phase A) — see the milestones in 08 and the `backend/internal/tagger` section below — so at M2 the scrape-success transaction creates no `tag` job and `links.status` goes straight to `done` (the `thumb` job is still chain-enqueued in M2 when there is an og:image). The M2 steady-state link transition is therefore `pending → scraping → done`, and the `tagging` state is only reachable from M3.

From the client's side it reads as "saving is instant, the rest takes care of itself". The share sheet closing inside two seconds is guaranteed by the extension writing directly to the shared SQLite in the App Group (it holds even with no server — see the iOS section of [02-TECH-SPEC.md](02-TECH-SPEC.md)), and this asynchronous structure is what produces the save API's p99 < 50ms response.

## 2. Repository layout and what each package does

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
│   │   ├── tagger/            # M3: 규칙 태거 (Phase B는 신호 하나로 합류 — 아래 참조)
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

The NLU boundary: runtime inference (the rule tagger, ONNX inference) is Go code in `backend/internal/tagger`. `nlu/` is not code but assets — the tag dictionary definitions, the golden set, the model conversion scripts (Python is allowed only here), the ONNX artifacts — and the backend only reads what nlu/ produces (dictionary seeds, .onnx files).

### backend/internal/api

The HTTP handler layer, standard `net/http` plus the chi router. It owns Bearer API-key auth middleware (`PUSHPOINT_API_KEY`, health check and thumbnails excluded), request validation and response serialisation. It holds no business logic; it calls store/queue. The server interface and the request/response types are generated from the contract source `api/openapi.yaml` (`gen/`, committed) — this is contract-first, so a handler is an implementation of a generated interface. `/debug/pprof` is mounted by default. Full endpoint list in [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md).

### backend/internal/store

The `Store` interface and its sqlite implementation. Every data access for links, tags, search and stats goes through this interface. The driver is modernc.org/sqlite (CGO-free, pure Go), which keeps the binary a single static one. Keeping FTS5 (`links_fts`) in sync is a store-layer responsibility too — it is refreshed with DELETE then INSERT in the same transaction as the link/tag write. Migrations are embedded in the binary with golang-migrate + `embed.FS` and applied automatically at startup.

### backend/internal/queue

The `Queue` interface and the SQLite `jobs` table implementation. It owns enqueue / claim / completion and failure handling / retry backoff (`run_after = unixepoch() + 30 * attempts`, `max_attempts` 3). A dispatcher goroutine runs the notify channel and a one-second polling ticker side by side, picking jobs up and handing them to each worker — the channel is the zero-latency reaction, the ticker is there to notice `run_after` coming due (retry scheduling).

### backend/internal/scraper

URL fetch + goquery parsing. It extracts `<title>`, og:title/description/image/site_name, meta keywords, article:published_time, author and lang, and decides content_type from domain and URL-pattern heuristics (youtube/vimeo → video, twitter/x → post, article by default). YouTube also goes through oEmbed (no API key required). The safety rails:

- `semaphore(PUSHPOINT_SCRAPE_CONCURRENCY, default 8)` — ceiling on concurrent fetches
- a per-domain rate limit (1 req/s per domain) — manners toward the site being fetched
- `singleflight` (keyed on url_hash) — removes concurrent scrapes of the same URL
- a 10s context timeout per request, response body capped at 5MB

### backend/internal/tagger

**There is no Tagger interface** — because the ensemble point is `score map[int64]float64` in `classify.go`. Six signals — domain, title, classification, description, note, body — are already composed additively in that map today, and Phase B (ONNX) enters the same map as **the seventh signal**. Splitting it behind an interface and swapping implementations would mean being able to pick only one of the two signals, and an ensemble is by definition the use of both, so that shape gets in the way instead. The problem is narrowed from free tag generation to classification against a controlled tag dictionary (30–50 tags), which is how quality is reached without an LLM. Phase A is domain heuristics plus candidate-phrase extraction with particle-suffix normalisation, TF-IDF scoring and dictionary matching; Phase B is an ensemble with ONNX embedding cosine similarity. Quality is measured with `just eval` over the golden set in `nlu/golden/`, and the gate is a relative condition — entering M5: Phase A beats the constant predictor (which never looks at the content, test 0.721) significantly on a paired test — met; leaving M5: the ensemble has 0 regressions and 5 or more improvements against Phase A (redefined 2026-07-26 — the smallest unit the 123-item golden set can resolve). Details in [02-TECH-SPEC.md](02-TECH-SPEC.md).

### backend/internal/thumbs

Download the og:image → resize to a maximum width of 640px → save as JPEG q80 at `data/thumbs/{hash[:2]}/{url_hash}.jpg`. One size only (v1's small/medium/large three-way resize is gone). The `thumb` job is best-effort — a failure has no effect on the link's status, it only leaves `thumb_path` NULL. Serving is `GET /thumbs/{path}`.

## 3. High-performance design points — how to get performance without k8s

v1 looked for performance in horizontal scaling (HPA, worker replicas). v2 looks for it in the quality of a single-process design.

### SQLite: WAL + busy_timeout + connection strategy

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA cache_size = -64000;   -- 64MB
```

Connections are **1 writer + a reader pool (N=4)**. Writes are serialised, but at personal scale (tens of saves a second) that is not the bottleneck, and thanks to WAL reads proceed alongside writes. `busy_timeout` chooses waiting over erroring when locks contend. Every write is a transaction.

### The jobs table = a durable queue

One SQLite table takes over what Redis Streams used to do in v1. With a single process there is no need for a network queue, and restart durability is guaranteed by the plain fact that the job sits inside a DB transaction. A claim is one atomic UPDATE statement:

```sql
UPDATE jobs SET status='running', claimed_at=unixepoch(), attempts=attempts+1
WHERE id = (
  SELECT id FROM jobs
  WHERE status='pending' AND run_after <= unixepoch()
  ORDER BY id LIMIT 1
)
RETURNING id, kind, link_id, attempts;
```

Because there is only one writer it runs without contention, and the `idx_jobs_claim(status, run_after)` index keeps the scan at a constant level. Since the `links` INSERT and the `jobs` INSERT are one transaction, the state "the save went through but the job was lost" is impossible by construction — a property that took separate effort in a Redis Streams + PostgreSQL pairing.

### FTS5: no separate search engine needed

title/description/note/tags are indexed into the `links_fts` virtual table (trigram tokeniser, substring matching for Korean) and searched with bm25 ranking. It lives in the same file as the body data and is synchronised in the same transaction, so there is no index lag and no divergence. The target is under 30ms at 10k links.

### keyset pagination

List and search use nothing but `(created_at, id)` cursor-based keyset pagination, no OFFSET. Combined with the `idx_links_list(created_at DESC, id DESC)` partial index, it keeps the scroll API's p99 sub-50ms even at 100k rows. OFFSET gets linearly slower the deeper it goes, so it is banned.

### pprof mounted by default

`/debug/pprof` is always on, and every milestone verifies the performance gate — the p99 verdict is `just bench-http`, microbenchmarks are `just bench`. **Performance claims are backed by measurement** — "it got faster" does not go into a document without a profile and bench numbers.

## 4. Concurrency model

The goroutines inside the one process:

```
main
├── HTTP 서버 (net/http, 요청당 goroutine)
├── dispatcher ×1     # notify 채널 + 1초 티커, jobs claim → 워커 분배
├── scraper pool ×8   # PUSHPOINT_SCRAPE_CONCURRENCY, semaphore 제한
├── tagger ×2         # NLU 파이프라인 (CPU 바운드)
└── thumb  ×2         # 이미지 리사이즈 (best-effort)
```

- **The only coupling between HTTP and the workers is the jobs table.** The channel notify is an optimisation that shaves latency; if one is lost, the ticker picks the job up.
- **graceful shutdown**: on SIGINT/SIGTERM the HTTP server finishes in-flight requests and closes the listener. The dispatcher stops claiming, and each worker aborts its in-flight job through context cancellation. An aborted job is free to stay in `running` — it is recovered on the next start.
- **crash recovery**: one statement at startup, `UPDATE jobs SET status='pending' WHERE status='running'`. Restart after `kill -9` and the unprocessed jobs resume exactly as they were (a verification item in the M2 DoD). No separate mechanism such as Consumer Group pending-entry management is needed.

## 5. Performance targets

On a local M-series machine. The p99 verdict comes from `just bench-http`, microbenchmarks from `just bench` (the verification matrix is in [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)).

| Metric | Target |
|---|---|
| Save API p99 | < 50ms |
| Save → tagging complete (async) | < 3s |
| Search (FTS5, 10k links) | < 30ms |
| List-scroll API at 100k links | < 50ms |
| Cold start (binary launch → serving) | < 1s |

## 6. The scaling path — the scenario where deploy/k8s-future comes back

v1's k8s manifests were not deleted; they are preserved in `deploy/k8s-future/`. This is folding them up for now, not throwing them away. The core storage and queue logic sits behind the `Store` / `Queue` interfaces, so scaling is an implementation swap rather than a rewrite. The tagger scales not through an interface but through **the signal-composition map** (§Tagger above).

| Trigger | What changes | What does not |
|---|---|---|
| Decision to go multi-user | Auth (API key → accounts), a user dimension in the schema | The API shape, the handler structure |
| Write volume passes the single-writer limit | The `Store` sqlite implementation → a PostgreSQL implementation | The `Store` interface and every call site |
| Splitting the workers into a separate process/node | The `Queue` jobs implementation → a network queue implementation, a separate worker binary | enqueue/claim semantics, the job kinds |
| Traffic passes what one node can take | The `deploy/k8s-future/` manifests revived, the process replicated | Application code |

The order matters. Every one of these transitions belongs to the time **after there are users**, and each step proceeds only once the measured bottleneck of the step before it has been confirmed. Until then, a single binary + SQLite is the architecture that is fastest, simplest, and whose backup is copying the `data/` directory.

## Related documents

- [02-TECH-SPEC.md](02-TECH-SPEC.md) — the rationale behind the technology choices, and the NLU pipeline in detail
- [04-DATA-FLOW.md](04-DATA-FLOW.md) — the save → scrape → tagging flow in detail
- [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md) — the full schema definition
- [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) — the API specification
- [07-DEPLOYMENT.md](07-DEPLOYMENT.md) — how to run and deploy
