# Data schema

> Push-Point v2.1 — last updated: 2026-07-22

v2's store is a single SQLite file (`data/pushpoint.db`). v1's PostgreSQL schema (users, notes, sync_logs, user_stats, stored_images, triggers) was thrown out entirely. There is one user, so `users` is unnecessary, and the memo (v1's `notes` table) was absorbed into the `links.note` column. The queue (v1's Redis Streams) is replaced by the `jobs` table, and object storage (v1's MinIO) by the `data/thumbs/` directory.

Related documents: [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md), [04-DATA-FLOW.md](04-DATA-FLOW.md), [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md)

---

## 1. ERD

```
                    ┌────────────┐
                    │    tags    │
                    └─────┬──────┘
                          │
      ┌───────────────────┼───────────────────┐
      │                   │                   │
┌─────┴──────┐      ┌─────┴──────┐     ┌──────┴────────┐
│ link_tags  │      │tag_feedback│     │tag_embeddings │
└─────┬──────┘      └─────┬──────┘     │ (M5, 캐시)     │
      │                   │            └───────────────┘
      │  ┌────────────────┘
      │  │
┌─────┴──┴───┐        ┌────────────┐
│   links    │───────<│    jobs    │
└─────┬──────┘        └────────────┘
      │
      │ rowid = links.id (store 계층이 동기화)
┌─────┴──────┐        ┌────────────┐
│ links_fts  │        │ corpus_df  │  ← 독립 테이블 (TF-IDF 문서 빈도)
│ (FTS5 가상) │        └─────┬──────┘
└────────────┘              │ link_terms가 원장 (links —< link_terms)
                      ┌─────┴──────┐
                      │ link_terms │
                      └────────────┘
```

- `links —< link_tags >— tags`: N:M. Tags are a controlled dictionary (an initial seed of 30~50), and `link_tags` keeps the attachment source (`source`) together with the confidence.
- `links —< jobs`: one link carries a chain of `scrape` / `tag` / `thumb` jobs.
- `links —< tag_feedback`: the history of the user adding and removing tags. Training data for M5 reranking.
- `links_fts`: an FTS5 virtual table. Joined by the `rowid = links.id` convention, not by a foreign key.
- `links —< link_terms`: the dictionary surfaces each link contributed to `corpus_df`. The ledger that lets a retag cancel out its previous contribution.
- `corpus_df`, `tag_embeddings`: auxiliary tables with no relation (statistics and cache for the tagging pipeline). `corpus_df` is the sum of `link_terms`.

---

## 2. Full DDL

Migrations are embedded in the binary with golang-migrate + `embed.FS` and applied automatically at startup (`backend/migrations/`). Every timestamp is an `INTEGER` unix epoch second (`unixepoch()`); deletion is soft, via `deleted_at`.

```sql
CREATE TABLE links (
  id           INTEGER PRIMARY KEY,
  url          TEXT NOT NULL,
  url_hash     TEXT NOT NULL UNIQUE,          -- SHA-256(url) hex, 중복 저장 방지
  domain       TEXT NOT NULL DEFAULT '',
  title        TEXT NOT NULL DEFAULT '',
  description  TEXT NOT NULL DEFAULT '',
  author       TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL DEFAULT 'other'  -- 'video' | 'article' | 'post' | 'other'
    CHECK (content_type IN ('video','article','post','other')),
  lang         TEXT NOT NULL DEFAULT '',
  published_at INTEGER,
  duration_sec INTEGER,
  word_count   INTEGER,
  thumb_path   TEXT,                          -- data/thumbs/ 이하 상대 경로
  note         TEXT NOT NULL DEFAULT '',      -- 개인 메모 (단일 사용자 → 별도 테이블 불필요)
  status       TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','scraping','tagging','done','failed')),
  error        TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL DEFAULT (unixepoch()),
  updated_at   INTEGER NOT NULL DEFAULT (unixepoch()),
  deleted_at   INTEGER,
  body_text    TEXT NOT NULL DEFAULT '',        -- 0004에서 ALTER ADD COLUMN (그래서 맨 뒤).
                                                  -- 본문 추출(go-trafilatura). 태거·요약 입력 전용 — FTS·API 미노출
  summary      TEXT NOT NULL DEFAULT '',        -- 0005에서 ALTER ADD COLUMN. 추출식 요약(M5 Phase A)
  body_source  TEXT NOT NULL DEFAULT ''          -- 0006. '' | 'server' | 'client'
    CHECK (body_source IN ('', 'server', 'client')),
  keywords     TEXT NOT NULL DEFAULT '',         -- 0008. 발행자 분류(meta keywords·article:section 등). 태거 입력 전용
  opened_at    INTEGER                            -- 0010. 마지막 열람 시각. 한 번도 안 열었으면 NULL
);
CREATE INDEX idx_links_unopened ON links(created_at DESC, id DESC)
  WHERE deleted_at IS NULL AND opened_at IS NULL;
CREATE INDEX idx_links_list   ON links(created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_links_status ON links(status) WHERE deleted_at IS NULL;

CREATE TABLE tags (                            -- 통제된 태그 사전 (초기 30~50개 시드)
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
  aliases    TEXT NOT NULL DEFAULT '[]',       -- JSON 배열: 동의어/영문·한글 표기
  created_at INTEGER NOT NULL DEFAULT (unixepoch()),
  facet      TEXT NOT NULL DEFAULT 'neutral'   -- 0003에서 ALTER ADD COLUMN (그래서 맨 뒤)
    CHECK (facet IN ('craft','media','life','neutral'))
);

CREATE TABLE link_tags (
  link_id    INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  tag_id     INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
  source     TEXT NOT NULL CHECK (source IN ('rules','embed','manual')),
  confidence REAL,                              -- manual이면 NULL
  created_at INTEGER NOT NULL DEFAULT (unixepoch()),
  PRIMARY KEY (link_id, tag_id)
);
CREATE INDEX idx_link_tags_tag ON link_tags(tag_id);

CREATE TABLE jobs (                            -- 내구성 있는 인프로세스 큐
  id          INTEGER PRIMARY KEY,
  kind        TEXT NOT NULL CHECK (kind IN ('scrape','tag','thumb')),
  link_id     INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  status      TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','running','done','failed')),
  attempts    INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 3,
  run_after   INTEGER NOT NULL DEFAULT (unixepoch()),  -- 재시도 백오프 스케줄
  error       TEXT NOT NULL DEFAULT '',
  claimed_at  INTEGER,
  finished_at INTEGER,
  created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_jobs_claim ON jobs(status, run_after);

CREATE TABLE tag_feedback (                    -- 사용자 태그 수정 이력 (M5 재랭킹 학습 데이터)
  id         INTEGER PRIMARY KEY,
  link_id    INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  tag_id     INTEGER NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
  action     TEXT NOT NULL CHECK (action IN ('added','removed')),
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE corpus_df (                       -- TF-IDF용 자체 코퍼스 문서 빈도 누적
  term TEXT PRIMARY KEY,
  df   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE link_terms (                      -- 0009. corpus_df의 원장 — "이 링크가 df에 무엇을 기여했는가"
  link_id INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  term    TEXT NOT NULL,
  PRIMARY KEY (link_id, term)
) WITHOUT ROWID;

CREATE TABLE tag_embeddings (                  -- M5: 태그 사전 임베딩 캐시
  tag_id    INTEGER PRIMARY KEY REFERENCES tags(id) ON DELETE CASCADE,
  model     TEXT NOT NULL,
  embedding BLOB NOT NULL                      -- float32 little-endian
);

-- FTS5 전문 검색: 한국어 부분 문자열 매칭을 위해 trigram 토크나이저
CREATE VIRTUAL TABLE links_fts USING fts5(
  title, description, note, tags,
  tokenize = 'trigram'
);
-- rowid = links.id. 링크/태그 쓰기와 같은 트랜잭션에서 store 계층이 동기화
-- (DELETE 후 INSERT). trigram 특성상 FTS5 매칭은 3자 이상 — 3자 미만 쿼리는 LIKE 폴백 (§5).
```

### Field notes, table by table

**links**

| Field | Description |
|---|---|
| `url` / `url_hash` | The original URL and its SHA-256 hex. `url_hash UNIQUE` blocks duplicate saves (on a duplicate the save API answers `200 {duplicate:true}`) |
| `domain` | The URL's host. For domain-heuristic tagging and list display |
| `title`, `description`, `author`, `lang` | Metadata the scraper fills in. Empty-string defaults instead of NULL — removes the nil checks on the Go side |
| `content_type` | `video` / `article` / `post` / `other`. Decided by domain and URL-pattern heuristics |
| `published_at`, `duration_sec`, `word_count` | Metadata of the source content. NULL when absent |
| `thumb_path` | A path relative to `data/thumbs/`. Stays NULL when the thumb job fails (best-effort) |
| `note` | A personal memo. Absorbs v1's `notes` table — single user, 1:1 relation, so a column is enough |
| `body_text` | The **body text** the scraper extracted with `go-trafilatura` (boilerplate removed). **Input for the rule tagger (M3) and the extractive summary (M5), nothing else** — it does not go into `links_fts` (a trigram's 3-character window explodes on body text) and it is not exposed in `api/openapi.yaml` either (an internal derivative). A length cap (32KB, on rune boundaries) trims only pathological outliers. Empty when extraction fails, on SPAs, and on non-articles (video/post) — the tagger degrades gracefully to title/description |
| `summary` | A **2–3-sentence extractive summary**: the key sentences picked out of body_text, joined with newlines (M5 Phase A — sentences selected from the original, no LLM, so hallucination is 0). The tag job produces it in the same body-processing pass as the tagging. **Exposed on `LinkDetail` only** — not carried in the list (`Link`) or in search (`SearchResult`), because a summary is not a substitute for the original but help with the "open it or not" call. It does not go into `links_fts` either (the risk of rebuilding a virtual table — revisit at stage 2). If the body is thin (below the 200-rune floor), or the prose runs to fewer than three sentences, or it is effectively the same as description (overlap 0.8 or higher), it is an empty string, and then the UI draws nothing |
| `body_source` | Where the body came from. `''` (none yet) / `server` (the scraper extracted it) / `client` (the browser extension or the Share Extension **captured it from the rendered page and shipped it with the save request**). When it is `client`, a later scrape does not overwrite `title`, `description` or `body_text` — the client supplied it precisely because the server cannot fetch that page (SPA, bot wall, login wall), so a server retry is always the worse result. For the same reason this link's `status` is `done` and not `failed` even when the scrape job fails for good (`error` is recorded all the same) |
| `keywords` | **The classification the publisher put on itself.** `meta[name=keywords]`, `news_keywords`, `article:section`, `article:tag` and JSON-LD `articleSection`, collected and joined with commas (512-byte cap). It is what the site declared, not what we inferred from the body, so the tagger weights it like the title. **It is a signal in the same class as the domain map, and it needs no per-site registration** — it works on sites nobody registered. Handled by the same rules as `body_text`: a scrape fills it, a client-captured value is never overwritten, and an empty value does not erase an existing one. **Tagger input only** — it appears nowhere in `links_fts` or in an API response |
| `opened_at` | **When this link was last opened.** NULL if it never was. Of the five stages of the core loop, save, scrape and tagging are instrumented, and only the last one — reopening — was zero. **There is no count (`open_count`)** — this is not a metric, it is a per-link fact. The signal only catches opens that went through Push-Point (browser history and opening the original app directly do not show up), so it undercounts structurally, and used as a ratio it manufactures the wrong conclusion that "I don't read what I save". **It does not touch `updated_at` either** — if an open bumped that, list ordering and the meaning of "modified" would wobble together. `POST /api/v1/links/{id}/open` records it and it is exposed on `LinkDetail` only (not on list items — a card has nowhere to show it) |
| `status` / `error` | Processing-pipeline state and the final failure reason (see §4) |
| `created_at` / `updated_at` / `deleted_at` | Epoch seconds. Deletion is soft |

Indexes: `idx_links_list` is a partial index that covers the list's keyset pagination sort (`created_at DESC, id DESC`) exactly as written; `idx_links_status` is for finding and retrying failures.

**tags**

| Field | Description |
|---|---|
| `name` | The tag name. `COLLATE NOCASE` makes it unique case-insensitively |
| `aliases` | A JSON array string. Synonyms and English/Korean spellings — the raw material for Phase A string matching |
| `facet` | The classification axis. `craft` (references used directly to make things) / `media` (the form itself is the information) / `life` (the world, daily life and myself) / `neutral` (unclassified, the default). The CHECK value set must be identical to the `TagFacet` enum in `api/openapi.yaml`, and `scripts/lint_enums.sh` compares them |

v1's `category`/`icon`/`usage_count` columns are gone. Usage counts come from a query instead of an aggregate column (§6). v1's `color` is gone too — **we do not store colour in the DB.** What is stored is meaning (`facet`) and nothing else; which colour a facet is drawn in is settled by each client in its own token system (colour comes as a light and a dark set, so one DB column cannot express it, and the moment it is stored, presentation is inverted onto the server).

**link_tags**

| Field | Description |
|---|---|
| `source` | `rules` (Phase A) / `embed` (Phase B) / `manual` (the user). The origin is clearer than v1's boolean `is_auto_generated` |
| `confidence` | The tagger's confidence score. NULL when `manual` |

**jobs**

| Field | Description |
|---|---|
| `kind` | `scrape` / `tag` / `thumb` |
| `status` | `pending` / `running` / `done` / `failed` |
| `attempts` / `max_attempts` | Retry counter. Three attempts at most by default |
| `run_after` | Claimable only after this instant — the store for the linear retry backoff (`unixepoch() + 30 * attempts`) |
| `claimed_at` / `finished_at` | When a worker took the job and when it finished |

`idx_jobs_claim(status, run_after)` covers the `WHERE status='pending' AND run_after <= unixepoch()` of the claim query (§6).

**tag_feedback** — every time the user adds or removes a tag, the `action` (`added`/`removed`) is recorded append-only. M5 uses it to correct the ensemble reranking weights.

**corpus_df** — document-frequency accumulation for a TF-IDF whose corpus is the saved links themselves. No external corpus dependency. It counts **dictionary surfaces (name and alias) only** — counting every token of a 32KB-capped body would make the table explode, and since scoring uses nothing but the DF of dictionary surfaces, the rest is cost paid for something thrown away.

**link_terms** — the ledger for `corpus_df`. It writes down, per link, exactly which surfaces that link contributed to df. Without it, `corpus_df` cannot be kept accurate: tagging runs **more than once** — retries, a body arriving late, an undelete — and if it only ever increments, df stops meaning "the number of documents containing that word" and starts meaning "the number of times tagging ran", so the longer the thing is used, the further the statistic quietly drifts from reality. With the ledger, a retag can cancel the previous contribution exactly and add the new one (the same transaction as `ApplyTags`). A soft delete reclaims its contribution too — a deleted link is not part of the corpus.

**tag_embeddings** — M5 precomputes and caches the tag-dictionary embeddings. The `model` column decides invalidation when the model is swapped.

**links_fts** — indexes 4 columns: `title`, `description`, `note`, `tags` (the tag names joined by spaces). **The `description` slot holds `links.description` and `links.summary` concatenated** (2026-07-26) — a virtual table's columns are under no obligation to match the links columns, search uses them only for MATCH and bm25, and every displayed value is read from links. A dedicated new column was not created because rebuilding a virtual table is risky, and because it would buy a bm25 weighting knob with no measurement set to justify it. The grounds are measured: **107 (87%)** of the 123 golden entries gain a 3-gram that exists only in the summary (`nlu/golden/README.md`). When `summary` changes it is reindexed in the same transaction (`SetSummary`). A virtual table has no foreign key constraint, so consistency rests on the `rowid = links.id` convention and the store layer's transactional sync (§5).

---

## 3. SQLite PRAGMA and connection strategy

Applied on every connection, always:

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA cache_size = -64000;   -- 64MB
```

| Setting | Why |
|---|---|
| `journal_mode = WAL` | Reads and writes do not block each other. A save arriving mid-scroll waits for nothing. The concurrency problem that was the reason for PostgreSQL in v1 is solved at personal scale by WAL |
| `synchronous = NORMAL` | Combined with WAL it does not force an fsync per commit, which cuts write latency sharply. A power cut can lose some of the most recent commits, but an app crash is safe — a sensible trade for personal link saving |
| `busy_timeout = 5000` | Under writer contention, wait up to five seconds instead of an immediate `SQLITE_BUSY`. Removes application-level retry code |
| `foreign_keys = ON` | SQLite defaults to OFF. Required for `ON DELETE CASCADE` (link_tags, jobs, tag_feedback, tag_embeddings) to work at all |
| `cache_size = -64000` | 64MB of page cache. The hot set of a 100k-link DB (~150MB) mostly sits in memory, so index traversal barely touches disk |

Connection strategy: **1 writer + a reader pool (N=4)**.

- SQLite serializes writes per database anyway, so pinning the writer to a single connection removes lock contention outright. At personal scale (tens of saves per second) serial writing is not the bottleneck.
- Reads run concurrently with writes thanks to WAL, so four readers handle list, search and detail in parallel.
- Every write is a transaction. The save API commits `INSERT link + INSERT job` as one transaction — an orphan state with a link but no job is structurally impossible.

The driver is `modernc.org/sqlite` (CGO-free) — the goal is keeping a single static binary, and FTS5 support is confirmed. If a performance problem shows up, mattn/go-sqlite3 (CGO) can replace it.

---

## 4. State transitions

### links.status

```
pending ──▶ scraping ──▶ tagging ──▶ done
   │            │            │
   └────────────┴────────────┴──▶ failed (+ links.error)
```

- `pending`: right after the save. The scrape job is in the queue
- `scraping`: the scrape job is running
- `tagging`: the scrape succeeded, the tag job is in progress
- `done`: tagging finished too
- `failed`: whenever a job at any stage exhausts `max_attempts`, the link transitions to `failed` as well and the reason is recorded in `error`. `POST /api/v1/links/{id}/retry` re-enqueues it

> **M2 interim (no tagger)**: the diagram above draws the steady state, with a tagger registered (M3 onwards). At M2 there is no tagger yet (see the milestones in 08), so a successful scrape does not create a `tag` job and `links.status` goes straight from `scraping` to `done` — that is, at M2 `done` means "scraping finished", not "tagging finished". The `tagging` state and `jobs.tag` become reachable only once the tagger handler is registered in M3. (The same family of interim note as the "at M1 there is only `scrape`" that the `jobs` summary in 06-API-SPEC spells out.)

### jobs.status

```
pending ──▶ running ──▶ done (+ finished_at)
   ▲            │
   │            ├──▶ pending (attempts < max_attempts,
   │            │      run_after = unixepoch() + 30 * attempts)
   └────────────┘
                └──▶ failed (attempts ≥ max_attempts)
```

- The claim is an atomic `UPDATE … RETURNING` (§6) — several worker goroutines never pick up the same job.
- On failure, if `attempts < max_attempts` it goes back to `pending` and `run_after` is pushed out by linear backoff. Past that: `failed`, and the link's status `failed`.
- Crash recovery: at process start, `UPDATE jobs SET status='pending' WHERE status='running'` — unprocessed jobs resume even after a `kill -9`.

### The best-effort rule for thumb jobs

The `thumb` job takes no part in link state transitions. If it fails, `links.status` proceeds regardless and only `thumb_path` stays NULL. A thumbnail is a nice thing to have, not a success condition of the save pipeline.

---

## 5. FTS5 sync strategy

`links_fts` is kept in sync not by triggers but by **the store layer, inside the same transaction as the link/tag write**:

```sql
-- 링크 메타데이터 갱신, 태그 부착/교체, note 수정 시 — 같은 트랜잭션에서:
DELETE FROM links_fts WHERE rowid = ?;
INSERT INTO links_fts(rowid, title, description, note, tags)
VALUES (?, ?, ?, ?, ?);   -- tags = 태그 이름들을 공백으로 연결한 문자열
```

- **The same transaction**, so a state where the base table and the index disagree cannot be observed from outside. If the commit fails, both roll back.
- **DELETE then INSERT**: simpler than managing the FTS index with partial column UPDATEs, and there is no need to track which field changed. Reindexing a single row costs a negligible amount.
- On a soft delete, only `DELETE FROM links_fts WHERE rowid = ?` runs, which takes it out of search.

**Why the trigram tokenizer was chosen**: the default unicode61 tokenizer splits on whitespace, which fails for Korean — the token "쿠버네티스를" does not match a search for "쿠버네티스" (the particle problem). trigram indexes text in overlapping three-character windows, so it matches through attached particles and inflections, and even on a substring in the middle of a word. The cheapest way to get Korean substring search without a morphological analyzer.

**The three-character constraint and the LIKE fallback**: by trigram's structural limit, a query shorter than three characters cannot use the FTS5 index. The API (`GET /api/v1/search`) uses FTS5 MATCH when `q` is three characters or longer (response `"mode":"fts"`), and when it is shorter falls back to a LIKE scan over the links table's title/note/description (response `"mode":"like"`, `rank` null, sorted `created_at DESC`). The measured LIKE full scan over 100k rows is inside the search budget (see the query examples in §6).

---

## 6. Key query examples

### List query (keyset cursor)

No OFFSET. The cursor is the last item's `(created_at, id)` pair. `idx_links_list` covers the sort as written, so even at 100k rows p99 < 50ms regardless of page position.

```sql
SELECT id, url, domain, title, description, content_type,
       thumb_path, status, note, created_at
FROM links
WHERE deleted_at IS NULL
  AND (created_at, id) < (?, ?)          -- 커서. 첫 페이지는 조건 생략
ORDER BY created_at DESC, id DESC
LIMIT ?;                                  -- limit + 1로 조회해 next_cursor 유무 판단
```

### Tag-filtered query

```sql
SELECT l.id, l.url, l.domain, l.title, l.description, l.content_type,
       l.thumb_path, l.status, l.note, l.created_at
FROM links l
JOIN link_tags lt ON lt.link_id = l.id
JOIN tags t       ON t.id = lt.tag_id
WHERE t.name = ? COLLATE NOCASE
  AND l.deleted_at IS NULL
  AND (l.created_at, l.id) < (?, ?)
ORDER BY l.created_at DESC, l.id DESC
LIMIT ?;
```

### FTS5 search (bm25 ranking)

```sql
SELECT l.id, l.url, l.domain, l.title, l.description, l.content_type,
       l.thumb_path, l.status, l.note, l.created_at,
       bm25(links_fts) AS rank
FROM links_fts f
JOIN links l ON l.id = f.rowid
WHERE links_fts MATCH ?                   -- q는 3자 이상 (trigram)
  AND l.deleted_at IS NULL
ORDER BY rank                             -- bm25는 낮을수록 관련도 높음
LIMIT ?;
```

### LIKE fallback search (q shorter than three characters)

When `q` is shorter than three characters, it falls back from FTS5 to a LIKE scan of the links table (§5). The bound value has the form `%q%`, and any `%`/`_`/`\` inside `q` is escaped with `\` before binding. The response carries `"mode":"like"` and a null `rank`.

```sql
SELECT id, url, domain, title, description, content_type,
       thumb_path, status, note, created_at
FROM links
WHERE deleted_at IS NULL
  AND (title       LIKE ? ESCAPE '\'
    OR note        LIKE ? ESCAPE '\'
    OR description LIKE ? ESCAPE '\')
ORDER BY created_at DESC, id DESC
LIMIT ?;
```

A measured full scan over 100k rows: 37ms — even a two-character query ("go", "ai") works inside the search budget.

### Job claim (atomic)

```sql
UPDATE jobs SET status='running', claimed_at=unixepoch(), attempts=attempts+1
WHERE id = (
  SELECT id FROM jobs
  WHERE status='pending' AND run_after <= unixepoch()
  ORDER BY id LIMIT 1
)
RETURNING id, kind, link_id, attempts;
```

A single UPDATE statement, so it is atomic without any extra lock. When nothing matches it returns 0 rows — the dispatcher waits for the next job on a notify channel plus a one-second polling ticker.

### Per-tag counts (`GET /api/v1/tags`)

```sql
SELECT t.id, t.name, t.aliases, t.facet, COUNT(l.id) AS link_count
FROM tags t
LEFT JOIN link_tags lt ON lt.tag_id = t.id
LEFT JOIN links l      ON l.id = lt.link_id AND l.deleted_at IS NULL
GROUP BY t.id
ORDER BY link_count DESC, t.name;
```

v1's `usage_count` aggregate column plus its triggers are gone, replaced by aggregation at query time — at a scale of 50 tags this GROUP BY is sub-millisecond, and the trigger maintenance cost disappears.

### Per-day counts (`GET /api/v1/stats`, last 30 days)

```sql
SELECT date(created_at, 'unixepoch', 'localtime') AS date,
       COUNT(*) AS count
FROM links
WHERE deleted_at IS NULL
  AND created_at >= unixepoch() - 30*86400
GROUP BY date
ORDER BY date;
```

---

## 7. Tag dictionary seed

Tags are not created freely; they are a classification against a controlled dictionary. A migration seeds an initial 30~50-tag set, after which the user refines it through `POST/PATCH /api/v1/tags`. An excerpt from the seed migration:

```sql
INSERT INTO tags (name, aliases) VALUES
  ('dev',      '["개발","programming","coding","코딩"]'),
  ('ai',       '["인공지능","머신러닝","machine learning","ml","llm"]'),
  ('video',    '["영상","동영상","유튜브","youtube"]'),
  ('design',   '["디자인","ui","ux"]'),
  ('article',  '["아티클","글","블로그","blog"]'),
  ('tutorial', '["튜토리얼","강의","가이드","how-to"]'),
  ('news',     '["뉴스","소식"]'),
  ('science',  '["과학","연구","paper","논문"]'),
  ('finance',  '["금융","투자","경제","재테크"]'),
  ('life',     '["라이프","일상","생활"]');
```

`aliases` is the raw material for Phase A string and synonym matching, so putting the English/Korean spellings and the common synonyms in together bears directly on tagging quality.

The `facet` of the 30-tag seed is assigned by `0003_tag_facet.up.sql` in three UPDATE statements — 18 in craft (`dev`, `golang`, `kubernetes`, `ios`, `swift`, `python`, `rust`, `javascript`, `frontend`, `backend`, `database`, `devops`, `security`, `opensource`, `ai`, `llm`, `data`, `design`), 5 in media (`article`, `video`, `tutorial`, `book`, `podcast`), 7 in life (`news`, `science`, `finance`, `career`, `productivity`, `travel`, `life`). Those three statements cover the seed with nothing left over, so no seeded tag stays `neutral`; `neutral` is the place where tags the user creates later are born.

---

## 8. Data size estimate and backup

| Scale | DB | Thumbnails (`data/thumbs/`) |
|---|---|---|
| 10k links | ~15MB | ~300MB |
| 100k links | ~150MB | ~3GB |

- The DB figure includes links, link_tags, the jobs history and the FTS5 index, all of it. At personal scale, years of use put no strain on a laptop disk.
- **Backup = copy the `data/` directory.** The DB files (`pushpoint.db` + `-wal`, `-shm`) and the thumbnails are all inside it. There is no pg_dump and no separate object-storage backup to look after the way v1 needed — that simplicity is half the reason SQLite + local disk was chosen.
