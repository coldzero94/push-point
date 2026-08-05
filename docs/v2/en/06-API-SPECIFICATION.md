# API Specification

> Push-Point v2.1 — last updated: 2026-07-22

> This document is the human-facing commentary and examples. **The machine-readable source of truth is [api/openapi.yaml](../../../api/openapi.yaml)**, and the backend and iOS client code are generated from it. If the two disagree, openapi.yaml wins; when the spec changes, this document is updated in the same commit.

The complete HTTP API of Push-Point v2. The server is one single Go binary (`pushpoint`), and every endpoint is served directly by that process. Schema field definitions are in [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md); the asynchronous processing that follows a save is in [04-DATA-FLOW.md](04-DATA-FLOW.md).

## 1. Basics

- **Base URL**: `http://localhost:8420` (changeable with `PUSHPOINT_ADDR`, default `:8420`)
- **Response format**: JSON
- **Time**: every time field (`created_at`, `published_at`, and so on) is a **unix epoch integer in seconds**. ISO 8601 strings are not used.

### Authentication

A single-user app has no notion of an account. There is 1 static API key (`PUSHPOINT_API_KEY`) and every request carries it in a header. Only `GET /healthz` and `GET /thumbs/{dir}/{file}` are exempt from authentication (why thumbnails are exempt is argued in the thumbnail-serving section).

```
Authorization: Bearer {PUSHPOINT_API_KEY}
```

```bash
curl -H "Authorization: Bearer dev-key" http://localhost:8420/api/v1/links
```

A server brought up with `just dev` has its key set to `dev-key`. In iOS standalone mode the app makes a random key on every launch and hands it to its own in-process server, so there is nothing to keep; the home-server mode key lives in the Keychain (shared through the app group).

### Error format

Every error comes back in the same shape.

```json
{
  "error": {
    "code": "invalid_input",
    "message": "url is required"
  }
}
```

| code | HTTP Status | Meaning |
|------|-------------|------|
| `unauthorized` | 401 | API key missing or mismatched |
| `invalid_input` | 400 | Malformed request (bad URL format, and so on) |
| `not_found` | 404 | No such resource (soft-deleted links included) |
| `internal` | 500 | Server-side error |

These four error codes are all of them. There is no `forbidden`, because there is one user, and no `rate_limit`, because the queue is in-process.

**Two responses hang off every operation.** Every endpoint that requires authentication can return 401 `unauthorized` (the only exemptions are `GET /healthz` and `GET /thumbs/{dir}/{file}`), and 500 `internal` is exempted on `GET /healthz` **alone** — the healthz handler is a single unconditional return, so it has no failure path to begin with. Thumbnail serving is exempt from authentication but not from 500: if `os.Open`/`Stat` fails for a reason other than ENOENT it answers with a 500-response (verified for real against a file it had no permission to read). The contract (`api/openapi.yaml`) declares exactly this boundary through its `Unauthorized` / `InternalError` response component references. The "status codes" line under each endpoint below does not repeat these two every time. A 500-response is the shared terminus for every error a handler failed to handle, and even there the server keeps the same `{error:{code,message}}` shape (it never emits HTML or an empty body).

## 2. Cursor pagination contract

The list (`GET /api/v1/links`) and search (`GET /api/v1/search`) use a **keyset cursor**.

- `cursor` (string, optional): pass the previous response's `next_cursor` through verbatim. Omit it for the first page. It is an opaque token, so the client never interprets or edits its contents. List cursors and search (FTS) cursors have different formats and are not interchangeable — a cursor from the wrong mode, or a malformed one, is a 400 `invalid_input`.
- `limit` (int, default 20, max 100): page size. Below 1 is a 400 `invalid_input`, and anything over the ceiling is clamped to 100.
- `next_cursor` (string | null) in the response: the cursor for the next page. `null` means this was the last page.

```json
{
  "links": [ ... ],
  "next_cursor": "eyJjcmVhdGVkX2F0IjoxNzUyOTgwMDAwLCJpZCI6MTIzNH0"
}
```

OFFSET pagination is not used — OFFSET scans every row it skips, so deeper pages get slower, whereas keyset rides the `(created_at, id)` index and can hold the list API's p99 < 50ms target even at 100k links.

