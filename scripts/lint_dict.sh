#!/usr/bin/env bash
# Push-Point — 사전 자산 드리프트 검사 (just dict-lint)
# nlu/dictionary/tags.json이 마이그레이션 시드(0002 aliases + 0003 facet)와 일치하는지,
# nlu/dictionary/domains.json이 tags.json에 있는 태그만 참조하는지 검사한다.
# 런타임 사전은 DB tags 테이블(마이그레이션이 시드)이고 tags.json은 그 커밋된 미러이므로,
# 둘이 어긋나면(태그 추가 시 한쪽만 갱신) eval·문서가 실제와 달라진다 — 그걸 막는다.
# 마이그레이션은 불변이라 새 태그는 새 마이그레이션 + tags.json 동시 갱신으로 들어온다.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# domains.json 임베드 사본 드리프트 가드 — 런타임 태거는 backend 안 사본을 go:embed하고
# (nlu/는 backend Go 모듈 밖이라 cross-module embed 불가), 정본은 nlu/dictionary/다.
# 둘이 어긋나면 태거가 낡은 도메인맵을 쓴다. 바이트 동일해야 통과.
if ! diff -q "$ROOT/nlu/dictionary/domains.json" "$ROOT/backend/internal/tagger/domains.json" >/dev/null; then
  echo "dict-lint 실패: nlu/dictionary/domains.json 과 backend/internal/tagger/domains.json 이 다름 (사본 동기화 필요: cp nlu/dictionary/domains.json backend/internal/tagger/domains.json)"
  exit 1
fi

python3 - "$ROOT/nlu/dictionary" "$ROOT/backend/migrations" <<'PY'
import glob
import json
import os
import re
import sys

dict_dir, mig_dir = sys.argv[1], sys.argv[2]

tags = json.load(open(os.path.join(dict_dir, "tags.json"), encoding="utf-8"))
domains = json.load(open(os.path.join(dict_dir, "domains.json"), encoding="utf-8"))

# tags.json → {name: (facet, [aliases])}
dict_tags = {t["name"]: (t["facet"], t["aliases"]) for t in tags}

mig_text = ""
for p in sorted(glob.glob(os.path.join(mig_dir, "*.up.sql"))):
    mig_text += open(p, encoding="utf-8").read() + "\n"

# 마이그레이션을 **순서대로** 재생해 최종 aliases를 구한다.
#
# 두 모양을 다 본다:
#   INSERT INTO tags (name, aliases) VALUES ('name', '["a","b"]'), ...
#   UPDATE tags SET aliases = '["a","b"]' WHERE name = 'x';
#
# **UPDATE를 안 보던 것이 결함이었다.** 마이그레이션은 불변이라 별칭을 고치려면 새 파일에
# UPDATE를 쓸 수밖에 없는데, 그러면 이 검사가 0002의 옛 값만 보고 tags.json과 어긋났다고
# 판정했다. 즉 **별칭을 고치는 유일한 방법이 lint를 깨는 방법**이었다.
#
# 파일명 정렬 순서로 이어 붙인 텍스트를 한 번에 훑되 **매치 순서대로** 적용한다(나중 것이 이김).
# INSERT와 UPDATE를 따로 두 번 훑으면 나중 파일의 INSERT가 앞 파일의 UPDATE에 덮이는
# 뒤바뀜이 생긴다 — 지금은 그런 조합이 없지만 규칙이 순서를 지키는 편이 옳다.
seed_aliases = {}
_pat = re.compile(
    r"\(\s*'([^']+)'\s*,\s*'(\[[^']*\])'\s*\)"                      # INSERT 튜플
    r"|UPDATE\s+tags\s+SET\s+aliases\s*=\s*'(\[[^']*\])'\s*"        # UPDATE
    r"WHERE\s+name\s*=\s*'([^']+)'",
    re.I | re.S,
)
for m in _pat.finditer(mig_text):
    if m.group(1) is not None:
        seed_aliases[m.group(1)] = json.loads(m.group(2))
    else:
        seed_aliases[m.group(4)] = json.loads(m.group(3))

# 0003: UPDATE tags SET facet='X' WHERE name IN ('a','b',...);  (+ ALTER … DEFAULT 'neutral')
seed_facet = {}
for facet, names_blob in re.findall(
    r"UPDATE\s+tags\s+SET\s+facet\s*=\s*'(\w+)'\s+WHERE\s+name\s+IN\s*\(([^)]*)\)",
    mig_text, re.I | re.S,
):
    for n in re.findall(r"'([^']+)'", names_blob):
        seed_facet[n] = facet

errs = []

# 1) 이름 집합 일치
d_names, s_names = set(dict_tags), set(seed_aliases)
if d_names != s_names:
    if d_names - s_names:
        errs.append(f"tags.json에만 있는 태그: {sorted(d_names - s_names)}")
    if s_names - d_names:
        errs.append(f"마이그레이션 0002에만 있는 태그: {sorted(s_names - d_names)}")

