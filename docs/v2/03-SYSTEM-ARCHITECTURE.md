# 시스템 아키텍처

> Push-Point v2 — 마지막 업데이트: 2026-07-21

## 1. 전체 구성

v2는 API 서버와 워커가 하나의 Go 프로세스(`backend/cmd/pushpoint/main.go`)에서 동작하는 단일 바이너리 아키텍처다. v1이 API Server / Worker / PostgreSQL / Redis / MinIO를 각각 k8s Pod로 띄웠던 것과 달리, v2의 모든 구성 요소는 goroutine과 SQLite 파일 하나로 수렴한다. 모노레포(backend / nlu / ios / frontend)는 저장소 배치일 뿐이며, 실행 단위는 여전히 backend가 빌드하는 단일 바이너리 하나다.

```
┌─────────────────────────────────────────────────┐
│                push-point (단일 바이너리)          │
│                                                 │
│  HTTP API ──▶ enqueue ──▶ jobs 테이블 (SQLite)   │
│     │                          │                │
│     │                    dispatcher (goroutine) │
│     │                          │                │
│     │              ┌───────────┴──────────┐     │
│     │              ▼                      ▼     │
│     │         scraper pool           tagger     │
│     │         (bounded N)         (NLU 파이프라인)│
│     │              │                      │     │
│     ▼              ▼                      ▼     │
│  SQLite (WAL) ◀── links / tags / FTS5 ◀──┘     │
│  data/thumbs/ ◀── 썸네일                        │
└─────────────────────────────────────────────────┘
        ▲                          ▲
   iOS Share Ext              iOS 앱 (목록/검색)
```

### 핵심 흐름

설계의 중심 원칙은 하나다. **저장 API는 enqueue만 하고 즉시 201을 반환한다. 무거운 일은 전부 비동기다.**

1. iOS Share Extension이 `POST /api/v1/links`를 호출한다.
2. 핸들러는 `INSERT INTO links` + `INSERT INTO jobs(kind='scrape')`를 한 트랜잭션으로 커밋하고, 인프로세스 채널로 dispatcher에 notify한 뒤 즉시 `201 {id, status:"pending", created_at}`을 반환한다. 동기 구간에 존재하는 작업은 INSERT 두 번이 전부다 — p99 < 50ms의 근거.
3. dispatcher가 jobs 테이블에서 잡을 claim해 scraper pool에 넘긴다.
4. scrape 성공 트랜잭션에서 `tag` 잡과 (og:image가 있으면) `thumb` 잡을 연쇄 enqueue한다.
5. tagger가 NLU 파이프라인으로 태그를 붙이면 링크 상태가 `done`이 된다. 저장부터 태그 완료까지 목표는 3초 미만이다.

> **M2 인터림 (tagger 부재)**: step 4~5는 tagger가 등록된 스테디 상태(M3 이후)다. tagger는 M3(Phase A)에서 도입되므로(08 마일스톤·아래 `backend/internal/tagger` 절), M2 시점에는 scrape 성공 트랜잭션이 `tag` 잡을 만들지 않고 `links.status`가 곧바로 `done`이 된다 (`thumb` 잡은 og:image가 있으면 M2에서도 연쇄 enqueue). 따라서 M2 스테디 상태의 링크 전이는 `pending → scraping → done`이며, `tagging` 상태는 M3부터 도달한다.

클라이언트 관점에서는 "저장은 순간, 나머지는 알아서"다. 공유 시트가 2초 내 닫히는 UX는 확장이 App Group의 공유 SQLite에 직접 쓰는 것이 보장하고(서버가 없어도 성립 — [02-TECH-SPEC.md](02-TECH-SPEC.md) iOS 절), 이 비동기 구조는 저장 API의 p99 < 50ms 응답을 만든다.

## 2. 저장소 구조와 패키지별 역할