## 3. Health check

### GET /healthz

No authentication. For confirming the process is alive.

**Response** (200 OK):
```json
{"status": "ok"}
```

## 4. Links

### 4.1 Save a link

```
POST /api/v1/links
```

**Request Body**:
```json
{
  "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "note": "주말에 볼 것"
}
```

- `url` (string, required)
- `note` (string, optional): a personal note

**Response** (201 Created):
```json
{
  "id": 1234,
  "status": "pending",
  "created_at": 1752980000
}
```

Behavior: `INSERT INTO links` + `INSERT INTO jobs(kind='scrape')` commit in one transaction and the response goes out immediately. Those two INSERTs are the whole of the synchronous work, which is what makes the 50ms-p99 guarantee possible — and it is the basis for the Share Extension saving with one tap from the share sheet and closing straight away. Scraping, tagging and thumbnails all run as background jobs; progress is read from `status` and `jobs` in the detail response.

**On a duplicate save** (200 OK): if the same `url_hash` (SHA-256(url)) already exists, nothing new is created and the existing id comes back.
```json
{
  "id": 987,
  "duplicate": true
}
```

This API is idempotent on `url_hash` — a client that retries the same request creates no duplicate (a duplicate answers 200 `duplicate:true`). That is the basis for re-sending whatever is left in an offline queue without worry. Saving the URL of a soft-deleted link restores the same row (back to `pending`, `note` replaced, scrape re-enqueued) and answers with a 201-Created just like a new save.

**Client capture fields (optional)** — for a page the server cannot fetch (SPA, bot wall, login wall), a client that has already rendered that page (browser extension, iOS Share Extension) sends the content along with it.

| Field | Cap | Description |
|---|---|---|
| `title` | 512B | title captured by the client |
| `description` | 2048B | description captured by the client |
| `body_text` | 32KB | plain-text body extracted by the client (newlines preserved — the summarizer uses them for sentence boundaries) |
| `keywords` | 512B | **the classification the publisher put on itself** — `meta[name=keywords]`, `news_keywords`, `article:section`, `article:tag` and JSON-LD `articleSection` collected and joined with commas. It is what the site declared, not what we inferred from the body, so the tagger gives it the same weight as the title. **Tagger input only** — it is exposed nowhere in any response |

- Going over a cap is **truncation, not a 400-rejection**. A client has no way to line its rune and byte boundaries up with the server's exactly, so rejecting at the boundary would make a perfectly good capture fail silently.
- When `body_text` is present the server marks the body source as `client` and **no later scrape overwrites the title, description, body or classification.** The scrape still runs, filling in the thumbnail, author and published_at, and even when it fails outright that link stays `done` rather than `failed` (the reason is recorded in `error`).
- When `body_text` is present the **tagging job is created at save time** — it does not wait for the scrape to succeed (this is the very page the scrape fails on).
- **A one-time backfill on the duplicate (200) path**: when the stored body came from the server but the request carries a client body, the title, description, body and classification are filled in (title, description and classification only from what the request supplied), tagging runs again, and a link that was `failed` is lifted to `done`. If the body is already from a client nothing happens, so repeated calls converge on the same state — **this is the path that later fills in a link that was already saved and already failed.**

**Status codes**: 201(new save) / 200(duplicate) / 400(`invalid_input`, url missing or malformed)

### 4.2 List links

```
GET /api/v1/links?cursor=&limit=&tag=&status=
```

**Query Parameters**:
- `cursor`, `limit`: the pagination contract above
- `tag` (string, optional): filter by tag name
- `status` (string, optional): `pending` | `scraping` | `tagging` | `done` | `failed`

**Response** (200 OK):
```json
{
  "links": [
    {
      "id": 1234,
      "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
      "domain": "youtube.com",
      "title": "Go 동시성 패턴 완전 정복",
      "description": "goroutine과 채널을 이용한 워커 풀 구성부터 singleflight까지…",
      "content_type": "video",
      "thumb_url": "/thumbs/a3/a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8.jpg",
      "status": "done",
      "tags": [
        {"id": 3, "name": "dev", "source": "rules", "confidence": 0.82},
        {"id": 7, "name": "video", "source": "rules", "confidence": 0.95}
      ],
      "note": "주말에 볼 것",
      "created_at": 1752980000
    }
  ],
  "next_cursor": "eyJjcmVhdGVkX2F0IjoxNzUyOTgwMDAwLCJpZCI6MTIzNH0"
}
```

