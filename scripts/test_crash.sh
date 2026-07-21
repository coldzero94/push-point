#!/usr/bin/env bash
# Push-Point M2 — 크래시 복구 검증 (just test-crash / 검증 매트릭스 M2).
#
# 빌드 → fixture 서버(지연 응답) → pushpoint 서버 → fixture URL N건 저장
# → kill 직전 미완료 잡 잔존 확인 → pushpoint kill -9 → 재기동
# → 타임아웃 내 전량 status='done' 단언. 하나라도 미도달이면 FAIL(exit 1).
#
# 외부 네트워크 의존 없음 — fixture 서버가 결정적으로 응답한다.
# (04-DATA-FLOW §6 크래시 복구 플로우를 자동 검증한다.)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 조정 가능한 파라미터 (env로 override). N은 목록 한 페이지(limit 100) 이내.
N="${N:-20}"
DELAY="${DELAY:-0.5s}"              # fixture page 응답 지연 — kill 시점에 running 잡 확보
DONE_TIMEOUT="${DONE_TIMEOUT:-60}"  # 재기동 후 전량 done 대기 상한(초). 도메인 rate-limit 여유
FIX_ADDR="127.0.0.1:19090"
FIX_BASE="http://${FIX_ADDR}"
PP_ADDR="127.0.0.1:18082"           # bench-http(18080)·coldstart(18081)와 겹치지 않게
PP_BASE="http://${PP_ADDR}"
KEY="crash-key"

if [ "$N" -gt 100 ]; then
  echo "FAIL: N=$N 는 100 이하만 지원(목록 단일 페이지). N을 줄이세요." >&2
  exit 1
fi

# ① 빌드 — pushpoint + fixture 서버 (리포 루트 기준 cd backend)
cd "$ROOT/backend"
go build -o bin/pushpoint ./cmd/pushpoint
go build -o bin/fixtureserver ./cmd/fixtureserver

# ② 임시 데이터 디렉터리 (매 실행 깨끗한 DB — 재기동은 같은 디렉터리 재사용)
TMP="$(mktemp -d)"
FIX_PID=""
PP_PID=""

# ⑨ trap — 두 서버 kill + 임시 디렉터리 정리 (모든 종료 경로에서 실행)
cleanup() {
  for pid in "$PP_PID" "$FIX_PID"; do
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  rm -rf "$TMP"
}
trap cleanup EXIT

# 서버 /healthz 200 대기 (최대 5s). 인자: $1=base URL, $2=pid
wait_healthz() {
  local base="$1" pid="$2" i=0
  until [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 1 "$base/healthz" || true)" = "200" ]; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "FAIL: 서버가 기동 중 종료됨 ($base)" >&2
      exit 1
    fi
    i=$((i + 1))
    if [ "$i" -ge 100 ]; then
      echo "FAIL: $base /healthz 5s 내 200 응답 없음" >&2
      exit 1
    fi
    sleep 0.05
  done
}

# link_stats — 목록 API로 "<total> <done> <failed>" 출력 (단일 페이지 limit=100).
# curl 실패/빈 응답에도 pipefail로 죽지 않도록 body를 먼저 잡는다.
link_stats() {
  local body
  body="$(curl -s --max-time 3 "$PP_BASE/api/v1/links?limit=100" \
    -H "Authorization: Bearer $KEY" || true)"
  printf '%s' "$body" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception:
    print("0 0 0"); sys.exit(0)
links = d.get("links", [])
done = sum(1 for x in links if x.get("status") == "done")
failed = sum(1 for x in links if x.get("status") == "failed")
print(len(links), done, failed)
'
}

# pushpoint 기동 헬퍼 — 같은 데이터 디렉터리/주소로 재사용(최초 기동 + 재기동 공용).
# PUSHPOINT_ALLOW_PRIVATE_HOSTS=1: fixture 서버가 127.0.0.1(루프백)이라 SSRF 가드를
# 꺼야 scrape가 성공한다 — 이 가드는 운영 기본 활성, 로컬 fixture 스크랩에서만 완화한다.
start_pushpoint() {
  PUSHPOINT_ADDR="$PP_ADDR" PUSHPOINT_API_KEY="$KEY" PUSHPOINT_DATA_DIR="$TMP" \
    PUSHPOINT_LOG_LEVEL="warn" PUSHPOINT_ALLOW_PRIVATE_HOSTS=1 "$ROOT/backend/bin/pushpoint" &
  PP_PID=$!
  wait_healthz "$PP_BASE" "$PP_PID"
}

# ③ fixture 서버 기동 (지연 응답, 백그라운드)
"$ROOT/backend/bin/fixtureserver" -addr "$FIX_ADDR" -delay "$DELAY" &
FIX_PID=$!
wait_healthz "$FIX_BASE" "$FIX_PID"

# ④ pushpoint 최초 기동
start_pushpoint

# ⑤ fixture URL N건 저장
for i in $(seq 1 "$N"); do
  code="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$PP_BASE/api/v1/links" \
    -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
    -d "{\"url\":\"${FIX_BASE}/page/${i}\"}" || true)"
  if [ "$code" != "201" ] && [ "$code" != "200" ]; then
    echo "FAIL: 링크 저장 실패 (i=$i, http=$code)" >&2
    exit 1
  fi
done

# 저장분이 목록에 모두 반영될 때까지 잠깐 대기 (total >= N)
for _ in $(seq 1 40); do
  read -r total dcount fcount <<<"$(link_stats)"
  if [ "$total" -ge "$N" ]; then
    break
  fi
  sleep 0.1
done

# ⑤' kill 시점에 미완료 잡이 남아 있어야 복구가 의미 있다 — total==N && done<N 확인.
read -r total dcount fcount <<<"$(link_stats)"
echo "kill 직전 상태: total=$total done=$dcount failed=$fcount (N=$N)"
if [ "$total" -lt "$N" ]; then
  echo "FAIL: 저장분이 목록에 반영되지 않음 (total=$total < N=$N)" >&2
  exit 1
fi
if [ "$dcount" -ge "$N" ]; then
  echo "FAIL: kill 전에 전량 done — 크래시 복구가 검증되지 않음. DELAY(현재 $DELAY) 또는 N($N)을 키우세요." >&2
  exit 1
fi

# ⑥ pushpoint 에 kill -9 (graceful shutdown 없음 — running 잡이 디스크에 남는다)
kill -9 "$PP_PID" 2>/dev/null || true
wait "$PP_PID" 2>/dev/null || true
PP_PID=""

# ⑦ 재기동 (같은 데이터 디렉터리 — 시작 시 RecoverStale이 running→pending 복구)
start_pushpoint

# ⑧ DONE_TIMEOUT 내 전량 done 폴링
deadline=$((SECONDS + DONE_TIMEOUT))
while :; do
  read -r total dcount fcount <<<"$(link_stats)"
  if [ "$dcount" -ge "$N" ]; then
    break
  fi
  if [ "$fcount" -gt 0 ]; then
    echo "FAIL: 재기동 후 failed=$fcount (done=$dcount/$N) — 복구가 아니라 실패로 종료" >&2
    exit 1
  fi
  if ! kill -0 "$PP_PID" 2>/dev/null; then
    echo "FAIL: 재기동한 서버가 종료됨 (done=$dcount/$N)" >&2
    exit 1
  fi
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "FAIL: ${DONE_TIMEOUT}s 내 전량 done 미도달 (done=$dcount/$N, failed=$fcount)" >&2
    exit 1
  fi
  sleep 0.5
done

# ⑩ 결과
echo "PASS: kill -9 후 ${dcount}/${N} 링크 복구 완료"
