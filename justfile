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

# B1 착수 게이트 — 요약이 색인에 새 3-gram을 더하는 링크 비율 (30% 미만이면 exit 1)
#
# 12 §4가 코드 전에 요구한 숫자다. 결과는 nlu/golden/README.md에 기록돼 있다.
b1-gate:
    @cd backend && go run ./cmd/b1gate

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

# 읽기 경로 성능 게이트 — 목록 100k p99 < 50ms · 검색 p99 < 30ms (초과 시 exit 1)
#
# 08 §4의 5지표 중 셋을 아무도 재지 않고 있었다. 저장 p99와 콜드 스타트만 명령이 있었고
# 나머지는 목표만 문서에 있었다 — 재지 않는 목표는 목표가 아니라 희망이다.
bench-read:
    @if [ -f scripts/bench_read.sh ]; then scripts/bench_read.sh; else echo "scripts/bench_read.sh가 없습니다."; fi

# 저장 → 태깅 완료 파이프라인 게이트 — p99 < 3s (초과 시 exit 1)
#
# 08 §4 다섯 지표의 마지막. 응답 시간이 아니라 **태그가 보이기까지**를 재므로 폴링이다.
# fixture 서버를 써서 네트워크에 의존하지 않는다.
bench-pipeline:
    @if [ -f scripts/bench_pipeline.sh ]; then scripts/bench_pipeline.sh; else echo "scripts/bench_pipeline.sh가 없습니다."; fi

# 추출의 **모양**을 잰다 — 벽 점수(경계 없이 이어지는 최장 구간)
#
# 기존 골든에는 원본 HTML이 없어서 `just eval`·`eval-search`는 이미 추출된 body_text를
# 읽는다 — 추출을 바꿔도 그 숫자들은 안 움직인다. 이 하네스가 그 사각을 본다.
# 서버 경로와 클라이언트(extract.js) 경로를 **같은 HTML**로 잰다.
eval-reader *args:
    @cd backend && go run ./cmd/pushpoint reader-eval {{args}}

# 크래시 복구 검증 — 빌드 → fixture 서버 → 저장 → kill -9 → 재기동 → 전량 done 단언 (M2+)
test-crash:
    @if [ -f scripts/test_crash.sh ]; then scripts/test_crash.sh; else echo "scripts/test_crash.sh가 아직 없습니다. M2에서 활성화됩니다."; fi

# Google 스프레드시트 연결 — 명령 한 번 + 붙여넣기 한 번으로 끝난다
#
# 클라우드 콘솔도, JSON 키도, API 켜기도 없다. 스크립트를 클립보드에 넣고 브라우저를
# 열어 주므로, 사용자는 붙여넣고 배포한 뒤 URL만 되돌려 주면 된다.
sheets-setup:
    @cd backend && go run ./cmd/pushpoint sheets-setup

# 아카이브를 Google 스프레드시트로 내보낸다 (단방향 — SQLite가 원본, 시트는 파생물)
#
# 저장 경로를 건드리지 않는다. 저장할 때마다 시트에 쓰면 외부 서비스가 저장 경로에
# 들어오는데, "네트워크 없이도 저장이 완결된다"가 M4 DoD로 확인된 이 제품의 성질이다.
# 그래서 실패해도 아카이브는 멀쩡하다.
#
# 필요한 환경변수는 인자 없이 실행하면 안내가 나온다.
sheets-sync:
    @cd backend && go run ./cmd/pushpoint sheets-sync

# 시트의 inbox 탭에 적힌 명령을 실행한다 (메모·태그·저장·삭제·재시도)
#
# 도는 서버가 필요하다 — 명령을 HTTP API로 실행한다(단일 라이터 SQLite에 두 번째 쓰기
# 프로세스를 만들지 않기 위해). «실행» 체크가 켜진 행만 돈다.
sheets-inbox *ARGS:
    @cd backend && go run ./cmd/pushpoint sheets-inbox {{ARGS}}

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