- In the list, `description` is cut to a 200-character prefix (the whole of it is in the detail response).
- `thumb_url` is a server-relative path of the form `/thumbs/{dir}/{file}` (path rules in the thumbnail-serving section). `null` when there is no thumbnail.
- `tags[].source` is `rules` | `embed` | `manual`; `confidence` is `null` when the source is `manual`.

**Status codes**: 200 / 400(`invalid_input` — malformed cursor, limit or status)

### 4.3 Link detail

```
GET /api/v1/links/{id}
```

**Response** (200 OK):
```json
{
  "id": 1234,
  "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "domain": "youtube.com",
  "title": "Go 동시성 패턴 완전 정복",
  "description": "goroutine과 채널을 이용한 워커 풀 구성부터 singleflight, errgroup을 활용한 에러 전파까지 실전 예제로 다룬다.",
  "content_type": "video",
  "thumb_url": "/thumbs/a3/a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8.jpg",
  "status": "done",
  "tags": [
    {"id": 3, "name": "dev", "source": "rules", "confidence": 0.82},
    {"id": 7, "name": "video", "source": "rules", "confidence": 0.95}
  ],
  "note": "주말에 볼 것",
  "created_at": 1752980000,
  "author": "Some Channel",
  "published_at": 1752800000,
  "duration_sec": 1420,
  "word_count": null,
  "lang": "ko",
  "summary": "쿠버네티스는 컨테이너를 선언적으로 운영하는 오케스트레이터다.\n파드가 배포의 최소 단위이고, 서비스가 트래픽을 분산한다.",
  "error": "",
  "jobs": {
    "scrape": "done",
    "tag": "done",
    "thumb": "failed"
  }
}
```

- Every field of a list item, plus `author`, `published_at`, `duration_sec`, `word_count`, `lang`, `summary` and `error`. `published_at`, `duration_sec` and `word_count` are `null` when there is no value; `author`, `lang` and `error` are empty strings (matching the NOT NULL DEFAULT '' definitions in the 05 schema).
- `summary` is a 2–3-sentence **extractive summary**: key sentences chosen out of the body and joined by newlines (M5 Phase A — no LLM, the original sentences are selected as they stand, so there is nothing to hallucinate). When the body is thin (under the 200-rune floor), when there are fewer than three prose sentences, or when it is effectively the same as `description`, it is an **empty string**, and the client then draws no summary area at all. **It is carried in neither the list (`Link`) nor search (`SearchResult`)** — a summary is not a substitute for the original, it is help with the "open it or not" call, and the list response stays light.
- `jobs` is the status summary of the jobs attached to this link, `{scrape, tag, thumb: status}`. Each value is `pending` | `running` | `done` | `failed`. A field is omitted when no job of that kind exists yet — the `scrape` job is always created in the save transaction so it is always there, `tag` appears after a successful scrape, and `thumb` only when there is an og:image (M1 has `scrape` only; M2 has no tagger yet, so a successful scrape creates no `tag` job and `links.status` goes from `scraping` straight to `done` — the `tag` field and the `tagging` status are reachable only once the tagger handler is registered in M3). As in the example above, `thumb` can be `failed` while the link `status` is `done` — the thumbnail job is best-effort and its failure does not touch link status (`thumb_url` simply stays `null`).

**Status codes**: 200 / 400(`invalid_input` — non-integer id) / 404(`not_found`)

### 4.4 Update a link (note/tags)

```
PATCH /api/v1/links/{id}
```

**Request Body**:
```json
{
  "note": "다시 보니 워커 풀 파트만 유용함",
  "tags": ["dev", "golang"]
}
```

- `note` (string, optional): replaces the note when present.
- `tags` (array of string, optional): an array of tag **names**. When present it **replaces this link's tags wholesale** — there is no partial add/remove API. `null` or an omitted field keeps the tags; an empty array `[]` is treated as remove-everything.
  - Added tags: stored as `link_tags(source='manual', confidence=NULL)` + a `tag_feedback(action='added')` record
  - Removed tags: deleted from `link_tags` + a `tag_feedback(action='removed')` record

