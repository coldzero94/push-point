# Data flow

> Push-Point v2.1 — last updated: 2026-08-03

Every flow happens inside a single process (the `pushpoint` binary).
There is no stretch that crosses the network the way v1 did — API server → Redis → RabbitMQ → Worker —
and the only movement between components is goroutines and SQLite transactions.
The schema and SQL follow [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md), the endpoints [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md).

## 1. Link save flow (end to end)

The synchronous stretch is two INSERTs and nothing more. Everything after that is an asynchronous chain through the jobs table.

```
┌──────────────┐
│ iOS Share Ext│ 공유 시트에서 한 탭
└──────┬───────┘
       │ 1. POST /api/v1/links
       │    Authorization: Bearer {PUSHPOINT_API_KEY}
       │    { "url": "https://youtube.com/watch?v=xxx", "note": "" }
       ↓
┌──────────────┐
│  HTTP API    │ 2. url_hash = SHA-256(url) 계산 → 중복 체크
└──────┬───────┘    SELECT id FROM links WHERE url_hash = ?
       │
       ├─ 중복 있음 → 200 { "id": 41, "duplicate": true } 로 종료
       │
       │ 3. 한 트랜잭션으로 커밋
       ↓
┌──────────────┐   BEGIN;
│ SQLite (WAL) │   INSERT INTO links(url, url_hash, status='pending');
└──────┬───────┘   INSERT INTO jobs(kind='scrape', link_id);
       │           COMMIT;
       │
       │ 4. 인프로세스 채널로 dispatcher notify
       │
       │ 5. 즉시 201 응답 (p99 < 50ms)
       ↓
┌──────────────┐
│ iOS Share Ext│ { "id": 42, "status": "pending", "created_at": ... }
└──────────────┘  응답 확인 후 시트 닫힘 (클라이언트 측 캡처 정책은 §7)

═══════════════ 여기부터 비동기 (같은 프로세스의 goroutine) ═══════════════

┌──────────────┐
│  dispatcher  │ 6. notify 수신 (+ 1초 폴링 티커 병행)
└──────┬───────┘
       │ 7. scraper 워커가 잡을 원자적으로 claim
       ↓
┌──────────────┐   UPDATE jobs SET status='running', claimed_at=unixepoch(),
│ SQLite (WAL) │                   attempts=attempts+1
└──────┬───────┘   WHERE id = (
       │             SELECT id FROM jobs
       │             WHERE status='pending' AND run_after <= unixepoch()
       │             ORDER BY id LIMIT 1
       │           )
       │           RETURNING id, kind, link_id, attempts;
       │
       │ 8. links.status = 'scraping'
       ↓
┌──────────────┐ semaphore(PUSHPOINT_SCRAPE_CONCURRENCY, 기본 8)
│ scraper pool │ 도메인당 1 req/s, singleflight(url_hash),
└──────┬───────┘ context timeout 10s, 본문 최대 5MB
       │
       │ 9. HTTP GET → goquery 파싱
       ↓
  [대상 웹사이트]
       │ - <title>, og:title / og:description / og:image / og:site_name
       │ - meta keywords, article:published_time, author, lang
       │ - 사이트 어댑터 분기:
       │   · youtube.com → oEmbed(https://www.youtube.com/oembed)
       │     + watch 페이지 og:description 병합
       │   · x.com / twitter.com → publish.twitter.com/oembed 분기
       │   · blog.naver.com → m.blog.naver.com으로 재작성 후 파싱
       │   · instagram.com → 메타 부재 허용 (domain + URL만으로 done)
       │ - content_type 판정: youtube/vimeo → video, twitter/x → post,
       │   기본 article
       │
       │ 10. 한 트랜잭션으로 결과 반영
       ↓
┌──────────────┐   BEGIN;
│ SQLite (WAL) │   UPDATE links SET title=?, description=?, domain=?,
└──────┬───────┘     author=?, content_type=?, lang=?, published_at=?,
       │             status='tagging', updated_at=unixepoch();
       │           -- FTS5 동기화 (store 계층, DELETE 후 INSERT)
       │           DELETE FROM links_fts WHERE rowid=?;
       │           INSERT INTO links_fts(rowid, title, description, note, tags);
       │           -- 연쇄 잡 enqueue
       │           INSERT INTO jobs(kind='tag', link_id);
       │           INSERT INTO jobs(kind='thumb', link_id);  -- og:image 있을 때만
       │           UPDATE jobs SET status='done', finished_at=unixepoch();
       │           COMMIT;
       │
       ├──────────────────────────────┐
       │ 11a. tag 잡                  │ 11b. thumb 잡 (best-effort)
       ↓                              ↓
┌──────────────┐              ┌──────────────┐
│   tagger     │              │   thumbs     │ og:image 다운로드
└──────┬───────┘              └──────┬───────┘ → 최대 폭 640px 리사이즈
       │ NLU 파이프라인               │        → JPEG q80 저장
       │ - Phase A (M3): 도메인       │  data/thumbs/{hash[:2]}/{url_hash}.jpg
       │   휴리스틱 + 후보구·TF-IDF   │        → links.thumb_path 갱신
       │   + 태그 사전 매칭           │  실패해도 링크 상태에 영향 없음
       │ - Phase B (M5): ONNX 임베딩  │  (thumb_path만 NULL 유지)
       │   코사인 유사도 + 앙상블      │
       │ → 상위 k(≤5), threshold 컷   │
       │                              │
       │ 12. 한 트랜잭션으로 커밋      │
       ↓                              │
┌──────────────┐   BEGIN;             │
│ SQLite (WAL) │   INSERT INTO link_tags(link_id, tag_id,
└──────────────┘     source='rules'|'embed', confidence);
                   UPDATE links SET status='done', updated_at=unixepoch();
                   -- FTS5 tags 컬럼 재동기화
                   COMMIT;
```