```
push-point/
├── api/                       # API 계약 (기계 원본)
│   ├── openapi.yaml           # OpenAPI 3.1 — 백엔드·클라이언트가 여기서 생성
│   └── README.md
├── backend/                   # Go 단일 바이너리 (API + worker + NLU 런타임 추론)
│   ├── cmd/pushpoint/main.go  # 단일 진입점
│   ├── internal/
│   │   ├── api/               # HTTP 핸들러 (chi)
│   │   │   └── gen/           # oapi-codegen 생성물 (just gen, 커밋 대상)
│   │   ├── config/            # PUSHPOINT_* 환경 변수 로딩
│   │   ├── store/             # Store 인터페이스 + sqlite 구현
│   │   ├── queue/             # Queue 인터페이스 + sqlite jobs 구현
│   │   ├── scraper/           # fetch + goquery 파싱, singleflight
│   │   ├── safedial/          # SSRF 가드 (사설 대역 다이얼 차단)
│   │   ├── tagger/            # M3: 규칙 태거 (Phase B는 신호 하나로 합류 — 아래 참조)
│   │   ├── thumbs/            # 썸네일 생성·저장
│   │   └── web/               # SPA 서빙 (embed_frontend 태그 전용)
│   ├── migrations/            # SQLite 마이그레이션 (golang-migrate, embed)
│   └── go.mod                 # module github.com/coby/push-point/backend
├── nlu/                       # NLU 오프라인 자산 (런타임 코드 아님)
│   ├── dictionary/            # 태그 사전 정의·시드 (커밋 대상)
│   ├── golden/                # 태깅 품질 golden set (JSONL, 커밋 대상)
│   └── models/                # M5: ONNX 변환 스크립트(Python)·모델 아티팩트
├── ios/                       # M4: SwiftUI 앱 + Share Extension
├── frontend/                  # 웹 SPA (Vite + React 19 + TS) — iOS와 대등한 full-feature 클라이언트
├── docs/
│   ├── README.md              # v1 ↔ v2 문서 인덱스·비교
│   ├── v1/                    # v1 기획서 아카이브 (수정 금지)
│   └── v2/                    # 현행 문서 (단일 진실 원천)
├── deploy/k8s-future/         # v1 k8s 매니페스트 보존 (미사용)
├── CLAUDE.md
└── justfile                   # dev / test / bench / lint / eval (backend 대상)
```

NLU 경계: 런타임 추론(규칙 태거, ONNX 추론)은 `backend/internal/tagger`의 Go 코드다. `nlu/`는 코드가 아니라 자산 — 태그 사전 정의, golden set, 모델 변환 스크립트(Python은 여기만 허용), ONNX 아티팩트 — 이며, backend는 nlu/의 산출물(사전 시드, .onnx 파일)을 읽기만 한다.

### backend/internal/api

표준 `net/http` + chi 라우터의 HTTP 핸들러 계층. Bearer API 키 인증 미들웨어(`PUSHPOINT_API_KEY`, 헬스체크·썸네일 제외), 요청 검증, 응답 직렬화를 담당한다. 비즈니스 로직은 갖지 않고 store/queue를 호출한다. 서버 인터페이스와 요청/응답 타입은 계약 원본 `api/openapi.yaml`에서 생성된다(`gen/`, 커밋 대상) — contract-first라 핸들러는 생성된 인터페이스의 구현이다. `/debug/pprof`가 기본 탑재된다. 엔드포인트 전체 목록은 [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) 참고.

### backend/internal/store

`Store` 인터페이스와 sqlite 구현. 링크/태그/검색/통계의 모든 데이터 접근이 이 인터페이스를 통과한다. 드라이버는 modernc.org/sqlite(CGO-free, 순수 Go)로 단일 정적 바이너리를 유지한다. FTS5(`links_fts`) 동기화도 store 계층의 책임 — 링크/태그 쓰기와 같은 트랜잭션에서 DELETE 후 INSERT로 갱신한다. 마이그레이션은 golang-migrate + `embed.FS`로 바이너리에 내장되어 시작 시 자동 적용된다.

### backend/internal/queue

`Queue` 인터페이스와 SQLite `jobs` 테이블 구현. enqueue / claim / 완료·실패 처리 / 재시도 백오프(`run_after = unixepoch() + 30 * attempts`, `max_attempts` 3)를 담당한다. dispatcher goroutine이 notify 채널과 1초 폴링 티커를 병행해 잡을 집어 각 워커에게 분배한다 — 채널은 지연 0의 즉시 반응, 티커는 `run_after` 도래(재시도 스케줄) 감지용이다.

### backend/internal/scraper

URL fetch + goquery 파싱. `<title>`, og:title/description/image/site_name, meta keywords, article:published_time, author, lang을 추출하고, 도메인·URL 패턴 휴리스틱으로 content_type을 판정한다(youtube/vimeo → video, twitter/x → post, 기본 article). YouTube는 oEmbed를 병용한다(API 키 불필요). 안전 장치:

- `semaphore(PUSHPOINT_SCRAPE_CONCURRENCY, 기본 8)` — 동시 fetch 상한
- 도메인별 rate limit(도메인당 1 req/s) — 대상 사이트 예의
- `singleflight`(url_hash 기준) — 동일 URL 동시 스크랩 제거
- 요청당 context timeout 10s, 응답 본문 최대 5MB

