# 데이터베이스 스키마

## PostgreSQL 스키마

### ERD (Entity Relationship Diagram)

```
users (1) ──< (N) links
            │
            ├──< (N) notes
            └──< (N) sync_logs

links (N) ──< (N) tags  (through link_tags)
      │
      ├──< (1) notes
      ├──< (N) stored_images
      └──< (N) job_queue

tags (N) ──> (N) links (through link_tags)

stored_images (N) ──> (1) links
```

## 테이블 정의

### 1. users (사용자)

```sql
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  
  -- 프로필
  display_name VARCHAR(100),
  avatar_url TEXT,
  
  -- 설정
  timezone VARCHAR(50) DEFAULT 'UTC',
  language VARCHAR(10) DEFAULT 'ko',
  
  -- 동기화
  last_sync_at TIMESTAMP,
  
  -- 메타
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  deleted_at TIMESTAMP
);

CREATE INDEX idx_users_email ON users(email);
```

**필드 설명**:
- `id`: UUID 기반 고유 식별자
- `email`: 로그인 ID (유니크)
- `password_hash`: bcrypt 해시
- `last_sync_at`: 마지막 동기화 시간

---

### 2. links (링크 저장소)

```sql
CREATE TABLE links (
  id BIGSERIAL PRIMARY KEY,
  user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  
  -- 기본 정보
  url TEXT NOT NULL,
  url_hash VARCHAR(64) UNIQUE NOT NULL, -- SHA256(url) for dedup
  title TEXT,
  description TEXT,
  
  -- 분류
  domain VARCHAR(255),
  content_type VARCHAR(50), -- 'youtube', 'article', 'tweet', 'reddit', etc.
  
  -- 이미지
  thumbnail_original_url TEXT, -- 원본 썸네일 URL
  thumbnail_storage_path TEXT, -- MinIO 저장 경로
  
  -- 컨텐츠 메타데이터
  author VARCHAR(255),
  published_at TIMESTAMP,
  duration INTEGER, -- 영상 길이 (초), 아티클 읽기 시간 등
  word_count INTEGER,
  language VARCHAR(10),
  
  -- 처리 상태
  status VARCHAR(20) DEFAULT 'pending', 
    -- 'pending': 저장만 됨
    -- 'scraping': 크롤링 중
    -- 'tagging': AI 태깅 중
    -- 'completed': 완료
    -- 'failed': 실패
  
  processing_error TEXT, -- 에러 메시지
  retry_count INTEGER DEFAULT 0,
  
  -- 통계
  view_count INTEGER DEFAULT 0,
  last_viewed_at TIMESTAMP,
  
  -- 타임스탬프
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW(),
  deleted_at TIMESTAMP,
  
  -- 동기화
  version INTEGER DEFAULT 1, -- Optimistic locking
  synced_at TIMESTAMP
);

CREATE INDEX idx_links_user_created ON links(user_id, created_at DESC);
CREATE INDEX idx_links_user_status ON links(user_id, status);
CREATE INDEX idx_links_url_hash ON links(url_hash);
CREATE INDEX idx_links_domain ON links(domain);
CREATE INDEX idx_links_content_type ON links(content_type);
```

**필드 설명**:
- `url_hash`: 중복 체크용 (SHA256)
- `content_type`: youtube, article, tweet 등
- `status`: 처리 상태 추적
- `version`: 낙관적 잠금 (동기화 충돌 방지)

---

### 3. tags (태그)

```sql
CREATE TABLE tags (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) UNIQUE NOT NULL,
  
  -- 태그 분류
  category VARCHAR(50), 
    -- 'topic': 주제 (기술, 디자인, 비즈니스...)
    -- 'type': 콘텐츠 타입 (튜토리얼, 인터뷰...)
    -- 'mood': 감정/톤 (영감적, 실용적...)
    -- 'difficulty': 난이도
    -- 'custom': 사용자 정의
  
  color VARCHAR(7), -- hex color: #FF5733
  icon VARCHAR(50), -- emoji or icon name
  
  -- 통계
  usage_count INTEGER DEFAULT 0,
  
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_tags_category ON tags(category);
CREATE INDEX idx_tags_usage ON tags(usage_count DESC);
```

**필드 설명**:
- `category`: 태그 유형 분류
- `usage_count`: 사용 빈도 (정렬용)

---

### 4. link_tags (링크-태그 관계)

```sql
CREATE TABLE link_tags (
  link_id BIGINT REFERENCES links(id) ON DELETE CASCADE,
  tag_id INTEGER REFERENCES tags(id) ON DELETE CASCADE,
  
  -- 태그 메타
  is_auto_generated BOOLEAN DEFAULT true,
  confidence FLOAT, -- AI 신뢰도 (0.0 ~ 1.0)
  
  created_at TIMESTAMP DEFAULT NOW(),
  created_by UUID REFERENCES users(id), -- NULL이면 시스템 생성
  
  PRIMARY KEY (link_id, tag_id)
);

CREATE INDEX idx_link_tags_tag ON link_tags(tag_id);
CREATE INDEX idx_link_tags_auto ON link_tags(is_auto_generated);
```

