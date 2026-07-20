#!/usr/bin/env bash
# Push-Point M1 — 콜드 스타트 게이트: 바이너리 exec → /healthz 200 < 1000ms
# 임시 데이터 디렉터리 사용 — 첫 마이그레이션 적용 시간까지 포함한 값이 곧 콜드 스타트.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADDR="127.0.0.1:18081"
BASE="http://${ADDR}"
GATE_MS=1000

# 빌드는 측정 밖 (측정 대상은 exec → 서빙)
cd "$ROOT/backend"
go build -o bin/pushpoint ./cmd/pushpoint

TMP="$(mktemp -d)"
SERVER_PID=""
cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

# macOS date는 %N 미지원 → python3로 ms 단위 시각
now_ms() { python3 -c 'import time; print(int(time.time() * 1000))'; }

START_MS="$(now_ms)"
PUSHPOINT_ADDR="$ADDR" PUSHPOINT_API_KEY="coldstart-key" PUSHPOINT_DATA_DIR="$TMP" \
  "$ROOT/backend/bin/pushpoint" &
SERVER_PID=$!

# 10ms 폴링. 루프 안에서는 시각 계산 없이 curl만 돌려 폴링 주기를 지키고,
# 200 관측 시점에 한 번만 elapsed를 계산한다. 대기 상한은 반복 횟수(약 5s)로 제한.
ELAPSED=""
i=0
while :; do
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 1 "$BASE/healthz" || true)"
  if [ "$code" = "200" ]; then
    ELAPSED=$(($(now_ms) - START_MS))
    break
  fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "FAIL: 서버가 기동 중 종료됨" >&2
    exit 1
  fi
  i=$((i + 1))
  if [ "$i" -ge 500 ]; then
    echo "FAIL: /healthz가 대기 상한(약 5s) 내에 200을 반환하지 않음" >&2
    exit 1
  fi
  sleep 0.01
done

if [ "$ELAPSED" -gt "$GATE_MS" ]; then
  echo "FAIL coldstart=${ELAPSED}ms (게이트 ${GATE_MS}ms)" >&2
  exit 1
fi
echo "PASS coldstart=${ELAPSED}ms (게이트 ${GATE_MS}ms)"
