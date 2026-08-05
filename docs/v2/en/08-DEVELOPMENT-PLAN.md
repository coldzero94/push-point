# Development plan

> Push-Point v2.1 — last updated: 2026-07-26

This document is the official version of the v2 plan the author has settled on. The v1 plan of 8~10 weeks (k8s deployment, OpenAI tagging, sync) is discarded, and a new 6-month plan is built around one goal: "an app I use every day".

---

## 1. What changed from v1 to v2, and why

The core principle: **product before infrastructure.** k8s/HPA/MinIO/Redis are problems you get after you have users. Performance comes from the quality of a single-process design, not from distribution.

| Area | v1 | v2 | Why |
|---|---|---|---|
| Deployment | Minikube + k8s + HPA | one Go binary (`just dev`, once) | autoscaling with zero users is backwards. Removes local test friction |
| DB | PostgreSQL (k8s pod) | SQLite (WAL mode) + FTS5 | fast enough at personal-app scale. Backup = copy a file |
| Message queue | Redis Streams | in-process worker pool (goroutines + a SQLite jobs table) | one process needs no network queue. The jobs table gives restart durability |
| Object storage | MinIO | local disk (`data/thumbs/`) | an S3 API for a few GB of thumbnails is overkill |
| AI tagging | OpenAI API | lightweight NLU (rule-based → ONNX embeddings, two stages) | cost 0, responses in hundreds of ms, privacy. This is the project's technical differentiator |
| Client | React Native (undecided) | iOS Share Extension first (SwiftUI) | if save friction goes past two seconds it never becomes a daily app |
| Auth | JWT + signup | single user, 1 static API key | multi-user is an explicit non-goal |

The k8s manifests are not deleted; they move to `deploy/k8s-future/` and are preserved. **This is folding them away, not throwing them out.** Store/Queue/Tagger sit behind interfaces, so when users show up only the implementations need swapping.

---

## 2. Full schedule: six months

| Stage | Duration | Contents | Definition of done (DoD) |
|---|---|---|---|
| M1 Core | 2 weeks | schema + Store/Queue + save/list API + bench harness + iOS Shortcut capture | `just bench-http` p99 < 50ms and cold start < 1s both pass, 1 real save from the phone Shortcut |
| M2 Scraper | 3 weeks | worker pool + parsing (site adapters) + thumbnails + retry/recovery + bookmark and Takeout import | `just test-crash` passes, title and thumbnail within 3s on the representative domain set, 300+ real links loaded, **daily real use begins** |
| M3 Tagging A + search | 4 weeks | rule tagger (Korean normalization) + tag dictionary + FTS5/LIKE search + eval harness | `just eval` runs, golden set built (dev/test split), measurements recorded against the baseline |
| M4 iOS | 5 weeks | Share Extension + list + search + detail editing + Tailscale on a real device | share-save succeeds in under-2s even with the server offline, 0 losses, 7 days straight with 1+ save a day |
| M5 Tagging B | **rescoped** | ~~4 weeks of monolithic ONNX ensemble~~ → Phase 0 instrumentation (1 week) → Phase 1 offline spike (1 week, disposable by assumption) → Phase 2 conditional integration (2 weeks) | Exit: the ensemble delivers **0 regressions + 5 or more improvements** over Phase A (frozen test). How the original plan was cancelled: §M5 |
| M6 Polish | 4 weeks | widget + performance tuning + a public write-up (Live Activity is a later candidate) | `scripts/streak.sh` shows 4 weeks of continuous daily use, one technical article |
| M-Web Web app | parallel track | Vite+React+TS SPA, consumes the `api/openapi.yaml` contract (openapi-typescript), 6 screens, served via Go embed | `just web-gen-check` drift 0 + `just web-build` succeeds (embedded in the single binary), feature parity with iOS |

**M-Web (the web app)** is a **first-class client on par with iOS (M4)**. It is a parallel track that does not displace iOS, and it covers the real demand for browsing, searching and managing what has been saved. Both clients consume the same `api/openapi.yaml` contract, so their features are the same and **only the two-second entry through the iOS share sheet is iOS-specific** (the web has a URL input field plus an optional bookmarklet). Stack, contract pipeline and embed deployment details are in [02-TECH-SPEC.md](02-TECH-SPEC.md) and `.claude/rules/frontend.md`. If M4's search and detail-editing screens had been cut, the web would have backfilled them, which made that cut safe — in the end nothing was cut and iOS got them too (2026-07-26).

What the ordering means: **daily real use moved forward to the end of M2 (week five).** Shortcut capture (M1) and import plus daily saving (M2) accumulate real data first, and M3~M6 all run on top of that data. M4 is not the start of real use; it is the step that swaps the save path from a Shortcut to the Share Extension and cuts the friction.

### Schedule operating principles

- Capacity assumption: about ten hours a week (adjust to your own situation) — milestone durations are stated against this assumption.
- 22 weeks + **4 weeks of explicit buffer** = six months. The buffer is budgeted into the plan, and **a cut fires once 2 weeks of buffer are gone**.
- Cut order (when late, cut in this order):
  1. Live Activity (already demoted to a post-M6 candidate)
  2. Widget
  3. All of M5 (run on Phase A)
  4. ~~M4 search screen~~ — **did not fire** (shipped 2026-07-26). The next thing to cut moves down one slot