### backend/internal/tagger

**Tagger 인터페이스는 없다** — 앙상블 지점이 `classify.go`의 `score map[int64]float64`이기 때문이다. 도메인·제목·분류·설명·메모·본문 여섯 신호가 지금도 거기서 가법 합성되고 있고, Phase B(ONNX)는 **일곱 번째 신호**로 같은 맵에 들어간다. 인터페이스로 갈라 구현체를 교체하면 두 신호 중 하나만 고를 수 있게 되는데, 앙상블은 정의상 둘을 함께 쓰는 것이라 그 모양이 오히려 방해가 된다. 자유 태그 생성이 아니라 통제된 태그 사전(30~50개)에 대한 분류로 문제를 좁혀 LLM 없이 품질을 확보한다. Phase A는 도메인 휴리스틱 + 조사 접미 정규화 기반 후보구 추출·TF-IDF 스코어링 + 사전 매칭, Phase B는 ONNX 임베딩 코사인 유사도와의 앙상블이다. 품질은 `nlu/golden/` golden set의 `just eval`로 측정하며, 게이트는 상대 조건이다 — M5 진입: Phase A가 "도메인 휴리스틱만" 베이스라인 대비 +15pp, M5 종료: 앙상블이 Phase A 대비 +10pp (절대 60%/80%는 참고치). 상세는 [02-TECH-SPEC.md](02-TECH-SPEC.md) 참고.

### backend/internal/thumbs

og:image 다운로드 → 최대 폭 640px 리사이즈 → JPEG q80으로 `data/thumbs/{hash[:2]}/{url_hash}.jpg` 저장. 단일 사이즈다(v1의 small/medium/large 3종 리사이즈 폐기). `thumb` 잡은 best-effort — 실패해도 링크 상태에 영향이 없고 `thumb_path`만 NULL로 남는다. 서빙은 `GET /thumbs/{path}`.

## 3. 고성능 설계 포인트 — k8s 없이 성능을 얻는 방법

v1은 성능을 수평 확장(HPA, 워커 복제)에서 찾았다. v2는 단일 프로세스 설계의 질에서 찾는다.