# 추출식 요약 회귀 측정 — Recall@3·중복도·커버리지를 lead-3 베이스라인과 병기 (M5+)
# 검색 품질 측정 — hit@1 · MRR@10 (네트워크 0, 커밋된 golden 코퍼스)
eval-search *ARGS:
    @if [ -d nlu/golden ] && [ -d backend/cmd/pushpoint ]; then cd backend && go run ./cmd/pushpoint eval-search {{ARGS}} ../nlu/golden; else echo "nlu/golden/ 또는 backend/cmd/pushpoint가 없습니다."; fi

# -dump를 붙이면 사람이 읽을 스팟체크 텍스트를 낸다: just eval-summary -dump
eval-summary *ARGS:
    @if [ -d nlu/golden ] && [ -d backend/cmd/pushpoint ]; then cd backend && go run ./cmd/pushpoint summary-eval {{ARGS}} ../nlu/golden; else echo "nlu/golden/ 또는 backend/cmd/pushpoint가 없습니다."; fi

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

# 프론트엔드 단위 테스트 (vitest) — 순수 로직만. DOM도 React도 안 띄운다.
#
# 이 자리가 오래 비어 있었고, 그 사이 `web-test`라는 이름을 아래 Go 레시피가 갖고 있어서
# "웹 테스트가 있다"처럼 보였다(실제로는 TypeScript를 한 줄도 실행하지 않는다).
# 2026-07-28에 이름을 제 주인에게 돌려줬다 — 그날 리뷰가 `weekOverWeek`의 실제 버그를
# 잡았는데, 테스트 러너가 있었으면 진작 빨개졌을 종류였다.
web-test: _web-required
    cd frontend && npm test

# spa.go는 embed_frontend 태그에서만 컴파일돼 태그 없는 just test가 커버하지 못하고,
# 임베드에 dist가 필요하므로 web-build 뒤에 돈다.
# 웹 embed 경로 테스트 — SPA 셸·자산 헤더·계약 표면 JSON 404
web-embed-test: web-build
    cd backend && go test -count=1 -tags embed_frontend ./internal/web/... ./internal/api/...

# 연속 저장 규칙이 웹·터미널에서 같은 답을 내는지 (testdata/streak-cases.json 공용 픽스처)
streak-selftest:
    bash scripts/streak.sh --self-test

# 랜딩 페이지(site/) 문구가 en·ko에서 갈라지지 않는지
#
# 두 벌을 손으로 관리하면 갈라진다 — streak·커버 패턴·iOS 골든에서 이미 세 번 당했고
# 전부 "양쪽 다 정상으로 보이는데 값이 다른" 형태였다. 그래서 문구는 site/copy.js 한
# 군데에 있고, 이 검사가 키 집합·오타·번역 누락을 막는다.
site-check:
    python3 scripts/site_copy_check.py

# docs/v2의 ko·en 두 벌이 갈라지지 않는지 (구조·표·코드·숫자 — 본문의 뜻은 사람이 본다)
docs-parity:
    python3 scripts/docs_parity_check.py

# 웹 앱 UI 문자열이 ko·en에서 갈라지지 않는지 (site-check와 같은 규칙, 대상만 앱)
web-i18n-check:
    python3 scripts/web_i18n_check.py

# iOS 앱 UI 문자열이 ko·en에서 갈라지지 않는지 (web-i18n-check와 같은 규칙)
ios-i18n-check:
    python3 scripts/ios_i18n_check.py

# 웹과 iOS가 같은 키를 같은 문장으로 말하는지 (각 클라이언트 내부 대칭은 위 두 레시피가 본다)
i18n-cross-check:
    python3 scripts/i18n_cross_check.py

# 커버 도형 픽스처를 웹 구현에서 다시 만든다 (testdata/cover-ops.json)
#
# 무늬를 고쳤으면 이걸 돌리고 결과를 같은 커밋에 넣는다 — 그러면 iOS 테스트가 빨개져
# 양쪽을 같이 고치게 된다. 그 강제가 이 픽스처의 존재 이유다.
cover-ops:
    cd frontend && node_modules/.bin/vite-node ../scripts/gen_cover_ops.ts > ../testdata/cover-ops.json
    @echo "testdata/cover-ops.json 재생성 완료 — just ios-test / web-test로 확인할 것"

