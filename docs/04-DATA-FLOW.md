# 데이터 플로우

## 1. 링크 저장 플로우 (전체)

```
┌──────────┐
│   App    │ 사용자가 유튜브/웹에서 공유 버튼 클릭
└─────┬────┘
      │ 1. POST /api/v1/links
      │    {
      │      "url": "https://youtube.com/watch?v=xxx",
      │      "source": "youtube_share"
      │    }
      ↓
┌─────────────┐
│ API Server  │
└─────┬───────┘
      │ 2. JWT 토큰 검증
      ↓
┌─────────────┐
│   Redis     │ GET user:{user_id}:session
└─────────────┘
      │ ✓ 인증 성공
      │
      │ 3. URL 중복 체크
      ↓
┌──────────────┐
│ PostgreSQL   │ SELECT * FROM links 
└──────────────┘  WHERE url_hash = SHA256(url) AND user_id = ?
      │
      │ 중복 없음
      │ 4. 기본 정보만 DB에 저장
      ↓
┌──────────────┐
│ PostgreSQL   │ INSERT INTO links (
└──────────────┘   user_id, url, url_hash, status='pending'
                ) RETURNING id
      │
      │ link_id = 12345
      │
      ├─────────────────────────────────────────┐
      │                                         │
      │ 5. 즉시 응답 (사용자는 기다리지 않음)     │
      ↓                                         │
┌──────────┐                                   │
│   App    │ ✓ 저장 완료! (link_id: 12345)       │
└──────────┘                                   │
                                               │
      ┌────────────────────────────────────────┘
      │ 6. 백그라운드 Job 발행
      ↓
┌─────────────┐
│  RabbitMQ   │ PUBLISH to 'scrape.queue'
└─────┬───────┘  {
      │            "job_id": "uuid-1",
      │            "link_id": 12345,
      │            "url": "https://...",
      │            "task_type": "scrape"
      │          }
      │
      │ 7. Worker가 Job 수신
      ↓
┌─────────────┐
│   Worker    │
└─────┬───────┘
      │ 8. 중복 처리 방지 Lock
      ↓
┌─────────────┐
│   Redis     │ SET lock:scrape:{url_hash} EX 300 NX
└─────────────┘
      │ ✓ Lock 획득
      │
      │ 9. 링크 상태 업데이트
      ↓
┌──────────────┐
│ PostgreSQL   │ UPDATE links SET status='scraping'
└──────────────┘  WHERE id = 12345
      │
      │ 10. URL 접속 및 크롤링
      ↓
  [Target Website]
      │ HTML 파싱
      │ - <title>
      │ - <meta property="og:description">
      │ - <meta property="og:image">
      │ - <meta property="author">
      │
      │ 11. 메타데이터 저장
      ↓
┌──────────────┐
│ PostgreSQL   │ UPDATE links SET
└──────────────┘   title = 'Amazing Video',
                  description = '...',
                  thumbnail_original_url = 'https://...',
                  author = 'Channel Name',
                  status = 'scraping_completed'
      │
      │ 12. 썸네일 다운로드 Job 발행
      ↓
┌─────────────┐
│  RabbitMQ   │ PUBLISH to 'thumbnail.queue'
└─────┬───────┘
      │
      │ 13. Worker가 썸네일 처리
      ↓
┌─────────────┐
│   Worker    │
└─────┬───────┘
      │ 14. 이미지 다운로드
      ↓
  [Download Image from thumbnail_original_url]
      │
      │ 15. 이미지 리사이징 (3가지 사이즈)
      │     - small: 150x150
      │     - medium: 300x300
      │     - large: 600x600
      ↓
┌─────────────┐
│   MinIO     │ PUT /thumbnails/resized/small/12345_hash.jpg
│             │ PUT /thumbnails/resized/medium/12345_hash.jpg
└─────────────┘ PUT /thumbnails/resized/large/12345_hash.jpg
      │
      │ 16. 이미지 경로 저장
      ↓
┌──────────────┐
│ PostgreSQL   │ INSERT INTO stored_images (
└──────────────┘   link_id, bucket, object_key, image_type, ...
                )
                UPDATE links SET
                  thumbnail_storage_path = '/thumbnails/...'
      │
      │ 17. AI 태그 생성 Job 발행
      ↓
┌─────────────┐
│  RabbitMQ   │ PUBLISH to 'tag.queue'
└─────┬───────┘
      │
      │ 18. Worker가 태그 생성
      ↓
┌─────────────┐
│   Worker    │
└─────┬───────┘
      │ 19. OpenAI API 호출
      ↓
┌─────────────┐
│  OpenAI API │ POST /v1/chat/completions
└─────┬───────┘  {
      │            "model": "gpt-4o-mini",
      │            "messages": [{
      │              "role": "system",
      │              "content": "태그 생성 AI"
      │            }, {
      │              "role": "user",
      │              "content": "제목: ..., 설명: ..."
      │            }]
      │          }
      │
      │ Response:
      │ {
      │   "category": "기술",
      │   "content_type": "튜토리얼",
      │   "keywords": ["React", "JavaScript", "웹개발"],
      │   "mood": "실용적",
      │   "difficulty": "중급"
      │ }
      ↓
┌─────────────┐
│   Worker    │
└─────┬───────┘
      │ 20. 태그 저장 (트랜잭션)
      ↓
┌──────────────┐
│ PostgreSQL   │ BEGIN;
└──────────────┘ 
                -- 태그가 없으면 생성
                INSERT INTO tags (name, category, ...)
                  ON CONFLICT (name) DO NOTHING;
                
                -- 링크-태그 연결
                INSERT INTO link_tags (
                  link_id, tag_id, 
                  is_auto_generated=true,
                  confidence=0.95
                );
                
                -- 링크 상태 최종 업데이트
                UPDATE links SET 
                  status = 'completed',
                  updated_at = NOW()
                WHERE id = 12345;
                
                COMMIT;
      │
      │ 21. 캐시 무효화
      ↓
┌─────────────┐
│   Redis     │ DEL cache:link:12345
└─────────────┘ DEL cache:links:user:{user_id}:*
      │
      │ 22. Lock 해제
      ↓
┌─────────────┐
│   Redis     │ DEL lock:scrape:{url_hash}
└─────────────┘
      │
      │ 23. WebSocket 알림 (Optional)
      ↓
┌──────────┐
│   App    │ 🔔 "태그 생성 완료!"
└──────────┘    새로고침 또는 실시간 업데이트
```

