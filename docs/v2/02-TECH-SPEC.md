# 기술 스택

> Push-Point v2.1 — 마지막 업데이트: 2026-07-21

v2의 기술 선택 기준은 하나다. **단일 프로세스, 단일 바이너리로 개인 규모에서 최고의 체감 성능을 내는 것.** 분산 인프라로 얻던 것(내구성, 동시성, 검색)을 SQLite와 Go 표준 라이브러리 수준에서 다시 설계한다.

## 1. v1 → v2 변경 요약

| 영역 | v1 | v2 | 이유 |
|---|---|---|---|
| 배포 | Minikube + k8s + HPA | 단일 Go 바이너리 (`just dev` 한 번) | 유저 0명에 오토스케일링은 역설계. 로컬 테스트 마찰 제거 |
| DB | PostgreSQL (k8s pod) | SQLite (WAL 모드) + FTS5 | 개인 앱 규모에서 충분히 고성능. 백업 = 파일 복사 |
| 메시지 큐 | Redis Streams | 인프로세스 워커 풀 (goroutine + SQLite jobs 테이블) | 프로세스 하나면 네트워크 큐 불필요. 재시작 내구성은 jobs 테이블이 보장 |
| 오브젝트 스토리지 | MinIO | 로컬 디스크 (`data/thumbs/`) | 썸네일 몇 GB에 S3 API는 과함 |
| AI 태깅 | OpenAI API | 경량 NLU (규칙 기반 → ONNX 임베딩 2단계) | 비용 0, 수백 ms 응답, 프라이버시. 이 프로젝트의 기술적 차별점 |
| 클라이언트 | React Native (미정) | iOS Share Extension 최우선 (SwiftUI) | 저장 마찰이 2초를 넘으면 매일 쓰는 앱이 못 됨 |
| 인증 | JWT + 회원가입 | 단일 사용자, 정적 API 키 1개 | 멀티유저는 명시적 비목표 |

스택 축소의 이유는 단순하다. v1은 "유저가 많아지면"을 전제로 컴포넌트를 골랐고, 그 결과 개발 루프마다 k8s 클러스터·Redis·PostgreSQL·MinIO를 띄워야 했다. v2는 전제를 뒤집는다 — **먼저 내가 매일 쓰는 앱을 만들고, 확장은 유저가 생긴 뒤의 문제로 미룬다.** 제거한 컴포넌트는 버린 것이 아니라 접은 것이다. k8s 매니페스트는 `deploy/k8s-future/`에 보존하고, 핵심 의존성(Store/Queue/Tagger)은 인터페이스 뒤에 두어 구현체 교체 경로를 열어둔다 (10장 참고).

## 2. 백엔드: Go 표준 라이브러리 중심

