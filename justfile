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
    # 이전 세션의 기록을 먼저 지운다 — 빈 포트를 고르는 동안 web-dev가 옛 값을 읽고
    # 굳어버리면(Vite는 시작할 때 프록시 대상을 한 번만 정한다) 그 포트를 지금 다른
    # worktree가 쓰고 있을 때 남의 백엔드에 붙는다.
    rm -f backend/data/.dev-api-port
    for i in $(seq 0 20); do
      p=$((base + i))
      if ! lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
        [ "$i" -eq 0 ] || echo "포트 $base 사용 중 → $p 로 실행합니다 (웹은 just web-dev가 자동 감지)"
        echo "http://127.0.0.1:$p"
        # 이 worktree가 쓰는 포트를 "<포트> <PID>"로 남긴다 — web-dev가 포트 스캔보다
        # 이 값을 먼저 본다(worktree 병렬 실행 시 남의 백엔드에 붙는 사고 방지).
        # PID를 함께 적는 이유: exec 이후엔 트랩을 못 걸어 종료 시 파일을 지울 수
        # 없다. /healthz만으로는 살아있는 기록과 죽은 기록을 구분할 수 없고(응답한
        # 쪽이 다른 worktree의 백엔드여도 200이다), PID 생존 확인은 구분할 수 있다.
        # exec는 PID를 유지하므로 $$가 곧 서버 프로세스의 PID다.
        # 기록 실패로 서버 기동을 막지는 않는다 — web-dev가 스캔으로 폴백하면 된다.
        if ! { mkdir -p backend/data && printf '%s %s\n' "$p" "$$" > backend/data/.dev-api-port; }; then
          echo "경고: 포트 기록 실패 — just web-dev가 포트 스캔으로 백엔드를 찾습니다"
        fi
        cd backend
        # 핫 리로드: air가 있으면 .go/.sql 변경 시 자동 재빌드·재시작(.air.toml). air는
        # 부모로 세션 내내 살아 있어 위에서 기록한 PID(=air)의 kill -0 판정이 그대로
        # 유효하다(web-dev 감지 무변경). 없으면 go run 폴백(핫 리로드 없음).
        # PUSHPOINT_LOG_FORMAT=text: air가 stderr를 파이프로 감싸 auto만으론 json이 되므로
        # dev에선 컬러 text를 강제한다. LOG_LEVEL=debug로 접근·잡 로그까지 보인다.
        run_env=(env PUSHPOINT_ADDR="127.0.0.1:$p" PUSHPOINT_API_KEY="${PUSHPOINT_API_KEY:-dev-key}"
                 PUSHPOINT_LOG_FORMAT=text PUSHPOINT_LOG_LEVEL="${PUSHPOINT_LOG_LEVEL:-debug}")
        if command -v air >/dev/null 2>&1; then
          exec "${run_env[@]}" air -c .air.toml
        else
          echo "air 미설치 — go run 폴백(핫 리로드 없음). 설치: go install github.com/air-verse/air@latest"
          exec "${run_env[@]}" go run ./cmd/pushpoint
        fi
      fi
    done
    echo "빈 포트를 찾지 못했습니다 ($base~$((base + 20))). PUSHPOINT_PORT로 다른 대역을 지정하세요."; exit 1

# just dev/just web-dev를 그대로 감싸므로 포트 자동 회피·.dev-api-port 격리가 그 안에서
# 원래대로 동작한다(새 로직 없음). mprocs는 dev 전용 — 없으면 안내만 하고, 그냥 터미널
# 2개로 just dev + just web-dev를 따로 띄워도 된다.
# 백엔드+웹을 한 화면(mprocs)에서 병렬 실행 — 패널·색 분리, 웹만 r로 재시작(백엔드는 air 자동 리로드)
dev-all:
    @command -v mprocs >/dev/null 2>&1 || { echo "mprocs 미설치 — 설치: brew install mprocs (또는 터미널 2개로 just dev + just web-dev)"; exit 1; }
    mprocs --names api,web "just dev" "just web-dev"

# go 개발 도구를 핀된 버전으로 설치(재현성) + 웹 의존성. golangci-lint·mprocs는 brew
# 관리라 여기서 다루지 않는다(just doctor가 안내). oapi-codegen은 CI와 같은 v2.8.0.
# air는 go.mod 밖 PATH 도구로 유지한다(핫 리로드 슈퍼바이저 — 바이너리에 링크할 이유 없음).
# 새 클론 온보딩 — 개발 도구·웹 의존성 일괄 설치(핀 버전)
setup:
    go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
    go install github.com/air-verse/air@v1.63.4
    go install gotest.tools/gotestsum@v1.13.0
    go install golang.org/x/tools/cmd/goimports@v0.48.0
    @if [ -f frontend/package.json ]; then just web-install; else echo "frontend 스캐폴드 전 — 웹 의존성 건너뜀"; fi
    @echo "setup 완료. 환경 점검: just doctor"