> **M2 interim (no tagger)**: step 10 above through the step-12 commit draws the steady state, the one with a tagger registered (M3 and later). At M2 there is no tagger yet (the 08 milestones), so the successful-scrape transaction **skips** both the `tag` job enqueue and the `status='tagging'` transition and takes `links.status` straight to `done` (the tagger path drawn in steps 11a-12 only goes live from M3). The `thumb` job (step 11b) is still enqueued at M2 whenever there is an og:image. So in the M2 steady state a link moves `pending → scraping → done`, and `tagging` is first reached at M3.

**Timings** (performance targets — the p99 verdict is `just bench-http`, the verification matrix is [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)):
- save request → 201 response: **p99 < 50ms** (the v1 target was < 500ms — losing the network hops took a digit off it)
- save → tagging done (async): **< 3s** (against the 5–15-second round trip in v1, OpenAI included)

## 2. List retrieval flow

```
┌──────────────┐
│   iOS 앱     │ GET /api/v1/links?cursor=&limit=20&tag=dev
└──────┬───────┘
       ↓
┌──────────────┐ 1. Bearer 키 검증 (정적 비교, I/O 없음)
│  HTTP API    │ 2. 커서 디코드 → (created_at, id) 경계값
└──────┬───────┘
       │ 3. keyset 쿼리 (OFFSET 금지)
       ↓
┌──────────────┐   SELECT ... FROM links
│ SQLite (WAL) │   WHERE deleted_at IS NULL
└──────┬───────┘     AND (created_at, id) < (?, ?)      -- 커서 경계
       │           ORDER BY created_at DESC, id DESC
       │           LIMIT 20;
       │           -- idx_links_list(created_at DESC, id DESC) 인덱스 워크만으로 해결
       │           -- tag 필터는 link_tags JOIN
       │
       │ 4. 응답: items[] + next_cursor
       ↓
┌──────────────┐ id, url, domain, title, description(200자 절단),
│   iOS 앱     │ content_type, thumb_url, status,
└──────────────┘ tags[{id,name,source,confidence}], note, created_at
```

v1's Redis cache layer (build a cache key → HIT/MISS branch → TTL → invalidation) was deleted whole.
SQLite in the same process already holds the hot data in memory through its `cache_size = -64000` (64MB) page cache, so a second cache layer on top would buy nothing but invalidation bugs.

Even at 100k rows the keyset cursor reads a 20-row window at the end of the index, so **p99 < 50ms**.

## 3. Search flow

