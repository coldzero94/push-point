# Push-Point

> Push-Point v2 — 마지막 업데이트: 2026-07-20

저장한 링크에 자동으로 태그가 붙는 개인 링크 아카이브. 단일 Go 바이너리로 동작한다.

v2의 방향은 단순하다. 인프라가 아니라 제품부터 만든다 — 유저가 0명인 서비스에 오토스케일링을 붙이는 대신, `just dev` 한 번으로 뜨는 단일 프로세스의 설계 품질로 성능을 확보한다. 자동 태깅은 외부 LLM API 없이 규칙 기반 + ONNX 임베딩의 경량 NLU 파이프라인으로 해결하며, 이것이 이 프로젝트의 기술적 차별점이다.

## 빠른 시작

### 요구사항

- Go 1.25+
- just (`brew install just`)

### 실행

```bash
just dev
# cd backend && PUSHPOINT_API_KEY=dev-key go run ./cmd/pushpoint
# 콜드 스타트 < 1s, http://localhost:8080 에서 서빙
```

### 링크 저장

```bash
curl -X POST http://localhost:8080/api/v1/links \
  -H "Authorization: Bearer dev-key" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article", "note": "나중에 읽기"}'
# 201 {"id": 1, "status": "pending", "created_at": 1784937600}
# 스크랩·태깅은 비동기로 진행되어 3초 이내에 제목·태그·썸네일이 채워진다
```

### 목록 조회

```bash
curl "http://localhost:8080/api/v1/links?limit=20" \
  -H "Authorization: Bearer dev-key"
# keyset 커서 페이지네이션 — 응답의 next_cursor를 ?cursor=로 전달
```

### 검색

```bash
curl "http://localhost:8080/api/v1/search?q=쿠버네티스" \
  -H "Authorization: Bearer dev-key"
# FTS5 trigram 전문 검색 (검색어 3자 이상), bm25 랭킹
```

## justfile 레시피

