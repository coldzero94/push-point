# Push-Point v2.1 — 모노레포 태스크 러너 (just, https://just.systems)
# 2026-07-20 도구 평가(vs make/go-task/mise/moon) 후 채택. Go 레시피는 backend/ 에서 실행된다.
# 미구현 가드("M1에서 활성화")는 해당 마일스톤에서 코드가 생기면 걷어낸다.

# 레시피 목록 출력 (기본)
default:
    @just --list --unsorted

# 로컬 실행 — API 서버 + 워커 단일 프로세스 (PUSHPOINT_API_KEY=dev-key)
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -d backend/cmd/pushpoint ]; then echo "backend/cmd/pushpoint가 없습니다."; exit 1; fi
    base="${PUSHPOINT_PORT:-8420}"
    for i in $(seq 0 20); do
      p=$((base + i))
      if ! lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
        [ "$i" -eq 0 ] || echo "포트 $base 사용 중 → $p 로 실행합니다 (웹은 just web-dev가 자동 감지)"
        echo "http://127.0.0.1:$p"
        cd backend
        exec env PUSHPOINT_ADDR="127.0.0.1:$p" PUSHPOINT_API_KEY="${PUSHPOINT_API_KEY:-dev-key}" go run ./cmd/pushpoint
      fi
    done
    echo "빈 포트를 찾지 못했습니다 ($base~$((base + 20))). PUSHPOINT_PORT로 다른 대역을 지정하세요."; exit 1

# backend/bin/pushpoint 단일 바이너리 빌드
build:
    @if [ -d backend/cmd/pushpoint ]; then cd backend && go build -o bin/pushpoint ./cmd/pushpoint; else echo "backend/cmd/pushpoint가 아직 없습니다. M1에서 활성화됩니다."; fi

# api/openapi.yaml → backend/internal/api/gen/ 코드 생성 (생성물은 커밋 대상)
gen:
    @if [ -f api/openapi.yaml ] && command -v oapi-codegen >/dev/null 2>&1; then mkdir -p backend/internal/api/gen && oapi-codegen -generate types,chi-server,strict-server,spec -package gen -o backend/internal/api/gen/api.gen.go api/openapi.yaml; else echo "oapi-codegen이 없습니다. 설치: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 (버전 핀 — 3.1 지원 실측 통과 버전)"; fi

# 드리프트 방지 — gen 후 git diff가 남으면 실패 (CI·검증 매트릭스, M1+)
gen-check:
    @if [ -f api/openapi.yaml ] && command -v oapi-codegen >/dev/null 2>&1; then just gen && git diff --exit-code backend/internal/api/gen; else echo "api/openapi.yaml 또는 oapi-codegen이 아직 없습니다. just gen-check는 M1에서 활성화됩니다."; fi

# openapi.yaml enum ↔ migrations CHECK 제약 값 일치 검사 (불일치 시 exit 1)
enum-lint:
    scripts/lint_enums.sh

# 전체 테스트
test:
    @if [ -d backend/cmd/pushpoint ]; then cd backend && go test ./...; else echo "backend/cmd/pushpoint가 아직 없습니다. M1에서 활성화됩니다."; fi

# 마이크로벤치 (p99 판정은 bench-http가 담당)
bench:
    @if [ -d backend/cmd/pushpoint ]; then cd backend && go test -bench=. -benchmem ./...; else echo "backend/cmd/pushpoint가 아직 없습니다. M1에서 활성화됩니다."; fi

# 저장 API HTTP 경로 p99 게이트 — p99 < 50ms 초과 시 exit 1 (M1+)
bench-http:
    @if [ -f scripts/bench_http.sh ]; then scripts/bench_http.sh; else echo "scripts/bench_http.sh가 아직 없습니다. M1에서 활성화됩니다."; fi

# 크래시 복구 검증 — 빌드 → fixture 서버 → 저장 → kill -9 → 재기동 → 전량 done 단언 (M2+)
test-crash:
    @if [ -f scripts/test_crash.sh ]; then scripts/test_crash.sh; else echo "scripts/test_crash.sh가 아직 없습니다. M2에서 활성화됩니다."; fi

# 벤치용 한영 혼합 시드 DB 생성 (고정 난수, 예: just seed 100000)
seed n='10000':
    @if [ -d backend/cmd/pushpoint ]; then cd backend && go run ./cmd/pushpoint seed -n {{n}}; else echo "backend/cmd/pushpoint가 아직 없습니다. M1에서 활성화됩니다."; fi

# golden set 태깅 정확도 측정 — top-3 Recall, 베이스라인 병기 (M3+)
eval:
    @if [ -d nlu/golden ] && [ -d backend/cmd/pushpoint ]; then cd backend && go run ./cmd/pushpoint eval ../nlu/golden/; else echo "nlu/golden/ 또는 backend/cmd/pushpoint가 아직 없습니다. just eval은 M3에서 활성화됩니다."; fi

# --- web (frontend) — api/openapi.yaml의 3번째 소비자, backend gen과 대칭 ---
# 가드는 frontend/package.json(스캐폴드 유무). 의존성 미설치는 안내 후 `just web-install`.