`tag_feedback` is not a plain log; it is the training data used to correct the reranking weights of the M5 embedding tagger. The more the user fixes tags, the better auto-tagging gets, so clients must send tag edits through this API and no other.

**Response** (200 OK): the updated link detail (the same shape as the 4.3-response).

**Status codes**: 200 / 400(a tag name that does not exist) / 404

### 4.5 Delete a link

```
DELETE /api/v1/links/{id}
```

Soft delete — only `deleted_at` is written and the row stays. It is excluded from the list, search and detail from then on. Saving the same URL again (4.1) restores the row.

**Response**: 204 No Content

**Status codes**: 204 / 400(`invalid_input` — non-integer id) / 404

### 4.6 Retry a failed link

```
POST /api/v1/links/{id}/retry
```

Re-enqueues the jobs of a link whose `status='failed'`. The link goes back to `pending` and the worker processes it again from the start.

**Response** (202 Accepted):
```json
{
  "id": 1234,
  "status": "pending"
}
```

**Status codes**: 202 / 400(a link that is not in the `failed` state) / 404

## 5. Tags

Tags are not free strings but a **controlled dictionary** (30–50 seeds to begin with, editable by the user). The NLU tagger only ever classifies against this dictionary, which makes the dictionary-management API the tool for managing tagging quality.

**facet — the classification axis of a tag**

Every tag carries one `facet`: `craft` / `media` / `life` / `neutral` (default `neutral`).

| facet | Meaning | Seed allocation |
|---|---|---|
| `craft` | reference I use directly for the things I make | 18 (`dev`, `golang`, `ios`, `ai`, `design`, and so on) |
| `media` | tags where the format itself is the information — it tells me the time cost of opening it again | 5 (`article`, `video`, `tutorial`, `book`, `podcast`) |
| `life` | the world, everyday life, and myself | 7 (`news`, `science`, `finance`, `career`, `productivity`, `travel`, `life`) |
| `neutral` | not classified yet | not in the 30-tag seed — newly created tags are born here |

**The server owns only "which facet" (data); "what color that facet is" (presentation) is owned by each client.** Color values (hex) are kept out of the contract because color comes in two sets, light and dark, while the contract can give only one — and doing it that way would invert things so that the server knows the token system. Web and iOS map from the same source (`Tag.facet`) into their own platform tokens.

`facet` **lives on `Tag` only and not on `LinkTag` (a tag attached to a link).** The list screen already holds all of `GET /api/v1/tags` for its filter bar, so it can resolve through a `Map<tagId, facet>`; and at the 100k-link target, putting a facet string on every `LinkTag` grows the payload by the number of tags per link. Rendering a tag that is not in the cache as `neutral` is the correct fallback.

### 5.1 Read the tag dictionary

```
GET /api/v1/tags
```

**Response** (200 OK):
```json
[
  {"id": 3, "name": "dev", "aliases": ["개발", "프로그래밍", "coding"], "link_count": 42, "facet": "craft"},
  {"id": 7, "name": "video", "aliases": ["영상"], "link_count": 28, "facet": "media"}
]
```

`link_count` is the number of (non-deleted) links carrying that tag. `facet` is required — the client treats this response as the single source for resolving tag color.

### 5.2 Create a tag

```
POST /api/v1/tags
```

**Request Body**:
```json
{
  "name": "ml",
  "aliases": ["머신러닝", "machine learning"],
  "facet": "craft"
}
```

`facet` is optional; omitted, it is stored as `neutral`.

**Response** (201 Created):
```json
{
  "id": 15,
  "name": "ml",
  "aliases": ["머신러닝", "machine learning"],
  "link_count": 0,
  "facet": "craft"
}
```

**Status codes**: 201 / 400(duplicate name — `name` is UNIQUE case-insensitively — or a `facet` outside the enum)

### 5.3 Update a tag

```
PATCH /api/v1/tags/{id}
```

**Request Body**:
```json
{
  "name": "ml",
  "aliases": ["머신러닝", "machine learning", "딥러닝"],
  "facet": "craft"
}
```

