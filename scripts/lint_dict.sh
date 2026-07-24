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

# 0002: INSERT INTO tags ... VALUES ('name', '["a","b"]'), ...
seed_aliases = {}
for name, aliases_json in re.findall(r"\(\s*'([^']+)'\s*,\s*'(\[[^']*\])'\s*\)", mig_text):
    seed_aliases[name] = json.loads(aliases_json)

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

if errs:
    print("dict-lint 실패:")
    for e in errs:
        print(f"  - {e}")
    sys.exit(1)

print(f"dict-lint OK — 태그 {len(dict_tags)}개, 도메인 {sum(1 for h in domains if not h.startswith('_'))}개, 시드와 일치")
PY