**소요 시간**:
- 사용자 저장 요청 → 응답: **< 500ms**
- 전체 처리 (크롤링 + 태그): **5-15초**

## 2. 링크 목록 조회 플로우 (캐싱)

```
┌──────────┐
│   App    │
└─────┬────┘
      │ GET /api/v1/links?page=1&tag=tech&limit=20
      ↓
┌─────────────┐
│ API Server  │
└─────┬───────┘
      │ 1. 인증 확인
      │ 2. 캐시 키 생성
      │    cache_key = "cache:links:user:123:page:1:tag:tech:limit:20"
      ↓
┌─────────────┐
│   Redis     │ GET cache_key
└─────┬───────┘
      │
      ├─ Cache HIT (캐시 있음) ──────────┐
      │                                  │
      │                                  ↓
      │                        ┌─────────────┐
      │                        │ API Server  │
      │                        └─────┬───────┘
      │                              │ JSON 파싱
      │                              │ 즉시 응답
      │                              ↓
      │                        ┌──────────┐
      │                        │   App    │ ⚡ 빠름! (~50ms)
      │                        └──────────┘
      │
      └─ Cache MISS (캐시 없음)
      │ 3. DB 쿼리
      ↓
┌──────────────┐
│ PostgreSQL   │ SELECT l.*, array_agg(t.name) as tags
└──────────────┘ FROM links l
                LEFT JOIN link_tags lt ON l.id = lt.link_id
                LEFT JOIN tags t ON lt.tag_id = t.id
                WHERE l.user_id = 123
                  AND l.deleted_at IS NULL
                  AND (t.name = 'tech' OR 'tech' IS NULL)
                GROUP BY l.id
                ORDER BY l.created_at DESC
                LIMIT 20 OFFSET 0;
      │
      │ 4. 썸네일 URL 생성
      ↓
┌─────────────┐
│   MinIO     │ Generate Pre-signed URLs
└─────┬───────┘  (만료: 1시간)
      │
      │ {
      │   "id": 12345,
      │   "title": "...",
      │   "thumbnail_url": "http://minio:9000/thumbnails/.../12345.jpg?X-Amz-Expires=3600&..."
      │ }
      ↓
┌─────────────┐
│ API Server  │
└─────┬───────┘
      │ 5. 캐시 저장 (TTL: 5분)
      ↓
┌─────────────┐
│   Redis     │ SET cache_key <JSON> EX 300
└─────┬───────┘
      │ 6. 응답
      ↓
┌──────────┐
│   App    │ (~200ms)
└──────────┘
```