```
┌──────────────┐
│   iOS 앱     │ GET /api/v1/search?q=고루틴 채널&limit=20
└──────┬───────┘
       ↓
┌──────────────┐ q 길이로 분기: 3자 이상 → FTS5, 3자 미만 → LIKE 폴백
│  HTTP API    │ (trigram 토크나이저는 3자 미만을 매칭하지 못함 — 400 거부 아님)
└──────┬───────┘
       │
       ├─ q < 3자 → LIKE 폴백 ("mode":"like")
       │      SELECT ... FROM links
       │      WHERE (title LIKE ? ESCAPE '\'
       │          OR note LIKE ? ESCAPE '\'
       │          OR description LIKE ? ESCAPE '\')   -- %·_ 이스케이프 처리
       │        AND deleted_at IS NULL
       │      ORDER BY created_at DESC LIMIT 20;
       │      -- rank=null. 실측 10만 행 풀스캔 37ms로 예산 내
       │
       │ q ≥ 3자 → FTS5 ("mode":"fts")
       ↓
┌──────────────┐   SELECT l.*, bm25(links_fts) AS rank
│ SQLite (WAL) │   FROM links_fts f
└──────┬───────┘   JOIN links l ON l.id = f.rowid
       │           WHERE links_fts MATCH ?          -- '고루틴 채널'
       │             AND l.deleted_at IS NULL
       │           ORDER BY rank                    -- bm25는 낮을수록 관련도 높음
       │           LIMIT 20;
       │           -- tag/from/to 필터는 WHERE 절 추가, 페이지네이션은 커서 동일
       │
       │ 응답: 목록과 동일 형태 + rank + 최상위 "mode" 필드
       ↓
┌──────────────┐
│   iOS 앱     │ 1만 링크 기준 < 30ms (FTS5 경로)
└──────────────┘
```

The FTS5 index needs no separate sync job — the store layer updates it in the **same transaction** as the links/tags write (§1-step 10 and the step-12 commit), so there is no moment at which search results disagree with the list.

## 4. Tag edit flow

A user's correction is training data. The replacement and the record of it go into one transaction.

```
┌──────────────┐
│   iOS 앱     │ 상세 화면에서 태그 편집
└──────┬───────┘
       │ PATCH /api/v1/links/42
       │ { "tags": ["dev", "go"] }        ← 전체 교체 시맨틱
       ↓
┌──────────────┐
│  HTTP API    │ 현재 태그 집합과 diff 계산
└──────┬───────┘
       ↓
┌──────────────┐   BEGIN;
│ SQLite (WAL) │   -- 제거분: link_tags 삭제 + 피드백 기록
└──────┬───────┘   DELETE FROM link_tags WHERE link_id=42 AND tag_id=?;
       │           INSERT INTO tag_feedback(link_id, tag_id, action='removed');
       │           -- 추가분: 수동 태그로 삽입 + 피드백 기록
       │           INSERT INTO link_tags(link_id, tag_id,
       │             source='manual', confidence=NULL);
       │           INSERT INTO tag_feedback(link_id, tag_id, action='added');
       │           -- FTS5 tags 컬럼 재동기화
       │           COMMIT;
       │
       │ 200 응답 (갱신된 상세)
       ↓
┌──────────────┐
│   iOS 앱     │
└──────────────┘
```

`tag_feedback` is the training data that corrects the reranking weights of the M5 embedding ensemble.
A tag the user deleted loses score; a tag the user attached by hand gains it.

## 5. Failure handling flow

The retry schedule is not a Redis delayed queue — it is the `run_after` column of the jobs table.

```
┌──────────────┐
│ scraper 워커  │ 스크랩 실패 (timeout, 404, 파싱 불가 ...)
└──────┬───────┘
       │
       │ attempts < max_attempts (기본 3) ?
       │
       ├─ YES → 선형 백오프로 재스케줄
       │          ↓
       │   ┌──────────────┐  UPDATE jobs SET
       │   │ SQLite (WAL) │    status='pending',
       │   └──────────────┘    run_after = unixepoch() + 30 * attempts,
       │                       error = '...';
       │   -- 1차 실패 후 30s, 2차 후 60s 뒤 재시도
       │   -- dispatcher의 1초 폴링 티커가 run_after 도래를 감지해 재claim
       │
       └─ NO → 최종 실패
                 ↓
          ┌──────────────┐  UPDATE jobs  SET status='failed', error='...';
          │ SQLite (WAL) │  UPDATE links SET status='failed', error='...';
          └──────────────┘
                 ↓
          ┌──────────────┐ 목록에서 failed 상태로 노출
          │   iOS 앱     │ 사용자가 수동 재시도:
          └──────┬───────┘
                 │ POST /api/v1/links/42/retry
                 ↓
          ┌──────────────┐ 잡 재-enqueue (attempts 리셋,
          │  HTTP API    │ links.status='pending') → 202
          └──────────────┘ 이후는 §1의 6단계부터 동일
```