# 없으면 얻는 법만 안내한다(설치는 하지 않는다 — 그건 just setup·brew).
# 개발 환경 점검 — 필수·권장·선택 도구의 존재와 위치를 확인
doctor:
    #!/usr/bin/env bash
    set -uo pipefail
    ok=0; miss=0
    # 이름 존재확인 힌트 필수?  형태로 점검한다.
    check() { # <표시명> <명령> <힌트> <필수여부(req|opt)>
      if command -v "$2" >/dev/null 2>&1; then
        printf '  ✅ %-14s %s\n' "$1" "$(command -v "$2")"; ok=$((ok+1))
      else
        printf '  ❌ %-14s 없음 — %s\n' "$1" "$3"
        [ "$4" = req ] && miss=$((miss+1)) || true
      fi
    }
    echo "필수:"
    check go go       "https://go.dev/dl (1.25+)" req
    check just just   "brew install just" req
    echo "웹(프런트엔드 작업 시):"
    check node node   "https://nodejs.org (22+)" opt
    echo "Go 개발 도구(just setup가 핀 버전으로 설치):"
    check oapi-codegen oapi-codegen "just setup" opt
    check goimports  goimports  "just setup" opt
    check gotestsum  gotestsum  "just setup (just test-watch용)" opt
    check golangci-lint golangci-lint "brew install golangci-lint" opt
    echo "핫 리로드·병렬 실행(선택):"
    check air air       "go install github.com/air-verse/air@v1.63.4 (just dev 핫 리로드)" opt
    check mprocs mprocs "brew install mprocs (just dev-all)" opt
    echo
    if [ "$miss" -gt 0 ]; then echo "필수 도구 $miss개 누락 — 위 힌트대로 설치하세요."; exit 1; fi
    echo "필수 도구 OK. 선택 도구가 빠졌으면 해당 기능만 폴백됩니다."

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

# nlu/dictionary/ 자산(tags.json·domains.json) ↔ 시드 마이그레이션 일치 검사 (불일치 시 exit 1)
dict-lint:
    scripts/lint_dict.sh

# 전체 테스트
test:
    @if [ -d backend/cmd/pushpoint ]; then cd backend && go test ./...; else echo "backend/cmd/pushpoint가 아직 없습니다. M1에서 활성화됩니다."; fi

# 파일 변경 시 테스트 자동 재실행 (gotestsum --watch). M3 태깅·eval 반복 루프에 유용.
test-watch:
    @command -v gotestsum >/dev/null 2>&1 || { echo "gotestsum 미설치 — just setup (또는 go install gotest.tools/gotestsum@v1.13.0)"; exit 1; }
    cd backend && gotestsum --watch --format pkgname

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