# 2) 이름별 aliases·facet 일치
for name in sorted(d_names & s_names):
    facet, aliases = dict_tags[name]
    if aliases != seed_aliases[name]:
        errs.append(f"{name}: aliases 불일치\n    tags.json  {aliases}\n    0002       {seed_aliases[name]}")
    # 0003이 명시 배정 안 한 태그는 ALTER의 DEFAULT 'neutral'
    want_facet = seed_facet.get(name, "neutral")
    if facet != want_facet:
        errs.append(f"{name}: facet 불일치 — tags.json={facet!r} 0003={want_facet!r}")

# 3) domains.json은 tags.json 태그만 참조
for host, tag_list in domains.items():
    if host.startswith("_"):  # _comment 등 메타 키
        continue
    unknown = [t for t in tag_list if t not in dict_tags]
    if unknown:
        errs.append(f"domains.json '{host}': 사전에 없는 태그 {unknown}")

# 4) golden의 expected_tags도 사전 태그만 참조 — domains.json과 같은 종류의 불변식인데
#    golden만 검사 밖에 있었다. 라벨 오타는 **구조적으로 맞힐 수 없는 정답**이 되어
#    Recall을 떨어뜨리고, 태그별 표에서 P=0.00 R=0.00 행으로 나타나 "태거가 못 맞히는 태그"와
#    구분되지 않는다. 실측: wild 한 항목의 `video`를 `vidoe`로 바꾸면 Recall이 조용히 내려간다.
#
#    eval에도 같은 검사가 있지만(validateExpectedTags) `just eval`은 CI에서 돌지 않는다.
#    커밋을 막는 것은 여기다.
golden_dir = os.path.join(os.path.dirname(os.path.abspath(sys.argv[1])), "golden")

# **태깅 세트만 이 검사를 받는다.** golden 디렉터리에는 스키마가 다른 파일도 있다 —
# search.jsonl은 {query,url,why}라 expected_tags가 없고, URL이 태깅 세트와 겹치는 것이
# 정상이다(질의의 정답이 코퍼스 안에 있어야 하므로). 파일 이름으로 세트를 열거하지 않고
# glob으로 훑던 것이 결함이었다 — 새 스키마 파일을 넣는 순간 검사가 거짓 실패를 낸다.
TAGGING_SETS = ("dev", "test", "wild")
seen_urls = {}
for setname in TAGGING_SETS:
    path = os.path.join(golden_dir, setname + ".jsonl")
    if not os.path.exists(path):
        continue
    if os.path.getsize(path) == 0:
        errs.append(f"golden/{setname}.jsonl이 비어 있습니다 — 캡처 실패이거나 파일이 잘렸습니다")
        continue
    with open(path, encoding="utf-8") as fh:
        lines = fh.read().splitlines()
    for lineno, line in enumerate(lines, 1):
        if not line.strip():
            continue
        try:
            row = json.loads(line)
        except json.JSONDecodeError as exc:
            errs.append(f"golden/{setname}.jsonl:{lineno} JSON 파싱 실패: {exc}")
            continue
        tags = row.get("expected_tags") or []
        if not tags:
            errs.append(f"golden/{setname}.jsonl:{lineno} expected_tags가 비었습니다 — 자동 miss가 되어 분모만 키웁니다")
        unknown = [t for t in tags if t not in dict_tags]
        if unknown:
            errs.append(f"golden/{setname}.jsonl:{lineno} 사전에 없는 태그 {unknown}: {row.get('url','')}")
        if len(tags) != len(set(tags)):
            errs.append(f"golden/{setname}.jsonl:{lineno} expected_tags에 중복이 있습니다: {tags}")
        # 세트 간 URL 중복 = 데이터 누수. dev로 튜닝한 항목이 동결 test에 있으면 게이트가 무의미해진다.
        url = row.get("url", "")
        if url in seen_urls and seen_urls[url] != setname:
            errs.append(f"golden URL이 {seen_urls[url]}와 {setname}에 모두 있습니다(누수): {url}")
        seen_urls[url] = setname

# 5) search.jsonl은 자기 스키마로 검사한다 — query·url 필수, 정답 URL은 태깅 세트 안에 있어야 한다.
#    URL이 코퍼스에 없으면 **구조적으로 못 찾는 정답**이 되어 검색 실패처럼 보인다.
search_path = os.path.join(golden_dir, "search.jsonl")
n_queries = 0
if os.path.exists(search_path):
    with open(search_path, encoding="utf-8") as fh:
        for lineno, line in enumerate(fh.read().splitlines(), 1):
            if not line.strip():
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError as exc:
                errs.append(f"golden/search.jsonl:{lineno} JSON 파싱 실패: {exc}")
                continue
            n_queries += 1
            if not row.get("query") or not row.get("url"):
                errs.append(f"golden/search.jsonl:{lineno} query와 url이 모두 있어야 합니다")
            elif row["url"] not in seen_urls:
                errs.append(f"golden/search.jsonl:{lineno} 정답 URL이 태깅 세트에 없습니다: {row['url']}")

if errs:
    print("dict-lint 실패:")
    for e in errs:
        print(f"  - {e}")
    sys.exit(1)

print(f"dict-lint OK — 태그 {len(dict_tags)}개, 도메인 {sum(1 for h in domains if not h.startswith('_'))}개, golden {len(seen_urls)}건, 검색질의 {n_queries}개, 시드와 일치")
PY
