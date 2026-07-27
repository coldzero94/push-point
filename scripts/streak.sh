#!/usr/bin/env bash
# M6 완료 기준 판정 — "최근 28일 연속 저장".
#
# 08 §2·§3(M6 Week 4)·§4가 세 곳에서 이 스크립트를 이름으로 지정해 두고 파일이 없었다.
# M6의 완료 조건이 "4주 연속 일일 사용"인데 그걸 판정할 수단이 없는 상태였다.
#
# **연속의 정의는 iOS(`ios/PushPoint/StatsView.swift`)와 같아야 한다.** 두 판정이 갈라지면
# 화면은 "12일 연속"인데 스크립트는 "11일"이라고 말하게 되고, 그때 어느 쪽을 믿을지 정할
# 근거가 없다. 규칙 두 가지를 그대로 옮긴다:
#
#   1. **오늘 아직 저장하지 않았으면 어제부터 센다.** 자정 직후에 연속이 0으로 보이면
#      그 지표는 아무도 안 믿는다.
#   2. **저장이 0인 날은 by_day에 행이 아예 없다** — GROUP BY 결과이기 때문이다.
#      없는 날짜를 0으로 채우는 것이 아니라, 있는 날짜 집합에 커서를 물어보는 방식으로 센다.
#
# **이 숫자는 화면에도 있다** — iOS 통계 탭이 연속일과 4주 목표까지 남은 일수를 보여주고
# (`StatsView.swift`의 goalLine), 웹 설정의 통계 섹션도 같다. 2026-07-27에 그렇게 정했다.
#
# 이 주석은 원래 반대로 적혀 있었다: "성공 지표를 화면에 띄우면 의미 없는 링크를 저장해
# 연속을 잇는 유인이 생기므로 판정기는 터미널에만 둔다." 그 우려 자체는 유효하지만
# 출하된 동작과 달랐고, **어긋난 주석은 우려를 지키지도 못하면서 읽는 사람만 속인다.**
# 목표 제시의 동기부여 효과를 택했고, 그 대가로 Goodhart 압력을 안다는 것을 여기 적어 둔다.
#
# 이 스크립트가 남아 있는 이유는 따로다: **판정은 화면이 아니라 여기서 한다.** 화면은
# 사람에게 보여주는 것이고, M6 완료 조건(4주 연속)의 판정은 exit code가 있는 이 커맨드다.
#
# 사용법:
#   scripts/streak.sh                    # PUSHPOINT_ADDR 또는 http://127.0.0.1:8420
#   PUSHPOINT_API_KEY=... scripts/streak.sh
#
# 28일에 못 미치면 exit 1 — 게이트로 쓸 수 있게.
set -euo pipefail

TARGET_DAYS=28

addr="${PUSHPOINT_ADDR:-127.0.0.1:8420}"
case "$addr" in
  http://*|https://*) base="$addr" ;;
  *) base="http://$addr" ;;
esac
key="${PUSHPOINT_API_KEY:-dev-key}"

resp=$(curl -fsS -H "Authorization: Bearer $key" "$base/api/v1/stats" 2>/dev/null) || {
  echo "통계를 가져오지 못했습니다: $base/api/v1/stats"
  echo "서버가 떠 있나요? (just dev)  키가 맞나요? (PUSHPOINT_API_KEY)"
  exit 1
}

# 응답을 환경변수로 넘긴다 — heredoc과 <<< 를 겹치면 stdin이 어긋나 조용히 빈 입력이 된다.
TARGET_DAYS="$TARGET_DAYS" STATS_JSON="$resp" python3 - <<'PY'
import json, os, sys
from datetime import date, timedelta

target = int(os.environ["TARGET_DAYS"])
stats = json.loads(os.environ["STATS_JSON"])

# by_day는 GROUP BY 결과라 **저장이 0인 날은 행이 아예 없다.** 그래서 "없는 날을 0으로
# 채운 배열"을 만들지 않고, 저장이 있는 날짜 집합에 커서를 물어보는 방식으로 센다.
saved = {row["date"] for row in stats.get("by_day", []) if row.get("count", 0) > 0}

if not saved:
    print("아직 저장 기록이 없습니다.")
    sys.exit(1)

cursor = date.today()
# 오늘 아직 저장하지 않았다고 어제까지의 연속이 끊긴 것은 아니다(iOS와 같은 규칙).
if cursor.isoformat() not in saved:
    cursor -= timedelta(days=1)

streak = 0
while cursor.isoformat() in saved:
    streak += 1
    cursor -= timedelta(days=1)

# by_day는 30일 창이므로 그보다 긴 연속은 여기서 잘린다. 그 사실을 숨기지 않는다 —
# 28일 판정에는 충분하지만 "정확히 며칠째인가"는 이 창 밖을 못 본다.
window = len(stats.get("by_day", []))
capped = " (30일 창 상한 — 실제로는 더 길 수 있습니다)" if streak >= 30 else ""

print(f"연속 저장 {streak}일{capped} / 활동일 {len(saved)}일 / 창 {window}일")
if streak >= target:
    print(f"PASS — {target}일 목표 달성.")
    sys.exit(0)
print(f"FAIL — {target}일까지 {target - streak}일 남았습니다.")
sys.exit(1)
PY