# 데모 영상을 다시 찍는다 — 시뮬레이터를 몰면서 녹화하고 손가락 커서를 합성한다
#
# 커서를 합성하는 이유: 녹화만 하면 화면이 저절로 움직이는 것처럼 보인다. 시뮬레이터의
# ShowSingleTouches는 Xcode 26에서 동작하지 않는다(2026-08-03 실측).
demo-record flow="scripts/demo-flows/share-ko.json" out="/tmp/demo.mp4":
    python3 scripts/demo_record.py {{flow}} {{out}}

# 합성된 데모에서 커서가 실제로 누른 자리에 그려졌는지 (배포 전 필수)
demo-check video out="/tmp/demo.mp4":
    python3 scripts/demo_check.py {{video}} {{video}}.events.json

# 랜딩 페이지를 로컬에서 띄운다 (http://localhost:8877)
site:
    @echo "http://localhost:8877 — Ctrl+C로 종료"
    cd site && python3 -m http.server 8877

# 아이덴티티 마크 → 네 표면의 아이콘 (소스: design/icon/mark.svg)
#
# 생성물은 커밋한다 — CI가 macOS·Chrome 없이 돌기 때문에 재생성으로 검사할 수 없다.
# 계약 생성물(gen-check)과 달리 드리프트 게이트가 없으므로, 마크를 고쳤으면
# 이 레시피를 직접 돌리고 결과를 같은 커밋에 넣는다.
icons:
    bash scripts/gen_icons.sh

# 정적 분석 (golangci-lint — govet 포함)
lint:
    @if command -v golangci-lint >/dev/null 2>&1; then cd backend && golangci-lint run; else echo "golangci-lint가 설치되어 있지 않습니다. 설치: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest (M1에서 backend/go.mod tool 지시자로 버전 핀 예정)"; fi

# 포맷팅 — gofmt + goimports (backend 대상)
fmt:
    gofmt -l -w backend
    @if command -v goimports >/dev/null 2>&1; then goimports -l -w backend; else echo "goimports가 설치되어 있지 않습니다. 설치: go install golang.org/x/tools/cmd/goimports@latest"; fi

# ---- iOS (M4) ----
# 소스는 ios/project.yml · Swift · entitlements 뿐이고, .xcodeproj·Frameworks·extract.js
# 사본은 전부 생성물이다(.gitignore). 체크아웃 직후 필요한 순서: ios-bind → ios-gen → ios-build

# 바인드를 둘로 나눈 이유는 Share Extension의 메모리 예산이다 — scraper를 링크만 해도
# RSS가 13.4MB → 64.2MB로 뛴다(docs/v2/08-DEVELOPMENT-PLAN.md M4 선행 검증).

# Go 백엔드 → iOS 프레임워크 2종(PPCore·PPShare) + 캡처 규칙 사본
ios-bind:
    @command -v gomobile >/dev/null 2>&1 || { echo "gomobile이 없습니다. 설치: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init"; exit 1; }
    mkdir -p ios/Frameworks
    cd backend && gomobile bind -target=ios -o ../ios/Frameworks/PPCore.xcframework ./mobile/ppcore
    cd backend && gomobile bind -target=ios -o ../ios/Frameworks/PPShare.xcframework ./mobile/ppshare
    # 캡처 규칙은 extension/src/extract.js 하나가 원본이다 — 사파리 공유용으로 복사만 한다.
    # 복사가 아니라 각자 관리하면 브라우저와 iOS의 저장 결과가 조용히 갈라진다.
    cp extension/src/extract.js ios/PushPointShare/extract.js
    # 무엇으로 만들었는지 남긴다 — ios-bind-check가 이 값을 다시 계산해 비교한다.
    bash scripts/bind_stamp.sh > ios/Frameworks/.bind.sha256
    @echo "ios-bind: PPCore/PPShare.xcframework + extract.js 준비 완료"