**필드 설명**:
- `is_auto_generated`: AI vs 사용자 수동
- `confidence`: AI 태그의 신뢰도 점수

---

### 5. notes (메모)

```sql
CREATE TABLE notes (
  id BIGSERIAL PRIMARY KEY,
  link_id BIGINT REFERENCES links(id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  
  content TEXT NOT NULL,
  
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_notes_link ON notes(link_id);
CREATE INDEX idx_notes_user ON notes(user_id);
```

---

### 6. stored_images (이미지 저장 기록)

```sql
CREATE TABLE stored_images (
  id BIGSERIAL PRIMARY KEY,
  link_id BIGINT REFERENCES links(id) ON DELETE CASCADE,
  
  -- MinIO 정보
  bucket VARCHAR(100) NOT NULL,
  object_key VARCHAR(500) NOT NULL, -- 파일 경로
  
  -- 이미지 메타
  size_bytes BIGINT,
  mime_type VARCHAR(50),
  width INTEGER,
  height INTEGER,
  
  -- 타입
  image_type VARCHAR(20), -- 'thumbnail_original', 'thumbnail_small', 'thumbnail_medium', 'thumbnail_large'
  
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_stored_images_link ON stored_images(link_id);
CREATE UNIQUE INDEX idx_stored_images_path ON stored_images(bucket, object_key);
```

---

### 7. job_queue (작업 큐 - DB 기반, Optional)

```sql
CREATE TABLE job_queue (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  link_id BIGINT REFERENCES links(id) ON DELETE CASCADE,
  
  job_type VARCHAR(50) NOT NULL, -- 'scrape', 'tag', 'thumbnail'
  priority INTEGER DEFAULT 5, -- 1 (highest) ~ 10 (lowest)
  
  payload JSONB, -- 작업 상세 정보
  
  status VARCHAR(20) DEFAULT 'pending',
    -- 'pending', 'processing', 'completed', 'failed'
  
  attempts INTEGER DEFAULT 0,
  max_attempts INTEGER DEFAULT 3,
  
  error TEXT,
  
  created_at TIMESTAMP DEFAULT NOW(),
  started_at TIMESTAMP,
  completed_at TIMESTAMP,
  
  -- Worker 정보
  worker_id VARCHAR(100),
  locked_at TIMESTAMP,
  locked_until TIMESTAMP
);

CREATE INDEX idx_jobs_status_priority ON job_queue(status, priority, created_at);
CREATE INDEX idx_jobs_link ON job_queue(link_id);
```

**참고**: RabbitMQ를 사용하면 이 테이블은 선택사항

---

### 8. sync_logs (동기화 로그)

```sql
CREATE TABLE sync_logs (
  id BIGSERIAL PRIMARY KEY,
  user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  
  device_id VARCHAR(100), -- 기기 식별자
  device_type VARCHAR(20), -- 'ios', 'android'
  
  sync_type VARCHAR(20), -- 'pull', 'push', 'full'
  
  items_synced INTEGER,
  items_failed INTEGER,
  
  started_at TIMESTAMP,
  completed_at TIMESTAMP,
  
  error TEXT
);

CREATE INDEX idx_sync_logs_user ON sync_logs(user_id, started_at DESC);
```

---

### 9. user_stats (사용자 통계 - 집계 테이블)

```sql
CREATE TABLE user_stats (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  
  total_links INTEGER DEFAULT 0,
  total_tags INTEGER DEFAULT 0,
  total_notes INTEGER DEFAULT 0,
  
  links_this_week INTEGER DEFAULT 0,
  links_this_month INTEGER DEFAULT 0,
  
  most_used_tags JSONB, -- [{"tag": "tech", "count": 42}, ...]
  
  updated_at TIMESTAMP DEFAULT NOW()
);
```

**용도**: 대시보드, 통계 화면에서 빠른 조회

---

## Triggers (자동화)

### 1. updated_at 자동 갱신

```sql
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
   NEW.updated_at = NOW();
   RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_links_updated_at 
  BEFORE UPDATE ON links
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_users_updated_at 
  BEFORE UPDATE ON users
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notes_updated_at 
  BEFORE UPDATE ON notes
  FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

### 2. 태그 사용 카운트 자동 업데이트

```sql
CREATE OR REPLACE FUNCTION update_tag_usage_count()
RETURNS TRIGGER AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE tags SET usage_count = usage_count + 1 WHERE id = NEW.tag_id;
  ELSIF TG_OP = 'DELETE' THEN
    UPDATE tags SET usage_count = usage_count - 1 WHERE id = OLD.tag_id;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_tag_count_on_link_tag
  AFTER INSERT OR DELETE ON link_tags
  FOR EACH ROW EXECUTE FUNCTION update_tag_usage_count();
