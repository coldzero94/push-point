#!/usr/bin/env python3
"""iOS UI 문자열이 두 로케일에서 갈라지지 않는지 본다.

`site_copy_check.py`와 같은 일을 앱 쪽에 한다. 검사 셋:
  1. ko·en의 키 집합이 완전히 같은가
  2. 소스가 `t('...')`로 부르는 키가 사전에 실재하는가 (없으면 화면에 키가 그대로 뜬다)
  3. 사전에 있는데 아무도 안 부르는 키가 없는가 (죽은 문구가 쌓이는 것)
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
SRC = ROOT / "ios"
DICT = ROOT / "ios" / "Shared" / "Strings.swift"

raw = DICT.read_text(encoding="utf-8")


def block(name: str) -> str:
    i = raw.index(f'"{name}": [')
    depth, start = 0, raw.index("[", i)
    for j in range(start, len(raw)):
        if raw[j] == "[":
            depth += 1
        elif raw[j] == "]":
            depth -= 1
            if depth == 0:
                return raw[start : j + 1]
    raise SystemExit(f"{name} 블록이 닫히지 않았다")


ENTRY = re.compile(r'^\s*"([A-Za-z0-9._]+)":', re.M)
ko = set(ENTRY.findall(block("ko")))
en = set(ENTRY.findall(block("en")))

# 주석은 걷어낸다. `t('...')` 같은 **설명**이 호출로 잡히면 있지도 않은 키를 찾게 된다
# (실제로 lib/time.ts의 주석이 그렇게 잡혔다).
#
# **`//`를 아무 데서나 지우면 안 된다.** 처음에 그렇게 했더니 정규식 리터럴
# `/^https?:\/\//i` 안의 `//`가 주석 시작으로 잡혀 같은 줄 뒤의 `t('save.urlScheme')`이
# 통째로 사라졌고, 멀쩡히 쓰이는 키가 "아무도 안 쓰는 키"로 보고됐다. 이 코드베이스는
# 주석을 줄 단위로 쓰므로, **줄 전체가 주석인 줄만** 버린다.
def strip_comments(text: str) -> str:
    out = []
    for line in text.splitlines():
        st = line.lstrip()
        if st.startswith(("//", "///", "/*", "*")):
            continue
        out.append(line)
    return "\n".join(out)

CALL = re.compile(r'\bt\(\s*"([A-Za-z0-9._]+)"')
# 키를 **데이터로** 들고 있다가 렌더 시점에 넘기는 자리가 있다 — 단축키 표가 그렇다
# (`{ action: 'shortcuts.actionPalette' }`). 그래서 사전 키와 **정확히 같은** 문자열
# 리터럴도 사용으로 친다. 점 찍힌 이름공간이라 우연히 겹칠 여지는 사실상 없고, 이렇게
# 하지 않으면 정당한 패턴이 "아무도 안 쓰는 키"로 잡혀 검사를 끄게 만든다.
LITERAL = re.compile(r'"([A-Za-z0-9._]+)"')

used = set()
for f in SRC.rglob("*.swift"):
    if f.name in ("Strings.swift", "Localized.swift"):
        continue
    body = strip_comments(f.read_text(encoding="utf-8"))
    used |= set(CALL.findall(body))
    used |= {m for m in LITERAL.findall(body) if m in ko}

fail = []
if ko - en:
    fail.append(f"en에 없는 키 {len(ko - en)}개: {', '.join(sorted(ko - en)[:12])}")
if en - ko:
    fail.append(f"ko에 없는 키 {len(en - ko)}개: {', '.join(sorted(en - ko)[:12])}")
if used - ko:
    fail.append(f"소스가 쓰는데 사전에 없는 키 {len(used - ko)}개: {', '.join(sorted(used - ko)[:12])}")
if ko - used:
    fail.append(f"사전에 있는데 아무도 안 쓰는 키 {len(ko - used)}개: {', '.join(sorted(ko - used)[:12])}")

if fail:
    print("iOS i18n 검사 실패")
    for f in fail:
        print(f"  - {f}")
    sys.exit(1)
print(f"ios-i18n-check OK — 키 {len(ko)}개가 ko·en 양쪽에 있고, 소스가 {len(used)}개를 쓴다")