### SQLite: WAL + busy_timeout + 커넥션 전략

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA cache_size = -64000;   -- 64MB
```

커넥션은 **writer 1개 + reader 풀(N=4)**. 쓰기는 직렬화되지만 개인 규모(초당 수십 저장)에서 병목이 아니고, WAL 덕에 읽기는 쓰기와 동시에 진행된다. `busy_timeout`은 락 경합 시 에러 대신 대기를 택한다. 모든 쓰기는 트랜잭션이다.

### jobs 테이블 = 내구성 큐

v1의 Redis Streams가 하던 일을 SQLite 테이블 하나가 대체한다. 프로세스가 하나이므로 네트워크 큐가 필요 없고, 재시작 내구성은 잡이 DB 트랜잭션 안에 있다는 사실 자체가 보장한다. claim은 원자적 UPDATE 한 문장이다:

```sql
UPDATE jobs SET status='running', claimed_at=unixepoch(), attempts=attempts+1
WHERE id = (
  SELECT id FROM jobs
  WHERE status='pending' AND run_after <= unixepoch()
  ORDER BY id LIMIT 1
)
RETURNING id, kind, link_id, attempts;
```

writer가 1개이므로 경합 없이 동작하고, `idx_jobs_claim(status, run_after)` 인덱스가 스캔을 상수 수준으로 유지한다. `links` INSERT와 `jobs` INSERT가 한 트랜잭션이므로 "저장은 됐는데 잡이 유실"되는 상태가 원천적으로 불가능하다 — Redis Streams + PostgreSQL 조합에서는 별도 노력이 필요했던 성질이다.

### FTS5: 별도 검색 엔진 불필요

`links_fts` 가상 테이블(trigram 토크나이저, 한국어 부분 문자열 매칭)에 title/description/note/tags를 인덱싱하고 bm25 랭킹으로 검색한다. 본문 데이터와 같은 파일, 같은 트랜잭션에서 동기화되므로 인덱스 지연·불일치가 없다. 1만 링크 기준 목표 30ms 미만.

### keyset 페이지네이션

목록/검색은 OFFSET 없이 `(created_at, id)` 커서 기반 keyset 페이지네이션만 사용한다. `idx_links_list(created_at DESC, id DESC)` 부분 인덱스와 결합해 10만 건에서도 스크롤 API p99 < 50ms를 유지한다. OFFSET은 깊어질수록 선형으로 느려지므로 금지.

### pprof 기본 탑재

`/debug/pprof`가 항상 켜져 있고, 매 마일스톤 성능 게이트를 검증한다 — p99 판정은 `just bench-http`, 마이크로벤치는 `just bench`. **성능 주장은 측정으로 뒷받침한다** — "빨라졌다"는 프로파일과 벤치 수치 없이는 문서에 쓰지 않는다.

## 4. 동시성 모델

프로세스 하나 안의 goroutine 구성:

```
main
├── HTTP 서버 (net/http, 요청당 goroutine)
├── dispatcher ×1     # notify 채널 + 1초 티커, jobs claim → 워커 분배
├── scraper pool ×8   # PUSHPOINT_SCRAPE_CONCURRENCY, semaphore 제한
├── tagger ×2         # NLU 파이프라인 (CPU 바운드)
└── thumb  ×2         # 이미지 리사이즈 (best-effort)
```

- **HTTP ↔ 워커 간 결합은 jobs 테이블뿐이다.** 채널 notify는 지연을 줄이는 최적화일 뿐, 유실돼도 티커가 잡는다.
- **graceful shutdown**: SIGINT/SIGTERM 수신 시 HTTP 서버는 진행 중 요청을 마치고 리스너를 닫는다. dispatcher는 새 claim을 멈추고, 각 워커는 context 취소로 진행 중 잡을 중단한다. 중단된 잡은 `running` 상태로 남아도 무방하다 — 다음 시작 시 복구되기 때문.
- **크래시 복구**: 시작 시 `UPDATE jobs SET status='pending' WHERE status='running'` 한 문장. `kill -9` 후 재시작해도 미처리 잡이 그대로 재개된다(M2 DoD의 검증 항목). Consumer Group의 pending entry 관리 같은 별도 메커니즘이 필요 없다.

## 5. 성능 목표

로컬 M-시리즈 기준. p99 판정은 `just bench-http`, 마이크로벤치는 `just bench`로 검증한다 (검증 매트릭스는 [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)).

| 지표 | 목표 |
|---|---|
| 저장 API p99 | < 50ms |
| 저장 → 태그 완료 (비동기) | < 3s |
| 검색 (FTS5, 1만 링크) | < 30ms |
| 링크 10만 건에서 목록 스크롤 API | < 50ms |
| 콜드 스타트 (바이너리 실행 → 서빙) | < 1s |

## 6. 확장 경로 — deploy/k8s-future가 부활하는 시나리오

v1의 k8s 매니페스트는 삭제하지 않고 `deploy/k8s-future/`에 보존했다. 지금 접는 것이지 버리는 것이 아니다. 핵심 저장·큐 로직이 `Store` / `Queue` 인터페이스 뒤에 있으므로, 확장은 재작성이 아니라 구현체 교체다. 태거는 인터페이스가 아니라 **신호 합성 맵**으로 확장한다(위 §Tagger).

| 트리거 | 바뀌는 것 | 바뀌지 않는 것 |
|---|---|---|
| 멀티유저 도입 결정 | 인증(API 키 → 계정), 스키마에 user 차원 추가 | API 형태, 핸들러 구조 |
| 쓰기량이 단일 writer 한계 초과 | `Store` sqlite 구현 → PostgreSQL 구현 | `Store` 인터페이스와 호출부 전체 |
| 워커를 별도 프로세스/노드로 분리 | `Queue` jobs 구현 → 네트워크 큐 구현, 워커 바이너리 분리 | enqueue/claim 시맨틱, 잡 종류 |
| 트래픽이 단일 노드 초과 | `deploy/k8s-future/` 매니페스트 부활, 프로세스 복제 | 애플리케이션 코드 |

순서에 의미가 있다. 이 전환들은 전부 **유저가 생긴 뒤**의 일이고, 각 단계는 이전 단계의 측정된 병목이 확인됐을 때만 진행한다. 그 전까지 단일 바이너리 + SQLite가 가장 빠르고, 가장 단순하고, 백업이 `data/` 디렉터리 복사인 아키텍처다.

## 관련 문서

- [02-TECH-SPEC.md](02-TECH-SPEC.md) — 기술 선택의 근거와 NLU 파이프라인 상세
- [04-DATA-FLOW.md](04-DATA-FLOW.md) — 저장 → 스크랩 → 태깅 플로우 상세
- [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md) — 전체 스키마 정의
- [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) — API 명세
- [07-DEPLOYMENT.md](07-DEPLOYMENT.md) — 실행·배포 방법