- **The anchor that holds under any circumstance = the start of daily real use (end of M2).** Cuts exist to protect the anchor.

---

## 3. Milestone breakdown

### M1 Core (2 weeks)

`backend/go.mod` was already initialized for v2 (2026-07-20, dependencies 0). M1 is not a reorganization but **new code written under `backend/`** — v1 backend code never existed; there was only a go.mod declaration.

**Week 1**
- Finalize `api/openapi.yaml` (OpenAPI 3.1 — the machine source of truth for the API; [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) is human-facing commentary) plus the oapi-codegen pipeline: `just gen` generates the chi server interfaces and types into `backend/internal/api/gen/` (generated output committed), `just gen-check` catches drift. oapi-codegen is **pinned to v2.8.0** — OpenAPI 3.1 generation verified by measurement (2026-07-20), with strict-server in the generate set
- enum consistency lint script: the enums in `api/openapi.yaml` against the DDL CHECK constraints in [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md)
- `backend/cmd/pushpoint/main.go` as the single entry point + a fresh `backend/internal/...` skeleton
- Write the SQLite schema + golang-migrate migrations (embedded in the binary via `embed.FS`, applied automatically at startup)
- SQLite PRAGMA settings (WAL, synchronous NORMAL, busy_timeout 5000, foreign_keys ON, cache_size 64MB)
- Store interface + sqlite implementation (1 writer + a reader pool of N=4)
- Queue interface + sqlite jobs implementation (enqueue only; workers are M2)

**Week 2**
- `POST /api/v1/links` — the links row and the scrape job INSERT in one transaction, 201 immediately. A duplicate url_hash returns `200 {duplicate:true}`
- `GET /api/v1/links` (keyset cursor pagination), `GET /api/v1/links/{id}`
- Bearer API key auth middleware, `GET /healthz`
- Build the bench harness: `just bench-http` (measures save p99 over the real HTTP path, exits 1 above 50ms), `scripts/coldstart.sh` (launch → `/healthz` 200 < 1s), `just seed 100000` (a Korean/English mixed seed DB for benchmarking, fixed random seed). `just bench` (go test microbenchmarks) stays, but go test benchmarks only report averages, so bench-http owns the p99 verdict
- iOS Shortcut capture: share sheet → "Get Contents of URL" POST (with the Authorization header) registered as a Shortcut → **1 real save succeeds from the phone** (M1 DoD)

### M2 Scraper (3 weeks)

**Week 1**
- dispatcher goroutine (notify channel + a one-second polling ticker) + worker pool
- atomic claim query (`UPDATE ... RETURNING`) and the state transitions

**Week 2**
- goquery scraper: parse title / og:* / meta keywords / published_time / author / lang
- Site adapters: youtube.com → oEmbed + merge the watch page's og:description, and feed the channel name (author) into the tagger's input features / x.com and twitter.com → branch to publish.twitter.com/oembed / blog.naver.com → rewrite to m.blog.naver.com before parsing / instagram.com → tolerate missing metadata (done on domain + URL alone)
- content_type heuristic
- singleflight (dedupes scrapes of the same URL by url_hash), per-domain rate limit (1 req/s), semaphore concurrency cap (default 8), context timeout 10s, 5MB body cap
- Thumbnail worker: og:image → resize to a max width of 640px → JPEG q80, `data/thumbs/{hash[:2]}/{url_hash}.jpg` + served through `GET /thumbs/{path}`

**Week 3**
- Retry with linear backoff (`run_after = unixepoch() + 30 * attempts`, max_attempts 3), `POST /api/v1/links/{id}/retry`
- `running → pending` recovery at startup + `just test-crash` (build → fixture server → save → kill -9 → restart → assert everything reaches done, M2 DoD)
- Bookmark and Takeout import: a one-off script that pushes browser bookmarks (HTML export) and YouTube Takeout into `POST /api/v1/links` → 300+ links of genuine interest loaded (which doubles as corpus_df warming)
- Save from the representative domain set (YouTube / general articles / Naver blog / X) → verify title and thumbnail within 3s
- **End of M2 = daily real use begins, via the Shortcut** (the anchor of the whole schedule)

### M3 Tagging A + search (4 weeks)

**Week 1**
- Seed the tag dictionary (an initial 30~50, name + aliases — the definitions are committed under `nlu/dictionary/`) + tag CRUD API (`/api/v1/tags`)
- Korean eojeol normalization: a normalize function that strips a list of common particle suffixes (을/를/이/가/은/는/의/에/에서/으로/와/과/도/만/보다/부터/까지/처럼/에게/한테/께서 and so on, 20~30 of them) — applied **identically on both sides**, corpus_df accumulation and dictionary matching
- `corpus_df` accumulation pipeline (with normalize applied)

**Week 2**
- Rule tagger: domain-to-tag map → keyword extraction (candidate phrases split on whitespace extended with particle suffixes + TF-IDF scoring, no morphological analyzer) → match against dictionary name/aliases → merge scores → top-k (≤5) + threshold cut
- Matching rules: Hangul entries match by prefix after normalization. Latin entries under three characters (ai, ml, ui and so on) require word-boundary (\b) matching — substring matches are forbidden
- Unit tests: "쿠버네티스를 처음 배우는 사람" matches kubernetes, "he said hello" does not match ai
- Store results as `link_tags(source='rules', confidence)` and wire the tag job into the pipeline

