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

# App Group이 안 열리면(무료 Personal Team에서 entitlement가 거부될 수 있다) 확장은 자기
# 컨테이너에 남긴다. 저장이 못 되는 기기에서도 메모리·시간 수치는 건지자는 설계라
# (ios/PushPointShare/SaveTiming.swift), 읽는 쪽도 두 자리를 다 본다.
if [ ! -f "$file" ]; then
  data_root=$(xcrun simctl get_app_container "$udid" "$BUNDLE_ID" data 2>/dev/null || true)
  if [ -n "$data_root" ]; then
    alt=$(find "$data_root/Library/Application Support/pushpoint" -name save-timing.jsonl 2>/dev/null | head -1 || true)
    [ -n "$alt" ] && file="$alt"
  fi
fi

if [ ! -f "$file" ]; then
  echo "계측 기록이 없습니다."
  echo "  App Group:  $container/data/save-timing.jsonl"
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

# App Group이 열렸는가 — 무료 Personal Team에서 가장 알고 싶은 한 가지다.
# 필드가 없는 옛 기록은 판정에서 뺀다(그때는 App Group만 있었다).
flags = [r["app_group"] for r in rows if "app_group" in r]
if flags and not all(flags):
    n = sum(1 for f in flags if not f)
    print(f"  ⚠ App Group이 열리지 않은 저장 {n}건 — 확장 자체 컨테이너에 기록됨.")
    print("    무료 프로비저닝에서 App Group entitlement가 거부되면 앱과 확장이 다른 DB를")
    print("    보므로 저장이 목록에 나타나지 않는다. 그 경우 $99가 실제로 필요하다.")

# 확장 메모리 — 실기기에서만 의미가 있다(시뮬레이터는 0).
mem = [r["mem_avail_mb"] for r in rows if r.get("mem_avail_mb", 0) > 0]
if mem:
    print(f"  확장 잔여 메모리: 최소 {min(mem)}MB / 최대 {max(mem)}MB  ← 실기기 실측")
elif any("mem_avail_mb" in r for r in rows):
    print("  확장 잔여 메모리: 미측정 (시뮬레이터는 0을 준다 — 실기기에서만 값이 나온다)")

# 실패한 저장은 **빠르다** — 예외가 나면 그 자리에서 바로 기록되므로, 실패가 늘수록
# p50/p95가 좋아진다. 시간만 보면 "전부 2초 이내 PASS"가 되는데 정작 저장은 한 건도
# 안 됐을 수 있다. M4 통과 조건은 "2초 미만 **그리고** 저장 성공"이다.
#
# "실패 1건이라도 FAIL"로 만들지는 않는다 — 기록이 append-only라 과거의 실패 한 번이
# 영구 red를 만든다. 성공이 0건일 때만 막는다.
if not saved:
    print()
    print(f"FAIL — 성공한 저장이 0건입니다 (실패 {len(failed)}건).")
    sys.exit(1)

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
