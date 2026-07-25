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

### 7.3 클라이언트 본문 캡처 (브라우저 확장 / Share Extension)

서버가 URL을 fetch해도 본문을 못 얻는 세 부류가 있다 — **SPA**(콘텐츠가 JS에 있음), **봇 차단**,
**로그인 벽·유료 구독**. 사용자는 이미 그 페이지를 자기 브라우저에서 보고 있으므로, 서버가 몰래
긁는 대신 **클라이언트가 렌더된 본문을 저장 요청에 실어 보낸다**. 세 부류가 한 번에 풀린다.

```
브라우저 확장(격리 월드) → 현재 탭에서 제목·설명·본문 텍스트 추출
                        → POST /api/v1/links {url, title, description, body_text}
                          (API 키는 확장 스토리지에 있고 페이지 JS는 접근 불가)
                        → 서버: body_source='client' 표시
                                + scrape 잡(썸네일·메타용) + **tag 잡 즉시 enqueue**
```

- **tag 잡을 저장 시점에 함께 넣는 이유**: tag 잡의 유일한 생성 지점이 `ApplyScrape`(스크랩 성공)라
  스크랩이 실패하는 바로 그 페이지에서는 태그·요약이 영원히 안 생긴다.
- **우선순위**: `body_source='client'`면 이후 스크랩이 3필드를 덮지 않는다. 스크랩은 계속 돌아
  썸네일·author·published_at을 채우고, 확정 실패해도 링크는 `done`으로 남는다.
- **이미 저장된 링크 보충**: 중복 저장 요청이 클라이언트 본문을 실어 오고 저장된 본문이 서버
  출처면 3필드를 **1회 보충**하고 태깅을 다시 돌린다(이미 클라이언트 본문이면 무동작).
  반복 호출은 같은 상태로 수렴하므로 재시도 안전성은 유지된다.
- 서버는 렌더된 HTML을 받지 않는다 — 클라이언트가 평문까지 뽑아 보낸다(요청 크기, 저장 API의
  동기 경로 예산, 적대적 HTML이 DB에 상주하지 않는 이점).
- **캡처 규칙은 플랫폼 간 한 파일로 공유한다** — `extension/src/extract.js`는 플랫폼 API를
  참조하지 않고 문서 하나를 받아 저장 계약을 만들며, 마지막 표현식으로 그 값을 돌려준다.
  Chrome `executeScript`와 iOS `WKWebView.evaluateJavaScript`가 값을 받는 방식이 같으므로
  **M4 Share Extension이 같은 파일을 그대로 쓴다.** 플랫폼을 더할 때 새로 쓰는 것은 전송
  코드뿐이고, 빼려면 그 폴더만 지운다. 서버는 어느 플랫폼이 보냈는지 알지 못한다.

### 7.3.1 공유 출처에 따라 무엇이 들어오는가

"공유 버튼을 누르면 알아서 되는" 정도는 **출처가 무엇을 주느냐**로 정해진다. Share Extension은
소스 앱이 넘긴 것 이상을 만들어낼 수 없다.

| 공유 출처 | Share Extension이 받는 것 | 얻는 것 |
|---|---|---|
| **Safari** | URL + **JS 전처리 결과** — `NSExtensionAttributes`의 `NSExtensionJavaScriptPreprocessingFile`에 지정한 JS를 Safari가 확장 시작 **전에** 페이지에서 실행해 그 반환값을 넘긴다 | **본문까지** — `extension/src/extract.js`를 그대로 지정하면 웹 확장과 같은 규칙으로 캡처된다 |
| **네이티브 앱**(인스타그램 등) | `NSItemProvider` 항목들 — 대개 `public.url`, 앱에 따라 `public.plain-text`·`public.image`도 | 그 앱이 준 것만. URL뿐이면 서버가 못 가져오는 사이트는 **빈 채로 저장**된다 |

그래서 설계 규칙 셋:

