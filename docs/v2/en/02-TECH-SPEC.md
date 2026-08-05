# Tech Stack

> Push-Point v2.1 — last updated: 2026-07-21

There is exactly one criterion behind v2's technology choices. **Get the best felt performance at personal scale out of a single process and a single binary.** What distributed infrastructure used to buy us — durability, concurrency, search — is redesigned at the level of SQLite and the Go standard library.

## 1. v1 → v2 change summary

| Area | v1 | v2 | Why |
|---|---|---|---|
| Deployment | Minikube + k8s + HPA | A single Go binary (one `just dev`) | Autoscaling with zero users is engineering in reverse. Removes local test friction |
| DB | PostgreSQL (k8s pod) | SQLite (WAL mode) + FTS5 | Fast enough at personal-app scale. Backup = copy the file |
| Message queue | Redis Streams | In-process worker pool (goroutine + SQLite jobs table) | One process needs no network queue. Restart durability is the jobs table's job |
| Object storage | MinIO | Local disk (`data/thumbs/`) | The S3 API is overkill for a few GB of thumbnails |
| AI tagging | OpenAI API | Lightweight NLU (rule-based → ONNX embedding, two stages) | 0 cost, response in the hundreds of ms, privacy. The technical differentiator of this project |
| Client | React Native (undecided) | iOS Share Extension first (SwiftUI) | Past a 2-second save it stops being an app you use every day |
| Auth | JWT + signup | Single user, 1 static API key | Multi-user is an explicit non-goal |

The reason for shrinking the stack is simple. v1 picked components on the premise of "once there are lots of users", and the result was that every development loop had to bring up a k8s cluster, Redis, PostgreSQL and MinIO. v2 flips the premise — **build the app I use every day first, and leave scale as a problem for after there are users.** The components that were removed were not thrown away, they were folded up. The k8s manifests are preserved in `deploy/k8s-future/`, and the core dependencies (Store/Queue/Tagger) sit behind interfaces so the path to swapping implementations stays open (see chapter ten).

## 2. Backend: Go standard library first