- **언어**: Go 1.25+
- **HTTP**: 표준 `net/http` + [chi](https://github.com/go-chi/chi) 라우터. Gin은 사용하지 않는다 — 필요한 것은 라우팅과 미들웨어 체인뿐이고, chi는 `http.Handler` 표준 인터페이스를 그대로 쓰므로 프레임워크 종속이 없다.
- **로깅**: 표준 `log/slog` (JSON 핸들러). zap/logrus 제거 — 표준 라이브러리로 구조화 로깅이 해결된 지 오래다.
- **설정**: 표준 `os.Getenv`, 접두어 `PUSHPOINT_`. viper 제거 — 환경 변수 5개에 설정 프레임워크는 과하다.

| 환경 변수 | 기본값 | 설명 |
|---|---|---|
| `PUSHPOINT_ADDR` | `:8420` | 리슨 주소 |
| `PUSHPOINT_DATA_DIR` | `./data` | DB·썸네일 저장 위치 |
| `PUSHPOINT_API_KEY` | (필수) | Bearer 인증 키. `just dev`는 `dev-key`로 설정 |
| `PUSHPOINT_SCRAPE_CONCURRENCY` | `8` | 스크래퍼 동시성 상한 |
| `PUSHPOINT_LOG_LEVEL` | `info` | slog 레벨 |

- **ORM 없음**: Ent 제거. 스키마가 테이블 7개 수준이므로 `database/sql` + 손으로 쓴 쿼리가 더 읽기 쉽고, FTS5·`RETURNING` 같은 SQLite 고유 기능을 그대로 쓸 수 있다.
- 진입점은 `backend/cmd/pushpoint/main.go` 하나 — API 서버와 워커가 한 프로세스에서 돈다. 전체 구조는 [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md) 참고.

## 3. 데이터: SQLite

### 드라이버 — modernc.org/sqlite

[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)는 SQLite를 순수 Go로 트랜스파일한 CGO-free 드라이버다. 선택 이유:

- **단일 정적 바이너리 유지.** CGO가 없으므로 크로스 컴파일이 `GOOS`/`GOARCH` 플래그만으로 끝나고, 배포물은 파일 하나다. (각주: M1~M4 기준 — M5의 ONNX 배포 형태 결정에 따라 변동될 수 있다. 7장 Phase B의 배포 형태 3택 참고.)
- FTS5 지원 확인됨 — trigram 토크나이저 포함.
- 개인 규모(초당 수십 쓰기)에서 성능 차이는 병목이 아니다.

> 각주: 성능 문제가 실측되면 mattn/go-sqlite3(CGO)로 교체 가능하다. 드라이버 교체는 import 한 줄과 DSN 수정 수준이며, Store 인터페이스 뒤라 상위 코드는 영향 없다.

### 연결 설정 (성능의 근거)

시작 시 다음 PRAGMA를 적용한다 ([05-DATA-SCHEMA.md](05-DATA-SCHEMA.md)와 동일 수치):

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA cache_size = -64000;   -- 64MB
```

- 커넥션 전략: **writer 1개 + reader 풀(N=4)**. 쓰기는 직렬화되지만 개인 규모에서 병목이 아니고, 읽기는 WAL 덕에 쓰기와 동시에 진행된다.
- 모든 쓰기는 트랜잭션. 저장 API는 `INSERT link + INSERT job`을 한 트랜잭션으로 묶는다.
- 데이터 파일: `data/pushpoint.db` (+ `-wal`, `-shm`). 백업은 `data/` 디렉터리 복사.

### 전문 검색 — FTS5 trigram

```sql
CREATE VIRTUAL TABLE links_fts USING fts5(
  title, description, note, tags,
  tokenize = 'trigram'
);
```

trigram 토크나이저를 쓰는 이유는 **한국어 부분 문자열 매칭** 때문이다. 기본 unicode61 토크나이저는 공백 단위 토큰이라 본문의 "쿠버네티스를"이라는 토큰이 "쿠버네티스" 검색에 매칭되지 않는다(조사 문제). trigram은 3자 이상 부분 문자열이면 매칭된다. 형태소 분석기 없이 한국어 검색을 해결하는 가장 저렴한 방법이다. 동기화는 링크/태그 쓰기와 같은 트랜잭션에서 store 계층이 담당한다.

검색어 `q`가 3자 미만이면 400을 반환하지 않고 **LIKE 폴백**으로 처리한다: `links` 테이블의 title/note/description에 대한 LIKE 스캔(ESCAPE 처리), `created_at DESC` 정렬, `rank=null`, 응답 `"mode":"like"` (3자 이상은 FTS5, `"mode":"fts"`). 실측 10만 행 풀스캔 37ms로 검색 예산 내다. 상세는 [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md).

### 마이그레이션 — golang-migrate + embed

`backend/migrations/` 디렉터리를 `embed.FS`로 바이너리에 내장하고, 시작 시 자동 적용한다. 별도 마이그레이션 커맨드나 배포 절차가 없다 — 바이너리를 실행하면 스키마가 맞춰진다.

## 4. 잡 큐: SQLite jobs 테이블 + goroutine 워커 풀

Redis Streams가 하던 일을 두 조각으로 나눠 대체한다. **내구성은 SQLite `jobs` 테이블이, 동시성은 goroutine 워커 풀이 맡는다.** ([04-DATA-FLOW.md](04-DATA-FLOW.md)에 전체 플로우)

1. **enqueue** — 저장 API가 `INSERT INTO links` + `INSERT INTO jobs(kind='scrape')`를 한 트랜잭션으로 커밋하고, 인프로세스 채널로 dispatcher를 notify한 뒤 즉시 201을 반환한다.
2. **claim** — 워커가 `UPDATE ... WHERE id = (SELECT ... LIMIT 1) RETURNING`으로 잡을 원자적으로 가져간다. 프로세스가 하나이므로 분산 락이 필요 없다.
3. **재시도** — 실패 시 `attempts < max_attempts`(기본 3)면 `run_after = unixepoch() + 30 * attempts` 선형 백오프로 `pending` 복귀, 초과면 `failed`.
4. **연쇄** — scrape 성공 트랜잭션에서 `tag` 잡 + (og:image 있으면) `thumb` 잡을 enqueue.
5. **크래시 복구** — 시작 시 `status='running'`인 잡을 전부 `pending`으로 되돌린다. `kill -9` 후에도 미처리 잡이 재개된다 (M2 DoD). dispatcher는 notify 채널과 1초 폴링 티커를 병행해 `run_after` 도래를 감지한다.

동시성 제어는 `golang.org/x/sync` 하나로 해결한다:

- `semaphore` — 스크래퍼 동시성 상한 (`PUSHPOINT_SCRAPE_CONCURRENCY`, 기본 8)
- `singleflight` — `url_hash` 기준 동일 URL 동시 스크랩 제거
- `errgroup` — 워커 풀 수명 관리와 graceful shutdown

tagger/thumb 워커는 각 2 goroutine이면 충분하다.

## 5. 스크래퍼

- **파싱**: [goquery](https://github.com/PuerkitoBio/goquery). `<title>`, og:title / og:description / og:image / og:site_name, meta keywords, article:published_time, author, lang을 추출한다. colly/chromedp 불필요 — 크롤러가 아니라 단건 fetch + 파싱이므로 `net/http` + goquery로 충분하다.
- **content_type 판정**: 도메인·URL 패턴 휴리스틱 (youtube/vimeo → `video`, twitter/x → `post`, 기본 `article`).
- **안전 장치**: 요청당 context timeout 10s, 응답 본문 최대 5MB, 도메인별 rate limit(도메인당 1 req/s).

### 사이트 어댑터

일반 og 메타 파싱만으로 부족한 도메인은 어댑터로 분기한다:

| 도메인 | 처리 |
|---|---|
| youtube.com / youtu.be | oEmbed(`https://www.youtube.com/oembed`, API 키 불필요) + watch 페이지 og:description **병합** — oEmbed에는 설명이 없다. 채널명(author)을 태거 입력 피처에 포함 |
| x.com / twitter.com | `publish.twitter.com/oembed` 분기 — 본문이 JS 렌더링이라 goquery 직접 파싱 불가 |
| blog.naver.com | `m.blog.naver.com`으로 URL 재작성 후 파싱 — 데스크톱 페이지는 iframe 구조 |
| instagram.com | 메타 부재 허용 — domain+URL만으로 `done` 처리 |

M2 DoD의 "대표 도메인 세트"는 이 표가 기준이다 — YouTube / 일반 아티클 / 네이버 블로그 / X에서 제목 확보.

## 6. 썸네일

- og:image 다운로드 → 최대 폭 640px 리사이즈 → JPEG q80으로 로컬 디스크에 저장. 경로는 `data/thumbs/{hash[:2]}/{url_hash}.jpg`.
- **단일 사이즈.** v1의 3종 리사이즈(small/medium/large)는 폐기 — 클라이언트가 iOS 목록 셀 하나뿐이므로 사이즈 변형이 필요 없다.
- 서빙은 `GET /thumbs/{path}` — **인증 면제** (Tailscale이 네트워크 경계이고 iOS AsyncImage가 커스텀 헤더를 지원하지 않으므로). MinIO·Pre-signed URL 없이 파일 서버로 끝난다.
- `thumb` 잡은 best-effort — 실패해도 링크 상태에 영향 없다 (`thumb_path`만 NULL 유지).

## 7. NLU 태깅 파이프라인

v2의 기술적 차별점이다. 원칙: **자유 태그 "생성"이 아니라, 통제된 태그 사전(30~50개, 사용자 수정 가능)에 대한 "분류"로 문제를 좁힌다.** LLM 없이 품질을 확보하는 유일한 길이다.

경계: 런타임 추론은 `backend/internal/tagger`(Go), 모델 변환·자산은 `nlu/` (Python은 `nlu/models/`에서만). backend는 nlu/의 산출물(사전 시드, .onnx 파일)을 읽기만 한다.

### Phase A — 규칙 + 통계 (순수 Go, M3)

1. 스크랩 결과에서 title / meta keywords / og:tags / 본문 추출 (goquery)
2. 도메인 휴리스틱: github.com → `dev`, youtube.com → `video` 등 도메인-태그 맵
3. **어절 정규화 (normalize)**: 형태소 분석기 없이, 대표 조사 접미 목록(을/를/이/가/은/는/의/에/에서/으로/와/과/도/만/보다/부터/까지/처럼/에게/한테/께서 등 20~30개)을 어절 끝에서 벗겨내는 normalize 함수를 둔다. 이 normalize는 `corpus_df` 누적과 태그 사전 매칭 **양쪽에 동일 적용**한다 — 한쪽만 정규화하면 DF 통계와 매칭 결과가 어긋난다.
4. **키워드 추출**: 구분자를 공백+조사 접미로 확장한 후보구 추출 + TF-IDF 스코어링 (`corpus_df` 테이블에 자체 코퍼스의 문서 빈도 누적)
5. **태그 사전 매칭 규칙** (name/aliases 대상):
   - 한글 항목: 정규화 후 **전방일치**
   - 라틴 항목 중 3자 미만(ai, ml, ui 등): **단어 경계(`\b`) 매칭 필수** — 부분 문자열 매칭 금지
6. 점수 병합 → 상위 k(≤5), threshold 컷 → 결과는 `link_tags(source='rules', confidence)`로 저장

M3 단위 테스트 예:
- "쿠버네티스를 처음 배우는 사람" → `kubernetes` 매칭 (조사 "를" 스트리핑 후 전방일치)
- "he said hello" → `ai` 미매칭 ("said"의 부분 문자열은 단어 경계 규칙에 걸린다)

### Phase B — 임베딩 분류 (ONNX, M5)

1. **모델 베이크오프 (M5 Week 1)** — 후보 3종을 golden set으로 실측 비교해 선정한다:
   - (a) multilingual-e5-small-ko 계열 — ONNX 기제공, 384-dim, **1순위 검토**. `query:` / `passage:` 프리픽스 규약을 지켜야 품질이 나온다
   - (b) jhgan/ko-sroberta-multitask int8 (110M)
   - (c) BM-K/KoSimCSE int8 (110M, ONNX 직접 변환 필요)
   "소형"·"경량" 같은 수식어는 베이크오프의 실측 수치(모델 크기·추론 시간·Recall)로 대체한다.
2. **토크나이저**: [yalue/onnxruntime_go](https://github.com/yalue/onnxruntime_go)는 텐서 입출력만 제공하므로 토크나이저는 Go 구현(sugarme/tokenizer 또는 knights-analytics/hugot)을 별도 선정한다. **Python HF 토크나이저와 토큰 ID 시퀀스 100% 일치 골든 테스트**(한글 NFC 정규화 포함) 통과가 M5 Week 2 진입 게이트다.
3. 문서 임베딩(title+description) vs 태그 사전 임베딩(`tag_embeddings` 캐시) 코사인 유사도 → 상위 k, threshold 컷
4. Phase A와 점수 앙상블. `tag_feedback` 데이터(사용자의 태그 추가/제거 이력)로 재랭킹 가중치 보정

**배포 형태 (3택, M5에서 결정)** — M1~M4는 CGO-free 단일 정적 바이너리이고, M5에서 ONNX 채택 시 다음 중 하나로 간다:

1. `libonnxruntime.dylib`을 바이너리에 embed하고 시작 시 `data/`로 추출 (cgo 빌드 감수)
2. hugot 순수 Go 백엔드 — 추론이 약 8배 느리지만 태깅은 비동기 3s 예산 내라 허용 가능
3. Phase A 유지 (게이트 미달 시)

### 품질 게이트

측정 없는 "잘 되는 것 같다"는 금지. 평가 프로토콜은 다음과 같다:

- **golden set**: 실제 저장 링크(M2 임포트 + 실사용 축적분에서 층화 샘플링) 100개로 구축한 `nlu/golden/` JSONL. 레코드 스키마:

  ```json
  {"url": "...", "snapshot": {"title": "...", "description": "...", "meta_keywords": "...", "body_text": "..."}, "expected_tags": ["..."]}
  ```

  eval은 **네트워크 접근 0** — snapshot만 입력으로 쓴다. 원본 페이지가 바뀌거나 사라져도 평가가 재현된다.
- **지표**: 링크당 hit = (예측 top-3 ∩ expected_tags) ≥ 1, **top-3 Recall = hit 수 / 전체**. `just eval`은 태그별 precision/recall과 태그별 부착 빈도도 표로 출력한다.
- **분할**: dev 50 / test 50. 규칙 튜닝은 dev만 보고, 게이트 판정은 동결된 test로만 한다.
- **베이스라인 상대 게이트**: "도메인 휴리스틱만" 구성을 항상 함께 측정한다. M5 진입 = Phase A가 베이스라인 대비 **+15pp 이상** (절대 60%는 참고치). M5 종료 = 앙상블이 Phase A 대비 **+10pp 이상** (절대 80%는 참고치).

스키마·샘플링 상세는 [nlu/golden/README.md](../../nlu/golden/README.md) 참고. 태그 사전 정의는 `nlu/dictionary/`에 커밋한다.

## 8. iOS 클라이언트

- **전제**: Apple Developer Program($99/년) — 무료 계정은 프로비저닝 7일 만료라 매일 쓰는 앱과 양립 불가.
- **SwiftUI.** React Native는 v2에서 제외 — 저장 경로의 핵심인 Share Extension을 네이티브 수준 품질로 만드는 것이 우선이다.
- **Share Extension 캡처 정책**: App Group 로컬 큐에 **우선 기록** → `timeoutInterval` 2~3s로 `POST /api/v1/links` → 성공 시 큐에서 제거, 실패/타임아웃 시 큐에 남긴 채 시트를 닫는다. 큐 드레인은 본앱 실행 시 + BGTaskScheduler가 담당한다. "요청 발사 후 즉시 닫기"는 금지 — extension 프로세스 종료로 in-flight 요청이 유실된다. `POST /api/v1/links`는 url_hash 멱등(중복 시 200 duplicate:true)이므로 재시도해도 중복 생성이 없다.
- **API 키 보관**: Keychain에 앱 그룹 공유로 저장 — 앱과 Share Extension이 같은 키를 읽는다.
- 앱 화면: 목록(커서 페이지네이션 무한 스크롤), 태그 필터, 검색, 상세(태그 수정 = PATCH). API는 [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) 참고.

## 9. 개발 도구

- **린트**: golangci-lint (`just lint`)
- **테스트**: 표준 `testing` + `httptest`. testcontainers 불필요 — SQLite는 인메모리/임시파일로 테스트되므로 Docker 없이 `go test ./...`가 끝까지 돈다. v1에서 통합 테스트마다 PostgreSQL/Redis 컨테이너를 띄우던 비용이 통째로 사라진다.
- **벤치마크**: 두 층으로 나뉜다. `just bench-http`는 실제 HTTP 경로로 저장 p99를 측정하는 성능 게이트(p99 < 50ms 초과 시 exit 1), `just bench`(`go test -bench=. -benchmem ./...`)는 검색·목록 등 마이크로벤치다. go test 벤치는 평균만 내므로 p99 판정 수단이 아니다 — p99 판정은 bench-http가 담당한다.
- **프로파일링**: `net/http/pprof`를 `/debug/pprof`에 기본 탑재. 성능 목표는 추측이 아니라 프로파일로 검증한다.
- **태깅 평가**: `just eval` — golden set 정확도 측정 (7장).

### API 계약과 코드 생성

API는 contract-first다. `api/openapi.yaml`(OpenAPI 3.1)이 API의 기계 원본이고, 서버·클라이언트 코드를 전부 여기서 생성한다. [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md)는 사람용 해설·예시 문서이며 둘이 다르면 openapi.yaml이 우선한다.

- **backend (M1+)**: [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) **v2.8.0 핀** — chi 서버 인터페이스 + 요청/응답 타입을 `backend/internal/api/gen/`에 생성한다. generate 세트는 `types,chi-server,strict-server,spec`. 생성물은 커밋 대상이고 `just gen`으로 재생성한다. v2.8.0은 OpenAPI 3.1 생성 실측 통과 버전이다 (2026-07-20).
- **ios (M4)**: swift-openapi-generator — Apple 공식, URLSession 트랜스포트. API 타입 수작성 금지.
- **frontend**: [openapi-typescript](https://openapi-ts.dev) 핀 버전 — `frontend/src/lib/api/schema.d.ts`를 생성한다(`just web-gen`, 생성물 커밋 대상). 드리프트 가드는 `just web-gen-check`이며 CI web job이 호출한다.
- **드리프트 방지**: `just gen-check` — `just gen` 재실행 후 git diff가 남으면 실패한다. 스펙과 생성물이 어긋난 커밋을 차단하며, 검증 매트릭스의 M1 행이다 ([08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md) 4장).

## 10. 미래 교체 경로: 인터페이스 설계

축소는 했지만 문을 닫지는 않았다. 세 핵심 의존성이 인터페이스 뒤에 있으므로, 유저가 생기면 호출부 수정 없이 구현체만 교체한다.

| 인터페이스 | v2 구현 | 미래 교체 후보 | 트리거 |
|---|---|---|---|
| `Store` (`backend/internal/store/`) | SQLite (modernc.org/sqlite) | PostgreSQL | 멀티유저 도입, 동시 쓰기 증가 |
| `Queue` (`backend/internal/queue/`) | SQLite jobs 테이블 + goroutine 풀 | Redis 기반 분산 큐 | 워커의 다중 프로세스/노드 분리 |
| `Tagger` (`backend/internal/tagger/`) | rules → onnx 2단계 | 외부 모델 서빙 / 더 큰 임베딩 모델 | 품질 게이트로 정당화될 때 |

v1의 k8s 매니페스트는 `deploy/k8s-future/`에 보존한다. 그 시점의 배포 이야기는 [07-DEPLOYMENT.md](07-DEPLOYMENT.md)에 있다.