1. **오는 것을 전부 계약에 매핑한다.** URL만 보지 말고 텍스트·이미지 항목도 확인해
   `title`/`description`에 채운다 — 앱에 따라 캡션을 함께 준다.
2. **Safari 공유는 JS 전처리로 본문을 얻는다.** `extract.js`는 플랫폼 API를 참조하지 않고
   마지막 표현식으로 페이로드를 돌려주므로 Safari 전처리 파일로 **그대로** 쓸 수 있다
   (§7.3의 "규칙은 한 파일"이 여기서 값을 한다).
3. **로그인 벽은 앱 내 세션으로 푼다(M4 결정 대상).** 앱의 `WKWebView`는 자체 쿠키 저장소를
   가지므로, 사용자가 앱 안에서 해당 사이트에 한 번 로그인해두면 이후 그 URL을 앱이 렌더해
   본문을 뽑을 수 있다. 네이티브 앱 공유로 들어온 로그인 벽 콘텐츠(인스타그램 등)를 살릴 수
   있는 유일한 경로다. **서버가 자격증명을 갖지 않는다는 원칙은 그대로다** — 세션은 기기 안에만 있다.

실측(2026-07-25): 인스타그램 게시물 URL은 서버가 받아도 HTTP 200 623KB에 og 메타가 0이고
스크립트 493KB 어디에도 캡션이 없다(로그인 상태에서 XHR로 불러온다). 그래서 어댑터가 아예
요청하지 않는 현재 선택이 옳고, 이 부류는 위 1~3 없이는 URL만 남는다.

### 7.4 서버 없이 도는 iOS (임베드 모드)와 저장 경로

iOS는 두 모드로 돌 수 있다 — **홈서버**(별도 Go 서버 + Tailscale)와 **내장 자립형**(백엔드를
gomobile로 앱에 넣고 폰 안 `127.0.0.1`에서 인프로세스로 실행). 저장 경로가 두 모드에서
갈라지지 않게 하는 것이 이 절의 요점이다.

**성립 조건은 하나 — 저장의 단위는 HTTP 호출이 아니라 페이로드다.**

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

- **Share Extension은 어느 모드에서도 서버에 직접 붙지 않는다.** 별개 프로세스라 앱의
  인프로세스 서버에 닿을 수 없고(앱이 떠 있지 않을 수도 있다), 홈서버가 꺼져 있을 수도 있다.
  그래서 항상 로컬 큐가 1차 목적지다 — §7.2의 유실 0 보장이 두 모드에 그대로 적용된다.
- **큐에 담는 바이트가 HTTP 바디와 동일**하므로 드레인 코드가 모드에 따라 갈라지지 않는다.
  모드 전환은 "어디로 POST하는가"(base URL) 하나뿐이고, 실패 시 재시도도 `url_hash` 멱등성
  덕에 그대로 안전하다.
- **검증·정제는 `store.SaveInput.Normalize`가 하고 `SaveLink`가 스스로 호출한다.** HTTP
  핸들러에 두면 큐 드레인이 gomobile 바인딩으로 직접 호출할 때 통째로 건너뛰어져, 같은
  페이로드가 경로에 따라 다르게 저장된다. 진입점이 우회할 수 없는 자리에 두는 것이 이 구조의
  전제다.
- 내장 모드에서 **인프로세스 서버를 loopback에 띄우는 선택**은 SwiftUI 앱 코드를 모드에
  무관하게 유지하기 위한 것이다(클라이언트는 계약만 안다). 바인딩 직접 호출로 바꾸더라도
  위 정제 보장은 그대로다.
- 워커(스크랩·태깅·요약)는 내장 모드에서 iOS 백그라운드 제약을 받는다 — 저장은 즉시지만
  보강은 "앱이 떠 있을 때 + BGTaskScheduler 창"에서 진행된다. 저장 자체가 네트워크에
  의존하지 않는 것이 이 설계에서 중요한 이유다.

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
