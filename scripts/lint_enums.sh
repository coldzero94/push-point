#!/usr/bin/env bash
# Push-Point — enum 드리프트 검사 (just enum-lint)
# api/openapi.yaml의 enum(LinkStatus/ContentType/source/JobStatus)과
# backend/migrations/0001_init.up.sql의 CHECK 제약 값을 비교, 불일치 시 exit 1.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

python3 - "$ROOT/api/openapi.yaml" "$ROOT/backend/migrations/0001_init.up.sql" <<'PY'
import re
import sys

yaml_path, sql_path = sys.argv[1], sys.argv[2]
yaml_text = open(yaml_path, encoding="utf-8").read()
sql_text = open(sql_path, encoding="utf-8").read()


def yaml_enum(anchor):
    """anchor 키 라인 바로 아래 블록(8줄 이내)에서 enum: [a, b, ...] 추출."""
    m = re.search(rf"^\s*{re.escape(anchor)}:\s*$", yaml_text, re.M)
    if not m:
        sys.exit(f"openapi.yaml에서 '{anchor}:' 블록을 찾지 못함")
    window = "\n".join(yaml_text[m.end():].splitlines()[:8])
    e = re.search(r"enum:\s*\[([^\]]*)\]", window)
    if not e:
        sys.exit(f"openapi.yaml '{anchor}' 아래 8줄 내에서 enum을 찾지 못함")
    return [v.strip().strip("'\"") for v in e.group(1).split(",") if v.strip()]


def sql_check(table, column):
    """CREATE TABLE <table> 블록에서 CHECK (<column> IN (...)) 값 목록 추출."""
    m = re.search(rf"CREATE TABLE {table}\s*\((.*?)\n\);", sql_text, re.S)
    if not m:
        sys.exit(f"SQL에서 CREATE TABLE {table} 블록을 찾지 못함")
    c = re.search(rf"CHECK\s*\(\s*{column}\s+IN\s*\(([^)]*)\)", m.group(1))
    if not c:
        sys.exit(f"SQL {table}.{column}에서 CHECK IN 제약을 찾지 못함")
    return [v.strip().strip("'") for v in c.group(1).split(",") if v.strip()]


# (이름, openapi enum, SQL CHECK 위치) — 값·순서 모두 일치해야 통과
pairs = [
    ("LinkStatus", yaml_enum("LinkStatus"), ("links", "status")),
    ("ContentType", yaml_enum("ContentType"), ("links", "content_type")),
    ("source", yaml_enum("source"), ("link_tags", "source")),
    ("JobStatus", yaml_enum("JobStatus"), ("jobs", "status")),
]

ok = True
for name, api_vals, (table, col) in pairs:
    db_vals = sql_check(table, col)
    if api_vals != db_vals:
        ok = False
        print(f"불일치: {name} — openapi.yaml {api_vals} != {table}.{col} CHECK {db_vals}")
    else:
        print(f"일치: {name} = {api_vals}")

if not ok:
    sys.exit(1)
print("enum-lint OK")
PY