# 프레임워크가 백엔드보다 낡았는지 검사한다. ios-build가 이걸 먼저 돌린다.
#
# 이 게이트가 없던 동안 **이틀 낡은 프레임워크**로 앱이 돌았고, 그 결과 앱 안의 태그
# 사전이 42개가 아니라 30개였다(마이그레이션 0008~0011 누락). 화면에서는 "왜 이 링크만
# 태그가 없지"로 보였고 원인을 찾는 데 한참 걸렸다.
#
# CI에는 macOS도 gomobile도 없으므로 이건 **로컬 게이트**다 — ios-api-gen-check와 같다.
ios-bind-check:
    #!/usr/bin/env bash
    set -euo pipefail
    f=ios/Frameworks/.bind.sha256
    if [ ! -d ios/Frameworks/PPCore.xcframework ]; then
        echo "프레임워크가 없습니다 — just ios-bind 를 먼저 실행하세요"; exit 1
    fi
    if [ ! -f "$f" ]; then
        echo "바인드 스탬프가 없습니다(이 게이트 이전에 만든 프레임워크입니다)."
        echo "just ios-bind 로 다시 만드세요 — 낡았는지 확인할 방법이 없습니다."; exit 1
    fi
    # 캡처 규칙 사본이 실제로 있는지, 원본과 같은지 본다.
    #
    # **스탬프만으로는 못 잡는다** — 스탬프는 원본(extension/src/extract.js)을 해싱하므로
    # 사본이 사라져도 값이 그대로다. 실제로 사본을 지우고 빌드하면 `ios-bind-check`가
    # OK를 내고 빌드도 성공하는데, 확장 번들에 파일이 없어 **사파리 캡처가 조용히
    # 죽는다**: Info.plist가 NSExtensionJavaScriptPreprocessingFile을 요구하지만 없으면
    # 시스템이 전처리를 건너뛰고, 저장은 URL만으로 진행돼 본문·요약·태그가 사라진다.
    # 배너는 그래도 "저장했습니다"라고 말한다(2026-07-29 재현).
    copy=ios/PushPointShare/extract.js
    if [ ! -f "$copy" ]; then
        echo "캡처 규칙 사본이 없습니다: $copy"
        echo "확장 번들에 안 들어가 사파리 본문 캡처가 조용히 죽습니다 — just ios-bind"
        exit 1
    fi
    if ! cmp -s extension/src/extract.js "$copy"; then
        echo "캡처 규칙 사본이 원본과 다릅니다 ($copy)"
        echo "브라우저 확장과 iOS가 다른 규칙으로 본문을 뽑게 됩니다 — just ios-bind"
        exit 1
    fi
    want=$(bash scripts/bind_stamp.sh)
    got=$(cat "$f")
    if [ "$want" != "$got" ]; then
        echo "프레임워크가 백엔드보다 낡았습니다."
        echo "  스탬프: $got"
        echo "  현재  : $want"
        echo "backend/ 또는 migrations/ 가 바뀌었는데 재바인드하지 않았습니다 — just ios-bind"
        exit 1
    fi
    echo "ios-bind-check OK"

# api/openapi.yaml → ios/PushPoint/Generated/ (swift-openapi-generator)
#
# 계약의 세 번째 소비자다(`just gen`=Go, `just web-gen`=TS). 지금까지 이 단계만 수동이라
# 스펙을 고칠 때 iOS 생성물이 조용히 뒤처질 수 있었다 — 규칙(.claude/rules/api.md)은 셋을
# 함께 재생성해야 스펙 변경이 끝난 것으로 본다.
#
# SPM 빌드 플러그인이 아니라 CLI로 부르고 산출물을 커밋한다(재현성·드리프트 검사).
# ios/tools/openapi-gen은 생성기를 가져오기 위해서만 존재하는 최소 패키지다.
ios-api-gen:
    cd ios/tools/openapi-gen && swift run swift-openapi-generator generate \
        ../../../api/openapi.yaml --mode types --mode client \
        --output-directory ../../PushPoint/Generated
    # 어떤 계약으로 생성했는지 스탬프를 남긴다. CI는 macOS·Swift 없이 이 해시만
    # 비교해 "스펙을 고치고 iOS 재생성을 잊은" 상태를 잡는다.
    @shasum -a 256 api/openapi.yaml | awk '{print $1}' > ios/PushPoint/Generated/.openapi.sha256
    @echo "ios-api-gen: ios/PushPoint/Generated/{Types,Client}.swift 갱신"

