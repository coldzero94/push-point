# 데이터 스키마

> Push-Point v2.1 — 마지막 업데이트: 2026-07-22

v2의 저장소는 SQLite 단일 파일(`data/pushpoint.db`)이다. v1의 PostgreSQL 스키마(users, notes, sync_logs, user_stats, stored_images, 트리거)는 전부 폐기했다. 단일 사용자이므로 `users`가 필요 없고, 메모(v1의 `notes` 테이블)는 `links.note` 컬럼으로 흡수됐다. 큐(v1 Redis Streams)는 `jobs` 테이블이, 오브젝트 스토리지(v1 MinIO)는 `data/thumbs/` 디렉터리가 대체한다.

관련 문서: [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md), [04-DATA-FLOW.md](04-DATA-FLOW.md), [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md)

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
│ (FTS5 가상) │        └────────────┘
└────────────┘
```

- `links —< link_tags >— tags`: N:M. 태그는 통제된 사전(초기 30~50개 시드)이고, `link_tags`가 부착 출처(`source`)와 신뢰도를 함께 보관.
- `links —< jobs`: 링크 하나에 `scrape` / `tag` / `thumb` 잡이 연쇄로 달림.
- `links —< tag_feedback`: 사용자의 태그 추가/제거 이력. M5 재랭킹 학습 데이터.
- `links_fts`: FTS5 가상 테이블. 외래키가 아니라 `rowid = links.id` 규약으로 연결.
- `corpus_df`, `tag_embeddings`: 관계 없는 보조 테이블 (태깅 파이프라인용 통계·캐시).

---

## 2. 전체 DDL

마이그레이션은 golang-migrate + `embed.FS`로 바이너리에 내장되어 시작 시 자동 적용된다 (`backend/migrations/`). 시간은 전부 `INTEGER` unix epoch 초(`unixepoch()`), 소프트 삭제는 `deleted_at`.

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
  body_text    TEXT NOT NULL DEFAULT ''         -- 0004에서 ALTER ADD COLUMN (그래서 맨 뒤).
);                                              -- 본문 추출(go-trafilatura). 태거·요약 입력 전용 — FTS·API 미노출
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

### 테이블별 필드 설명

**links**

| 필드 | 설명 |
|---|---|
| `url` / `url_hash` | 원본 URL과 SHA-256 hex. `url_hash UNIQUE`로 중복 저장 차단 (저장 API가 중복이면 `200 {duplicate:true}`) |
| `domain` | URL의 호스트. 도메인 휴리스틱 태깅과 목록 표시용 |
| `title`, `description`, `author`, `lang` | 스크래퍼가 채우는 메타데이터. NULL 대신 빈 문자열 기본값 — Go 쪽 nil 검사 제거 |
| `content_type` | `video` / `article` / `post` / `other`. 도메인·URL 패턴 휴리스틱으로 판정 |
| `published_at`, `duration_sec`, `word_count` | 원본 콘텐츠 메타. 없으면 NULL |
| `thumb_path` | `data/thumbs/` 이하 상대 경로. thumb 잡 실패 시 NULL 유지 (best-effort) |
| `note` | 개인 메모. v1의 `notes` 테이블을 흡수 — 단일 사용자·1:1 관계라 컬럼이면 충분 |
| `body_text` | 스크래퍼가 `go-trafilatura`로 추출한 **본문 텍스트**(보일러플레이트 제거). **규칙 태거(M3)·추출식 요약(M5)의 입력 전용** — `links_fts`에 넣지 않고(trigram 3자 윈도우가 본문에 폭증) `api/openapi.yaml`에도 노출하지 않는다(내부 파생물). 길이 상한(32KB, 룬 경계)으로 병적 outlier만 자른다. 추출 실패·SPA·비-아티클(video/post)이면 빈 문자열 — 태거는 title/description으로 graceful degrade |
| `status` / `error` | 처리 파이프라인 상태와 최종 실패 사유 (§4 참고) |
| `created_at` / `updated_at` / `deleted_at` | epoch 초. 삭제는 소프트 삭제 |

인덱스: `idx_links_list`는 목록 keyset 페이지네이션(`created_at DESC, id DESC`)의 정렬을 그대로 커버하는 부분 인덱스, `idx_links_status`는 failed 조회·재시도용.

**tags**

| 필드 | 설명 |
|---|---|
| `name` | 태그 이름. `COLLATE NOCASE`로 대소문자 무시 유니크 |
| `aliases` | JSON 배열 문자열. 동의어·영문/한글 표기 — Phase A 문자열 매칭의 재료 |
| `facet` | 분류 축. `craft`(만드는 데 직접 쓰는 레퍼런스) / `media`(형식 자체가 정보) / `life`(일 바깥과 나 자신) / `neutral`(미분류, 기본값). CHECK 값 집합은 `api/openapi.yaml`의 `TagFacet` enum과 같아야 하며 `scripts/lint_enums.sh`가 대조한다 |

v1의 `category`/`icon`/`usage_count` 컬럼은 폐기. 사용 수는 집계 컬럼 대신 쿼리로 구한다 (§6). v1의 `color`도 폐기했다 — **DB에 색을 저장하지 않는다.** 저장하는 것은 의미(`facet`)뿐이고, 그 facet을 어떤 색으로 그릴지는 각 클라이언트가 자기 토큰 체계로 정한다 (색은 라이트/다크 2벌이라 DB 컬럼 하나로 표현할 수 없고, 저장하는 순간 표현이 서버로 역전된다).

**link_tags**

| 필드 | 설명 |
|---|---|
| `source` | `rules`(Phase A) / `embed`(Phase B) / `manual`(사용자). v1의 boolean `is_auto_generated`보다 출처가 명확 |
| `confidence` | 태거의 신뢰 점수. `manual`이면 NULL |

**jobs**

| 필드 | 설명 |
|---|---|
| `kind` | `scrape` / `tag` / `thumb` |
| `status` | `pending` / `running` / `done` / `failed` |
| `attempts` / `max_attempts` | 재시도 카운터. 기본 최대 3회 |
| `run_after` | 이 시각 이후에만 claim 가능 — 재시도 선형 백오프(`unixepoch() + 30 * attempts`)의 저장소 |
| `claimed_at` / `finished_at` | 워커가 잡은 시각·끝낸 시각 |

`idx_jobs_claim(status, run_after)`이 claim 쿼리(§6)의 `WHERE status='pending' AND run_after <= unixepoch()`를 커버한다.

**tag_feedback** — 사용자가 태그를 추가/제거할 때마다 `action`(`added`/`removed`)을 append-only로 기록. M5에서 앙상블 재랭킹 가중치 보정에 쓴다.

**corpus_df** — 저장된 링크 자체를 코퍼스로 삼는 TF-IDF의 문서 빈도(document frequency) 누적. 외부 코퍼스 의존 없음.

**tag_embeddings** — M5에서 태그 사전 임베딩을 미리 계산해 캐시. `model` 컬럼으로 모델 교체 시 무효화 판별.

**links_fts** — `title`, `description`, `note`, `tags`(태그 이름을 공백 연결한 텍스트) 4개 컬럼을 색인. 외래키 제약이 없는 가상 테이블이므로 `rowid = links.id` 규약과 store 계층의 트랜잭션 동기화(§5)로 정합성을 보장한다.

---

## 3. SQLite PRAGMA와 커넥션 전략

연결 시 항상 적용하는 설정:

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA cache_size = -64000;   -- 64MB
```