v1's DLQ (dead letter queue) is gone. A row with `status='failed'` is itself the DLQ, and reading it is a single `GET /api/v1/links?status=failed`.

## 6. Crash recovery flow

This is the case for a broker-less in-process queue being durable. The M2 verification command `just test-crash` (build → fixture server → save → kill -9 → restart → assert everything done) checks this flow automatically.

```
┌──────────────┐
│  pushpoint   │ scrape 잡 3개 running 중
└──────┬───────┘
       │
       X  kill -9  (graceful shutdown 없음)
       │
       │  jobs 테이블에는 status='running' 행 3개가 그대로 남음
       │  (claim이 트랜잭션 커밋이므로 디스크에 있음)
       │
┌──────────────┐
│  pushpoint   │ 재시작 (콜드 스타트 < 1s)
└──────┬───────┘
       │ 1. 마이그레이션 자동 적용 (embed.FS)
       │ 2. 시작 시 복구 쿼리 한 번
       ↓
┌──────────────┐   UPDATE jobs SET status='pending'
│ SQLite (WAL) │   WHERE status='running';
└──────┬───────┘
       │ 3. dispatcher 기동 → 복구된 잡을 §1의 7단계 claim으로 재처리
       ↓
┌──────────────┐
│ scraper pool │ 중단됐던 잡 이어서 처리
└──────────────┘  (스크랩은 멱등 — 같은 URL 재파싱은 같은 결과)
```

There is a single process, so the chance that another worker is holding a running job at restart is zero, which makes it safe to run the recovery query unconditionally.

## 7. Client capture flow

There are two ways a link gets from the phone to the server. Either way the server-side entry point is the same single §1-endpoint, `POST /api/v1/links`.

### 7.1 iOS Shortcut (M1+)

This is the path that starts real phone use with the M1 server alone, no app installed (M1 DoD: 1 real save through a phone shortcut).

```
공유 시트 → 단축어 실행
         → "URL 콘텐츠 가져오기"로 POST /api/v1/links
           (Authorization: Bearer 헤더 포함)
         → 201/200 응답 확인 → 종료
```

### 7.2 Share Extension — the original design (App Group local queue)

> **Never implemented. The §7.4-design replaced it (2026-07-26).** Once the extension writes straight into the
> App Group's shared SQLite there is no POST left to send, and so nothing for a queue or a drain to act on. The
> reason this section stays is that it is **the place to fall back to** — if `0xdead10cc` (suspended while holding
> a file lock) shows up on a real device, the response is to return to the design written here. What follows is
> that original plan.

"Fire the request, then close immediately" loses whatever is in flight, so it is forbidden. Writing to the local queue first is the capture policy.

```
┌──────────────┐
│ iOS Share Ext│ 공유 시트에서 한 탭
└──────┬───────┘
       │ 1. App Group 로컬 큐에 URL 우선 기록 (디스크 커밋)
       │ 2. POST /api/v1/links (timeoutInterval 2~3s)
       │
       ├─ 성공(201/200) → 3a. 큐에서 제거 → 시트 닫힘
       │
       └─ 실패/타임아웃 → 3b. 큐에 잔류한 채 시트 닫힘
       │                  (저장은 이미 로컬에 완료 — 사용자 체감 동일)
       ↓
       4. 본앱 실행 시 + BGTaskScheduler가 큐 드레인
          → 잔류 항목을 POST /api/v1/links로 재시도
          → url_hash 멱등이므로 이미 올라간 항목도
            200 duplicate:true로 안전하게 정리 (중복 생성 0)
```

