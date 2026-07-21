# Push-Point 문서

> Push-Point v2.1 — 마지막 업데이트: 2026-07-21

**Push-Point**는 유튜브 영상이나 웹 아티클을 공유하면 자동으로 태그가 붙고, 태그·검색으로 다시 찾아볼 수 있는 개인용 링크 아카이브다.

v2의 핵심 가치는 네 가지다.

- **빠른 저장** — 저장 API p99 < 50ms. iOS 공유 시트에서 한 탭으로 저장이 끝난다.
- **LLM 없는 자동 태깅** — 외부 API 대신 규칙 기반 → ONNX 임베딩 2단계 경량 NLU 파이프라인. 비용 0, 수백 ms 응답. 이 프로젝트의 기술적 차별점이다.
- **프라이버시** — 저장한 링크와 메모가 외부 서비스로 나가지 않는다. 모든 처리가 로컬에서 끝난다.
- **단일 바이너리** — API 서버 + 워커가 Go 프로세스 하나. `just dev` 한 번으로 전체가 뜬다. 백업은 `data/` 디렉터리 복사.

리포는 api(API 계약 — `openapi.yaml` 기계 원본) / backend(Go 단일 바이너리) / nlu(NLU 오프라인 자산) / ios(SwiftUI) / frontend(웹 SPA — iOS와 대등한 full-feature 클라이언트) 5개 워크스페이스의 모노레포로 구성된다.

## 문서 목차

| 문서 | 내용 |
|---|---|
| [00-README.md](00-README.md) | 이 문서. 프로젝트 소개와 문서 인덱스 |
| [01-PROJECT-OVERVIEW.md](01-PROJECT-OVERVIEW.md) | 프로젝트 목표, 사용자 시나리오, v2로 방향을 바꾼 이유 |
| [02-TECH-SPEC.md](02-TECH-SPEC.md) | 기술 스택 선택과 근거 (Go, SQLite, chi, NLU 파이프라인) |
| [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md) | 단일 프로세스 아키텍처, 컴포넌트 구성, SQLite 설정 |
| [04-DATA-FLOW.md](04-DATA-FLOW.md) | 저장 → 스크랩 → 태깅 플로우, 잡 큐 동작, 재시도·크래시 복구 |
| [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md) | SQLite 스키마 전체 (links, tags, jobs, FTS5), 마이그레이션 |
| [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) | REST API 명세, 인증, 에러 형식, 커서 페이지네이션 |
| [07-DEPLOYMENT.md](07-DEPLOYMENT.md) | 실행·운영 방법, 환경 변수, 백업, `deploy/k8s-future/` 보존 정책 |
| [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md) | 마일스톤 M1~M6 (6개월), 각 단계 완료 기준 |
| [09-PLAN-REVIEW.md](09-PLAN-REVIEW.md) | 계획 점검 결과 (2026-07-20) — 권고 8건 v2.1 반영 완료 |

## 빠른 시작

```bash
git clone https://github.com/your-org/push-point.git
cd push-point
just dev
```

이게 전부다. `just dev`는 `PUSHPOINT_API_KEY=dev-key`로 단일 프로세스(API + worker)를 띄우고, 마이그레이션은 시작 시 자동 적용된다.

링크 저장:

```bash
curl -X POST http://localhost:8080/api/v1/links \
  -H "Authorization: Bearer dev-key" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "note": "나중에 보기"}'
# 201 {"id": 1, "status": "pending", "created_at": ...}
```

목록 조회 (몇 초 뒤 제목·태그·썸네일이 채워져 있다):

```bash
curl -H "Authorization: Bearer dev-key" \
  "http://localhost:8080/api/v1/links?limit=20"
```

## 아키텍처 개요

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