| 설정 | 이유 |
|---|---|
| `journal_mode = WAL` | 읽기와 쓰기가 서로 블로킹하지 않음. 목록 스크롤 중에 저장이 들어와도 대기 없음. v1에서 PostgreSQL을 쓴 이유였던 동시성 문제를 개인 규모에서는 WAL이 해결 |
| `synchronous = NORMAL` | WAL 조합에서 커밋마다 fsync를 강제하지 않아 쓰기 지연 대폭 감소. 정전 시 최근 커밋 일부를 잃을 수 있으나 앱 크래시에는 안전 — 개인 링크 저장에 합리적인 트레이드오프 |
| `busy_timeout = 5000` | writer 경합 시 즉시 `SQLITE_BUSY` 대신 5초까지 대기. 애플리케이션 레벨 재시도 코드 제거 |
| `foreign_keys = ON` | SQLite는 기본 OFF. `ON DELETE CASCADE`(link_tags, jobs, tag_feedback, tag_embeddings)가 실제로 동작하려면 필수 |
| `cache_size = -64000` | 페이지 캐시 64MB. 10만 건 DB(~150MB)의 핫셋이 대부분 메모리에 올라가 인덱스 탐색이 디스크를 거의 안 탐 |

커넥션 전략: **writer 1개 + reader 풀(N=4)**.

- SQLite의 쓰기는 어차피 DB 단위로 직렬화되므로, writer 커넥션을 하나로 고정해 락 경합 자체를 없앤다. 개인 규모(초당 수십 저장)에서 직렬 쓰기는 병목이 아니다.
- 읽기는 WAL 덕에 쓰기와 동시에 진행되므로 reader 4개로 목록·검색·상세를 병렬 처리.
- 모든 쓰기는 트랜잭션. 저장 API는 `INSERT link + INSERT job`을 한 트랜잭션으로 커밋한다 — 링크만 있고 잡이 없는 고아 상태가 원천적으로 불가능.

드라이버는 `modernc.org/sqlite`(CGO-free) — 단일 정적 바이너리 유지가 목적이며, FTS5 지원 확인됨. 성능 문제가 확인되면 mattn/go-sqlite3(CGO)로 교체 가능하다.