(The original claim) Even with the server down, a share-sheet save always succeeds within a 2-second budget (it lands in the local queue), and once the server is back the automatic upload loses nothing. **The shipped implementation gets the same guarantee over a shorter §7.4-route** — there is no network, so there is no upload to lose. What makes a retry safe is the `url_hash` idempotency of `POST /api/v1/links` ([06-API-SPECIFICATION.md](06-API-SPECIFICATION.md), the §4.1-clause).

### 7.3 Client-side body capture (browser extension / Share Extension)

There are three kinds of page the server cannot get a body from, however it fetches the URL — **SPAs** (the content is in JS), **bot blocking**,
and **login walls and paid subscriptions**. The user is already looking at that page in their own browser, so instead of the server
scraping behind their back, **the client puts the rendered body into the save request**. All three are solved at once.

```
브라우저 확장(격리 월드) → 현재 탭에서 제목·설명·본문 텍스트·발행자 분류 추출
                        → POST /api/v1/links {url, title, description, body_text, keywords}
                          (API 키는 확장 스토리지에 있고 페이지 JS는 접근 불가)
                        → 서버: body_source='client' 표시
                                + scrape 잡(썸네일·메타용) + **tag 잡 즉시 enqueue**
```

- **Why the tag job is enqueued at save time as well**: the only place a tag job is created is `ApplyScrape` (a successful scrape),
  so on exactly the pages where scraping fails, tags and summary would never appear at all.
- **Precedence**: with `body_source='client'`, a later scrape does not overwrite the three fields or `keywords`. The scrape still runs
  and fills in the thumbnail, author and published_at, and even when it fails for good the link stays `done`.
- **Backfilling a link already saved**: when a duplicate save request carries a client body and the stored body came from the server,
  the three fields are **backfilled once** and tagging runs again (a no-op if the stored body is already a client one).
  Repeated calls converge on the same state, so retry safety holds.
- The server never receives rendered HTML — the client extracts the plain text too and sends that (request size, the
  synchronous-path budget of the save API, and the benefit of adversarial HTML never living in the DB).
- **The capture rule is one file shared across platforms** — `extension/src/extract.js` refers to no platform API; it takes a
  single document, builds the save contract and returns that value as its last expression.
  Chrome's `executeScript` and iOS's `WKWebView.evaluateJavaScript` receive the value the same way, so
  **the M4 Share Extension uses that same file as it is.** Adding a platform means writing transport code and nothing else;
  removing one means deleting that folder. The server does not know which platform sent it.

### 7.3.1 What arrives depends on where the share came from

How far "press share and it just works" goes is decided by **what the source hands over**. A Share Extension
cannot invent anything beyond what the source app passed it.

| Share source | What the Share Extension receives | What you get | `source` |
|---|---|---|---|
| **Safari** | **One JS preprocessing result.** Safari runs the JS named in `NSExtensionJavaScriptPreprocessingFile` under `NSExtensionAttributes` on the page **before** the extension starts, and hands over its return value | **The body too** — point it straight at `extension/src/extract.js` and it captures by the same rules as the web extension | `captured` |
| **Chrome, Firefox and other browsers** | One `public.url` | **The URL only.** Preprocessing is a feature of Safari's share sheet, so it does not run here — title, body and tags all hang on the server scrape, and a site behind a bot wall is saved empty | `url` |
| **Native apps** (Instagram and the like) | `NSItemProvider` items — usually `public.url`, and depending on the app `public.plain-text` or `public.image` as well | Only what that app gave. When a caption comes with it, that is sometimes the only content there is | `url_with_text` |
| **Notes and Messages** | One `public.plain-text` | It finds the URL inside the text (`NSDataDetector`). A scheme-less `example.com` is promoted to `http://` as well | `text_only` |

**Declare preprocessing and Safari gives you the propertyList only** — no separate `public.url` attachment arrives.
So when the JS fails to produce a URL the save fails outright, and all that is left on screen is `URL을 찾을 수 없습니다`.
On 2026-08-03 it failed exactly like that (§7.3.2).

All four branches are pinned by `ios/PushPointTests/SharePayloadTests.swift` — it builds a real `NSItemProvider`
and calls **the same function** the extension calls. Until then this rule had no test at all and the only means of
verification was "share from Safari in the simulator", so the other three branches had never once been checked.

