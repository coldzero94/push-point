# 데이터 플로우

> Push-Point v2.1 — 마지막 업데이트: 2026-07-21

모든 흐름은 단일 프로세스(`pushpoint` 바이너리) 안에서 일어난다.
v1처럼 API 서버 → Redis → RabbitMQ → Worker로 네트워크를 건너다니는 구간이 없고,
컴포넌트 간 이동은 goroutine과 SQLite 트랜잭션뿐이다.
스키마·SQL은 [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md), 엔드포인트는 [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md)를 따른다.

## 1. 링크 저장 플로우 (전체)

동기 구간은 INSERT 두 번이 전부다. 나머지는 전부 jobs 테이블을 통한 비동기 연쇄.

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

> **M2 인터림 (tagger 부재)**: 위 step 10~12는 tagger가 등록된 스테디 상태(M3 이후)를 그린다. M2 시점에는 tagger가 아직 없어(08 마일스톤) scrape 성공 트랜잭션이 `tag` 잡 enqueue와 `status='tagging'` 전이를 **생략**하고 `links.status`를 곧바로 `done`으로 올린다 (step 11a·12의 tagger 경로는 M3부터 활성). `thumb` 잡(step 11b)은 M2에서도 og:image가 있으면 그대로 enqueue된다. 즉 M2 스테디 상태의 링크 전이는 `pending → scraping → done`이며, `tagging`은 M3에서 도달한다.

**소요 시간** (성능 목표 — p99 판정은 `just bench-http`, 검증 매트릭스는 [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)):
- 저장 요청 → 201 응답: **p99 < 50ms** (v1 목표는 < 500ms — 네트워크 홉이 없어져 자릿수가 줄었다)
- 저장 → 태그 완료 (비동기): **< 3s** (v1의 OpenAI 왕복 포함 5–15초 대비)

## 2. 목록 조회 플로우

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

v1의 Redis 캐시 계층(캐시 키 생성 → HIT/MISS 분기 → TTL → 무효화)은 통째로 삭제했다.
같은 프로세스 안의 SQLite가 `cache_size = -64000`(64MB) 페이지 캐시로 핫 데이터를 이미 메모리에 들고 있어, 캐시를 한 층 더 얹으면 무효화 버그만 산다.

10만 건에서도 keyset 커서는 인덱스 끝에서 20건만 읽으므로 **p99 < 50ms**.

## 3. 검색 플로우

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

FTS5 인덱스는 별도 동기화 잡 없이 links/tags 쓰기와 **같은 트랜잭션**에서 store 계층이 갱신하므로(§1의 10, 12단계), 검색 결과가 목록과 어긋나는 시점이 없다.

## 4. 태그 수정 플로우

사용자의 수정은 곧 학습 데이터다. 교체와 기록을 한 트랜잭션으로 묶는다.

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

`tag_feedback`은 M5에서 임베딩 앙상블의 재랭킹 가중치를 보정하는 학습 데이터로 쓰인다.
사용자가 지운 태그는 점수를 깎고, 손으로 붙인 태그는 올린다.

## 5. 실패 처리 플로우

재시도 스케줄이 Redis 지연 큐가 아니라 jobs 테이블의 `run_after` 컬럼이다.

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

v1의 DLQ(dead letter queue)는 사라졌다. `status='failed'`인 행이 곧 DLQ이고, 조회는 `GET /api/v1/links?status=failed` 한 번이다.

## 6. 크래시 복구 플로우

브로커 없는 인프로세스 큐가 내구성을 갖는 근거. M2 검증 커맨드 `just test-crash`(빌드 → fixture 서버 → 저장 → kill -9 → 재기동 → 전량 done 단언)가 이 흐름을 자동으로 검증한다.

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

단일 프로세스이므로 재시작 시 다른 워커가 running 잡을 들고 있을 가능성이 0이고, 복구 쿼리를 무조건 실행해도 안전하다.

## 7. 클라이언트 캡처 플로우

폰에서 서버로 링크가 들어오는 경로는 두 가지다. 어느 쪽이든 서버 측 진입점은 §1의 `POST /api/v1/links` 하나다.

### 7.1 iOS 단축어 (M1+)

앱 설치 없이 M1 서버만으로 폰 실사용을 시작하는 경로다 (M1 DoD: 폰 단축어로 실제 저장 1건).

```
공유 시트 → 단축어 실행
         → "URL 콘텐츠 가져오기"로 POST /api/v1/links
           (Authorization: Bearer 헤더 포함)
         → 201/200 응답 확인 → 종료
```

### 7.2 Share Extension (M4+, App Group 로컬 큐)

"요청 발사 후 즉시 닫기"는 in-flight 유실이 생기므로 금지한다. 로컬 큐에 우선 기록하는 순서가 캡처 정책이다.

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

서버가 꺼져 있어도 공유 시트 저장은 항상 2초 내 성공(로컬 큐 적재)하고, 서버 복구 후 자동 업로드 유실이 0건이다 (M4 DoD). 재시도가 안전한 근거는 `POST /api/v1/links`의 `url_hash` 멱등성이다 ([06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) 4.1절).

## 8. 플로우 요약

| 작업 | 동기/비동기 | 목표 시간 |
|------|------------|----------|
| 링크 저장 (201 응답) | 동기 | p99 < 50ms |
| 저장 → 태그 완료 | 비동기 | < 3s |
| 목록 조회 (10만 건, keyset) | 동기 | < 50ms |
| 검색 (FTS5, 1만 링크) | 동기 | < 30ms |
| 태그 수정 (PATCH) | 동기 | < 50ms (단일 트랜잭션) |
| 썸네일 생성 | 비동기 (best-effort) | 링크 상태와 무관 |
| 실패 재시도 | 비동기 | run_after = now + 30 × attempts |
| 크래시 복구 | 시작 시 1회 | 콜드 스타트 < 1s 포함 |

v1의 동기화(sync pull/push) 플로우는 삭제됐다. 단일 사용자 + 서버가 유일한 진실 원천이므로 충돌 해소·버전 관리가 필요 없고, iOS 앱은 커서 페이지네이션으로 항상 서버를 직접 읽는다. Redis 캐시 플로우 역시 §2의 이유로 존재하지 않는다.