---

## 4. 상태 전이

### links.status

```
pending ──▶ scraping ──▶ tagging ──▶ done
   │            │            │
   └────────────┴────────────┴──▶ failed (+ links.error)
```

- `pending`: 저장 직후. scrape 잡이 큐에 있음
- `scraping`: scrape 잡이 running
- `tagging`: scrape 성공, tag 잡 처리 중
- `done`: 태깅까지 완료
- `failed`: 어느 단계든 잡이 `max_attempts`를 소진하면 링크도 `failed`로 전이하고 `error`에 사유 기록. `POST /api/v1/links/{id}/retry`로 재-enqueue 가능

> **M2 인터림 (tagger 부재)**: 위 전이도는 tagger가 등록된 스테디 상태(M3 이후)를 그린다. M2 시점에는 tagger가 아직 없어(08 마일스톤 참조) scrape 성공 시 `tag` 잡을 만들지 않고 `links.status`가 `scraping`에서 곧바로 `done`으로 전이한다 — 즉 M2에서 `done`은 "태깅까지 완료"가 아니라 "스크랩 완료"를 뜻한다. `tagging` 상태와 `jobs.tag`는 M3에서 tagger 핸들러가 등록돼야 비로소 도달 가능하다. (06-API-SPEC의 `jobs` 요약이 밝히는 "M1에서는 `scrape`만 있다"와 같은 계열의 인터림 명시.)

### jobs.status

```
pending ──▶ running ──▶ done (+ finished_at)
   ▲            │
   │            ├──▶ pending (attempts < max_attempts,
   │            │      run_after = unixepoch() + 30 * attempts)
   └────────────┘
                └──▶ failed (attempts ≥ max_attempts)
```

- claim은 원자적 `UPDATE … RETURNING`(§6)으로 수행 — 워커 여러 goroutine이 같은 잡을 집는 일이 없다.
- 실패 시 `attempts < max_attempts`면 `pending`으로 되돌리고 `run_after`를 선형 백오프로 미룬다. 초과하면 `failed` + 링크 상태 `failed`.
- 크래시 복구: 프로세스 시작 시 `UPDATE jobs SET status='pending' WHERE status='running'` — `kill -9` 후에도 미처리 잡이 재개된다.

### thumb 잡의 best-effort 규칙

`thumb` 잡은 링크 상태 전이에 관여하지 않는다. 실패해도 `links.status`는 그대로 진행되고 `thumb_path`만 NULL로 남는다. 썸네일은 있으면 좋은 것이지 저장 파이프라인의 성공 조건이 아니다.

---

## 5. FTS5 동기화 전략

`links_fts`는 트리거가 아니라 **store 계층이 링크/태그 쓰기와 같은 트랜잭션 안에서 동기화**한다:

```sql
-- 링크 메타데이터 갱신, 태그 부착/교체, note 수정 시 — 같은 트랜잭션에서:
DELETE FROM links_fts WHERE rowid = ?;
INSERT INTO links_fts(rowid, title, description, note, tags)
VALUES (?, ?, ?, ?, ?);   -- tags = 태그 이름들을 공백으로 연결한 문자열
```

- **같은 트랜잭션**이므로 본 테이블과 색인이 어긋난 상태가 외부에 관측될 수 없다. 커밋 실패 시 둘 다 롤백.
- **DELETE 후 INSERT**: 부분 컬럼 UPDATE로 FTS 색인을 관리하는 것보다 단순하고, 어떤 필드가 바뀌었는지 추적할 필요가 없다. 행 하나 재색인 비용은 무시할 수준.
- 소프트 삭제 시에는 `DELETE FROM links_fts WHERE rowid = ?`만 수행해 검색에서 제외.

**trigram 토크나이저를 선택한 이유**: 기본 unicode61 토크나이저는 공백 단위 토큰이라 한국어에서 실패한다 — "쿠버네티스를"이라는 토큰은 "쿠버네티스" 검색에 매칭되지 않는다(조사 문제). trigram은 텍스트를 3자 단위로 겹쳐 색인하므로 조사·활용이 붙어도, 단어 중간 부분 문자열이라도 매칭된다. 형태소 분석기 없이 한국어 부분 문자열 검색을 얻는 가장 값싼 방법.

**3자 제약과 LIKE 폴백**: trigram의 구조적 한계로 검색어가 3자 미만이면 FTS5 색인을 탈 수 없다. API(`GET /api/v1/search`)는 `q`가 3자 이상이면 FTS5 MATCH(응답 `"mode":"fts"`), 3자 미만이면 links 테이블 title/note/description LIKE 스캔(응답 `"mode":"like"`, `rank`는 null, `created_at DESC` 정렬)으로 폴백한다. 실측 10만 행 LIKE 풀스캔 37ms로 검색 예산 내다 (§6 쿼리 예시 참고).