Which branch it was is recorded in `source` in `save-timing.jsonl`. The saved link alone cannot tell them apart
(there is no such column), which is why "how long does the capture path take in real use" had no answer.

Hence three design rules:

1. **Map everything that arrives onto the contract.** Do not look at the URL only — check the text item too and fill it into
   `description`; some apps hand over a caption with it. **Images are not mapped** (decided 2026-07-26):
   `LinkInput` has no place to receive an image and the unit of a save is a URL, so a share carrying only an image has
   nothing to save in the first place. The caption goes to `description` rather than `title` because a title is something the
   scrape can fetch later, and the two should not compete.
2. **A Safari share gets the body through JS preprocessing.** `extract.js` refers to no platform API and returns the payload
   as its last expression, so it can be used **as it is** as Safari's preprocessing file
   (this is where the §7.3-rule — the rule is one file — pays off).
3. **Login walls are solved with an in-app session (an M4 decision).** The app's `WKWebView` has its own cookie store, so once
   the user has logged into that site inside the app, the app can render the URL afterwards and pull the body out. It is the
   only path that can rescue login-walled content arriving through a native app share (Instagram and the like).
   **The principle that the server holds no credentials is unchanged** — the session lives only on the device.

### 7.3.2 2026-08-03 measurement — what the 2.1-second failure actually was

`just save-timing` catches one 2121.5ms-long `failed` record and reports FAIL. The first read of the cause was
"a cluster page has thousands of `div`s, so the capture is slow" — **that was not it.**
That URL was already dead, and the page raises a `현재 유효하지 않은 클러스터입니다` modal.
On a page showing that modal the preprocessing JS never finishes, and since Safari hands over the propertyList only
there is no substitute URL attachment either — the extension's 2.1-second wait ended with nothing to save.

Measuring again against a **live** page of the same kind (an `n.news.naver.com` article):

```
saved  48ms  over=False  tags=5  source=captured   본문 1990자
```

So there are two facts standing right now.

- **The success path is inside budget** — 48ms (a Korean news article, capture included), 91.8ms (go.dev).
- **M4 DoD ① is not closed.** There are failures that take 2.1-seconds, and the gate counts that as over budget
  (by design — a slow failure is a different problem from a slow success). The record does not get deleted to make it
  pass. It cannot be reproduced against a dead URL, so it stays open until the next failure is caught.

One thing this measurement did settle: **the defence inside `extract.js` (keep `location.href` even when the capture
throws) does not save this case.** If the JS itself never starts, the `try` inside it never runs either.
What would save it is `SharePayload` not swallowing the propertyList failure and continuing through the remaining
attachments, and that is worth something only when Safari hands over a URL attachment alongside.

Measured (2026-07-25): even when the server does receive an Instagram post URL, it gets HTTP 200 and a 623KB-response
with zero og metadata, and nowhere in the 493KB of script is there a caption (it is loaded by XHR while logged in). So the
current choice — the adapter does not request it at all — is right, and without 1–3 above this class leaves nothing but a URL.

### 7.4 iOS running without a server (embedded mode) and the save path

iOS can run in two modes — **home server** (a separate Go server + Tailscale) and **embedded standalone** (the backend goes
into the app through gomobile and runs in-process on the phone at `127.0.0.1`). The point of this section is to keep the
save path from forking between the two modes.

**There is exactly one condition for that — the unit of a save is the payload, not the HTTP call.**

```
캡처 규칙(extension/src/extract.js) → {url, title, description, body_text}
                                       └ 이 JSON이 이동·보관·재시도의 단위다
Share Extension (별개 프로세스)
  → App Group 공유 저장소의 큐에 **그 JSON 그대로** 기록하고 즉시 닫힘 (2초, 오프라인 OK)
앱이 큐를 드레인
  ├ 홈서버 모드   → POST /api/v1/links  (바디가 곧 그 JSON)
  └ 내장 모드     → 인프로세스 서버(127.0.0.1)로 같은 POST
                     (또는 gomobile 바인딩으로 store.SaveLink 직접 호출)
```

