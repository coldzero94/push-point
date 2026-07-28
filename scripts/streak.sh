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
#   2. **by_day의 마지막 칸이 오늘이다.** 계약이 빈 날까지 채운 30칸을 보장하므로
#      (api/openapi.yaml Stats.by_day) 날짜를 맞춰 볼 필요 없이 뒤에서부터 세면 된다.
#      2026-07-28 이전에는 GROUP BY 결과를 그대로 받아 날짜 집합에 커서를 물어봤는데,
#      그러면 "오늘"을 이 스크립트가 도는 호스트의 시계로 정하게 된다. 서버와 타임존이
#      다르면 하루가 어긋나고, 어긋나는 대상이 M6 완료 판정 숫자다.
#
# **일치는 주장이 아니라 테스트다**: `scripts/streak.sh --self-test`와 웹의
# `rhythm.test.ts`가 `testdata/streak-cases.json` 한 파일을 같이 읽는다. 갈라지면 빨개진다.
# (iOS는 아직 이 픽스처를 안 읽는다 — streak가 뷰 안의 private func이라 테스트 타깃에서
# 안 보이고, 그걸 꺼내는 것은 별건이다.)
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
#   scripts/streak.sh --self-test        # 서버 없이 계산 규칙만 검증
#
# 28일에 못 미치면 exit 1 — 게이트로 쓸 수 있게.
set -euo pipefail

TARGET_DAYS=28
CASES="$(cd "$(dirname "$0")/.." && pwd)/testdata/streak-cases.json"

# --self-test는 웹(frontend/src/lib/rhythm.test.ts)과 **같은 픽스처**를 읽는다. 두 구현이
# 갈라지면 둘 중 하나가 빨개진다 — 예전에는 "대조해서 일치를 확인했다"는 문장이 docs에
# 있었을 뿐이라 다시 돌려볼 수가 없었다.
if [ "${1:-}" = "--self-test" ]; then
  CASES="$CASES" python3 - <<'PY'
import json, os, sys

cases = json.load(open(os.environ["CASES"]))["cases"]
sys.path.insert(0, os.path.dirname(os.environ["CASES"]))

def streak_of(counts):
    i = len(counts) - 1
    if i < 0:
        return 0
    if counts[i] == 0:
        i -= 1
    n = 0
    while i >= 0 and counts[i] > 0:
        n += 1
        i -= 1
    return n

bad = 0
for c in cases:
    got = streak_of(c["counts"])
    ok = got == c["streak"]
    capped = got > 0 and got >= len(c["counts"])
    ok = ok and capped == c["capped"]
    if not ok:
        bad += 1
        print(f"FAIL  {c['name']}: streak={got} (want {c['streak']}), capped={capped} (want {c['capped']})")
    else:
        print(f"ok    {c['name']}")
print()
if bad:
    print(f"{bad}건 불일치 — 웹 구현(frontend/src/lib/rhythm.ts)과 갈라졌습니다.")
    sys.exit(1)
print(f"{len(cases)}건 전부 일치.")
PY
  exit 0
fi

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

target = int(os.environ["TARGET_DAYS"])
stats = json.loads(os.environ["STATS_JSON"])

# by_day는 계약이 **빈 날까지 채운 30칸**으로 보장한다 — 오름차순이고 **마지막 칸이
# 서버 로컬타임 기준 오늘**이다(api/openapi.yaml Stats.by_day). 그래서 여기서 date.today()를
# 부르지 않는다: 이 스크립트가 도는 셸 호스트의 오늘과 서버의 오늘이 다를 수 있고,
# 그 차이는 연속 일수를 하루 어긋나게 만든다 — M6 완료 판정에 쓰는 바로 그 숫자다.
# 뒤에서부터 세면 그 문제가 없다.
counts = [row.get("count", 0) for row in stats.get("by_day", [])]

if not any(c > 0 for c in counts):
    print("아직 저장 기록이 없습니다.")
    sys.exit(1)

i = len(counts) - 1
# 오늘 아직 저장하지 않았다고 어제까지의 연속이 끊긴 것은 아니다(iOS·웹과 같은 규칙).
if counts[i] == 0:
    i -= 1

streak = 0
while i >= 0 and counts[i] > 0:
    streak += 1
    i -= 1

# 연속이 창 끝까지 닿으면 진짜 길이는 창 밖이라 모른다. 그 사실을 숨기지 않는다 —
# 28일 판정에는 충분하지만 "정확히 며칠째인가"는 이 창 밖을 못 본다.
window = len(counts)
capped = f" ({window}일 창 상한 — 실제로는 더 길 수 있습니다)" if streak >= window else ""

print(f"연속 저장 {streak}일{capped} / 활동일 {sum(1 for c in counts if c > 0)}일 / 창 {window}일")
if streak >= target:
    print(f"PASS — {target}일 목표 달성.")
    sys.exit(0)
print(f"FAIL — {target}일까지 {target - streak}일 남았습니다.")
sys.exit(1)
PY
