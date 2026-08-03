#!/usr/bin/env python3
"""웹과 iOS가 **같은 키를 같은 문장으로** 말하는지 본다.

각 클라이언트 안의 ko·en 대칭은 `web_i18n_check.py`·`ios_i18n_check.py`가 이미 본다.
여기는 그 둘 사이를 본다 — 두 클라이언트가 같은 키에 다른 말을 넣는 것이 이 저장소가
반복해서 당한 갈라짐이고(streak 규칙, 커버 패턴, 손으로 옮겨 적은 iOS 골든), 매번
**양쪽 다 정상으로 보이는데 값이 달랐다.**

플랫폼 때문에 갈라져야 하는 것만 아래 허용 목록에 둔다. 목록에 넣으려면 이유를 적어야
하고, 이유가 적히면 리뷰에서 읽힌다 — 조용히 갈라지는 것과의 차이가 그것이다.
"""
import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

# 갈라져도 되는 키와 그 이유. 여기 없는 차이는 전부 실패다.
ALLOWED = {
    "rhythm.collectedHint": "마우스는 click, 손가락은 tap — 같은 동작의 플랫폼별 이름이다",
}


def web_tables():
    raw = (ROOT / "frontend/src/lib/strings.ts").read_text(encoding="utf-8")

    def block(name):
        i = raw.index(f"\n  {name}: {{")
        depth, start = 0, raw.index("{", i)
        for j in range(start, len(raw)):
            if raw[j] == "{":
                depth += 1
            elif raw[j] == "}":
                depth -= 1
                if depth == 0:
                    return raw[start : j + 1]
        raise SystemExit(f"web {name} 블록이 닫히지 않았다")

    entry = re.compile(r"^\s*'([A-Za-z0-9._]+)': '(.*)',$", re.M)
    unesc = lambda s: s.replace("\\'", "'").replace("\\\\", "\\")
    return ({k: unesc(v) for k, v in entry.findall(block("ko"))},
            {k: unesc(v) for k, v in entry.findall(block("en"))})


def ios_tables():
    raw = (ROOT / "ios/Shared/Strings.swift").read_text(encoding="utf-8")

    def block(name):
        i = raw.index(f'"{name}": [')
        depth, start = 0, raw.index("[", i)
        for j in range(start, len(raw)):
            if raw[j] == "[":
                depth += 1
            elif raw[j] == "]":
                depth -= 1
                if depth == 0:
                    return raw[start : j + 1]
        raise SystemExit(f"ios {name} 블록이 닫히지 않았다")

    entry = re.compile(r'^\s*"([A-Za-z0-9._]+)": "(.*)",$', re.M)
    unesc = lambda s: s.replace('\\"', '"').replace("\\\\", "\\")
    return ({k: unesc(v) for k, v in entry.findall(block("ko"))},
            {k: unesc(v) for k, v in entry.findall(block("en"))})


wko, wen = web_tables()
iko, ien = ios_tables()
shared = sorted(set(wko) & set(iko))

fail = []
for k in shared:
    if k in ALLOWED:
        continue
    if wko[k] != iko[k]:
        fail.append(f"{k} — 한국어가 다르다\n      web {wko[k]!r}\n      ios {iko[k]!r}")
    if wen[k] != ien[k]:
        fail.append(f"{k} — 영어가 다르다\n      web {wen[k]!r}\n      ios {ien[k]!r}")

# 허용 목록이 낡는 것도 막는다 — 값이 같아졌으면 목록에서 빼야 한다.
for k, why in ALLOWED.items():
    if k not in shared:
        fail.append(f"{k} — 허용 목록에 있지만 두 클라이언트가 함께 쓰지 않는 키다")
    elif wko[k] == iko[k] and wen[k] == ien[k]:
        fail.append(f"{k} — 값이 같아졌으니 허용 목록에서 빼라 ({why})")

if fail:
    print("클라이언트 간 문구 대조 실패")
    for f in fail:
        print(f"  - {f}")
    sys.exit(1)

print(f"i18n-cross-check OK — 함께 쓰는 키 {len(shared)}개가 일치한다 "
      f"(허용된 플랫폼 차이 {len(ALLOWED)}건)")
