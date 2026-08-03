#!/usr/bin/env python3
"""site/ 문구가 두 언어에서 갈라지지 않는지 본다.

두 벌을 손으로 관리하면 갈라진다 — 이 저장소는 같은 모양의 실수로 이미 세 번 다쳤다
(streak 규칙, 커버 패턴, 손으로 옮겨 적은 iOS 골든). 전부 "양쪽 다 정상으로 보이는데
값이 다른" 형태였다.

그래서 검사하는 것 셋:
  1. en·ko의 키 집합이 완전히 같은가 (번역 누락·잉여)
  2. index.html의 data-t가 전부 실재하는 키인가 (오타가 화면을 비운다)
  3. 두 언어의 문구가 서로 다른가 (번역을 안 하고 영어를 복사해 둔 자리 찾기)
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
COPY = ROOT / "site" / "copy.js"
HTML = ROOT / "site" / "index.html"

# copy.js는 ESM이라 파이썬이 실행할 수 없다. 키는 `'a.b': '...'` 형태의 리터럴뿐이므로
# 로케일 블록을 잘라 키만 뽑는다 — 값을 해석하려 들지 않는 것이 요점이다.
src = COPY.read_text(encoding="utf-8")


def locale_block(name: str) -> str:
    start = src.index(f"\n  {name}: {{")
    depth, i = 0, src.index("{", start)
    for j in range(i, len(src)):
        if src[j] == "{":
            depth += 1
        elif src[j] == "}":
            depth -= 1
            if depth == 0:
                return src[i : j + 1]
    raise SystemExit(f"{name} 블록이 닫히지 않았다")


ENTRY = re.compile(r"^\s*'([a-z0-9.]+)':\s*(.+?),?\s*$", re.M)


def entries(block: str) -> dict[str, str]:
    out = {}
    for m in ENTRY.finditer(block):
        out[m.group(1)] = m.group(2)
    return out


en, ko = entries(locale_block("en")), entries(locale_block("ko"))
fail = []

only_en = sorted(set(en) - set(ko))
only_ko = sorted(set(ko) - set(en))
if only_en:
    fail.append(f"ko에 없는 키 {len(only_en)}개: {', '.join(only_en)}")
if only_ko:
    fail.append(f"en에 없는 키 {len(only_ko)}개: {', '.join(only_ko)}")

used = sorted(set(re.findall(r'data-t="([a-z0-9.]+)"', HTML.read_text(encoding="utf-8"))))
missing = [k for k in used if k not in en]
if missing:
    fail.append(f"index.html이 쓰는데 copy.js에 없는 키: {', '.join(missing)}")

unused = sorted(k for k in en if k not in used and not k.startswith(("meta.", "html.")))
if unused:
    fail.append(f"copy.js에 있는데 아무도 안 쓰는 키: {', '.join(unused)}")

# 번역이 안 된 자리. 숫자·명사(Apache-2.0, GitHub, 1.96 ms 등)는 같아야 정상이므로
# 한글이 한 글자도 없고 값이 en과 똑같은 항목만 짚는다.
HANGUL = re.compile(r"[가-힣]")
same = [
    k
    for k in en
    if k in ko and en[k] == ko[k] and not HANGUL.search(ko[k]) and len(ko[k]) > 30
]
if same:
    fail.append(f"ko가 en을 그대로 복사한 것으로 보이는 키: {', '.join(same)}")

# 화면 사진도 언어를 따라간다. `data-asset="web-list"`는 런타임에
# `assets/web-list-{lang}.png`가 되므로, 한쪽 언어의 파일이 없으면 **그 언어에서만**
# 이미지가 깨진다 — 영어로 보지 않으면 끝까지 모른다.
ASSETS = ROOT / "site" / "assets"
for name in sorted(set(re.findall(r'data-asset="([a-z-]+)"', HTML.read_text(encoding="utf-8")))):
    for lang in ("ko", "en"):
        if not (ASSETS / f"{name}-{lang}.png").exists():
            fail.append(f"{name}-{lang}.png 가 없다 (index.html의 data-asset이 참조한다)")
# 데모 영상은 지금 없다(잘못 찍혀 내렸다). 다시 넣을 때 이 검사를 되살린다.

# README도 같은 자산을 가리킨다. 로케일 접미사를 붙이면서 `demo.gif`가 사라졌는데
# README는 그대로였다 — GitHub 첫 화면의 깨진 이미지는 아무도 대신 알려주지 않는다.
import re as _re
readme = (ROOT / "README.md").read_text(encoding="utf-8")
for ref in sorted(set(_re.findall(r'(?:src|poster)="(site/assets/[^"]+)"', readme))):
    if not (ROOT / ref).exists():
        fail.append(f"README가 가리키는 {ref} 가 없다")

if fail:
    print("site 문구 검사 실패")
    for f in fail:
        print(f"  - {f}")
    sys.exit(1)

print(f"site-copy-check OK — 키 {len(en)}개가 en·ko 양쪽에 있고, index.html이 {len(used)}개를 쓴다")