# M6 완료 기준 판정 — 최근 28일 연속 저장 (미달 시 exit 1)
#
# 연속의 정의는 iOS StatsView와 **같아야 한다** — 갈라지면 화면과 스크립트가 다른 숫자를
# 말하고 어느 쪽을 믿을지 정할 근거가 없다.
streak:
    @scripts/streak.sh

# M4 DoD 판정 — 공유 저장이 2초를 지켰는지 (확장이 남긴 계측 기록을 읽는다)
save-timing udid="":
    @scripts/save_timing.sh {{udid}}

# Maestro 플로우 — 부팅된 시뮬레이터의 **실제 데이터**에 대고 화면이 멀쩡한지 본다
#
# XCUITest(just ios-uitest)와 역할이 다르다. 저쪽은 픽스처를 심는 CI 게이트이고,
# 이쪽은 내 진짜 아카이브가 든 앱을 그대로 훑는다 — 그래서 내용은 단언하지 않는다.
flow file="maestro/smoke.yaml":
    @command -v maestro >/dev/null 2>&1 || { echo "maestro 미설치 — brew install mobile-dev-inc/tap/maestro"; exit 1; }
    maestro test {{file}}

# 실기기 설치 — 무료 프로비저닝(Personal Team)으로도 된다
#
# 무료 계정은 프로파일이 **7일마다 만료**돼 매일 쓰는 앱과는 양립하지 않지만, 1회성 실측
# (확장 메모리·0xdead10cc)에는 충분하다. $99가 실제로 필요해지는 것은 M4 DoD의 "연속 7일"과
# M6의 28일 스트릭뿐이다.
#
# 선행 조건(사람이 해야 하는 것):
#   1. Xcode → Settings → Accounts 에 Apple ID 로그인 (자격증명 입력이라 자동화 대상 아님)
#   2. 폰을 USB로 연결하고 "이 컴퓨터를 신뢰" 승인
#   3. 팀 ID 확인 후 아래처럼 실행 — `just ios-device TEAMID`
#      팀 ID는 `just ios-teams` 가 찾아 준다.
#
# **App Group이 무료 팀에서 거부될 수 있다.** 그러면 설치 자체가 실패하거나 저장이 죽는데,
# 그때도 확장은 자기 컨테이너에 계측을 남기므로(SaveTiming) 메모리 수치는 건진다.
ios-device team="": ios-gen
    #!/usr/bin/env bash
    set -euo pipefail
    team="{{team}}"
    if [ -z "$team" ]; then
      echo "팀 ID가 필요합니다. 먼저 'just ios-teams'로 확인한 뒤 'just ios-device <TEAMID>'."
      exit 1
    fi
    export PUSHPOINT_TEAM_ID="$team"
    cd ios && xcodegen generate >/dev/null
    # 연결된 기기를 먼저 찾는다 — 없으면 빌드만 하고 안내한다.
    udid=$(xcrun devicectl list devices 2>/dev/null | awk '/connected/ {print $(NF-1); exit}' || true)
    if [ -n "$udid" ]; then
      dest="-destination id=$udid"
    else
      dest="-destination generic/platform=iOS"
    fi
    set +e
    xcodebuild -project PushPoint.xcodeproj -scheme PushPoint \
        $dest -derivedDataPath .build \
        DEVELOPMENT_TEAM="$team" -allowProvisioningUpdates 2>&1 | \
        { grep -E "error:|warning: .*[Pp]rovisioning|\*\* BUILD" || true; }
    rc=${PIPESTATUS[0]}
    set -e
    [ "$rc" -eq 0 ] || { echo "빌드 실패 — 위 오류 참조. App Group 관련이면 무료 팀에서 거부된 것일 수 있습니다."; exit "$rc"; }
    if [ -z "$udid" ]; then
      echo
      echo "기기가 연결돼 있지 않아 빌드만 했습니다. USB로 연결하고 \"이 컴퓨터를 신뢰\"를 승인한 뒤 다시 실행하세요."
      exit 0
    fi
    app=.build/Build/Products/Debug-iphoneos/PushPoint.app
    xcrun devicectl device install app --device "$udid" "$app"
    echo
    echo "설치 완료. 이제 폰에서:"
    echo "  1. 설정 → 일반 → VPN 및 기기 관리 에서 개발자 앱을 신뢰"
    echo "  2. 사파리에서 아무 페이지나 공유 시트 → Push-Point"
    echo "  3. 다시 여기서:  just save-timing"
    echo
    echo "무료 프로비저닝은 7일 만료입니다 — M4 DoD(연속 7일)는 오늘부터 세면 재설치 없이 들어갑니다."