# 웹 의존성 설치 — npm ci(package-lock.json 고정 버전 재현 설치, CI와 동일 경로)
web-install:
    @if [ -f frontend/package.json ]; then cd frontend && npm ci; else echo "frontend/package.json이 없습니다 (frontend 스캐폴드 전)."; fi

# Vite dev 서버 :8421 (프록시로 /api·/thumbs·/healthz → Go :8420, 실행 중인 포트 자동 감지)
web-dev:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f frontend/package.json ]; then echo "frontend/package.json이 없습니다 (frontend 스캐폴드 전)."; exit 1; fi
    if [ ! -d frontend/node_modules ]; then echo "frontend/node_modules가 없습니다. 먼저: just web-install"; exit 1; fi
    api="${PUSHPOINT_API_PORT:-}"
    if [ -z "$api" ]; then
      for i in $(seq 0 20); do
        p=$((8420 + i))
        if curl -sf -m 0.3 "http://127.0.0.1:$p/healthz" >/dev/null 2>&1; then api="$p"; break; fi
      done
    fi
    if [ -z "$api" ]; then
      api=8420
      echo "실행 중인 백엔드를 못 찾았습니다 — 프록시를 :$api 로 둡니다 (먼저 just dev 를 띄우세요)"
    else
      echo "백엔드 감지: :$api → 프록시 연결"
    fi
    cd frontend
    exec env PUSHPOINT_API_PORT="$api" npm run dev

# 게이트 레시피(CI가 호출)이므로 전제가 없으면 조용히 통과하지 않고 exit 1 한다.
# api/openapi.yaml → frontend/src/lib/api/schema.d.ts 계약 타입 생성 (핀 버전, @latest 금지). 생성물 커밋 대상
web-gen: _web-required
    cd frontend && npm run gen

# 드리프트 방지 — web-gen 후 git diff가 남으면 실패 (CI·완료 정의)
web-gen-check: _web-required
    just web-gen && git diff --exit-code frontend/src/lib/api/schema.d.ts

# 게이트 레시피 공용 전제 검사 — frontend 스캐폴드·의존성이 없으면 실패시킨다.
# (web-dev 같은 개발 편의 레시피는 안내만 하고 넘어가도 되지만, CI가 도는 경로는
#  전제 미충족을 "아무 일 없이 통과"로 보이게 두면 게이트가 아니게 된다.)
_web-required:
    @if [ ! -f frontend/package.json ]; then echo "frontend/package.json이 없습니다 (frontend 스캐폴드 전) — 게이트 레시피는 이 상태로 통과시키지 않습니다."; exit 1; fi
    @if [ ! -d frontend/node_modules ]; then echo "frontend/node_modules가 없습니다. 먼저: just web-install"; exit 1; fi

# go:embed는 패키지 디렉터리 하위만 가능(상대경로 embed 불가) → frontend/dist를
# backend/internal/web/dist로 복사해야 `go build -tags embed_frontend`가 그것을 내장한다.
# frontend/dist·backend/internal/web/dist 둘 다 미커밋(gitignore, CI가 빌드).
# 프로덕션 번들 → frontend/dist/ 후 backend embed 위치(backend/internal/web/dist)로 복사
web-build: _web-required
    ( cd frontend && npm run build ) && \
        rm -rf backend/internal/web/dist && \
        mkdir -p backend/internal/web/dist && \
        cp -R frontend/dist/. backend/internal/web/dist/ && \
        echo "web-build: frontend/dist → backend/internal/web/dist 복사 완료 (릴리스: just release)"

# embed_frontend 태그는 web-build가 만든 backend/internal/web/dist를 요구한다(순서 고정).
# 릴리스 빌드 — 웹 번들을 내장한 단일 바이너리 backend/bin/pushpoint
release: web-build
    cd backend && go build -tags embed_frontend -o bin/pushpoint ./cmd/pushpoint
    @echo "release: backend/bin/pushpoint (웹 embed 포함) 빌드 완료"

# 웹 정적 분석 (oxlint)
web-lint: _web-required
    cd frontend && npm run lint

# spa.go는 embed_frontend 태그에서만 컴파일돼 태그 없는 just test가 커버하지 못하고,
# 임베드에 dist가 필요하므로 web-build 뒤에 돈다.
# 웹 embed 경로 테스트 — SPA 셸·자산 헤더·계약 표면 JSON 404
web-test: web-build
    cd backend && go test -count=1 -tags embed_frontend ./internal/web/... ./internal/api/...

# 정적 분석 (golangci-lint — govet 포함)
lint:
    @if command -v golangci-lint >/dev/null 2>&1; then cd backend && golangci-lint run; else echo "golangci-lint가 설치되어 있지 않습니다. 설치: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest (M1에서 backend/go.mod tool 지시자로 버전 핀 예정)"; fi

# 포맷팅 — gofmt + goimports (backend 대상)
fmt:
    gofmt -l -w backend
    @if command -v goimports >/dev/null 2>&1; then goimports -l -w backend; else echo "goimports가 설치되어 있지 않습니다. 설치: go install golang.org/x/tools/cmd/goimports@latest"; fi