- **The Share Extension never attaches to a server directly, in either mode.** It is a separate process, so it cannot reach
  the app's in-process server (the app may not even be running), and the home server may be off.
  So the first destination of a save is **the shared SQLite on the device** — no network is involved, which is how
  the zero loss the §7.2-queue was meant to buy holds over a shorter path.

  **The price is that switching modes is no longer one base URL.** The extension knows nothing about the home server and
  always writes to the phone's DB, so pointing only the app's base URL at the home server leaves **saves on the phone and
  the list on the home server**, and the two drift apart. Actually using home-server mode needs a branch in the extension's
  save path as well (restoring the §7.2-queue → drain), and that work has not been done. Home-server mode today is
  **open by design and not working.**
- ~~**The bytes put in the queue are identical to the HTTP body**, so the drain code does not fork by mode.
  Switching modes is one thing only: "where do we POST" (base URL).~~ — **not building the queue broke this claim.**
  The extension always writes to the phone's SQLite, so changing the base URL alone makes saving and reading look at
  different DBs (see the item above). `url_hash` idempotency still holds, but that is the story of the web and shortcut paths.
- **Validation and normalisation are done by `store.SaveInput.Normalize`, and `SaveLink` calls it itself.** Put it in the HTTP
  handler and it is skipped wholesale when the extension calls `SaveLink` directly through the gomobile binding (`ppshare`),
  and the same payload gets stored differently depending on the path. Putting it where the entry point cannot bypass it is the
  premise of this structure.
- In embedded mode, **the choice to bring the in-process server up on loopback** exists to keep the SwiftUI app code
  indifferent to the mode (the client knows only the contract). Even if it changes to calling the binding directly, the
  normalisation guarantee above is unchanged. The implementation is `Start` in `backend/mobile/ppcore` bringing up
  `internal/app` as it is, so standalone mode and server mode serve **the same contract from the same code**.

**The extension's first destination — writing straight to the shared SQLite, not a separate queue file (implementation decision, 2026-07-25)**

The "App Group queue" in the diagram above was implemented as **the shared SQLite itself** rather than a separate file
(`backend/mobile/ppshare`). The extension calls `store.SaveLink` directly and then finishes tagging and summarising in the
same process (measured: the tagger and summariser do not register in the extension's memory budget —
[08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md), M4 preliminary verification). This way the link appears with its tags the
moment it is shared, with no drain step, and the payload is not stored twice.

Only the storage medium changed; the §7.2-guarantee — 0 saves lost — and the forced `Normalize` are as before. If anything,
the intermediate state of "in the queue but not yet in the DB" disappears.

**The remaining risk is an iOS-specific one.** When an app extension is suspended while holding a file lock on the App Group
shared container, iOS kills it with `0xdead10cc`. `ppshare` keeps the connection short to reduce this (`Open` → `Save` →
`Close`), but the risk is not zero. **A real device decides it** — if it becomes a problem, falling back to the original
design in the diagram above (an atomic file-write queue → the app drains it) is the answer, and that path is already
written down here.
- Workers (scrape, tagging, summarising) are subject to iOS background limits in embedded mode — the save is immediate, but
  the enrichment proceeds **only while the app is up** (`Backend` ties the server's lifetime to the foreground).
  The BGTaskScheduler window is not used yet — tags and summary are already finished by the extension at save time, so the
  only thing pushed back is the scrape (title, thumbnail), and that catches up the next time the app is opened. This is why it
  matters that the save itself does not depend on the network.

## 8. Flow summary

| Operation | Sync/async | Target time |
|------|------------|----------|
| Link save (201 response) | sync | p99 < 50ms |
| Save → tagging done | async | < 3s |
| List retrieval (100k rows, keyset) | sync | < 50ms |
| Search (FTS5, 10k links) | sync | < 30ms |
| Tag edit (PATCH) | sync | < 50ms (single transaction) |
| Thumbnail generation | async (best-effort) | independent of link status |
| Failure retry | async | run_after = now + 30 × attempts |
| Crash recovery | once at startup | cold start < 1s included |

v1's sync (pull/push) flow was deleted. With a single user, and the server as the only source of truth, there is no conflict resolution or version management to do, and the iOS app always reads the server directly through cursor pagination. The Redis cache flow does not exist either, for the §2-reason.