태스크 러너는 [just](https://just.systems)다 (2026-07-20 도구 평가 후 채택). 모든 Go 레시피는 루트 justfile이 `backend/` 디렉터리에서 실행한다 (`cd backend && ...`).

| 레시피 | 설명 |
|---|---|
| `just` | 레시피 목록 출력 (default) |
| `just dev` | `PUSHPOINT_API_KEY=dev-key go run ./cmd/pushpoint` — 로컬 개발 서버 |
| `just build` | `go build -o bin/pushpoint ./cmd/pushpoint` |
| `just gen` | `api/openapi.yaml` → `backend/internal/api/gen/` 코드 생성 (oapi-codegen v2.8.0 핀, 생성물은 커밋 대상) |
| `just gen-check` | 드리프트 방지 — gen 후 git diff가 남으면 실패 (CI·검증 매트릭스, M1+) |
| `just test` | `go test ./...` |
| `just bench` | 마이크로벤치: `go test -bench=. -benchmem ./...` (p99 판정은 bench-http가 담당) |
| `just bench-http` | 저장 API HTTP 경로 p99 게이트 — p99 < 50ms 초과 시 exit 1 (M1+) |
| `just test-crash` | 크래시 복구 검증 — 저장 → kill -9 → 재기동 → 전량 done 단언 (M2+) |
| `just seed 100000` | 벤치용 한영 혼합 시드 DB 생성 (고정 난수, 인자 생략 시 n=10000) |
| `just eval` | golden set 태깅 정확도 측정 — top-3 Recall, 베이스라인 병기 (M3+) |
| `just lint` | `golangci-lint run` |
| `just fmt` | `gofmt` / `goimports` |

## 아키텍처

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

핵심 흐름:

1. 저장 API는 `INSERT links` + `INSERT jobs(scrape)`를 한 트랜잭션으로 커밋하고 즉시 201을 반환한다 (p99 < 50ms).
2. dispatcher goroutine이 jobs 테이블에서 잡을 원자적으로 claim해 워커에 배분한다 — 재시작해도 잡이 유실되지 않는다.
3. scraper pool이 메타데이터를 파싱하고, 성공 트랜잭션에서 tag 잡(og:image가 있으면 thumb 잡도)을 연쇄 enqueue한다.
4. tagger가 통제된 태그 사전에 대해 분류를 수행해 태그를 붙이면 링크 상태가 `done`이 된다 — 저장부터 태그 완료까지 3초 이내.

## 프로젝트 구조

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
│   │   ├── store/             # Store 인터페이스 + sqlite 구현
│   │   ├── queue/             # Queue 인터페이스 + sqlite jobs 구현
│   │   ├── scraper/           # fetch + goquery 파싱, singleflight
│   │   ├── tagger/            # Tagger 인터페이스 + rules / onnx 구현
│   │   └── thumbs/            # 썸네일 생성·저장
│   ├── migrations/            # SQLite 마이그레이션 (golang-migrate, embed)
│   └── go.mod                 # module github.com/coby/push-point/backend
├── nlu/                       # NLU 오프라인 자산 (런타임 코드 아님)
│   ├── dictionary/            # 태그 사전 정의·시드 (커밋 대상)
│   ├── golden/                # 태깅 품질 golden set (JSONL, 커밋 대상)
│   └── models/                # M5: ONNX 변환 스크립트(Python)·모델 아티팩트
├── ios/                       # M4: SwiftUI 앱 + Share Extension
├── frontend/                  # 웹 프론트 — 명시적 비목표(M6 이후 검토), 자리만 예약
├── docs/
│   ├── README.md              # v1 ↔ v2 문서 인덱스·비교
│   ├── v1/                    # v1 기획서 아카이브 (수정 금지)
│   └── v2/                    # 현행 문서 (단일 진실 원천)
├── deploy/k8s-future/         # v1 k8s 매니페스트 보존 (미사용)
├── CLAUDE.md
└── justfile                   # 태스크 러너 레시피 13개 — dev / test / bench / eval 등 (backend 대상)
```

### 워크스페이스

- `api/` — API 계약의 기계 원본 `openapi.yaml` (OpenAPI 3.1) — 백엔드·클라이언트 코드 생성원
- `backend/` — Go 단일 바이너리 (API + worker + NLU 런타임 추론)
- `nlu/` — NLU 오프라인 자산: 태그 사전 정의, golden set, 모델 변환 스크립트 (런타임 코드 아님, backend는 산출물만 읽는다)
- `ios/` — M4: SwiftUI 앱 + Share Extension
- `frontend/` — 웹 프론트엔드 자리 예약 (명시적 비목표, M6 이후 검토)

## 성능 목표

로컬 M-시리즈 기준. p99 판정은 `just bench-http`, 마이크로벤치는 `just bench`로 검증한다 (마일스톤별 검증 매트릭스는 [docs/v2/08-DEVELOPMENT-PLAN.md](docs/v2/08-DEVELOPMENT-PLAN.md)).

| 지표 | 목표 |
|---|---|
| 저장 API p99 | < 50ms |
| 저장 → 태그 완료 (비동기) | < 3s |
| 검색 (FTS5, 1만 링크) | < 30ms |
| 링크 10만 건에서 목록 스크롤 API | < 50ms |
| 콜드 스타트 (바이너리 실행 → 서빙) | < 1s |

## 문서

현행 문서는 `docs/v2/`가 단일 진실 원천이다. v1 기획서는 docs/v1/ 아카이브, 비교는 [docs/README.md](docs/README.md) 참고.

- [00-README.md](docs/v2/00-README.md) — 문서 목차와 읽는 순서
- [01-PROJECT-OVERVIEW.md](docs/v2/01-PROJECT-OVERVIEW.md) — 프로젝트 목표, v1→v2 전환 배경, 비목표
- [02-TECH-SPEC.md](docs/v2/02-TECH-SPEC.md) — 기술 스택 선정과 근거 (SQLite, chi, NLU 파이프라인)
- [03-SYSTEM-ARCHITECTURE.md](docs/v2/03-SYSTEM-ARCHITECTURE.md) — 단일 프로세스 아키텍처와 컴포넌트 경계
- [04-DATA-FLOW.md](docs/v2/04-DATA-FLOW.md) — 저장부터 태깅 완료까지의 잡 큐 동작과 복구
- [05-DATA-SCHEMA.md](docs/v2/05-DATA-SCHEMA.md) — SQLite 스키마, FTS5, PRAGMA 설정
- [06-API-SPECIFICATION.md](docs/v2/06-API-SPECIFICATION.md) — REST API 명세 (인증, 커서 페이지네이션, 에러 형식)
- [07-DEPLOYMENT.md](docs/v2/07-DEPLOYMENT.md) — 단일 바이너리 배포·백업 전략과 deploy/k8s-future/
- [08-DEVELOPMENT-PLAN.md](docs/v2/08-DEVELOPMENT-PLAN.md) — M1~M6 마일스톤과 완료 기준

## v1 인프라에 대하여

v1은 Minikube 위에 PostgreSQL, Redis Streams, MinIO를 올리고 HPA로 스케일링하는 구조였다. 유저 0명 단계에서 이 구성은 로컬 테스트 마찰만 키우는 역설계라 판단해, v2는 SQLite(WAL) + 인프로세스 워커 풀 + 로컬 디스크의 단일 바이너리로 전환했다. 다만 k8s 매니페스트는 삭제하지 않고 `deploy/k8s-future/`에 보존한다 — 지금 접는 것이지 버리는 것이 아니다. Store/Queue/Tagger가 인터페이스 뒤에 있으므로, 실제 유저가 생겨 분산 구성이 필요해지는 시점에 구현체만 교체해 부활시킨다.
