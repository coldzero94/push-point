#!/usr/bin/env bash
# Push-Point M1 — 저장 API HTTP 경로 p99 게이트 (just bench-http)
# 빌드 → 임시 데이터 디렉터리로 서버 기동 → 워밍 100회 → loadgen 2000회.
# p99 판정과 exit 코드는 loadgen이 담당한다 (게이트 50ms).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ADDR="127.0.0.1:18080"
BASE="http://${ADDR}"
KEY="bench-key"

# ① 빌드 (리포 루트 기준: cd backend && go build)
cd "$ROOT/backend"
go build -o bin/pushpoint ./cmd/pushpoint

# ⑤ trap — 서버 kill + 임시 디렉터리 정리 (실패 경로 포함 항상 실행)
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

# ② 서버 백그라운드 기동 (임시 데이터 디렉터리 — 매 실행 깨끗한 DB)
PUSHPOINT_ADDR="$ADDR" PUSHPOINT_API_KEY="$KEY" PUSHPOINT_DATA_DIR="$TMP" \
  "$ROOT/backend/bin/pushpoint" &
SERVER_PID=$!

# ③ /healthz 200까지 대기 (최대 5s)
deadline=$((SECONDS + 5))
until [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 1 "$BASE/healthz" || true)" = "200" ]; do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "FAIL: 서버가 기동 중 종료됨" >&2
    exit 1
  fi
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "FAIL: /healthz 5s 내 200 응답 없음" >&2
    exit 1
  fi
  sleep 0.05
done

# ④-1 워밍 100회 — JIT/페이지 캐시 예열, 판정에 미포함 (url은 매회 상이 → 중복 경로 회피)
for i in $(seq 1 100); do
  curl -s -o /dev/null -X POST "$BASE/api/v1/links" \
    -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
    -d "{\"url\":\"https://warmup.invalid/item/$i\"}"
done

# ④-2 본 측정 — p99 판정·exit 코드는 loadgen 담당
RESULT_JSON="$TMP/bench_result.json"
set +e
"$ROOT/backend/bin/pushpoint" loadgen -addr "$BASE" -key "$KEY" -n 2000 >"$RESULT_JSON"
LOADGEN_EXIT=$?
set -e

# ⑥ 결과 JSON 그대로 출력 + 한 줄 요약
cat "$RESULT_JSON"
P99="$(python3 -c '
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    sys.exit(0)
for k in ("p99_ms", "p99", "p99ms"):
    if k in d:
        print(d[k])
        break
' "$RESULT_JSON" 2>/dev/null || true)"
P99="${P99:-?}"

if [ "$LOADGEN_EXIT" -eq 0 ]; then
  echo "PASS p99=${P99}ms (게이트 50ms)"
else
  echo "FAIL p99=${P99}ms (게이트 50ms)"
fi
# ⑦ 클라이언트 캡처 경로도 같은 게이트로 잰다. 기본 요청은 ~50B라 새 경로(본문 정제·절단 +
# 큰 INSERT)를 한 번도 지나지 않으므로, 그것만 재면 그린이 증거가 되지 못한다.
# 캡 최대치(body_text 32KB + title/description 2560B)로 500회.
echo "--- 클라이언트 캡처 페이로드(32KB 본문) ---"
CAPTURE_JSON="$TMP/bench_capture.json"
set +e
"$ROOT/backend/bin/pushpoint" loadgen -addr "$BASE" -key "$KEY" -n 500 \
  -body-bytes 32768 -meta-bytes 2560 >"$CAPTURE_JSON"
CAPTURE_EXIT=$?
set -e
cat "$CAPTURE_JSON"
if [ "$CAPTURE_EXIT" -ne 0 ]; then
  echo "FAIL 클라이언트 캡처 경로가 게이트를 넘김"
  LOADGEN_EXIT="$CAPTURE_EXIT"
else
  echo "PASS 클라이언트 캡처 경로도 게이트 통과"
fi

exit "$LOADGEN_EXIT"