## 3. 태그별 필터링 플로우

```
┌──────────┐
│   App    │ 사용자가 '기술' 태그 클릭
└─────┬────┘
      │ GET /api/v1/tags/tech/links?page=1
      ↓
┌─────────────┐
│ API Server  │
└─────┬───────┘
      │ 1. 캐시 확인
      ↓
┌─────────────┐
│   Redis     │ GET cache:tag:tech:links:page:1
└─────┬───────┘
      │ Cache MISS
      ↓
┌──────────────┐
│ PostgreSQL   │ SELECT l.*
└──────────────┘ FROM links l
                INNER JOIN link_tags lt ON l.id = lt.link_id
                INNER JOIN tags t ON lt.tag_id = t.id
                WHERE t.name = 'tech'
                  AND l.user_id = 123
                  AND l.deleted_at IS NULL
                ORDER BY l.created_at DESC
                LIMIT 20;
      │
      ↓
      (MinIO URL 생성, 캐싱, 응답)
```

## 4. 검색 플로우

```
┌──────────┐
│   App    │ 사용자가 "React hooks" 검색
└─────┬────┘
      │ GET /api/v1/search?q=React+hooks
      ↓
┌─────────────┐
│ API Server  │
└─────┬───────┘
      │ 1. 검색어 정규화
      │    "react hooks" → ["react", "hooks"]
      ↓
┌──────────────┐
│ PostgreSQL   │ SELECT l.*, 
└──────────────┘   ts_rank(
                    to_tsvector('english', l.title || ' ' || l.description),
                    to_tsquery('english', 'react & hooks')
                  ) AS rank
                FROM links l
                WHERE to_tsvector('english', l.title || ' ' || l.description)
                  @@ to_tsquery('english', 'react & hooks')
                  AND l.user_id = 123
                ORDER BY rank DESC, l.created_at DESC
                LIMIT 20;
      │
      ↓
      (응답)
```

## 5. 동기화 플로우 (Pull)

```
┌──────────┐
│   App    │ 앱 시작 시 또는 수동 새로고침
└─────┬────┘
      │ 1. 마지막 동기화 시간 확인 (로컬 DB)
      │    last_sync = "2025-10-05T09:00:00Z"
      │
      │ 2. 서버에 변경사항 요청
      │ GET /api/v1/sync/pull?since=2025-10-05T09:00:00Z
      ↓
┌─────────────┐
│ API Server  │
└─────┬───────┘
      │ 3. 변경된 링크 조회
      ↓
┌──────────────┐
│ PostgreSQL   │ SELECT * FROM links
└──────────────┘ WHERE user_id = 123
                  AND updated_at > '2025-10-05T09:00:00Z'
                  AND deleted_at IS NULL
                ORDER BY updated_at ASC;
      │
      │ 4. 썸네일 URL 포함
      ↓
┌─────────────┐
│   MinIO     │ Generate Pre-signed URLs
└─────┬───────┘
      │
      │ 5. 응답
      │ {
      │   "links": [...],
      │   "sync_timestamp": "2025-10-05T10:30:00Z"
      │ }
      ↓
┌──────────┐
│   App    │
└─────┬────┘
      │ 6. 로컬 DB 업데이트 (SQLite)
      │    - 새 링크 INSERT
      │    - 기존 링크 UPDATE
      │    - 삭제된 링크 처리
      │
      │ 7. last_sync 업데이트
      ↓
  [Local SQLite]
```