# 이 머신에서 쓸 수 있는 서명 팀 목록 (무료 Personal Team 포함)
ios-teams:
    #!/usr/bin/env bash
    ids=$(security find-identity -v -p codesigning 2>/dev/null | grep -c "Apple Development" || true)
    if [ "${ids:-0}" -eq 0 ]; then
      echo "서명 인증서가 없습니다 — Xcode → Settings → Accounts 에서 Apple ID로 먼저 로그인하세요."
      echo "(자격증명 입력이라 이 명령이 대신할 수 없습니다.)"
      exit 1
    fi
    security find-identity -v -p codesigning | grep "Apple Development"
    echo
    echo "괄호 안 10자리가 팀 ID입니다 → just ios-device <TEAMID>"

# 단위 테스트 (PushPointTests) — 규칙을 고정하는 자리. 화면은 ios-uitest가 본다.
#
# 이게 없어서 `CoverPatternTests`가 **한 번도 돌지 않았다** — ios-uitest가
# `-only-testing:PushPointUITests`로 잘라내기 때문이다. 그 테스트는 웹과 iOS의 커버 해시가
# 갈라지는 것을 잡으려고 기준값을 박아 둔 것인데, 갈라져도 양쪽 다 정상 동작하는 것처럼
# 보이므로 안 돌면 존재하지 않는 것과 같다.
ios-test device="iPhone 17": ios-gen
    #!/usr/bin/env bash
    # set -o pipefail이 없으면 파이프라인 종료 코드가 grep 것이 되고, `|| true`가 그마저
    # 0으로 덮는다 — 실측으로 xcodebuild exit 65가 레시피 exit 0이 됐다. 즉 **게이트가
    # 실패를 보고할 수 없었다.** `|| true`를 grep 단계 안으로 옮기면 xcodebuild의 상태가
    # 파이프라인 상태로 남고, grep의 no-match만 무력화된다.
    set -uo pipefail
    cd ios && xcodebuild test -project PushPoint.xcodeproj -scheme PushPoint \
        -destination 'platform=iOS Simulator,name={{device}}' \
        -only-testing:PushPointTests \
        -derivedDataPath .build CODE_SIGN_IDENTITY="-" | \
        { grep -E "Test Case|error:|\*\* TEST" || true; }

# iOS 생성물 드리프트 검사 — Go(gen-check)·웹(web-gen-check)과 같은 자리
#
# 계약의 세 소비자 중 iOS만 검사가 없었다. 생성 레시피(ios-api-gen)는 있는데 드리프트를
# 잡는 쪽이 없으면, 스펙을 고치고 iOS만 재생성을 잊어도 아무것도 실패하지 않는다.
ios-api-gen-check:
    @just ios-api-gen >/dev/null && git diff --exit-code ios/PushPoint/Generated

# 계약 스탬프만 검사 — Swift 툴체인 없이 돌아서 리눅스 CI에서도 쓸 수 있다.
#
# 완전한 검사(ios-api-gen-check)는 macOS + Swift가 필요해 CI에 넣기 비싸다. 스탬프는
# "이 생성물이 어느 openapi.yaml에서 나왔는가"만 보므로 **스펙을 고치고 iOS 재생성을
# 잊은** 경우를 잡는다 — 그게 실제로 일어나는 드리프트다.
ios-stamp-check:
    #!/usr/bin/env bash
    set -euo pipefail
    f=ios/PushPoint/Generated/.openapi.sha256
    if [ ! -f "$f" ]; then echo "스탬프가 없습니다 — just ios-api-gen 실행 후 커밋하세요"; exit 1; fi
    want=$(shasum -a 256 api/openapi.yaml | awk '{print $1}')
    got=$(cat "$f")
    if [ "$want" != "$got" ]; then
      echo "iOS 생성물이 지금 계약에서 나오지 않았습니다."
      echo "  api/openapi.yaml : $want"
      echo "  스탬프           : $got"
      echo "just ios-api-gen 을 돌리고 생성물과 스탬프를 함께 커밋하세요."
      exit 1
    fi
    echo "ios-stamp-check OK"