**Week 3**
- FTS5 (trigram) index sync (DELETE then INSERT inside the same transaction as the link and tag writes)
- `GET /api/v1/search` — queries of three characters or more use FTS5 MATCH + bm25 (`"mode":"fts"`), shorter ones fall back to LIKE (`"mode":"like"`), with tag and date-range filters
- `PATCH /api/v1/links/{id}` replaces the whole tag set and records tag_feedback

**Week 4**
- Build the golden set: a 100-link stratified sample (preserving domain and content_type proportions) from what M2 imported and real use accumulated, written as JSONL under `nlu/golden/` — **dev / test split, test frozen** (actual result: dev 62 / test 61 → an 84-entry test set on 2026-07-27, plus wild 28 from a real web set). Schema `{url, snapshot: {title, description, body_text, keywords}, expected_tags: []}` (eval does 0 network access; the snapshot is the only input)
- Implement `just eval`: top-3 Recall (hit = predicted top-3 ∩ expected_tags ≥ 1) + a per-tag precision/recall and attachment-frequency table. Always measure the "domain heuristic only" baseline alongside it
- Tune the rules (using dev only) → **record the measurements against the baseline. The accuracy gate does not block entry into M4 — the verdict moves to the M5 entry condition** (the redefined M5 entry gate — 02-TECH-SPEC.md)

### M4 iOS (5 weeks)

**Pre-flight verification (completed 2026-07-25)** — the two most fragile assumptions were cleared by measurement before starting.

