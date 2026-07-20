---
paths:
  - "backend/**"
---

# Backend 규칙 (Go 단일 바이너리)

## 스택 (고정)

- `net/http` + chi 라우터, `log/slog`(JSON), 설정은 `os.Getenv`(접두어 `PUSHPOINT_`). **gin/ent/viper/zap 금지.**
- DB: `modernc.org/sqlite` (CGO-free). 마이그레이션은 golang-migrate + `embed.FS`, 시작 시 자동 적용.
- 이미 커밋·적용된 마이그레이션 파일은 불변 — 수정·삭제·번호 재배열 금지, 스키마 변경은 항상 **새 마이그레이션 파일 추가**로 한다 (기존 파일 변경 시 golang-migrate dirty version으로 기동 실패).
- 동시성 유틸은 `golang.org/x/sync` (semaphore/singleflight/errgroup).
- 테스트: 표준 `testing` + `httptest`. testcontainers 금지 (SQLite는 임시 파일/인메모리로 충분).

## 인터페이스 계약 (내부 경계)

- `Store`/`Queue` 등 인터페이스 정의 파일(store.go, queue.go)이 내부 계약이다. **인터페이스를 바꾸면 같은 변경 세트에서 모든 구현·사용처를 함께 수정한다** — 인터페이스만 바꾸고 끝내는 커밋 금지. `cd backend && go build ./...` 0 에러가 그 변경의 완료 조건이다.
- 각 구현 파일 상단에 컴파일 타임 단언을 둔다: `var _ Store = (*sqliteStore)(nil)` — 사용처가 없어도 정의 지점에서 불일치가 즉시 빌드 에러로 잡힌다.
- 병렬 작업 시 인터페이스 정의 파일은 **한 작업자만 소유**한다 (docs 규칙의 원본-파생 쌍과 같은 원리 — 계약과 구현을 쪼개 배정하면 불일치가 생긴다).

## SQLite 불변식

- PRAGMA: `journal_mode=WAL; synchronous=NORMAL; busy_timeout=5000; foreign_keys=ON; cache_size=-64000`.
- 커넥션: **writer 1개 + reader 풀**. 모든 쓰기는 트랜잭션.
- 저장 API는 `INSERT links + INSERT jobs`를 한 트랜잭션으로 커밋 후 즉시 201 — 동기 경로에 스크랩·태깅 넣지 말 것.
- FTS5(links_fts, trigram)는 링크/태그 쓰기와 **같은 트랜잭션**에서 DELETE 후 INSERT로 동기화.
- 잡 claim은 `UPDATE ... WHERE id = (SELECT ... LIMIT 1) RETURNING` 원자 패턴. 시작 시 `running → pending` 복구.
- 목록·검색은 keyset 커서 페이지네이션. **OFFSET 금지.**

## 성능 게이트 (p99 판정은 `just bench-http`, 마이크로벤치는 `just bench`)

| 지표 | 목표 |
|---|---|
| 저장 API p99 | < 50ms |
| 저장 → 태그 완료 (비동기) | < 3s |
| 검색 (FTS5, 1만 링크) | < 30ms |
| 10만 건 목록 스크롤 API | < 50ms |
| 콜드 스타트 → 서빙 | < 1s |

## 원본과 생성물

API를 바꿀 땐 `api/openapi.yaml`(기계 원본)을 먼저 고치고 `just gen`으로 `backend/internal/api/gen/`을 재생성한다 — `docs/v2/06-API-SPECIFICATION.md`는 해설로 동기화. 스키마는 `docs/v2/05-DATA-SCHEMA.md`가 원본 — 먼저 갱신하고 코드를 맞춘다.

- `backend/internal/api/gen/`은 생성물 — **직접 편집 금지**. 컴파일 에러·타입 불일치도 `api/openapi.yaml`을 고친 뒤 `just gen` 재생성으로만 해결한다.
