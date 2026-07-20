---
paths:
  - "backend/**"
---

# Backend 규칙 (Go 단일 바이너리)

## 스택 (고정)

- `net/http` + chi 라우터, `log/slog`(JSON), 설정은 `os.Getenv`(접두어 `PUSHPOINT_`). **gin/ent/viper/zap 금지.**
- DB: `modernc.org/sqlite` (CGO-free). 마이그레이션은 golang-migrate + `embed.FS`, 시작 시 자동 적용.
- 동시성 유틸은 `golang.org/x/sync` (semaphore/singleflight/errgroup).
- 테스트: 표준 `testing` + `httptest`. testcontainers 금지 (SQLite는 임시 파일/인메모리로 충분).

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

API를 바꿀 땐 `api/openapi.yaml`(기계 원본)을 먼저 고치고 `just gen`으로 `backend/internal/api/gen/`을 재생성한다 — `docs/v2/06-API-SPECIFICATION.md`는 해설로 동기화. 스키마는 `docs/v2/05-DATA-SCHEMA.md`가 원본 — 먼저 갱신하고 코드를 맞춘다.