```

---

## 주요 쿼리 예시

### 1. 사용자의 최근 링크 조회 (태그 포함)

```sql
SELECT 
  l.id,
  l.url,
  l.title,
  l.thumbnail_storage_path,
  l.created_at,
  array_agg(t.name) FILTER (WHERE t.name IS NOT NULL) as tags
FROM links l
LEFT JOIN link_tags lt ON l.id = lt.link_id
LEFT JOIN tags t ON lt.tag_id = t.id
WHERE l.user_id = $1
  AND l.deleted_at IS NULL
GROUP BY l.id
ORDER BY l.created_at DESC
LIMIT 20 OFFSET 0;
```

### 2. 특정 태그의 링크 조회

```sql
SELECT DISTINCT l.*
FROM links l
INNER JOIN link_tags lt ON l.id = lt.link_id
INNER JOIN tags t ON lt.tag_id = t.id
WHERE t.name = $1
  AND l.user_id = $2
  AND l.deleted_at IS NULL
ORDER BY l.created_at DESC;
```

### 3. 전체 텍스트 검색 (제목 + 설명)

```sql
-- Full-text search index 생성
CREATE INDEX idx_links_fts ON links 
  USING gin(to_tsvector('english', title || ' ' || description));

-- 검색 쿼리
SELECT 
  l.*,
  ts_rank(
    to_tsvector('english', l.title || ' ' || l.description),
    to_tsquery('english', $1)
  ) AS rank
FROM links l
WHERE to_tsvector('english', l.title || ' ' || l.description)
  @@ to_tsquery('english', $1)
  AND l.user_id = $2
ORDER BY rank DESC, l.created_at DESC
LIMIT 20;
```

### 4. 날짜별 링크 개수 (캘린더 뷰)

```sql
SELECT 
  DATE(created_at) as date,
  COUNT(*) as count
FROM links
WHERE user_id = $1
  AND created_at >= $2  -- 시작 날짜
  AND created_at < $3   -- 종료 날짜
  AND deleted_at IS NULL
GROUP BY DATE(created_at)
ORDER BY date;
```

### 5. 인기 태그 TOP 10

```sql
SELECT 
  t.id,
  t.name,
  t.category,
  t.color,
  COUNT(lt.link_id) as link_count
FROM tags t
INNER JOIN link_tags lt ON t.id = lt.tag_id
INNER JOIN links l ON lt.link_id = l.id
WHERE l.user_id = $1
  AND l.deleted_at IS NULL
GROUP BY t.id, t.name, t.category, t.color
ORDER BY link_count DESC
LIMIT 10;
```

---

## Redis 데이터 구조

### 1. 세션

```
Key: user:{user_id}:session:{session_id}
Type: Hash
TTL: 7 days

Fields:
  - user_id: UUID
  - email: string
  - device_id: string
  - created_at: timestamp
```

### 2. API 캐시

```
Key: cache:links:user:{user_id}:page:{page}:tag:{tag}
Type: String (JSON)
TTL: 5 minutes

Value: JSON array of links
```

### 3. Rate Limiting

```
Key: ratelimit:{user_id}:{endpoint}
Type: String (counter)
TTL: 1 minute

Limit: 100 requests/minute
```

### 4. Distributed Lock

```
Key: lock:scrape:{url_hash}
Type: String
TTL: 5 minutes (auto-release)

Value: worker_id
```

---

## MinIO Bucket 구조

```
Bucket: thumbnails
├── original/
│   └── {link_id}_{hash}.{ext}
└── resized/
    ├── small/
    │   └── {link_id}_{hash}.jpg (150x150)
    ├── medium/
    │   └── {link_id}_{hash}.jpg (300x300)
    └── large/
        └── {link_id}_{hash}.jpg (600x600)

Bucket: content-cache (Optional)
└── {link_id}_{hash}.html
```

**파일명 예시**: `12345_a1b2c3d4e5f6.jpg`

---

## 데이터 크기 예상

**1만 개 링크 기준**:
- links: ~5MB
- tags: ~50KB
- link_tags: ~300KB (평균 3개 태그/링크)
- notes: ~2MB
- stored_images: ~200KB
- 썸네일 (MinIO): ~500MB (50KB/이미지 x 3 사이즈)

**총**: PostgreSQL ~10MB, MinIO ~500MB

**10만 개 링크**: PostgreSQL ~100MB, MinIO ~5GB