- **Language**: Go 1.25+
- **HTTP**: standard `net/http` + the [chi](https://github.com/go-chi/chi) router. Gin is not used — all that is needed is routing and a middleware chain, and chi uses the standard `http.Handler` interface as-is, so there is no framework lock-in.
- **Logging**: standard `log/slog` (JSON handler). zap/logrus are gone — structured logging has been a solved problem in the standard library for a long time.
- **Config**: standard `os.Getenv`, prefix `PUSHPOINT_`. viper is gone — a config framework for five environment variables is overkill.

| Env var | Default | Description |
|---|---|---|
| `PUSHPOINT_ADDR` | `:8420` | Listen address |
| `PUSHPOINT_DATA_DIR` | `./data` | Where the DB and thumbnails live |
| `PUSHPOINT_API_KEY` | (required) | Bearer auth key. `just dev` sets it to `dev-key` |
| `PUSHPOINT_SCRAPE_CONCURRENCY` | `8` | Scraper concurrency ceiling |
| `PUSHPOINT_LOG_LEVEL` | `info` | slog level |

- **No ORM**: Ent is gone. The schema is around 7 tables, so `database/sql` plus hand-written queries reads better and lets SQLite-specific features like FTS5 and `RETURNING` be used directly.
- The entry point is a single `backend/cmd/pushpoint/main.go` — the API server and the workers run in one process. Full structure in [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md).

## 3. Data: SQLite

### Driver — modernc.org/sqlite

[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) is a CGO-free driver: SQLite transpiled to pure Go. Why it was chosen:

- **It keeps the single static binary.** With no CGO, cross-compilation is nothing but `GOOS`/`GOARCH` flags and the artifact is one file. (Footnote: true as of M1–M4 — this may move depending on how M5 decides to ship ONNX. See the three packaging options under Phase B in chapter seven.)
- FTS5 support confirmed — trigram tokenizer included.
- At personal scale (tens of writes per second) the performance difference is not the bottleneck.

> Footnote: if a performance problem is actually measured, mattn/go-sqlite3 (CGO) can take its place. Swapping the driver is one import line and a DSN edit, and since it sits behind the Store interface, nothing above it is affected.

### Connection settings (the basis for the performance)

These PRAGMAs are applied at startup (same numbers as [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md)):

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA cache_size = -64000;   -- 64MB
```

- Connection strategy: **1 writer + a reader pool (N=4)**. Writes serialize, which is not a bottleneck at personal scale, and thanks to WAL reads proceed alongside writes.
- Every write is a transaction. The save API wraps `INSERT link + INSERT job` in a single transaction.
- Data files: `data/pushpoint.db` (+ `-wal`, `-shm`). Backup is a copy of the `data/` directory.

### Full-text search — FTS5 trigram

```sql
CREATE VIRTUAL TABLE links_fts USING fts5(
  title, description, note, tags,
  tokenize = 'trigram'
);
```

The reason for the trigram tokenizer is **Korean substring matching**. The default unicode61 tokenizer splits on whitespace, so the token "쿠버네티스를" in a body does not match a search for "쿠버네티스" (the particle problem). trigram matches on any substring of three characters or more. It is the cheapest way to make Korean search work without a morphological analyzer. Keeping it in sync is the store layer's job, inside the same transaction as the link/tag write.

When a query `q` is shorter than three characters it does not get a 400-response; it is handled by a **LIKE fallback**: a LIKE scan (with ESCAPE handling) over title/note/description in the `links` table, sorted `created_at DESC`, `rank=null`, response `"mode":"like"` (three characters and up go to FTS5, `"mode":"fts"`). A measured full scan over 100k rows came in at 37ms-flat, inside the search budget. Details in [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md).

### Migrations — golang-migrate + embed

The `backend/migrations/` directory is embedded into the binary as an `embed.FS` and applied automatically at startup. There is no separate migration command or deployment step — run the binary and the schema lines up.

## 4. Job queue: SQLite jobs table + goroutine worker pool

What Redis Streams did is replaced by splitting it in two. **Durability is the SQLite `jobs` table's job, concurrency is the goroutine worker pool's.** (Full flow in [04-DATA-FLOW.md](04-DATA-FLOW.md))

1. **enqueue** — the save API commits `INSERT INTO links` + `INSERT INTO jobs(kind='scrape')` in one transaction, notifies the dispatcher over an in-process channel, and immediately returns a 201-Created.
2. **claim** — a worker takes a job atomically with `UPDATE ... WHERE id = (SELECT ... LIMIT 1) RETURNING`. With a single process there is no need for a distributed lock.
3. **Retry** — on failure, if `attempts < max_attempts` (default 3) the job returns to `pending` with linear backoff `run_after = unixepoch() + 30 * attempts`; past that, `failed`.
4. **Chaining** — the transaction that completes a scrape enqueues a `tag` job + (if there is an og:image) a `thumb` job.
5. **Crash recovery** — at startup every job with `status='running'` is put back to `pending`. Unfinished jobs resume even after `kill -9` (M2 DoD). The dispatcher runs a notify channel alongside a 1-second polling ticker so it notices `run_after` coming due.

Concurrency control is solved with `golang.org/x/sync` alone:

- `semaphore` — the scraper concurrency ceiling (`PUSHPOINT_SCRAPE_CONCURRENCY`, default 8)
- `singleflight` — deduplicates concurrent scrapes of the same URL by `url_hash`
- `errgroup` — worker pool lifetime and graceful shutdown

2 goroutines each is enough for the tagger/thumb workers.

## 5. Scraper

- **Parsing**: [goquery](https://github.com/PuerkitoBio/goquery). Extracts `<title>`, og:title / og:description / og:image / og:site_name, meta keywords, article:published_time, author, lang. No colly/chromedp — this is a single fetch plus parse, not a crawler, so `net/http` + goquery is enough.
- **content_type decision**: domain and URL pattern heuristics (youtube/vimeo → `video`, twitter/x → `post`, default `article`).
- **Safety rails**: a 10s context timeout per request, response body capped at 5MB, per-domain rate limit (1 req/s per domain).

### Site adapters

Domains that plain og meta parsing cannot handle branch off into an adapter:

| Domain | Handling |
|---|---|
| youtube.com / youtu.be | oEmbed (`https://www.youtube.com/oembed`, no API key needed) **merged** with og:description from the watch page — oEmbed carries no description. The channel name (author) goes into the tagger's input features |
| x.com / twitter.com | Branches to `publish.twitter.com/oembed` — the body is JS-rendered, so goquery cannot parse it directly |
| blog.naver.com | Rewrite the URL to `m.blog.naver.com` and parse that — the desktop page is an iframe structure |
| instagram.com | Missing meta is tolerated — marked `done` on domain+URL alone |

The "representative domain set" in the M2 DoD is this table — a title obtained from YouTube / a general article / a Naver blog / X.

## 6. Thumbnails

- Download og:image → resize to a max width of 640px → save to local disk as JPEG q80. The path is `data/thumbs/{hash[:2]}/{url_hash}.jpg`.
- **A single size.** v1's three-way resize (small/medium/large) is dropped — the only client surface is one iOS list cell, so size variants are not needed.
- Serving is `GET /thumbs/{path}` — **exempt from auth** (Tailscale is the network boundary and iOS AsyncImage does not support custom headers). It ends at a file server, with no MinIO and no pre-signed URLs.
- The `thumb` job is best-effort — a failure does not affect link status (`thumb_path` just stays NULL).

## 7. NLU tagging pipeline

This is v2's technical differentiator. The principle: **narrow the problem from "generating" free-form tags to "classifying" against a controlled tag dictionary (30–50 entries, user-editable).** It is the only way to get quality without an LLM.

The boundary: runtime inference is `backend/internal/tagger` (Go); model conversion and assets are `nlu/` (Python only under `nlu/models/`). The backend only reads what nlu/ produces (dictionary seeds, .onnx files).

### Phase A — rules + statistics (pure Go, M3)

1. Extract title / meta keywords / og:tags / body from the scrape result (goquery)
2. Domain heuristics: github.com → `dev`, youtube.com → `video`, and so on — a domain-to-tag map
3. **Word normalization (normalize)**: with no morphological analyzer, a normalize function strips a list of representative particle suffixes (을/를/이/가/은/는/의/에/에서/으로/와/과/도/만/보다/부터/까지/처럼/에게/한테/께서 and the like, 20–30 of them) off the end of a word. This normalize is applied **identically on both sides** — `corpus_df` accumulation and tag dictionary matching — because normalizing only one side puts the DF statistics and the match results out of step.
4. **Keyword extraction**: candidate-phrase extraction with separators widened to whitespace + particle suffixes, plus TF-IDF scoring (the `corpus_df` table accumulates document frequency over our own corpus)
5. **Tag dictionary matching rules** (over name/aliases):
   - Hangul entries: **prefix match** after normalization
   - Latin entries shorter than three characters (ai, ml, ui, …): **word-boundary (`\b`) matching is mandatory** — no substring matching
6. Merge scores → top k (≤5), threshold cut → results stored as `link_tags(source='rules', confidence)`

M3 unit test examples:
- "쿠버네티스를 처음 배우는 사람" → matches `kubernetes` (prefix match after stripping the particle "를")
- "he said hello" → does not match `ai` (the substring inside "said" is caught by the word-boundary rule)

### Phase B — embedding classification (ONNX, M5)

> **Correction, 2026-07-26.** Items 1 onward below are the original plan, and measurement broke
> several of its premises. Each item carries its correction. The re-scoping of M5 itself is in
> [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md) §M5.

1. **Model bake-off** — pick by measuring three candidates against the golden set:
   - ~~(a) the multilingual-e5-small-ko family — ONNX provided, 384-dim, **first candidate to examine**~~
     → **ONNX is not provided.** `dragonkue/multilingual-e5-small-ko` has **0** `.onnx` files
     (safetensors only, checked against the HF API). What does ship ONNX is the base
     `intfloat/multilingual-e5-small`, not the Korean-tuned one. **The reason it was the first candidate is gone.**
   - (b) jhgan/ko-sroberta-multitask — **the only candidate that ships an int8**
     (`onnx/model_qint8_avx512_vnni.onnx`). The label the original plan pinned on (a) actually belongs here.
   - (c) BM-K/KoSimCSE — needs a hand-rolled ONNX conversion (`.onnx`: 0 files, checked).
   - ~~only (b)(c) are "110M"~~ → **e5-small is the largest of the three at 117.65M** (the others are 110.62M).
     The int8 files are 112.9 vs 106.2 MiB, so **size does not separate the three candidates.**
   - The axis that does separate them: 81.6% of e5-small's parameters are a 250000-entry vocabulary
     lookup table and the body is only 21.3M, whereas 76.9% of ko-sroberta is body (85.1M), so it does
     roughly 4x the work per token. Write the selection criterion not as "int8 file size" but as
     **body (latency) / vocabulary table (memory)**, two axes.
   - The prefix convention needs correcting too: the original plan said "query:/passage: asymmetry",
     but the model author's README instructs using **`query: ` on both sides** for classification and clustering.
   - `nlu/models/` holds nothing but a `.gitkeep` — whichever path is chosen, a conversion and quantization
     script has to be written. "Only (c) needs conversion" does not hold.
2. **Tokenizer**: `onnxruntime_go` only provides tensor in/out, so a Go tokenizer is selected separately.
   ~~**100% token-ID agreement with Python HF** is the gate for entering M5 Week 2~~
   → **no candidate can reach it.** Measured (123 golden records): sugarme/tokenizer × e5 = 110/123,
   and with whitespace normalization added, 120/123 (the remaining three are a Unigram Viterbi defect
   inside the library, unfixable from the call site). hugot pure Go × e5 = **0/123**.
   And **"(including Hangul NFC normalization)" had the direction backwards** — XLM-R's charsmap has no
   Hangul composition, so on NFD input Go and Python agree 100% *while both are equally broken*.
   **The gate is green and the system is broken.** The real requirement was forcing NFC on the input path,
   and since that is a Phase A defect with nothing to do with Phase B, it was fixed separately on
   2026-07-26 (`tagger.Normalize`).
   → Unless the gate moves from "document-level exact match" to **"fix a whitespace normalization
   convention before tokenizing + token-level agreement rate"**, M5 stops in its second week.
   → Choosing hugot **removes the tokenizer choice** (no injection hook, so it is locked to hftokenizer).
3. ~~Document embedding (title+description)~~ → **the input definition is stale.** `body_text` (0004) and
   `keywords` (0008) are the largest contributing signals today (Δbody +0.066–0.081). What Phase B encodes
   has to be defined again. And **there is no sentence on the tag side to embed** — `tags.json` entries are
   only `{name, facet, aliases}`, and the 42-tag name roster is entirely single words like `dev` and `ai`.
   A prototype-sentence field or an alias-centroid design is a precondition, and the original plan had neither.
4. **Score ensemble with Phase A** — the combination rule was nowhere in the original plan. The current score
   map is an **integer lattice** (weights 1/2/3 × integer match counts) with a 1.0-point threshold, and nothing
   defines the scale, normalization or threshold adjustment for adding a [-1,1] cosine on top. That scale comes
   out of the cosine distribution, so it is **not a Week 3 item but the output of an offline spike**.
   ~~Correct the reranking weights from `tag_feedback`~~ → `tag_feedback` has **0** `removed` rows.
   There is no data to learn from.

**Packaging shape (three options, decided in M5)** — M1–M4 is a CGO-free single static binary, and if M5 adopts ONNX it goes one of the following ways.

**There are two axes to judge on.** Not just the server's single binary but the **`mobile/ppshare` extension
memory budget** (~120MB; the measured table is in [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md) M4) is
looked at alongside it — the dependency chain is `ppshare → tagjob → tagger`, so whatever enters the tagger is
linked into the extension too. If an option the extension must not link is chosen, the split has to be made by
**package layout** (put ONNX in its own package and import it only from `internal/app`, and the extension stays
clean without touching `tagjob`). The module allowlist in `ppshare_test.go` is what holds that boundary.

1. Embed `libonnxruntime` and extract it at startup (accepting cgo) — **correction: the risk is not iOS, it is
   the server.** iOS requires cgo linking anyway, so `GOOS=ios GOARCH=arm64` simply works. What actually breaks
   is the **`GOOS=linux` cross-compile**, and at that point chapter two of this document — "cross-compilation
   with nothing but GOOS/GOARCH" — collapses and each target needs its own C toolchain. And while the extension
   has an allowlist tripwire, **the server's CGO-free property has no counterpart** — it changes silently.
2. ~~hugot pure-Go backend — about 8x slower but inside the 3s budget~~ → **not a candidate.** Measured: token
   agreement 0/123 (point two above); latency is not a constant but a function of length (a 15-token input costs
   24.5x … a 481-token input 43.6x = **a 1.348-second call**); inputs past the 512-token limit fail; and it
   unpacks int8 into float32 on the Go heap, throwing the quantization win away.
3. Keep Phase A (when the gate is not met)

**And this is not three options but two dimensions** — the server axis × the extension axis. **The extension
axis has exactly one option: the extension keeps Phase A.** The int8 weights alone are 106MiB–113MiB while the
extension has ~106MB of headroom, and measured peak RSS after load is 367MB (ONNX Runtime) / 603MB (pure Go),
which is 3~5x the budget. **It does not fit even assuming zero overhead.** So what M5 can actually sell is not
"iOS standalone mode gets Phase A+B too" but **"the same link gets different tags depending on which path saved
it"**, and the number of tests that verify that is currently zero.

### Quality gate

No unmeasured "seems to work". The evaluation protocol:

- **golden set**: a 123-link `nlu/golden/` JSONL built from real saved links (M2 import + stratified sampling of what actual use has piled up). Record schema:

  ```json
  {"url": "...", "snapshot": {"title": "...", "description": "...", "body_text": "...", "keywords": "..."}, "expected_tags": ["..."]}
  ```

  eval makes **0 network calls** — the snapshot is the only input. The evaluation reproduces even when the original page changes or disappears.
- **Metric**: per link, hit = (predicted top-3 ∩ expected_tags) ≥ 1, **top-3 Recall = hits / total**. `just eval` also prints per-tag precision/recall and per-tag attachment frequency as a table.
- **Splits**: dev 77 / test 84 / wild 28 (the plan was a 50/50-split and curation is what produced these numbers. test was expanded from 61 on 2026-07-27; wild is a real-web set). Rule tuning looks at dev only, and the gate verdict comes from the frozen test only.
- **Baseline-relative gate (redefined 2026-07-26)**: always measure a "domain heuristics only" configuration alongside, but **that is not the line the verdict is measured against.**

  The old gate ("+15pp to enter / +10pp to exit") was wrong in two ways. **Entry** inflated the margin —
  the 0.344-recall of domain-only is not the real floor, and a **constant predictor** that reads no content
  at all, `{article, tutorial, dev}`, already scores **0.721** on the frozen test. Phase A's real advantage is
  not +54.1pp but a paired **+16.4pp** (McNemar p=0.0063). **Exit** turned into a demand for a perfect score
  once Phase A passed expectations (the 80% reference) — from 0.885-recall, +10pp is 61/61-perfect.

  And the 123-record golden set cannot decompose that difference: **one item = 1.64pp**, and assuming zero
  regressions, the smallest improvement distinguishable from chance is **five items (≈8.2pp, McNemar exact
  p=0.031)**. A "+2pp improvement" is one item, which is a coin flip.

  - **M5 entry** = Phase A is **significant against the constant predictor on paired samples** (McNemar p < 0.05). Already met.
  - **M5 exit** = the ensemble shows **zero regressions and 5 or more improvements** over Phase A (frozen test).
    The reason it is written in **item counts** rather than absolute percentages is that item counts are the
    unit this sample can actually decompose. Percentages claim a precision that is not there.

Schema and sampling details in [nlu/golden/README.md](../../../nlu/golden/README.md). The tag dictionary definition is committed in `nlu/dictionary/`.

## 8. iOS client

- **Premise**: the Apple Developer Program ($99/year) is **needed for M6, not for M4** (re-examined 2026-07-26).
  Free provisioning expires in 7 days and the M4 DoD is exactly "7 consecutive days", so counting from the day
  it is installed, it fits with zero reinstalls. What the $99-a-year membership actually buys is M6's streak of
  28 days and a daily life without reinstalls. The full arithmetic is in [07-DEPLOYMENT.md](07-DEPLOYMENT.md) §1.
- **SwiftUI.** React Native is excluded from v2 — the priority is making the Share Extension, the core of the save path, native-grade.
- **Share Extension capture policy**: the extension writes **directly** to the App Group's shared SQLite through `mobile/ppshare` and finishes tagging and summarizing in the same process. No network is involved, so there is no in-flight window to lose anything in — the original local queue + POST + drain design was never built because its target disappeared (in [04-DATA-FLOW.md](04-DATA-FLOW.md), the §7.2-original is preserved as the design to revert to and the §7.4-flow is the current implementation). The url_hash idempotency of `POST /api/v1/links` still holds (the web and Shortcuts paths).
- **API key storage**: in standalone mode it is **not stored** — the app generates a random key on every run and hands it to the in-process server (iOS loopback is shared beyond the app sandbox, so a fixed key baked into the code would remove the defense entirely). The extension writes to SQLite directly without going through the server, so it needs no key. The Keychain (App Group shared) is where that server's key goes **when home-server mode is implemented**.
- App screens: list (cursor-paginated infinite scroll), tag filter, search, detail (tag edit = PATCH). API in [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md).

## 9. Development tooling

- **Lint**: golangci-lint (`just lint`)
- **Tests**: standard `testing` + `httptest`. No testcontainers — SQLite is tested in memory or on temp files, so `go test ./...` runs end to end without Docker. The v1 cost of standing up a PostgreSQL/Redis container for every integration test disappears entirely.
- **Benchmarks**: two layers. `just bench-http` is the performance gate that measures save p99 over the real HTTP path (exit 1 when p99 < 50ms is exceeded); `just bench` (`go test -bench=. -benchmem ./...`) is microbenchmarks for search, list and the rest. go test benchmarks only report averages, so they are not a means of judging p99 — bench-http owns that verdict.
- **Profiling**: `net/http/pprof` mounted at `/debug/pprof` by default. Performance targets are verified with a profile, not with a guess.
- **Tagging evaluation**: `just eval` — golden set accuracy (chapter seven).

### The API contract and code generation

The API is contract-first. `api/openapi.yaml` (OpenAPI 3.1) is the machine source of truth for the API, and all server and client code is generated from it. [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) is human-facing commentary and examples; when the two disagree, openapi.yaml wins.

- **backend (M1+)**: [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) **pinned to v2.8.0** — generates the chi server interface plus request/response types into `backend/internal/api/gen/`. The generate set is `types,chi-server,strict-server,spec`. The output is committed and regenerated with `just gen`. v2.8.0 is the version measured to pass OpenAPI 3.1 generation (2026-07-20).
- **ios (M4)**: swift-openapi-generator — Apple's own, URLSession transport. Hand-writing API types is forbidden.
- **frontend**: a pinned [openapi-typescript](https://openapi-ts.dev) — generates `frontend/src/lib/api/schema.d.ts` (`just web-gen`, output committed). The drift guard is `just web-gen-check`, called by the CI web job.
- **Drift prevention**: `just gen-check` — fails if a git diff remains after re-running `just gen`. It blocks commits where the spec and the generated output have come apart, and it is the M1 row of the verification matrix ([08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md), chapter four).

## 10. Future replacement paths: interface design

We shrank, but we did not close the door. The three core dependencies sit behind interfaces, so once there are users, only the implementation is swapped without touching the call sites.

| Interface | v2 implementation | Future replacement candidate | Trigger |
|---|---|---|---|
| `Store` (`backend/internal/store/`) | SQLite (modernc.org/sqlite) | PostgreSQL | Multi-user, more concurrent writes |
| `Queue` (`backend/internal/queue/`) | SQLite jobs table + goroutine pool | Redis-based distributed queue | Splitting workers across processes/nodes |
| `Tagger` (`backend/internal/tagger/`) | rules → onnx, two stages | External model serving / a larger embedding model | When the quality gate justifies it |

v1's k8s manifests are preserved in `deploy/k8s-future/`. The deployment story for that point in time is in [07-DEPLOYMENT.md](07-DEPLOYMENT.md).