---

## 6. 주요 쿼리 예시

### 목록 조회 (keyset 커서)

OFFSET 금지. 커서는 마지막 항목의 `(created_at, id)` 쌍. `idx_links_list`가 정렬을 그대로 커버해 10만 건에서도 페이지 위치와 무관하게 p99 < 50ms.

```sql
SELECT id, url, domain, title, description, content_type,
       thumb_path, status, note, created_at
FROM links
WHERE deleted_at IS NULL
  AND (created_at, id) < (?, ?)          -- 커서. 첫 페이지는 조건 생략
ORDER BY created_at DESC, id DESC
LIMIT ?;                                  -- limit + 1로 조회해 next_cursor 유무 판단
```

### 태그 필터 조회

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

### FTS5 검색 (bm25 랭킹)

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

### LIKE 폴백 검색 (q 3자 미만)

`q`가 3자 미만이면 FTS5 대신 links 테이블 LIKE 스캔으로 폴백한다 (§5). 바인딩 값은 `%q%` 형태이며, `q` 안의 `%`/`_`/`\`는 바인딩 전에 `\`로 이스케이프한다. 응답은 `"mode":"like"`, `rank`는 null.

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

실측 10만 행 풀스캔 37ms — 2자 쿼리(예: "go", "ai")도 검색 예산 내에서 동작한다.

### 잡 claim (원자적)

```sql
UPDATE jobs SET status='running', claimed_at=unixepoch(), attempts=attempts+1
WHERE id = (
  SELECT id FROM jobs
  WHERE status='pending' AND run_after <= unixepoch()
  ORDER BY id LIMIT 1
)
RETURNING id, kind, link_id, attempts;
```

단일 UPDATE 문이므로 별도 락 없이 원자적이다. 행이 없으면 0건 반환 — dispatcher는 notify 채널과 1초 폴링 티커로 다음 잡을 기다린다.

### 태그별 카운트 (`GET /api/v1/tags`)

```sql
SELECT t.id, t.name, t.aliases, t.facet, COUNT(l.id) AS link_count
FROM tags t
LEFT JOIN link_tags lt ON lt.tag_id = t.id
LEFT JOIN links l      ON l.id = lt.link_id AND l.deleted_at IS NULL
GROUP BY t.id
ORDER BY link_count DESC, t.name;
```

v1의 `usage_count` 집계 컬럼 + 트리거를 폐기하고 조회 시 집계로 대체 — 태그 50개 규모에서 이 GROUP BY는 밀리초 미만이고, 트리거 유지보수 비용이 사라진다.

### 날짜별 카운트 (`GET /api/v1/stats`, 최근 30일)

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

## 7. 태그 사전 시드

태그는 자유 생성이 아니라 통제된 사전에 대한 분류다. 초기 30~50개를 마이그레이션으로 시드하고, 이후 `POST/PATCH /api/v1/tags`로 사용자가 다듬는다. 시드 마이그레이션 예시 (발췌):

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

`aliases`가 Phase A 문자열·동의어 매칭의 재료이므로, 영문/한글 표기와 흔한 동의어를 함께 넣는 것이 태깅 품질에 직결된다.

시드 30개의 `facet`은 `0003_tag_facet.up.sql`이 UPDATE 3문으로 배정한다 — craft 18개(`dev`, `golang`, `kubernetes`, `ios`, `swift`, `python`, `rust`, `javascript`, `frontend`, `backend`, `database`, `devops`, `security`, `opensource`, `ai`, `llm`, `data`, `design`), media 5개(`article`, `video`, `tutorial`, `book`, `podcast`), life 7개(`news`, `science`, `finance`, `career`, `productivity`, `travel`, `life`). 세 문장이 시드 30개를 남김 없이 덮으므로 `neutral`로 남는 시드는 없고, `neutral`은 이후 사용자가 새로 만든 태그가 태어나는 자리다.

---

## 8. 데이터 크기 추정과 백업

| 규모 | DB | 썸네일 (`data/thumbs/`) |
|---|---|---|
| 링크 1만 건 | ~15MB | ~300MB |
| 링크 10만 건 | ~150MB | ~3GB |

- DB에는 links, link_tags, jobs 이력, FTS5 색인이 모두 포함된 수치다. 개인 사용 규모에서 수년을 써도 노트북 디스크에 부담이 없다.
- **백업 = `data/` 디렉터리 복사.** DB 파일(`pushpoint.db` + `-wal`, `-shm`)과 썸네일이 전부 그 안에 있다. v1처럼 pg_dump와 오브젝트 스토리지 백업을 따로 챙길 필요가 없다 — 이 단순함이 SQLite + 로컬 디스크를 선택한 이유의 절반이다.