# 화면을 실제로 조작하는 UI 테스트 (XCUITest, 시뮬레이터)
#
# 앱을 `-uitest`로 띄운다 — 임시 디렉터리 + 자체 픽스처라 시뮬레이터에 무엇이 들어
# 있든 결과가 같고, **사용자의 실제 아카이브는 건드리지 않는다**(ios/PushPoint/UITestMode.swift).
#
# 이게 있어야 목록·검색·태그 편집을 사람 눈 없이 검증할 수 있다. 지금까지 이 영역의
# 실패는 전부 "타입은 맞고 화면만 틀린" 종류였고, 컴파일러도 단위 테스트도 못 잡았다.
ios-uitest device="iPhone 17": ios-gen
    #!/usr/bin/env bash
    # 종료 코드 처리는 ios-test와 같은 이유다 — 그쪽 주석 참조.
    set -uo pipefail
    cd ios && xcodebuild test -project PushPoint.xcodeproj -scheme PushPoint \
        -destination 'platform=iOS Simulator,name={{device}}' \
        -only-testing:PushPointUITests \
        -derivedDataPath .build CODE_SIGN_IDENTITY="-" | \
        { grep -E "Test Case|error:|\*\* TEST" || true; }

# ios/project.yml → ios/PushPoint.xcodeproj (XcodeGen)
ios-gen:
    @command -v xcodegen >/dev/null 2>&1 || { echo "xcodegen이 없습니다. 설치: brew install xcodegen"; exit 1; }
    cd ios && xcodegen generate

# ad-hoc 서명("-")을 쓴다. 서명을 꺼버리면(CODE_SIGNING_ALLOWED=NO) entitlement가
# 적용되지 않아 App Group 컨테이너 조회가 "client is not entitled"로 실패하고,
# 확장과 본체가 같은 DB를 볼 수 없다 — 이 앱의 핵심 경로가 통째로 죽는다.
# ad-hoc이면 Apple 계정 없이도 시뮬레이터에서 entitlement가 살아 있다.

# 시뮬레이터용 앱 + 확장 빌드 (ad-hoc 서명 — 계정 불요, App Group 동작)
ios-build device="iPhone 17": ios-bind-check ios-gen
    #!/usr/bin/env bash
    set -euo pipefail
    cd ios && xcodebuild -project PushPoint.xcodeproj -scheme PushPoint \
        -destination 'platform=iOS Simulator,name={{device}}' \
        -derivedDataPath .build \
        CODE_SIGN_IDENTITY="-" CODE_SIGNING_REQUIRED=NO CODE_SIGNING_ALLOWED=YES build
    # **산출물을 본다.** 입력 검사(ios-bind-check)는 빌드 전 상태만 알고, 리소스가
    # 번들에 실제로 들어갔는지는 모른다 — XcodeGen에서 리소스 선언이 조용히 무시된
    # 전례가 있다(테스트 타깃에 `resources:` 키를 썼다가 폰트가 안 들어갔다).
    # Info.plist가 요구하는 파일이 없으면 시스템은 전처리를 건너뛰고, 저장은 URL만으로
    # 진행돼 본문·요약·태그가 사라지는데 배너는 "저장했습니다"라고 말한다.
    appex=$(find .build/Build/Products/Debug-iphonesimulator/PushPoint.app/PlugIns \
              -maxdepth 1 -name '*.appex' | head -1)
    if [ -n "$appex" ] && [ ! -f "$appex/extract.js" ]; then
        echo "확장 번들에 extract.js가 없습니다 — 사파리 본문 캡처가 죽습니다"
        exit 1
    fi

# 시뮬레이터에 설치·실행 (부팅 포함)
ios-run device="iPhone 17": (ios-build device)
    xcrun simctl boot "{{device}}" 2>/dev/null || true
    open -a Simulator
    xcrun simctl install booted ios/.build/Build/Products/Debug-iphonesimulator/PushPoint.app
    xcrun simctl launch booted com.pushpoint.app
