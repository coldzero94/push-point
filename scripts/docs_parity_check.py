#!/usr/bin/env python3
"""ko·en 문서 두 벌이 갈라지지 않는지 본다.

**이 검사가 못 하는 일부터 적는다.** 본문이 다른 말을 하는 것은 못 막는다 — 한국어 문단이
"30일"이라 하고 영어 문단이 "60 days"라 해도, 같은 자리에 같은 개수의 문단으로 있으면
통과한다. 번역의 뜻은 사람이 본다.

막을 수 있는 것은 **구조와 숫자**다. 이 저장소의 갈라짐은 대개 거기서 시작한다: 한쪽에만
섹션을 더하고, 한쪽 표만 고치고, 한쪽 수치만 갱신한다.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
KO = ROOT / "docs" / "v2" / "ko"
EN = ROOT / "docs" / "v2" / "en"
fail = []


def headings(t):
    return [len(m.group(1)) for m in re.finditer(r"^(#{1,6}) ", t, re.M)]


def tables(t):
    out, rows, cols = [], 0, 0
    for line in t.splitlines():
        st = line.strip()
        if st.startswith("|") and st.endswith("|"):
            rows += 1
            cols = max(cols, line.count("|") - 1)
        elif rows:
            out.append((rows, cols))
            rows, cols = 0, 0
    if rows:
        out.append((rows, cols))
    return out


def code_blocks(t):
    return re.findall(r"```[a-z]*\n(.*?)```", t, re.S)


# **측정값만 본다.** 처음에는 모든 숫자를 다중집합으로 비교했는데, 산문에서 정당하게
# 갈리는 것이 너무 많았다 — `5`가 28회 대 26회, `0`이 13회 대 7회. "4주(28일)"과
# "four weeks"는 같은 말이고 숫자 개수는 다르다. 거짓 경보를 내는 게이트는 꺼지게 되고,
# 꺼진 게이트는 없는 것과 같다(.claude/rules/verification.md).
#
# 그래서 **소수와 기계 단위가 붙은 수**만, 그것도 집합으로 센다. 0.905, 50ms, 106MiB —
# 한쪽만 갱신되면 조용히 틀리는 수치들의 모양이다.
#
# 큰 정수(250000)를 뺀 이유: `10만`과 `100k`는 같은 값의 다른 표기이고, 그걸 맞추라고
# 요구하면 문서가 자기 언어로 읽히지 않게 된다. 개수가 아니라 **집합**으로 비교하는 이유도
# 같다 — 같은 사실을 한 언어가 두 번 말하고 다른 언어가 한 번 말하는 것은 번역이지 갈라짐이 아니다.
# 한국어 단위(건·개·일…)는 **빼 놓았다.** 영어는 "3 links"라 쓰고 한국어는 "3건"이라
# 쓰므로 같은 사실이 한쪽에서만 단위로 잡히고, 그러면 매 문서가 빨개진다.
# 양쪽이 같은 글자로 쓰는 기계 단위만 센다.
UNIT = r"(?:ms|MB|MiB|GB|KB)"
MEASURE = re.compile(
    # 뒤의 `.`는 **숫자가 따라올 때만** 배제한다. `(?![\d.])`로 두면 문장 끝의
    # `… 0.23.`에서 소수를 통째로 놓친다 — 영어에서만 생기는 일이고(한국어는 조사가
    # 붙는다) 그래서 한쪽만 조용히 사라진다. 검사기를 만든 목적 그 자체가 이런 결함이다.
    r"(?<![\d.])(\d+\.\d+)(?!\d)(?!\.\d)"      # 소수: 0.905, 문장 끝의 0.23.도
    # `\b`를 쓰면 안 된다 — 한국어는 `37ms로`처럼 단위 뒤에 조사가 바로 붙고 한글은
    # 단어 문자라 경계가 서지 않아, 같은 측정값이 한쪽에서만 잡힌다(실제로 그랬다).
    # 뒤에 오면 안 되는 것은 영문자·숫자뿐이다.
    r"|(?<![\d.])(\d+)\s*" + UNIT + r"(?![A-Za-z0-9])"   # 50ms, 106MiB
)


def numbers(t):
    t = re.sub(r"```.*?```", "", t, flags=re.S)      # 코드 블록은 따로 비교한다
    t = re.sub(r"\d{4}-\d{2}-\d{2}", "", t)        # 날짜
    t = re.sub(r"\[[^\]]*\]\([^)]*\)", "", t)      # 링크(문서 번호가 섞인다)
    out = []
    for m in MEASURE.finditer(t):
        out.append(next(g for g in m.groups() if g))
    return sorted(set(out))


if not KO.is_dir() or not EN.is_dir():
    print(f"docs/v2/ko 와 docs/v2/en 이 둘 다 있어야 한다 (ko={KO.is_dir()} en={EN.is_dir()})")
    sys.exit(1)

ko_files = {p.name for p in KO.glob("*.md")}
en_files = {p.name for p in EN.glob("*.md")}
if ko_files - en_files:
    fail.append(f"en에 없는 문서: {', '.join(sorted(ko_files - en_files))}")
if en_files - ko_files:
    fail.append(f"ko에 없는 문서: {', '.join(sorted(en_files - ko_files))}")

for name in sorted(ko_files & en_files):
    k = (KO / name).read_text(encoding="utf-8")
    e = (EN / name).read_text(encoding="utf-8")

    if headings(k) != headings(e):
        fail.append(f"{name}: 헤딩 구조가 다르다 — ko {len(headings(k))}개, en {len(headings(e))}개")
    if tables(k) != tables(e):
        fail.append(f"{name}: 표가 다르다 — ko {tables(k)}, en {tables(e)}")

    ck, ce = code_blocks(k), code_blocks(e)
    if len(ck) != len(ce):
        fail.append(f"{name}: 코드 블록 개수가 다르다 — ko {len(ck)}, en {len(ce)}")
    else:
        for i, (a, b) in enumerate(zip(ck, ce)):
            if a.strip() != b.strip():
                fail.append(f"{name}: 코드 블록 #{i + 1}이 다르다 — 명령·DDL은 번역 대상이 아니다")

    # 같은 값을 다른 단위로 쓰면 여기서 걸린다 — 한국어 `25만`을 영어가 `250k`로 쓰면
    # 25와 250이 되어 어긋난다. 그건 파서 결함이 아니라 **두 문서가 다른 숫자를 보여주고
    # 있다**는 사실이고, 독자가 대조할 수 없게 되므로 걸리는 게 맞다(2026-08-05 실제 사례).
    nk, ne = numbers(k), numbers(e)
    if nk != ne:
        fail.append(f"{name}: 숫자가 다르다 — ko만 {[x for x in nk if x not in ne][:6]}, "
                    f"en만 {[x for x in ne if x not in nk][:6]}")

# 상대 링크가 실제로 가리키는 곳이 있는지. **문서를 옮길 때마다 깨진다** — ko/·en/으로
# 한 단계 깊어지면서 `../../api/openapi.yaml`이 전부 어긋났고(2026-08-05, 12개), 그건
# 사람이 클릭하기 전까지 아무 증상이 없다.
LINK = re.compile(r"\]\((?!http|#)([^)#]+)\)")
for p in sorted(list(KO.glob("*.md")) + list(EN.glob("*.md")) + [ROOT / "README.md", ROOT / "docs" / "README.md"]):
    if not p.exists():
        continue
    for link in LINK.findall(p.read_text(encoding="utf-8")):
        if not (p.parent / link).resolve().exists():
            fail.append(f"{p.relative_to(ROOT)}: 깨진 링크 {link}")

if fail:
    print("문서 이중 언어 대조 실패")
    for f in fail:
        print(f"  - {f}")
    print("\n본문의 뜻은 이 검사가 못 본다 — 구조·표·코드·숫자만 본다.")
    sys.exit(1)

print(f"docs-parity OK — 문서 {len(ko_files)}쌍의 구조·표·코드·숫자가 일치한다")