1. **Does the Go backend bind into iOS** — confirmed `.xcframework` generation with `gomobile bind -target=ios`
   (device arm64 + simulator slices). modernc sqlite and go-trafilatura both build for `ios/arm64`.
   Insisting on a CGO-free stack paid off here. For the record, `wazero` does not get linked
   (it is only a go.mod indirect entry — trafilatura's re2go emits generated Go code, not WASM).
2. **Does it fit the extension's memory budget** — see the table below.

**The bind surface is split in two** (`backend/mobile/`). The reason is a measurement — max RSS
when a cold process saves 1 link into a 97MB DB holding 20,000 rows:

| Linked package | Max RSS |
|---|---|
| store + queue only | 13.4 MB |
| **+ scraper** | **64.2 MB** |
| + summarizer | 13.2 MB |
| + tagger | 13.4 MB |

`scraper` is **+51MB-on-link, without ever being called** (the package `init()`s in trafilatura,
readability, domdistiller and htmldate build their regex and selector tables up front). Starting
64MB-deep into the Share Extension budget (~120MB) is dangerous, so:

- **`mobile/ppshare`** (for the extension, 19MB device slice) — `Open`/`Save`/`Close`. It does not
  import scraper. The body text comes out of the DOM via `extract.js`, so it was never needed. tagger
  and summarizer do not register in RSS at all, so they are included, which is why **tags and a summary
  are attached offline at share time** and saved with the link. A test that inspects the dependency
  graph enforces this invariant.
- **`mobile/ppcore`** (for the main app, 49MB) — `Start`/`Addr`/`Stop`. It exposes no individual CRUD
  functions and instead **brings up the in-process chi server on `127.0.0.1:0` and returns only the address.**
  Exporting functions would let the standalone mode's JSON drift away from `api/openapi.yaml` and leave
  Swift with two decoders. This way the app keeps one generated OpenAPI client and only swaps the base URL.
  The wiring lives in `internal/app`, so it is **the same code** as the server binary.
  The app generates a random API key on every launch and passes it in — iOS loopback is shared beyond the app sandbox.

SQLite `cache_size(-64000)` is a ceiling, not a reservation, so the save path's RSS does not climb as the
DB grows — a separate run put a 97MB DB at 13.8MB against the 14.4MB-empty-DB number, and the difference
does not even keep a consistent direction. So **measurement noise is about ±1MB** (the 13.2~13.4 spread in
the table above is the same width), which is why tagger and summarizer can be called "within error" and
`scraper`'s +51MB-jump cannot be confused with noise. The 5 connections × 64MB = 320MB worry was unfounded.

The measurements are on macOS arm64. The Go runtime and the pure-Go dependencies are identical, so they
should carry over, but **once there is an Xcode project this gets re-confirmed on a real device** (a Week 2 item).

**Week 1**
- Join the Apple Developer Program ($99/year — free provisioning expires after 7 days, which is incompatible with daily use)
- ATS decision: use only the IP form for the server address (`http://100.x.y.z:8420`), since IPs are exempt from ATS. Using a MagicDNS name would require HTTPS via `tailscale cert`
- Set up the Xcode project in the `ios/` workspace, the SwiftUI app skeleton, the API client. **The API key is not stored in the Keychain** — standalone mode uses a random key per launch, so there is nothing to keep (:154). The Keychain becomes necessary when home-server mode exists
- The API client is generated from `api/openapi.yaml` with swift-openapi-generator — hand-writing API types is forbidden. Run the CLI and commit the output instead of using the SPM plugin (avoids the clean-build penalty). Decide whether to flatten allOf in the spec after measuring what Swift generates (value1/value2)

**Week 2**
- Minimal Share Extension: one tap to save from the share sheet
- **Input handling per share source** ([04-DATA-FLOW.md](04-DATA-FLOW.md) §7.3.1): a Safari share points
  `NSExtensionJavaScriptPreprocessingFile` at `extension/src/extract.js` and receives the body text too, while a
  native-app share checks every `NSItemProvider` for `public.url` and `public.plain-text` (`public.image` is not
  mapped — the contract (`LinkInput`) has no slot for an image and the unit of saving is a URL) and fills the
  contract fields. Either way it hands **exactly the save contract** to the shared SQLite in the App Group.
- Whether login-walled sites (Instagram and friends) received through a native share can be handled by reusing an
  in-app `WKWebView` session gets decided on a real device (§7.3.1 rule 3). The server holds no credentials.
- **Re-measure extension memory on a real device**: the 13.4MB-number from the pre-flight check is a macOS arm64
  value. Confirm with `os_proc_available_memory()` that the extension really runs inside the jetsam limit, and if it
  falls outside expectations the first response is to drop tagger and summarizer from ppshare (scraper is already out)
- Verify the save path against a simulator + localhost server
  — **passed 2026-07-26**: Safari share → extension → direct save into the App Group SQLite, confirmed on real hardware.
  An 11.8KB-body landed in `body_source='client'` (which means the JS preprocessing really did get through the DOM),
  and tags and a summary were attached in the same process. With ad-hoc signing (`CODE_SIGN_IDENTITY="-"`) the
  simulator keeps entitlements alive, so this entire path can be verified without an Apple account —
  turn signing off (`CODE_SIGNING_ALLOWED=NO`) and the App Group dies with `client is not entitled`.

**Week 3**
- ~~App Group local queue: on share, **write to the queue first** → POST with a 2~3s-`timeoutInterval` → remove from the queue on success; on failure or timeout it stays queued and the sheet closes~~
- ~~Drain the queue on main-app launch and via BGTaskScheduler~~

**Neither of these was implemented — the problem disappeared.** The queue exists to keep a save from being
lost "even if the POST fails", but once the extension writes **directly** to the App Group's shared SQLite
through `ppshare`, there is no POST to send. The save finishes and commits inside the extension process,
tags and summary included, so it completes in airplane mode. There is nothing left for a queue, a drain, or
BGTaskScheduler to do.

The §7.4-entry in [04-DATA-FLOW.md](04-DATA-FLOW.md) records the divergence, and **the condition for
reverting** is there too: if a real device suspends the extension while it holds the App Group file lock and
`0xdead10cc` shows up, the response is to go back to the original queue design.

**Week 4 — three screens (revised 2026-07-26)**

Once the Week 1~3-save-path was actually running in the simulator, using the app with nothing but a list
screen made one thing obvious: **saving works, but the thing is unusable.** The three items below are the
concrete defects that surfaced then — not "make the screens pretty", but three features you cannot work
without.

- **The list has no time axis.** With only title, URL and tags, what was saved yesterday is indistinguishable
  from what was saved last month. In a personal archive the cue for recall is usually "when". Break it up with
  date section headers (today, yesterday, this week, earlier) — the keyset cursor already sorts by
  `(created_at, id)`, so page boundaries and section boundaries line up naturally. Inside a row, put a
  relative time ("three hours ago").
- **Tapping does nothing.** Opening a link means leaving for the original page, which makes "skimming what
  I saved" impossible as an activity. The detail view becomes a **card-news layout**: the 2~3-sentence
  extractive summary gets one card per sentence, alongside the thumbnail, the tags and a link to the
  original. This is the first place a summary built without an LLM shows up on the product surface, so a
  regression in summary quality is immediately visible here.
- **There is no overview of what has been collected.** `GET /api/v1/stats` is already in the contract and
  returns `total_links`, `links_this_week`, `by_tag` and `by_day` (30 days) — no server work, just build the
  screen. Daily bars (Swift Charts), top tags, headline numbers.

Since the number of screens grows, the single `NavigationStack` becomes a **TabView (list, stats)**. Search is
not a tab but `.searchable` inside the list — the thing being searched is that list, so splitting it into a tab
would produce two nearly identical screens, "filtered list" and "search results".

This is where the `allOf` verdict becomes a real problem. The detail screen uses `LinkDetail`, and
swift-openapi-generator wraps it into `value1`/`value2`, giving `detail.value1.title` /
`detail.value2.summary` (measured during pre-flight verification). **Decision: `api/openapi.yaml` is not
touched** — the Go and TS generators handle `allOf` correctly, so there is no reason to flatten a contract
shared by 3 consumers for one generator's ergonomics. A thin forwarding extension on the iOS side
(`var title: String { value1.title }`) confines the problem where it belongs.

**Week 5**
- Configure Tailscale on a real device: VPN On Demand set to Always for both Wi-Fi and Cellular (a required step)
- Verification scenario: share tap → sub-2s response (`just save-timing`), **the share save completes on the spot with 0 losses even when there is no server** (M4 DoD), 7 days straight with 1+ save a day

**Cut candidates revised.** Originally the cut candidates were "the search screen and the detail editing
screen". Real use showed that **the detail view (card news) is not cuttable** — without a way to look at
what you saved, the archive is a trash can. The cut candidates narrowed to **the search screen** (replaced by
tag filters) and **tag editing in the detail view** (keep viewing, defer editing to M5). The stats tab admits
a partial cut: keep the `by_day` bars and fold `by_tag` away.

**Result (2026-07-26): no cut fired.** Search (`.searchable` + `GET /api/v1/search`) and tag and memo editing
in the detail view (`PATCH /api/v1/links/{id}`) both shipped. Tag editing was not deferred to M5 out of screen
greed but because **`tag_feedback` is only produced there** — that is the training data for M5 reranking, and
if editing lives only on the web then no data accumulates on the device where saving actually happens.

**How the share result is shown (settled 2026-07-26).** The extension draws no screen of its own. iOS presents
share extensions as a sheet, so whatever it draws covers the page you were reading, and attempts to shrink the
height with a custom detent did not work because the system owns the sheet size (measured). Instead it
**dismisses immediately after saving and reports the result through a local notification banner** — nothing on
screen is obscured, and you still learn what was saved and which tags were attached. Showing the tags in the
banner is not decoration: it is where the user confirms, every time, the standalone mode's claim that tagging
finished without a server or a network.

### M5 Tagging B — **original plan cancelled, rescoped (2026-07-26)**

> **The original plan (4 weeks of monolithic ONNX ensemble) was cancelled before it started.** The reason
> is not difficulty but **that it cannot be adjudicated.** What follows is the scope re-derived from
> measurements, and every number is reproduced from the 2026-07-26 investigation.

#### Why the original plan was cancelled — three pieces of arithmetic

**(a) The exit gate demands a perfect score.** Frozen test, Phase A = 54/61 = 0.885. "+10pp or better"
means a 0.985-plus score, which the 0.9836-mark of 60/61 misses, so **the only outcome that can pass is
61/61**. The passing combination is `(7 improvements, 0 regressions)` and nothing else. This gate only
holds when Phase A ≤ 0.90-or-lower, and Phase A blew past expectations (the 80% reference figure), which
turned it into **a structure that punishes you for doing well**.

**(b) Most of the misses cannot be fixed by reranking.** Dissect the seven misses on test and **six of them
have the correct tag scoring exactly 0.000** — not below threshold, but no match at all. Only one is fixable
by reordering, so **the reranking ceiling is 55/61 = +1.6pp**.
>
> **[added 2026-07-27] That +1.6pp has already been spent.** The fix for defect E (Korean matching now looks
> at both sides of an eojeol) turned that one rank-slipped case into a hit. The frozen test went
> 54/61 = 0.885 → 55/61 = 0.902-mark, and **all remaining misses score 0 on the gold tag, so the reranking
> ceiling is +0.000**. In other words the conclusion in (b) — "reranking cannot reach the gate" — got
> stronger. The evidence is the "defect E fix" section of `nlu/golden/README.md`. The mode in which the
> original plan's "score ensemble" naturally lives is
reranking, but reaching the gate means **promoting** tags that score 0 above the threshold, which is a
different risk: a flood of false positives. The original plan never distinguished the two modes.

**(c) The tuning set is saturated.** dev Phase A = 59/62, **3 misses**. The Week 3~4-weight-correction has
exactly three observable signals. On top of that, **73% of dev links have identical rank-3 and rank-4 scores**
and break ties by alphabetical tag name; touch the weights and that whole block gets reshuffled, and the
hit@3-metric barely sees it. **We were about to start four weeks with no steering.**

#### Assumptions of the original plan that measurement broke

| What the original plan wrote | Measured |
|---|---|
| (a) multilingual-e5-small-ko — **ONNX provided**, first choice | **0** `.onnx` files (safetensors only). The one that does ship int8 is **(b) ko-sroberta** (`onnx/model_qint8_avx512_vnni.onnx`) — the basis for ranking it first is inverted |
| (b) and (c) only, "110M" | e5-small is **the largest of the three at 117.65M** (the other two 110.62M). The int8 files are 112.9 vs 106.2 MiB, so **size does not separate the three candidates** |
| Deploy option (2) hugot pure Go, "~8x slower but inside the 3s budget" | the tokenizer **silently ignores** the first-choice model's normalizer → golden token agreement **0/123**. Latency is not a constant but a function of length (15-token input 24.5x … 481-token input 43.6x, a **1.35s-latency**), and anything over 512-tokens fails. **Not a candidate** |
| Deploy option (1) cgo — the risk is iOS | iOS requires cgo linking anyway, so it actually works. What really breaks is the **server's `GOOS=linux` cross-compile**, and that is when the 02 §2-claim, "cross-compile with GOOS/GOARCH alone", collapses |
| Entry gate "+15pp over the baseline" (currently +54.1pp) | the real floor is not the 0.344-baseline. **A content-blind constant predictor `{article, tutorial, dev}` scores test 0.721**. Phase A's actual paired-sample advantage is **+16.4pp** (McNemar p=0.0063) — the margin was massively overstated |
| Entry gate "100% token ID agreement" | unreachable with any candidate (best 97.6%). And **"including Hangul NFC" has its direction backwards** — on NFD input Go and Python agree 100% *while both are equally broken*. The gate is green and the system is broken |
| **Phase B in the extension (ppshare) too** | it was never mentioned. The int8 weights alone are 106~113MiB while the extension's headroom is ~106MB-total, and measured RSS after loading is **367MB (ORT) / 603MB (pure Go)** — 3~5x the budget. **Physically impossible** |

As a side effect, one **current defect** unrelated to the original plan turned up: Unicode normalization is
missing in the backend, so on NFD input dev went 0.952 → 0.710 and test 0.885 → 0.689-level. Fixed 2026-07-26
([nlu/golden/README.md](../../../nlu/golden/README.md)).

#### Rescoped — three phases, each with a kill criterion

**Phase 0 — making the invisible visible (roughly 1 week).**

**This is not paying down technical debt.** Debt requires that something was borrowed — that you took speed
now and pay interest later. What is here was **never built as instrumentation in the first place**. Nothing was
borrowed, so there is nothing to repay, and "let's clean it up later" does not apply.

One item does come close to debt: `just eval` was **built at too coarse a grain.** A metric that reports only
hit@3 is structurally blind to the two things below. That much really is a debt taken on at build time.

**These are not preparatory work for M5; they are the basis for deciding whether to do M5 at all.** All three
pay off even if M5 is cancelled:

- **Tie visibility** — `just eval` also reports the rank-3/rank-4 tie rate. Right now 73% of dev links have the
  same score at rank three and rank four, so **they break by alphabetical tag name and the metric cannot see it.**
  That is exactly why E1 (a dictionary expansion that dropped the frozen test by 1.7pp) went unnoticed for three
  days, and touching the dictionary again means missing it again.
- **Miss dissection** — for every miss, report "did the gold tag score zero, or did it get pushed down the
  ranking". That distinction is what revealed the +1.6pp reranking ceiling. **Whatever improvement you attempt
  needs it for aiming**, and without it you stare at numbers with no idea what you fixed.
- **golden round two — 30 real links with no body text.** Right now golden has **0** entries with an empty
  `body_text`. Which means **the already-shipped client capture path (SPAs, bot blocks, login walls) has never
  once been measured.** That is a hole regardless of M5.

- **Kill criterion: none.** All three above pay off even if M5 is cancelled — that is both why this phase comes
  first and why spending time here is not a bet on M5.

> Redefining the gate belonged to this phase but **was finished first, on 2026-07-26** (see "assumptions of the
> original plan that measurement broke" above and [02-TECH-SPEC.md](02-TECH-SPEC.md) §quality gates). Leaving an
> unpassable gate in place drains the meaning out of every plan beneath it, so it was pulled forward.

> ### ⛔ **[2026-07-27] Phase 1 ran → the kill criterion fired → M5 stopped. The third cut (run all of M5 on Phase A) has fired.**
>
> **The spike ran after the axis of judgment moved from tagging to search.** Tagging's reranking headroom was
> confirmed to be zero on all three sets (every remaining miss really does score 0 on the gold tag), so for
> embeddings to pay off the only route is promotion, which is a false-positive risk; search, meanwhile, has eight
> in ten of its misses at the language boundary, which is exactly what embeddings are good at.
>
> **Result**: the 1.0GB model (`paraphrase-multilingual-mpnet-base-v2`) rescued three of 7 language-boundary
> cases and cleared the bar **exactly at the threshold**. But drop to a shippable size
> (`multilingual-e5-small`, fp32 470MB — the real candidate, the int8 118MB-variant, would be worse) and it
> falls **3 → 1** and misses. The small model is actually better at same-language matching
> (14/15 vs 11/15) and **only collapses when crossing languages.**
>
> Size aside, the **CGO-free gate** was still standing — using ONNX from Go requires CGO, and that same
> constraint is what decided the SQLite driver.
>
> **Cost and benefit**: two index fixes already delivered +12pp without a model and without a byte of disk.
> The embedding ceiling is the same +12pp, and at a shippable size it is +4pp.
>
> The full numbers, how to reproduce them, and the conditions for reopening are in
> [`nlu/models/README.md`](../../../nlu/models/README.md). The original plan below is left **exactly as it was
> executed** — what was wagered and what settled it is the record.

**Phase 1 — offline feasibility spike (1 week). 0 code integration, disposable by assumption.**
- Run it in Python only, under `nlu/models/`. **No** Go integration, contract, or migration whatsoever
- Build document and tag embeddings for the 123-entry golden set with the candidate models and measure top-3 Recall **from cosine similarity alone**
- **Kill criteria (any one of these stops M5 and fires cut order three)**:
  - If embedding-only top-3 Recall cannot beat the constant predictor (test 0.721) — it is not a signal worth ensembling
  - If it cannot rescue **at least 3** of the 6 links Phase A scored zero on — it stays trapped under the +1.6pp reranking ceiling
  - If the problem that the 42-tag dictionary has no sentence to embed goes unsolved (today `tags.json` holds only `{name, facet, aliases}` and every name is a single English word)

**Phase 2 — server-side integration (2 weeks). Only if Phase 1 passes.**
- Deployment is **server-side only**. The extension stays on Phase A, settled (see the table above)
- Decide the combination rule first — the current score map is an **integer lattice** (weights 1/2/3 × integer match counts) with a 1.0-threshold, and the original plan has no scale factor or normalization for adding a [-1,1] cosine on top of it. That factor falls out of the Phase 1-cosine distribution, so it is **a Phase 1 deliverable, not a Week 3 item**
- **Kill criterion**: if the frozen test after combination fails the redefined gate, or if the price of losing the server's CGO-free property is judged larger than what is gained

#### Explicit non-scope

- **Phase B in the extension (ppshare)** — impossible per the measurements in the table above. What is needed instead is a test that verifies "the same link gets different tags depending on the save path" (there are currently 0)
- **The hugot pure-Go backend** — token agreement 0/123, and a 1.35s-latency on a 481-token input
- **tag_feedback reranking** — `tag_feedback` has zero `removed` rows. There is no data to learn from. The backlog's `feedback-golden` (turning real-use misclassifications into golden candidates) comes first

#### What survived from the original plan — extractive summarization

Summarization was a Week 3 item in the original plan, but **it was already implemented back in M3 and has
nothing to do with Phase B.** The design record exists nowhere else, so it stays here as it was.

- **Extractive summarization Phase A (implemented, no LLM)**: an extractive summary that picks the 2~3-sentence core of the body — selection of original sentences rather than generation, so hallucination is 0, in pure Go (`backend/internal/summarizer`). **TextRank** (PageRank over a sentence graph whose similarity is lexical overlap) + **description-aware MMR** picks the central sentences while suppressing sentences that overlap the description, so the inspector does not say the same thing twice. It reuses M3's `tagger.Tokenize` (particle normalization), and because the tag job already reads the body, **one pass over the body does tagging and summarization together** (0 extra I/O).
  - Storage and exposure: `links.summary` (migration 0005) + **only `LinkDetail.summary`** in the contract — it is not carried in the list (`Link`) or in search (`SearchResult`). The web draws it only in **the inspector's "Summary" section** (directly above the description) and **does not change the card**: a summary is not a substitute for the original but an aid to the "open it or not" decision, and the list response stays light. Because the contract is narrow, the list and search store paths (`linkCols`, `scanLink`, `sqlite_search.go`) are not touched by a single character. Not indexed in `links_fts` (risk of rebuilding the virtual table — revisit in stage 2 below).
  - A 5-layer quality guard: a 200-rune floor on the body / a 3-sentence floor on prose / a per-sentence prose gate (drops tables of contents, code, email lists) / a 450-rune total cap / total discard if the overlap with description is 0.8 or higher. Failing any of them yields an empty string and the UI draws nothing.
  - Measurement (`just eval-summary`, 50 each from golden dev/test): **there is no reference summary, so ROUGE is impossible**, so only relative gates against a lead-3 baseline are applied (description overlap, tag signal retention, determinism). The measured values are recorded in [nlu/golden/README.md](../../../nlu/golden/README.md).
  - **stage 2 candidates** (out of scope here): swap sentence similarity for Phase B embeddings (the skeleton is unchanged — only `similarity()` changes), index summary into `links_fts` (thanks to desc-aware MMR most summary tokens fall outside the description, so it is a genuinely new search surface), backfill the card body slot (`description || summary`) — the revisit trigger is "links with an empty description exceed 25% or summary coverage exceeds 85%".

  - **stage 2 status (2026-07-26)**: indexing summary into `links_fts` is **done** (backlog B1,
    the gate passed at 87% — a 107-link majority of the 123 golden entries gets a 3-gram that exists only in the
    summary). Swapping sentence similarity for Phase B is a question for after Phase 2 of the rescope above.

### M6 Polish (4 weeks)

**Week 1~2**
- iOS widget (based on `GET /api/v1/stats`). Live Activity is excluded from M6 scope — moved to "post-M6 candidate"

**Week 3**
- Performance tuning: `/debug/pprof` profiling, check for regressions with `just bench-http`

**Week 4**
- ~~`scripts/streak.sh`~~ — **implemented 2026-07-26** (`just streak`). The streak definition matches iOS `StatsView` (if nothing is saved today yet, counting starts from yesterday). All that remains in M6 is the verdict
- Publish one technical article on "automatic tagging without an LLM" — just editing the notes accumulated since M5

---

## 4. Milestone × verification command

| Milestone | Verification command | Passing condition |
|---|---|---|
| M1 | `just bench-http` / `scripts/coldstart.sh` | save p99 over the HTTP path < 50ms (exit 1 if exceeded) / launch → /healthz 200 < 1s |
| M1 | `just seed 100000` | generates a Korean/English mixed seed DB for benchmarking (fixed random seed) |
| M1 | `just gen-check` | generated-output drift 0 (no git diff after re-running `just gen`) |
| M2 | `just test-crash` | build → fixture server → save → kill -9 → restart → assert everything reached done |
| M3 | `just eval` | report recorded against the baseline (the gate verdict happens at M5 entry) |
| M4 | simulator share procedure + `just save-timing` | share tap → sub-2s response (exit 1 if exceeded), save succeeds with 0 loss even when the server is offline |
| M5 | `just eval` (frozen test) | Entry: McNemar p<0.05 against the constant predictor (0.721) — **met**. Exit: 0 regressions + 5 or more improvements |
| M6 | `scripts/streak.sh` (GET /api/v1/stats by_day) | count > 0 for 28 days straight |

`just save-timing` reads the `save-timing.jsonl` the Share Extension accumulates in the App Group
(`ios/PushPointShare/SaveTiming.swift`). The share itself is a procedure a human walks through, so it was not
automated, and **only the verdict** is scripted — successes and failures are counted separately because a slow
failure (usually a timeout) is a different problem from a slow success, and mixing them just makes the average
look good.

The bench-http / test-crash / seed recipes are already in the justfile under the existing guard pattern (the "enabled in M1/M2" notice). `just bench` (go test microbenchmarks) stays, but **go test benchmarks only report averages, so they are not a p99 instrument — bench-http owns the p99 verdict.**

---

## 5. Quality gate principles

"It seems to work" without measurement is forbidden. Pass or fail is decided by numbers.

- Performance gates: `just bench-http` save p99 < 50ms, `scripts/coldstart.sh` cold start < 1s, search (10k links) < 30ms, a 100k-row list < 50ms. These are the passing conditions for their milestones
- The tagging gates are **relative conditions**. The absolute figures (60% / 80%) are references only, not the verdict:
  - M5 entry = Phase A is significant on a paired sample against the **constant predictor** (content-blind, test 0.721) — **met**
  - M5 exit = the ensemble delivers **0 regressions + 5 or more improvements** over Phase A (redefined 2026-07-26)
- dev/test separation and freezing: the golden set is split into dev / test (actual: dev 62 / test 61). Rule tuning uses dev only, and gate verdicts are made on the frozen test alone
- M3's tagging measurement is a record and does not block entry into M4 — the moment of judgment is M5 entry

---

## 6. Risk management

| Risk | Response |
|---|---|
| Korean tagging quality falls short (Phase A not significant against the constant predictor) | shrink the tag dictionary to lower classification difficulty, and accumulate manual correction data (tag_feedback) as material for M5 reranking |
| Go tokenizer token mismatch (ID sequence differs from Python HF) | make a golden test of 100% token ID agreement (including Hangul NFC normalization) the M5 Week 2 entry gate. If the mismatch is not resolved, swap the candidate tokenizer (sugarme/tokenizer ↔ hugot) |
| ONNX deployment complexity (dynamic dylib linking destroys the single static binary) | the M5 Week 2-decision picks one of three: embed the dylib and extract at startup (accept cgo) / hugot pure-Go backend (~8x slower but inside the async 3s budget) / stay on Phase A |
| ONNX Go bindings turn out hard (onnxruntime_go build and compatibility problems) | hold on Phase A (rule-based). Phase A quality is usable in practice, and all of M5 is the third cut |
| Scraping blocked (bot blocks, empty responses) | fix up the User-Agent, per-domain rate limit (1 req/s), retry with backoff. If it still fails, mark it failed and retry manually through the retry API |
| Lack of iOS development experience | M4 was given a generous five weeks. Core first, in order: ADP and ATS decisions → Share Extension → list. Search and detail editing were cut candidates but **shipped without a cut** (2026-07-26) |
| Loss of interest (the biggest risk in a side project) | **daily real use starts in week five (end of M2)**, moved forward deliberately. The moment it becomes an app used every day, motivation sustains itself |

---

## 7. Explicit non-goals (what v2 does not do)

- k8s / HPA / multi-node — revive `deploy/k8s-future/` if users appear
- signup / multi-user — single user, authenticated with one API key
- dependence on external LLM APIs like OpenAI — the NLU pipeline is this project's identity
- Android — decide after iOS is validated in real use
- (updated 2026-07-21: the web frontend is removed from the non-goals — promoted to a first-class client on par with iOS, see M-Web below. Background in [09-PLAN-REVIEW.md](09-PLAN-REVIEW.md))

---

## 8. Post-M6 candidates

All of these are considered only after real-use validation (4 weeks of continuous daily use) is complete.

1. Live Activity — excluded from M6 scope (the first cut). Reconsider if widget use proves the need
2. Android — after the saving habit has settled on iOS
3. Multi-user — when it is a thing worth recommending to someone else. Handled by swapping the implementations behind the Store/Queue/Tagger interfaces and reviving `deploy/k8s-future/`

Feature candidates are managed separately in [12-BACKLOG.md](12-BACKLOG.md) (created 2026-07-26). If the three
items above are "directions", that document holds **concrete candidates with start and kill conditions**. The
four that are alive right now:

| Candidate | Gist | Start condition |
|---|---|---|
| `scripts/streak.sh` | the M6 Week 4-line already names the file and it does not exist — a streak-day judge | none (an M6 deliverable) |
| Search instrumentation harness | **the repo has no command that measures** the search p99 < 30ms gate. `nlu/golden/search.jsonl` + `just eval-search` + `just bench-search` | whenever there is an appetite to actually touch search |
| Summary into the FTS index | there is a measurement that `summary` picks sentences that do not overlap desc (overlap 0.10~0.13), so the new search surface is real | the share of links in the 123-link golden set that gain a "3-gram only in the summary" reaches **30% or more** |
| `links.opened_at` | the last of the core loop's five stages (revisiting) is the only one with zero instrumentation | none |

**The more valuable section of that document is not the third but the fourth** — it holds the twenty
candidates that were reviewed and cut, each with its reason, and preventing the same idea from being
re-litigated is that document's main reason to exist. When a new feature occurs to you, look at the fourth
section before the third.

---

Related documents: [02-TECH-SPEC.md](02-TECH-SPEC.md), [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md), [07-DEPLOYMENT.md](07-DEPLOYMENT.md), [12-BACKLOG.md](12-BACKLOG.md)
