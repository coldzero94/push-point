#!/usr/bin/env bash
# Push-Point M6 — 저장 → 태깅 완료 파이프라인 게이트 (just bench-pipeline)
#
# 08 §4의 다섯 번째 지표. `bench-read`가 셋 중 둘을 닫았고 이것이 마지막이다.
#
# **다른 모양의 측정이다.** 저장 API는 201을 즉시 돌려주므로(그게 p99 50ms 게이트의
# 내용이다) 여기서 재는 것은 응답 시간이 아니라 **사용자가 태그를 보게 되기까지**다.
# 폴링 말고는 볼 방법이 없다.
#
# **도메인 예의 간격을 끈다(-1).** 켜 두면 fixture 한 호스트에 20건을 몰아넣는 이 하네스가
# 그 대기를 재게 된다 — 실측 p50 2000ms vs 27ms, 즉 잰 것의 99%가 하네스가 만든 것이었다.
# 실사용은 저장마다 도메인이 다르니 그 상수는 걸리지 않는다. 남의 사이트에 대한 예의는
# 파이프라인의 일부가 아니다.
#
# 네트워크에 의존하지 않는다 — `cmd/fixtureserver`가 og 메타를 담은 HTML을 결정적으로
# 돌려준다(test_crash.sh와 같은 것을 쓴다). 실제 웹을 긁으면 측정이 남의 사이트 사정에
# 흔들리고, 재현되지 않는 수치는 게이트가 못 된다.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADDR="127.0.0.1:18086"
FIX_ADDR="127.0.0.1:19091"
BASE="http://${ADDR}"
KEY="bench-key"
N="${N:-20}"
# fixture 지연 0 — 남의 서버 왕복이 아니라 **우리 파이프라인**을 재는 것이 목적이다.
DELAY="${DELAY:-0s}"

cd "$ROOT/backend"
go build -o bin/pushpoint ./cmd/pushpoint
go build -o bin/fixtureserver ./cmd/fixtureserver

TMP="$(mktemp -d)"
PIDS=()
cleanup() {
  for p in "${PIDS[@]:-}"; do
    [ -n "$p" ] && kill "$p" 2>/dev/null || true
  done
  wait 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

./bin/fixtureserver -addr "$FIX_ADDR" -delay "$DELAY" >"$TMP/fixture.log" 2>&1 &
PIDS+=($!)

# ALLOW_PRIVATE_HOSTS: fixture가 루프백이라 SSRF 가드를 꺼야 수집이 성공한다.
# 운영 기본은 활성이고 로컬 fixture 수집에서만 완화한다(test_crash.sh와 같은 판단).
PUSHPOINT_ADDR="$ADDR" PUSHPOINT_API_KEY="$KEY" PUSHPOINT_DATA_DIR="$TMP" \
  PUSHPOINT_LOG_LEVEL="warn" PUSHPOINT_ALLOW_PRIVATE_HOSTS=1 \
  PUSHPOINT_SCRAPE_RATE_INTERVAL=-1s \
  ./bin/pushpoint >"$TMP/server.log" 2>&1 &
PIDS+=($!)

deadline=$((SECONDS + 15))
until [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 1 "$BASE/healthz" || true)" = "200" ]; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "FAIL: /healthz 15s 내 200 응답 없음" >&2
    tail -5 "$TMP/server.log" >&2
    exit 1
  fi
  sleep 0.1
done

echo "--- 저장 → 태깅 완료 (${N}건, 게이트 3000ms) ---"
set +e
./bin/pushpoint pipegen -addr "$BASE" -key "$KEY" -target "http://${FIX_ADDR}/page" -n "$N"
STATUS=$?
set -e

if [ "$STATUS" -eq 0 ]; then
  echo "PASS 파이프라인이 예산 안이다 (게이트 3000ms)"
else
  echo "FAIL 파이프라인 게이트 초과"
fi
exit "$STATUS"