## 6. 동기화 플로우 (Push)

```
┌──────────┐
│   App    │ 사용자가 오프라인에서 메모 추가
└─────┬────┘
      │ 로컬 DB에 저장
      │ sync_pending = true
      │
      │ (나중에 네트워크 연결)
      │
      │ POST /api/v1/sync/push
      │ {
      │   "changes": [{
      │     "link_id": 12345,
      │     "type": "note_update",
      │     "content": "이 영상 나중에 다시 보기",
      │     "timestamp": "2025-10-05T10:15:00Z"
      │   }]
      │ }
      ↓
┌─────────────┐
│ API Server  │
└─────┬───────┘
      │ 1. 충돌 감지
      ↓
┌──────────────┐
│ PostgreSQL   │ SELECT version, updated_at
└──────────────┘ FROM links WHERE id = 12345;
      │
      │ version = 3, updated_at = "2025-10-05T10:00:00Z"
      │ 클라이언트 timestamp = "2025-10-05T10:15:00Z"
      │ → 충돌 없음 (서버가 더 오래됨)
      │
      │ 2. 업데이트
      ↓
┌──────────────┐
│ PostgreSQL   │ UPDATE notes SET content = '...'
└──────────────┘ WHERE link_id = 12345;
                
                UPDATE links SET 
                  version = version + 1,
                  updated_at = NOW()
                WHERE id = 12345;
      │
      │ 3. 응답
      │ { "status": "success", "new_version": 4 }
      ↓
┌──────────┐
│   App    │ sync_pending = false
└──────────┘
```

## 7. 실패 처리 플로우

```
┌─────────────┐
│   Worker    │ URL 크롤링 중 에러 발생
└─────┬───────┘
      │ Exception: Timeout or 404
      │
      │ 1. 재시도 카운트 증가
      ↓
┌──────────────┐
│ PostgreSQL   │ UPDATE links SET
└──────────────┘   retry_count = retry_count + 1
                WHERE id = 12345;
      │
      │ retry_count < 3 ?
      │
      ├─ YES → 재시도
      │          ↓
      │   ┌─────────────┐
      │   │  RabbitMQ   │ PUBLISH to 'scrape.queue'
      │   └─────────────┘ (delay: 30초)
      │
      └─ NO → 실패 처리
                ↓
          ┌──────────────┐
          │ PostgreSQL   │ UPDATE links SET
          └──────────────┘   status = 'failed',
                            processing_error = '...'
                ↓
          ┌─────────────┐
          │  RabbitMQ   │ PUBLISH to 'dlq.queue'
          └─────────────┘ (수동 처리 대기)
                ↓
          ┌──────────┐
          │   App    │ 🔔 "링크 처리 실패: 확인 필요"
          └──────────┘
```

## 플로우 요약

| 작업 | 동기/비동기 | 평균 소요시간 | 캐싱 여부 |
|------|------------|--------------|----------|
| 링크 저장 (응답) | 동기 | < 500ms | ❌ |
| 전체 처리 | 비동기 | 5-15초 | ❌ |
| 목록 조회 (캐시 히트) | 동기 | ~50ms | ✅ |
| 목록 조회 (캐시 미스) | 동기 | ~200ms | ✅ |
| 검색 | 동기 | ~300ms | ❌ |
| 동기화 (Pull) | 동기 | 1-3초 | ❌ |
| 동기화 (Push) | 동기 | ~500ms | ❌ |