#!/usr/bin/env bash
# Push-Point — enum 드리프트 검사 (just enum-lint)
# api/openapi.yaml의 enum(LinkStatus/ContentType/source/JobStatus/TagFacet)과
# backend/migrations/*.sql의 CHECK 제약 값을 비교, 불일치 시 exit 1.
# CHECK는 CREATE TABLE(0001)에도, ALTER TABLE ADD COLUMN(0003 tags.facet)에도 있으므로
# 마이그레이션 파일 전체를 파일 순서대로 훑는다 — 새 컬럼이 어느 파일에서 오든 잡힌다.
# 같은 컬럼의 CHECK가 여러 파일에 있으면 전부 수집해 값이 동일한지 확인하고,
# 다르면(= 나중 마이그레이션이 제약을 재정의) 통과시키지 않고 실패한다.
#
# 알려진 한계 2가지 (실측 확인, 지금은 Go 컴파일·테스트가 먼저 막아준다):
#   - 컬럼이 나중에 DROP되면 죽은 CHECK와 비교해 통과한다 (열이 사라진 건 안 본다).
#   - CHECK 값 집합만 대조하고 SQL DEFAULT ↔ openapi default 짝은 보지 않는다.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

python3 - "$ROOT/api/openapi.yaml" "$ROOT/backend/migrations" <<'PY'
import glob
import os
import re
import sys

yaml_path, mig_dir = sys.argv[1], sys.argv[2]
yaml_text = open(yaml_path, encoding="utf-8").read()

up_files = sorted(glob.glob(os.path.join(mig_dir, "*.up.sql")))
if not up_files:
    sys.exit(f"마이그레이션 up 파일을 찾지 못함: {mig_dir}")
# (파일명, 본문) — 파일명 정렬 = 마이그레이션 적용 순서. 어느 파일에서 온 제약인지
# 보존해야 재정의(테이블 재생성·DROP/ADD COLUMN)를 감지할 수 있다.
sql_files = [(os.path.basename(p), open(p, encoding="utf-8").read()) for p in up_files]


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
    """<table>.<column>의 CHECK (<column> IN (...)) 값 목록 추출.
    CREATE TABLE 블록 안(0001)과 ALTER TABLE ADD COLUMN 문(0003) 양쪽을,
    **모든 마이그레이션 파일에 걸쳐 전부** 수집한다. 첫 매치에서 멈추면
    나중 마이그레이션이 제약을 재정의(테이블 재생성, DROP+ADD COLUMN)했을 때
    낡은 제약과 비교해 조용히 통과한다. 두 개 이상 발견되면 값이 전부 같아야
    하고, 다르면 어느 쪽이 최신인지 스크립트가 판단하지 않고 시끄럽게 실패한다."""
    found = []  # (출처, 값 목록) — 마이그레이션 순서
    for fname, text in sql_files:
        scopes = []
        for m in re.finditer(rf"CREATE TABLE {table}\s*\((.*?)\n\);", text, re.S):
            scopes.append((f"{fname} CREATE TABLE {table}", m.group(1)))
        for a in re.finditer(
            rf"ALTER TABLE {table}\s+ADD COLUMN\s+{column}\b(.*?);", text, re.S | re.I
        ):
            scopes.append((f"{fname} ALTER TABLE {table} ADD COLUMN {column}", a.group(1)))
        for origin, scope in scopes:
            c = re.search(rf"CHECK\s*\(\s*{column}\s+IN\s*\(([^)]*)\)", scope)
            if c:
                vals = [v.strip().strip("'") for v in c.group(1).split(",") if v.strip()]
                found.append((origin, vals))
    if not found:
        sys.exit(f"SQL {table}.{column}에서 CHECK IN 제약을 찾지 못함")
    distinct = {tuple(v) for _, v in found}
    if len(distinct) > 1:
        detail = "\n".join(f"  - {origin}: {vals}" for origin, vals in found)
        sys.exit(
            f"SQL {table}.{column}의 CHECK 제약이 마이그레이션마다 다름 — "
            f"어느 것이 유효한지 자동 판별하지 않는다:\n{detail}"
        )
    return found[-1][1]


# (이름, openapi enum, SQL CHECK 위치) — 값·순서 모두 일치해야 통과
pairs = [
    ("LinkStatus", yaml_enum("LinkStatus"), ("links", "status")),
    ("ContentType", yaml_enum("ContentType"), ("links", "content_type")),
    ("source", yaml_enum("source"), ("link_tags", "source")),
    ("JobStatus", yaml_enum("JobStatus"), ("jobs", "status")),
    ("TagFacet", yaml_enum("TagFacet"), ("tags", "facet")),
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