# 썸네일(data/thumbs)·포트 기록은 건드리지 않는다. 로컬 dev 전용(운영 데이터 아님).
# 개발 DB 초기화 — backend/data/의 SQLite 파일만 삭제(다음 just dev가 마이그레이션으로 재생성)
db-reset:
    #!/usr/bin/env bash
    set -euo pipefail
    shopt -s nullglob
    files=(backend/data/*.db backend/data/*.db-wal backend/data/*.db-shm)
    if [ ${#files[@]} -eq 0 ]; then echo "지울 dev DB가 없습니다 (backend/data/*.db)"; exit 0; fi
    printf '지움: %s\n' "${files[@]}"
    rm -f "${files[@]}"
    echo "dev DB 초기화 완료 — 다음 just dev가 마이그레이션으로 재생성합니다."

# golden set 태깅 정확도 측정 — top-3 Recall, 베이스라인 병기 (M3+)
eval:
    @if [ -d nlu/golden ] && [ -d backend/cmd/pushpoint ]; then cd backend && go run ./cmd/pushpoint eval ../nlu/golden/; else echo "nlu/golden/ 또는 backend/cmd/pushpoint가 아직 없습니다. just eval은 M3에서 활성화됩니다."; fi

# --- web (frontend) — api/openapi.yaml의 3번째 소비자, backend gen과 대칭 ---
# 가드는 frontend/package.json(스캐폴드 유무). 의존성 미설치는 안내 후 `just web-install`.

# 웹 의존성 설치 — npm ci(package-lock.json 고정 버전 재현 설치, CI와 동일 경로)
web-install:
    @if [ -f frontend/package.json ]; then cd frontend && npm ci; else echo "frontend/package.json이 없습니다 (frontend 스캐폴드 전)."; fi

# Vite dev 서버 :8421 (프록시로 /api·/thumbs·/healthz → Go, 같은 체크아웃의 just dev가
# 기록한 포트 우선 — 기록이 없을 때만 :8420부터 스캔)
web-dev:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ ! -f frontend/package.json ]; then echo "frontend/package.json이 없습니다 (frontend 스캐폴드 전)."; exit 1; fi
    if [ ! -d frontend/node_modules ]; then echo "frontend/node_modules가 없습니다. 먼저: just web-install"; exit 1; fi
    portfile=backend/data/.dev-api-port
    # 살아있는 기록일 때만 포트를 돌려준다. 형식이 깨졌거나(수동 편집, 권한 오류)
    # 기록을 남긴 just dev가 이미 죽었으면 없는 셈 친다 — 죽은 기록의 포트는 지금
    # 다른 worktree의 백엔드가 쓰고 있을 수 있고, 그때 /healthz는 200을 준다.
    live_port() {
      local port pid
      # -r까지 보는 이유: 읽을 수 없는 파일이면 리다이렉션 단계에서 셸이 직접
      # "Permission denied"를 찍는다(read의 2>/dev/null로는 못 막는다).
      [ -f "$portfile" ] && [ -r "$portfile" ] || return 1
      read -r port pid < "$portfile" 2>/dev/null || return 1
      case "${port:-}" in ''|*[!0-9]*) return 1 ;; esac
      [ "$port" -ge 1 ] && [ "$port" -le 65535 ] || return 1
      [ -n "${pid:-}" ] || return 1
      kill -0 "$pid" 2>/dev/null || return 1
      echo "$port"
    }
    api=""
    if [ -n "${PUSHPOINT_API_PORT:-}" ]; then
      api="$PUSHPOINT_API_PORT"
      echo "PUSHPOINT_API_PORT 지정: :$api → 프록시 연결"
    else
      # 1순위: 같은 worktree의 just dev가 남긴 포트. 포트 스캔은 다른 worktree(Orca 등
      # 병렬 작업)의 백엔드를 먼저 찾을 수 있어서, 내 worktree의 값이 있으면 그걸 쓴다.
      # api·web 탭이 동시에 뜨면 dev가 포트를 적기 전(실측 0.6초)에 여기 도달하므로
      # 잠깐 기다렸다 스캔으로 넘어간다.
      api="$(live_port || true)"
      if [ -z "$api" ]; then
        echo "이 worktree의 백엔드 포트 기록을 기다리는 중 (최대 2초)…"
        for _ in $(seq 1 8); do
          api="$(live_port || true)"
          if [ -n "$api" ]; then break; fi
          sleep 0.25
        done
      fi
      if [ -n "$api" ]; then
        if curl -sf -m 0.3 "http://127.0.0.1:$api/healthz" >/dev/null 2>&1; then
          echo "이 worktree의 백엔드: :$api → 프록시 연결"
        else
          echo "이 worktree의 백엔드 포트 :$api (아직 응답 없음 — just dev 가 뜨면 자동 연결)"
        fi
      else
        # 폴백: 이 worktree에 쓸 만한 기록이 없다 — 살아있는 아무 백엔드나 찾는다.
        # 다른 체크아웃의 것일 수 있으므로 그 사실을 감추지 않고 알린다.
        for i in $(seq 0 20); do
          p=$((8420 + i))
          if curl -sf -m 0.3 "http://127.0.0.1:$p/healthz" >/dev/null 2>&1; then api="$p"; break; fi
        done
        if [ -n "$api" ]; then
          echo "백엔드 감지: :$api → 프록시 연결 (이 worktree의 기록이 없습니다 — 다른 체크아웃의 백엔드일 수 있습니다)"
        else
          api=8420
          echo "실행 중인 백엔드를 못 찾았습니다 — 프록시를 :$api 로 둡니다 (먼저 just dev 를 띄우세요)"
        fi
      fi
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
# -ldflags "-s -w"는 배포 바이너리의 심볼 테이블·DWARF를 벗겨 크기를 줄인다(28→21MB). Go
# 런타임 트레이스백은 그대로라 panic 스택은 계속 나온다(운영 디버깅 무손상). dev용 just build는
# 심볼 유지(delve 등 로컬 디버깅).
# 릴리스 빌드 — 웹 번들을 내장한 단일 바이너리 backend/bin/pushpoint
release: web-build
    cd backend && go build -tags embed_frontend -ldflags "-s -w" -o bin/pushpoint ./cmd/pushpoint
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