`name`, `aliases` and `facet` are all optional and only the fields passed are replaced. `aliases` is the array holding synonyms and English/Korean spellings, and it is what the rule-based tagger matches against — filling aliases in well is the cheapest way to raise tagging accuracy. Change a `facet` and that tag's chip color changes on every client along with it.

**Response** (200 OK): the updated tag (the same shape as the 5.2 response).

**Status codes**: 200 / 400(duplicate name, a `facet` outside the enum) / 404

### 5.4 Delete a tag

```
DELETE /api/v1/tags/{id}
```

Removes it from the dictionary. The links in `link_tags` are deleted along with it by FK CASCADE.

**Response**: 204 No Content

**Status codes**: 204 / 400(`invalid_input` — non-integer id) / 404

## 6. Search

### GET /api/v1/search

```
GET /api/v1/search?q=&tag=&from=&to=&cursor=&limit=
```

**Query Parameters**:
- `q` (string, required): the query. After trimming surrounding whitespace, an empty string is a 400 `invalid_input`. Its length decides which search path is taken.
  - **Three characters or more**: FTS5 trigram `MATCH` + `bm25` ranking. Korean matches as substrings too, with no morphological analysis. Response `"mode": "fts"`.
  - **Fewer than three characters**: trigram makes FTS5 matching impossible, but rather than reject with a 400-error it falls back to **LIKE** — it scans title/note/description on the `links` table with `LIKE` (`%` and `_` in the query are ESCAPEd) and sorts by `created_at DESC`. Response `"mode": "like"`, `rank` is `null`. A full scan over 100k rows measured 37ms-flat, inside the budget.
- `tag` (string, optional): filter by tag name
- `from`, `to` (int, optional): `created_at` range (unix epoch seconds)
- `cursor`, `limit`: the pagination contract above

At three characters or more the search target is `links_fts` (title, description, note, tags), sorted by FTS5 `MATCH` + `bm25` ranking. Below that it reads the `links` table directly.

**Response** (200 OK): the same item shape as the list (4.2) with `rank` added, plus a top-level `mode` field (`"fts"` | `"like"`) saying which search path was taken.
```json
{
  "mode": "fts",
  "links": [
    {
      "id": 1234,
      "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
      "domain": "youtube.com",
      "title": "Go 동시성 패턴 완전 정복",
      "description": "goroutine과 채널을 이용한 워커 풀 구성부터 singleflight까지…",
      "content_type": "video",
      "thumb_url": "/thumbs/a3/a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8.jpg",
      "status": "done",
      "tags": [
        {"id": 3, "name": "dev", "source": "rules", "confidence": 0.82}
      ],
      "note": "주말에 볼 것",
      "created_at": 1752980000,
      "rank": -7.42
    }
  ],
  "next_cursor": null
}
```

`rank` is the bm25 score — the smaller the value, the more relevant. In a `"mode": "like"` response every item's `rank` is `null` and the ordering is `created_at DESC`. Performance target: < 30ms at 10k links (the FTS5 path).

The FTS-mode cursor is a keyset on the bm25 rank, so a write between pages (save, delete, reindex) can move the FTS page boundary (acceptable at single-user scale).

## 7. Stats

### GET /api/v1/stats

**Implemented** — the iOS stats tab and the rhythm section of web settings both use it in a single call (widget use is M6).

**Response** (200 OK):
```json
{
  "total_links": 1560,
  "links_this_week": 12,
  "by_tag": [
    {"name": "dev", "count": 420},
    {"name": "video", "count": 280}
  ],
  "by_day": [
    {"date": "2026-06-28", "count": 0},
    {"date": "2026-06-29", "count": 3},
    "...",
    {"date": "2026-07-27", "count": 5}
  ]
}
```

**`by_day` guarantees three things** — always **exactly 30 entries** (a day with no saves still gets `count: 0`),
`date` ascending, and **the last element is today in the server's local time**.

The third is the point. Without it the client has to build "today" in its own timezone and match it
against the date strings — and those dates are made in the server's local time. With the guarantee you
**count from the end** and no date arithmetic is needed at all — that is the basis for the streak
calculation giving the same answer on all three clients (iOS, web, `scripts/streak.sh`).

