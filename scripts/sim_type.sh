#!/usr/bin/env bash
# 시뮬레이터의 포커스된 텍스트 필드에 글자를 넣는다 — 키보드를 거치지 않고.
#
# 왜 이게 필요한가. `axe type`은 **US HID 키코드**를 쏘는 것이 전부다(`axe type --help`가
# 그렇게 적어 놓았다: "Only US keyboard characters are supported via HID keycodes").
# 그래서 시뮬레이터의 하드웨어 입력 모드가 한글이면 영문 "Hello demo"가 `ㅗㄷ| |ㅐ ㅇㄷㅔ`로
# 들어가고, 한글 문자열은 대응하는 키코드가 없어 **아무 일도 일어나지 않는다.**
# `AppleKeyboards`를 다시 쓰고 시뮬레이터를 재부팅해도 이건 안 바뀐다 — 자판 설정의
# 문제가 아니라 전송 계층의 문제이기 때문이다.
#
# 대신 페이스트보드로 우회한다. `xcrun simctl pbcopy`로 기기 클립보드에 넣고,
# 붙여넣기를 시킨다. 붙여넣기는 IME를 안 거치므로 한글·영문·이모지·혼합 문자열이
# 전부 그대로 들어간다.
#
# 모드
#   paste  (기본) Cmd+V HID 조합. 즉시 들어간다. 화면에는 아무 동작도 안 보인다.
#   menu   길게 눌러 편집 메뉴를 띄우고 "붙여넣기"를 탭한다. **데모 촬영용** —
#          손가락이 실제로 뭔가를 누르는 장면이 남는다.
#   stream 한 글자씩 pbcopy+Cmd+V. 타이핑처럼 글자가 차례로 나타난다(실측 0.32 s/자).
#          **한글 전용이다.** iOS의 smart insert가 붙여넣는 조각을 "단어"로 보고 앞에
#          공백을 끼워 넣으며, 진짜 공백 문자는 붙여넣을 때 잘라 낸다. 그래서 라틴
#          문자열은 `Read this later`가 `R e a d t h i s l a t e r`로 들어간다(실측).
#          한글은 음절 사이에 공백을 끼우지 않아 그대로 들어간다.
#   stream-exact
#          매 글자마다 Cmd+A로 전체를 고르고 "여기까지의 앞부분" 전체를 붙여넣는다.
#          라틴 문자도 정확하다(실측 `Read this later — 팀 공유 ✅` 그대로). 대신
#          **선택 하이라이트와 초록 잡이가 프레임에 찍힌다** — 80프레임 녹화 중 2장에서
#          확인했다. 화면에 안 나올 때만 써라.
#
# 전제: 대상 필드가 이미 first responder여야 한다. --tap X,Y를 주면 먼저 탭해서 포커스한다.
# 좌표는 **포인트**(402x874)다. 스크린샷 픽셀(1206x2622)이 아니다 — 3으로 나눠라.
#
# 사용 예
#   scripts/sim_type.sh --tap 201,608 '나중에 팀에 공유할 것'
#   scripts/sim_type.sh --mode menu --tap 201,608 '주말에 정리해서 팀 위키로'
#   scripts/sim_type.sh --mode stream-exact --delay 0.05 'Read this later — 팀 공유'

set -euo pipefail

UDID=""
MODE="paste"
TAP=""
DELAY="0"
TEXT=""

usage() {
  sed -n '2,36p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --udid)  UDID="$2"; shift 2 ;;
    --mode)  MODE="$2"; shift 2 ;;
    --tap)   TAP="$2"; shift 2 ;;
    --delay) DELAY="$2"; shift 2 ;;
    -h|--help) usage ;;
    --) shift; TEXT="$*"; break ;;
    -*) echo "unknown option: $1" >&2; usage ;;
    *)  TEXT="$1"; shift ;;
  esac
done

[ -n "$TEXT" ] || { echo "error: no text given" >&2; usage; }

