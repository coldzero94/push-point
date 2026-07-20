# Push-Point v2.1 — 모노레포 태스크 러너 (just, https://just.systems)
# 2026-07-20 도구 평가(vs make/go-task/mise/moon) 후 채택. Go 레시피는 backend/ 에서 실행된다.
# 미구현 가드("M1에서 활성화")는 해당 마일스톤에서 코드가 생기면 걷어낸다.

# 레시피 목록 출력 (기본)
default:
    @just --list --unsorted

# 로컬 실행 — API 서버 + 워커 단일 프로세스 (PUSHPOINT_API_KEY=dev-key)
dev:
    @if [ -d backend/cmd/pushpoint ]; then cd backend && PUSHPOINT_API_KEY=dev-key go run ./cmd/pushpoint; else echo "backend/cmd/pushpoint가 아직 없습니다. M1에서 활성화됩니다."; fi

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

# 정적 분석 (golangci-lint — govet 포함)
lint:
    @if command -v golangci-lint >/dev/null 2>&1; then cd backend && golangci-lint run; else echo "golangci-lint가 설치되어 있지 않습니다. 설치: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest (M1에서 backend/go.mod tool 지시자로 버전 핀 예정)"; fi

# 포맷팅 — gofmt + goimports (backend 대상)
fmt:
    gofmt -l -w backend
    @if command -v goimports >/dev/null 2>&1; then goimports -l -w backend; else echo "goimports가 설치되어 있지 않습니다. 설치: go install golang.org/x/tools/cmd/goimports@latest"; fi