> **Before 2026-07-28 the `GROUP BY` result was handed over as it came.** A day with no saves had no row
> at all, and a client that indexed that array by position was quietly wrong — the five bars of someone
> who saved five times in a month sat **bunched together** at one end and read as "saved a lot
> recently". The web bunched them to the right and iOS to the left, so **two screens drew the same
> response in opposite directions.** Filling was put on the server side because there are three
> consumers — it avoids writing the same code three times in three languages.

`links_this_week` is **the sum of the last seven cells of that window**. That is, not a calendar week
but a rolling seven days ending today, which is why **the screen does not call this value "this
week"** — on every day that is not Sunday the label contradicts the facts (14 §1.3). The field name
stays as it is for contract compatibility.
It used to be `unixepoch() - 7*86400`, a rolling window in **seconds**, and back then the window and the reference point were different things entirely.

**Status codes**: 200 — there is no 404-case. It is an aggregate-only endpoint, so a "missing" state does not exist, and even on an empty DB it answers 200-OK with `total_links: 0` and a 30-slot `by_day` (all 0).

## 7.1 Spreadsheet (Sheets)

### GET /api/v1/sheets

Whether it is connected, and the result of the last sync (`connected`, `sheet_url`, `last_sync_at`, `last_rows`, `last_error`).

### POST /api/v1/sheets/sync

Rewrites the whole archive to the sheet. It is a synchronous call and can take seconds in proportion to the link count —
it is not the save API, so it is not subject to the p99 gate.

**Connecting is not something this API does.** There is a step where Google's approval has to be walked
through in a browser and the server cannot do it on your behalf; `pushpoint sheets-setup` handles that guidance. When nothing is connected, it is a 409-Conflict.

Even when a sync fails it **returns the status with a 200-response** (the reason in `last_error`). Throwing a
500-response and swallowing the reason leaves the screen unable to show what needs fixing. Fuller background in [07-DEPLOYMENT.md](07-DEPLOYMENT.md) §7.1.

---

## 8. Static thumbnail serving

### GET /thumbs/{dir}/{file}

Serves the thumbnail JPEGs under `data/thumbs/` as they are. The path format is `{first two chars of url_hash}/{url_hash}.jpg` (dir = the first two characters, file = `{url_hash}.jpg`), and `thumb_url` in the list and detail responses points at exactly this path.

```
GET /thumbs/a3/a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8.jpg
```

This endpoint is **exempt from authentication** — Tailscale forms the network boundary, and iOS `AsyncImage` does not support custom headers. There is only one thumbnail size (max width 640px, JPEG q80), so there is no v1-style notion of picking a size variant through the path.

**Status codes**: 200(`image/jpeg`) / 404 / 500(the file is there but opening or `stat` failed — only authentication is exempt, not 500)

## 9. Profiling

### GET /debug/pprof/*

Go's standard `net/http/pprof` is mounted by default. Use it to verify a performance target (the save p99 < 50ms, and so on) or to track down a bottleneck.

```bash
go tool pprof http://localhost:8420/debug/pprof/profile?seconds=10
```

It is a local single-user server, so it stays on permanently with no worry about production exposure.

## 10. APIs deleted from v1

The APIs below do not exist in v2. Any reference left in client code or documentation is a removal target.

| v1 API | Reason for removal |
|--------|----------|
| `POST /api/v1/auth/register` | single-user app — signing up is itself a non-goal |
| `POST /api/v1/auth/login` | no need to issue a JWT; replaced by one static API key |
| `POST /api/v1/auth/refresh` | no token expires, so nothing refreshes |
| `POST /api/v1/auth/logout` | no session state on the server, so there is no logging out |
| `GET /api/v1/sync/pull` | the server is the single source of truth — clients always query the API directly |
| `POST /api/v1/sync/push` | multi-device sync needing cross-device conflict resolution is a non-goal |
| WebSocket (`/ws`) | the save API answers inside its 50ms-budget and polling is enough for processing status — holding a connection open costs more than it returns |
| Rate Limiting (429 + `X-RateLimit-*` headers) | the user is me, all one of me — no reason to throttle myself |
| Pre-signed URL (MinIO) | object storage removed; `/thumbs/` served straight off local disk |