**데이터 흐름 요약**:
1. 저장 API가 `links` INSERT + `scrape` 잡 INSERT를 한 트랜잭션으로 커밋하고 즉시 201 응답
2. dispatcher가 워커 goroutine에 잡을 넘기고, scraper pool이 메타데이터·썸네일을 채움
3. scrape 성공 시 `tag` 잡이 연쇄 enqueue되어 NLU 파이프라인이 태그를 붙임 (저장 → 태그 완료 < 3s)
4. 클라이언트는 목록/검색 API로 결과를 조회 — 상태 전이는 `pending → scraping → tagging → done`

자세한 내용은 [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md), [04-DATA-FLOW.md](04-DATA-FLOW.md) 참고.

## v1 → v2 변경 요약

v2의 원칙은 하나다: **인프라가 아니라 제품부터.** 성능은 분산이 아니라 단일 프로세스 설계의 질로 확보한다.

| 영역 | v1 | v2 | 이유 |
|---|---|---|---|
| 배포 | Minikube + k8s + HPA | 단일 Go 바이너리 (`just dev` 한 번) | 유저 0명에 오토스케일링은 역설계. 로컬 테스트 마찰 제거 |
| DB | PostgreSQL (k8s pod) | SQLite (WAL 모드) + FTS5 | 개인 앱 규모에서 충분히 고성능. 백업 = 파일 복사 |
| 메시지 큐 | Redis Streams | 인프로세스 워커 풀 (goroutine + SQLite jobs 테이블) | 프로세스 하나면 네트워크 큐 불필요. 재시작 내구성은 jobs 테이블이 보장 |
| 오브젝트 스토리지 | MinIO | 로컬 디스크 (`data/thumbs/`) | 썸네일 몇 GB에 S3 API는 과함 |
| AI 태깅 | OpenAI API | 경량 NLU (규칙 기반 → ONNX 임베딩 2단계) | 비용 0, 수백 ms 응답, 프라이버시. 이 프로젝트의 기술적 차별점 |
| 클라이언트 | React Native (미정) | iOS Share Extension 최우선 (SwiftUI) | 저장 마찰이 2초를 넘으면 매일 쓰는 앱이 못 됨 |
| 인증 | JWT + 회원가입 | 단일 사용자, 정적 API 키 1개 | 멀티유저는 명시적 비목표 |

v1 k8s 매니페스트는 삭제하지 않고 `deploy/k8s-future/`로 이동해 보존한다. 지금 접는 것이지 버리는 것이 아니다 — Store/Queue/Tagger가 인터페이스 뒤에 있으므로 유저가 생기면 구현체만 교체한다.

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
├── frontend/                  # 웹 SPA (Vite + React 19 + TS) — iOS와 대등한 full-feature 클라이언트
├── docs/
│   ├── README.md              # v1 ↔ v2 문서 인덱스·비교
│   ├── v1/                    # v1 기획서 아카이브 (수정 금지)
│   └── v2/                    # 현행 문서 (단일 진실 원천)
├── deploy/k8s-future/         # v1 k8s 매니페스트 보존 (미사용)
├── CLAUDE.md
└── justfile                   # dev / test / bench / lint / eval (backend 대상)
```

## 성능 목표

로컬 M-시리즈 기준. 매 마일스톤 검증 커맨드로 확인한다 — p99 판정은 `just bench-http`, 마이크로벤치는 `just bench` (검증 매트릭스는 [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)).

| 지표 | 목표 |
|---|---|
| 저장 API p99 | < 50ms |
| 저장 → 태그 완료 (비동기) | < 3s |
| 검색 (FTS5, 1만 링크) | < 30ms |
| 링크 10만 건에서 목록 스크롤 API | < 50ms |
| 콜드 스타트 (바이너리 실행 → 서빙) | < 1s |

## 명시적 비목표

v2에서 하지 않는 것들이다. 안 하는 것이 결정 사항이다.

- **k8s / HPA / 멀티 노드** — 유저가 생기면 `deploy/k8s-future/` 부활
- **회원가입 / 멀티유저** — 단일 사용자, API 키 하나로 인증
- **OpenAI 등 외부 LLM API 의존** — NLU 파이프라인이 이 프로젝트의 정체성
- **Android** — iOS 실사용 검증 후 판단
