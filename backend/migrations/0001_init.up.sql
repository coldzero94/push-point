-- Push-Point v2 초기 스키마 (docs/v2/05-DATA-SCHEMA.md §2 DDL 원본 그대로)
-- 시간은 전부 INTEGER unix epoch 초 (unixepoch()), 소프트 삭제는 deleted_at.

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
  deleted_at   INTEGER
);
CREATE INDEX idx_links_list   ON links(created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_links_status ON links(status) WHERE deleted_at IS NULL;

CREATE TABLE tags (                            -- 통제된 태그 사전 (초기 30~50개 시드)
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
  aliases    TEXT NOT NULL DEFAULT '[]',       -- JSON 배열: 동의어/영문·한글 표기
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
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
-- (DELETE 후 INSERT). trigram 특성상 FTS5 매칭은 3자 이상 — 3자 미만은 LIKE 폴백.
