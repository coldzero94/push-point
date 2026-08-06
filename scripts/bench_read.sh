#!/usr/bin/env bash
# Push-Point M6 — 읽기 경로 성능 게이트 (just bench-read)
#
# 08 §4가 지표를 다섯 적어 뒀는데 재는 명령은 **둘**뿐이었다(저장 p99, 콜드 스타트).
# 목록 100k와 검색 10k는 목표만 문서에 있었고 한 번도 측정된 적이 없다 — 12 §4.6도
# "재는 명령이 리포에 없다"고 적어 두고 있었다. 이 스크립트가 그 둘을 잰다.
#
# **규모가 곧 조건이다.** 목록은 100k행, 검색은 10k링크에서 재도록 08이 지정했으므로
# 작은 DB로 재면 초록이 아무 의미가 없다. 그래서 시딩이 이 스크립트의 절반이고,
# 시간이 걸린다(100k 시딩 ~1분).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADDR="127.0.0.1:18082"
BASE="http://${ADDR}"
KEY="bench-key"
LIST_N="${LIST_N:-100000}"
SEARCH_N="${SEARCH_N:-10000}"

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

# 시딩은 서버를 거치지 않는다 — 10만 건을 HTTP로 넣으면 측정이 아니라 대기가 된다.
# `seed`는 고정 난수라 실행 간 비교가 성립한다(08 M1).
echo "--- 시딩 ${LIST_N}건 (고정 난수) ---"
PUSHPOINT_DATA_DIR="$TMP" ./bin/pushpoint seed -n "$LIST_N" >/dev/null

PUSHPOINT_ADDR="$ADDR" PUSHPOINT_API_KEY="$KEY" PUSHPOINT_DATA_DIR="$TMP" \
  ./bin/pushpoint >"$TMP/server.log" 2>&1 &
SERVER_PID=$!

deadline=$((SECONDS + 15))
until [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 1 "$BASE/healthz" || true)" = "200" ]; do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "FAIL: 서버가 기동 중 종료됨" >&2
    tail -5 "$TMP/server.log" >&2
    exit 1
  fi
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "FAIL: /healthz 15s 내 200 응답 없음" >&2
    exit 1
  fi
  sleep 0.1
done

STATUS=0

# ① 목록 스크롤 — 커서를 이어 가며 깊은 페이지까지 간다(게이트 50ms).
echo "--- 목록 스크롤 (${LIST_N}행, 커서 페이지네이션) ---"
set +e
./bin/pushpoint readgen -addr "$BASE" -key "$KEY" -mode list -n 500
[ $? -ne 0 ] && STATUS=1
set -e

# ② 검색 — 08은 10k 기준이라 지금 DB가 더 크면 **더 불리한 조건**에서 재는 셈이다.
#    통과하면 그대로 유효하고, 실패하면 10k로 다시 재서 원인을 가른다.
echo "--- 검색 (한영 혼합 질의, 게이트 30ms) ---"
set +e
./bin/pushpoint readgen -addr "$BASE" -key "$KEY" -mode search -n 300
[ $? -ne 0 ] && STATUS=1
set -e

if [ "$STATUS" -eq 0 ]; then
  echo "PASS 읽기 경로 두 게이트 통과 (목록 50ms · 검색 30ms)"
else
  echo "FAIL 읽기 경로 게이트 초과"
fi
exit "$STATUS"