if [ -z "$UDID" ]; then
  UDID=$(xcrun simctl list devices booted -j \
    | python3 -c 'import json,sys
d=json.load(sys.stdin)["devices"]
u=[x["udid"] for v in d.values() for x in v if x.get("state")=="Booted"]
print(u[0] if u else "")')
fi
[ -n "$UDID" ] || { echo "error: no booted simulator" >&2; exit 1; }

# Cmd+V. 227 = Left GUI(Command), 25 = v.
paste_combo() { axe key-combo --modifiers 227 --key 25 --udid "$UDID" >/dev/null; }
copy() { printf '%s' "$1" | xcrun simctl pbcopy "$UDID"; }

if [ -n "$TAP" ]; then
  axe tap -x "${TAP%%,*}" -y "${TAP##*,}" --udid "$UDID" >/dev/null
  sleep 1
fi

case "$MODE" in
  paste)
    copy "$TEXT"; sleep 0.4
    paste_combo
    ;;

  stream|stream-exact)
    case "$TEXT" in
      *[A-Za-z]*)
        [ "$MODE" = "stream" ] && echo "warning: stream 모드는 라틴 문자를 뭉갠다 (iOS smart insert). stream-exact 또는 paste를 써라." >&2 ;;
    esac
    # 파이썬으로 도는 이유는 문자 단위 순회 때문이다 — 한글은 바이트가 아니라 문자로 잘라야 한다.
    python3 - "$UDID" "$DELAY" "$MODE" "$TEXT" <<'PY'
import subprocess, sys, time
udid, delay, mode, text = sys.argv[1], float(sys.argv[2]), sys.argv[3], sys.argv[4]

def key(*args):
    subprocess.run(["axe", *args, "--udid", udid], capture_output=True, check=True)

for i, ch in enumerate(text, 1):
    chunk = text[:i] if mode == "stream-exact" else ch
    subprocess.run(["xcrun", "simctl", "pbcopy", udid], input=chunk.encode(), check=True)
    if mode == "stream-exact":
        key("key-combo", "--modifiers", "227", "--key", "4")   # Cmd+A
    key("key-combo", "--modifiers", "227", "--key", "25")      # Cmd+V
    if delay:
        time.sleep(delay)
PY
    ;;

  menu)
    [ -n "$TAP" ] || { echo "error: --mode menu needs --tap X,Y (길게 누를 지점)" >&2; exit 1; }
    copy "$TEXT"; sleep 0.4
    axe touch -x "${TAP%%,*}" -y "${TAP##*,}" --down --up --delay 1.0 --udid "$UDID" >/dev/null
    sleep 1.5
    # 편집 메뉴는 캐럿 위치를 따라다닌다 — 좌표를 고정하면 다음 실행에서 빗나간다.
    # 그래서 매번 계층에서 "붙여넣기"/"Paste"를 찾아 그 중심을 누른다.
    XY=$(maestro --device "$UDID" hierarchy 2>/dev/null | python3 -c '
import json, re, sys
raw = sys.stdin.read(); d = json.loads(raw[raw.find("{"):])
hit = []
def walk(n):
    a = n.get("attributes", {})
    t = a.get("accessibilityText", "") or a.get("text", "")
    if t in ("붙여넣기", "Paste"):
        m = re.match(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]", a.get("bounds", ""))
        if m:
            x1, y1, x2, y2 = map(int, m.groups())
            hit.append(((x1 + x2) // 2, (y1 + y2) // 2))
    for c in n.get("children") or []:
        walk(c)
walk(d)
print("%d %d" % hit[0] if hit else "")')
    [ -n "$XY" ] || { echo "error: 편집 메뉴에서 붙여넣기를 못 찾았다 (메뉴가 안 떴을 수 있다)" >&2; exit 1; }
    axe tap -x "${XY%% *}" -y "${XY##* }" --udid "$UDID" >/dev/null
    ;;

  *) echo "unknown mode: $MODE" >&2; usage ;;
esac

sleep 0.5
echo "typed into ${UDID} (mode=${MODE}): ${TEXT}"
