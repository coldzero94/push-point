#!/usr/bin/env bash
# M4 DoD 판정 — "공유 탭 → 응답 2초 미만"을 수치로 답한다.
#
# 계획(08 §4)이 M4 검증 커맨드로 "시뮬레이터 공유 절차 + 클라이언트 계측 로그"를 지정했다.
# 절차는 사람이 밟고(공유 시트에서 저장), 판정은 이 스크립트가 한다.
#
# 데이터는 Share Extension이 App Group에 쌓는 save-timing.jsonl이다
# (ios/PushPointShare/SaveTiming.swift). 한 줄이 저장 한 건이고, 실패한 저장도 들어 있다 —
# 실패가 느린 것은 성공이 느린 것과 다른 문제라서 섞으면 평균만 좋아 보인다.
#
# 사용법:
#   scripts/save_timing.sh              # 부팅된 시뮬레이터
#   scripts/save_timing.sh <UDID>
#
# 초과 건이 하나라도 있으면 exit 1 — 게이트로 쓸 수 있게.
set -euo pipefail

BUNDLE_ID="com.pushpoint.app"
BUDGET_MS=2000

udid="${1:-}"
if [ -z "$udid" ]; then
  udid=$(xcrun simctl list devices booted -j | python3 -c '
import json,sys
d = json.load(sys.stdin)["devices"]
b = [x["udid"] for v in d.values() for x in v if x["state"] == "Booted"]
print(b[0] if b else "")')
fi
if [ -z "$udid" ]; then
  echo "부팅된 시뮬레이터가 없습니다. just ios-run 후 다시 실행하세요."
  exit 1
fi

# App Group 컨테이너 경로는 실행마다 UUID가 달라 하드코딩할 수 없다.
group_line=$(xcrun simctl get_app_container "$udid" "$BUNDLE_ID" groups 2>/dev/null | head -1 || true)
if [ -z "$group_line" ]; then
  echo "App Group 컨테이너를 찾지 못했습니다 — 앱이 설치돼 있나요? (just ios-run)"
  exit 1
fi
container=$(printf '%s' "$group_line" | awk -F'\t' '{print $NF}')
file="$container/data/save-timing.jsonl"

if [ ! -f "$file" ]; then
  echo "계측 기록이 없습니다: $file"
  echo "공유 시트로 한 번 이상 저장한 뒤 다시 실행하세요 (기록은 확장이 남깁니다)."
  exit 1
fi

BUDGET_MS="$BUDGET_MS" python3 - "$file" <<'PY'
import json, os, sys

budget = float(os.environ["BUDGET_MS"])
rows = []
for line in open(sys.argv[1]):
    line = line.strip()
    if not line:
        continue
    try:
        rows.append(json.loads(line))
    except json.JSONDecodeError:
        # 확장이 죽는 순간 반쯤 쓰인 줄이 남을 수 있다. 그 한 줄 때문에 판정을
        # 통째로 포기하는 것보다 세고 넘어가는 편이 낫다.
        rows.append(None)

broken = rows.count(None)
rows = [r for r in rows if r]
if not rows:
    print("읽을 수 있는 기록이 없습니다.")
    sys.exit(1)


def pct(values, p):
    """작은 표본에서 보간은 없는 정밀도를 주장한다 — 가장 가까운 순위값을 쓴다."""
    s = sorted(values)
    i = min(len(s) - 1, max(0, round((p / 100) * len(s) + 0.5) - 1))
    return s[i]


def report(label, subset):
    if not subset:
        return
    ms = [r["ms"] for r in subset]
    over = [r for r in subset if r["ms"] > budget]
    print(f"  {label:10} n={len(subset):3}  p50={pct(ms,50):7.1f}ms  p95={pct(ms,95):7.1f}ms  "
          f"max={max(ms):7.1f}ms  초과={len(over)}")


saved = [r for r in rows if r.get("outcome") in ("saved", "duplicate")]
failed = [r for r in rows if r.get("outcome") == "failed"]

print(f"저장 계측 — 예산 {budget:.0f}ms (M4 DoD)")
report("성공", saved)
report("실패", failed)
if broken:
    print(f"  (깨진 줄 {broken}개 무시)")

over = [r for r in rows if r["ms"] > budget]
if over:
    print()
    print(f"예산 초과 {len(over)}건:")
    for r in over[-5:]:
        print(f"    {r.get('at','?')}  {r['ms']:.1f}ms  {r.get('outcome')}  tags={r.get('tags')}")
    print()
    print("FAIL — 2초를 넘긴 저장이 있습니다.")
    sys.exit(1)

print()
print(f"PASS — {len(rows)}건 전부 {budget:.0f}ms 이내.")
PY
