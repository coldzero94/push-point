# backend — Push-Point Go 단일 바이너리

> Push-Point v2 — 마지막 업데이트: 2026-07-20

API 서버 + 워커 + NLU 런타임 추론이 한 프로세스로 동작하는 Go 워크스페이스다.
바이너리 이름은 `pushpoint`, 진입점은 `cmd/pushpoint/main.go`.

## 현재 상태

`go.mod`만 존재한다 (의존성 0, go.sum 없음). 코드는 M1에서 신규 작성한다 —
v1 백엔드 코드는 존재한 적 없고 go.mod 선언만 있었다.

## 목표 내부 구조 (스펙 §1의 backend 하위 트리)

```
├── backend/                   # Go 단일 바이너리 (API + worker + NLU 런타임 추론)
│   ├── cmd/pushpoint/main.go  # 단일 진입점
│   ├── internal/
│   │   ├── api/               # HTTP 핸들러 (chi)
│   │   ├── store/             # Store 인터페이스 + sqlite 구현
│   │   ├── queue/             # Queue 인터페이스 + sqlite jobs 구현
│   │   ├── scraper/           # fetch + goquery 파싱, singleflight
│   │   ├── tagger/            # Tagger 인터페이스 + rules / onnx 구현
│   │   └── thumbs/            # 썸네일 생성·저장
│   ├── migrations/            # SQLite 마이그레이션 (golang-migrate, embed)
│   └── go.mod                 # module github.com/coby/push-point/backend
```

## 실행

리포 루트에서:

```sh
just dev    # cd backend && PUSHPOINT_API_KEY=dev-key go run ./cmd/pushpoint
```

`just build` / `just test` / `just bench` / `just lint`도 루트 justfile이 backend를 대상으로 실행한다.

## 설계 원칙 요약

- HTTP: 표준 `net/http` + chi 라우터, `/debug/pprof` 기본 탑재. zap/viper/gin/ent는 쓰지 않는다.
- 로깅: 표준 `log/slog` (JSON 핸들러).
- DB: modernc.org/sqlite (CGO-free, 순수 Go) — 단일 정적 바이너리 유지. WAL 모드 + FTS5.
- SQLite PRAGMA (스펙 §2와 동일 수치): `journal_mode=WAL`, `synchronous=NORMAL`, `busy_timeout=5000`, `foreign_keys=ON`, `cache_size=-64000` (64MB).
- 커넥션 전략: writer 1개 + reader 풀(N=4). 모든 쓰기는 트랜잭션.
- NLU 경계: 런타임 추론(규칙 태거, ONNX)은 `internal/tagger`의 Go 코드다. `../nlu/`는 자산(사전 시드, golden set, .onnx)만 담고, backend는 그 산출물을 읽기만 한다.

## 더 보기

- 아키텍처: [../docs/v2/03-SYSTEM-ARCHITECTURE.md](../docs/v2/03-SYSTEM-ARCHITECTURE.md)
- 스키마: [../docs/v2/05-DATA-SCHEMA.md](../docs/v2/05-DATA-SCHEMA.md)
