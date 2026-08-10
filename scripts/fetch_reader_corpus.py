#!/usr/bin/env python3
"""nlu/golden/reader/ 원본 HTML 코퍼스를 한 번 받아 커밋 가능한 형태로 남긴다.

**왜 HTML을 커밋하는가.** 기존 골든(dev/test/wild.jsonl)은 `snapshot{title, description,
body_text}`만 담는다 — **원본 HTML이 어디에도 없다.** 그래서 추출을 바꾼 뒤 "골든을 다시
돌려 비교한다"가 물리적으로 불가능하고, `just eval`·`just eval-search`는 이미 추출된
body_text를 읽으므로 **추출을 어떻게 바꿔도 숫자가 안 움직인다.** 추출의 모양을 재려면
입력이 있어야 하고, 그 입력이 이 코퍼스다.

**한 번만 받는다.** 페이지는 변하고 사라진다(wild 세트는 39개 URL에서 30개를 건졌다 —
7건 봇 차단, 1건 사망). 매번 받으면 측정이 남의 사이트 사정을 따라 흔들리고, 재현되지
않는 수치는 게이트가 못 된다. 그래서 받은 것을 gzip으로 커밋한다.

사용:
    python3 scripts/fetch_reader_corpus.py            # 기본 목표 수만큼
    python3 scripts/fetch_reader_corpus.py --limit 8  # 맛보기
"""

import argparse
import collections
import gzip
import json
import pathlib
import random
import sys
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
GOLDEN = ROOT / "nlu" / "golden"
OUT = GOLDEN / "reader"

# 브라우저인 척한다. 이건 우회가 아니라 **정직한 재현**이다 — 서버 스크레이퍼가 실제로
# 보내는 것과 같은 헤더여야 여기서 받은 HTML이 런타임이 보는 HTML과 같다.
UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " \
     "(KHTML, like Gecko) Chrome/126.0 Safari/537.36"

# 반드시 들어가야 하는 어려운 경우. 둘 다 실측으로 알려진 실패다 —
# blog.naver.com은 데스크톱이 iframe 껍데기라 본문이 19자로 나오고,
# medium.com은 95KB에서 281자만 남는다. 코퍼스가 쉬운 페이지만 담으면 벽 점수가
# 실제보다 좋게 나온다.
MUST_INCLUDE_HOSTS = ["blog.naver.com", "medium.com"]


def golden_urls():
    seen, rows = set(), []
    for name in ("dev", "test", "wild"):
        path = GOLDEN / f"{name}.jsonl"
        if not path.exists():
            continue
        for line in path.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            d = json.loads(line)
            u = d.get("url")
            if u and u not in seen:
                seen.add(u)
                rows.append(u)
    return rows


def host_of(u):
    try:
        return u.split("/")[2].replace("www.", "")
    except IndexError:
        return ""


def pick(urls, limit):
    """호스트 분포를 따라 뽑되 한 호스트가 코퍼스를 지배하지 않게 한다.

    brunch.co.kr 하나가 16건이라 그대로 뽑으면 코퍼스의 4할이 한 사이트가 된다 —
    그러면 벽 점수가 그 사이트의 템플릿 하나를 재는 것이 된다.
    """
    by_host = collections.defaultdict(list)
    for u in urls:
        by_host[host_of(u)].append(u)
    rng = random.Random(20260810)  # 고정 — 코퍼스가 실행마다 달라지면 안 된다
    out = []
    for h in MUST_INCLUDE_HOSTS:
        if by_host.get(h):
            out.append(by_host[h][0])
    hosts = sorted(by_host)
    rng.shuffle(hosts)
    per_host = 2
    while len(out) < limit:
        added = False
        for h in hosts:
            pool = [u for u in by_host[h] if u not in out]
            if not pool:
                continue
            take = pool[: max(0, per_host - sum(1 for u in out if host_of(u) == h))]
            for u in take:
                if len(out) >= limit:
                    break
                out.append(u)
                added = True
            if len(out) >= limit:
                break
        if not added:
            break
    return out[:limit]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--limit", type=int, default=30)
    args = ap.parse_args()

    OUT.mkdir(parents=True, exist_ok=True)
    urls = pick(golden_urls(), args.limit)
    print(f"대상 {len(urls)}건 — 호스트 {len(set(host_of(u) for u in urls))}개")

    index, ok, fail = [], 0, 0
    for i, u in enumerate(urls, 1):
        name = f"{i:03d}-{host_of(u).replace('.', '_')}.html.gz"
        try:
            req = urllib.request.Request(u, headers={"User-Agent": UA, "Accept-Language": "ko,en;q=0.8"})
            with urllib.request.urlopen(req, timeout=20) as r:
                raw = r.read()
        except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, OSError) as e:
            # 실패는 숨기지 않고 센다. 코퍼스가 왜 40이 아니라 31인지는 기록돼야 한다.
            print(f"  [skip] {host_of(u)}: {e}")
            fail += 1
            continue
        (OUT / name).write_bytes(gzip.compress(raw))
        index.append({"file": name, "url": u, "host": host_of(u), "bytes": len(raw)})
        ok += 1
        print(f"  [{i:3d}] {host_of(u):28} {len(raw):>8,} B")

    (OUT / "index.json").write_text(
        json.dumps({"fetched": ok, "skipped": fail, "pages": index}, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print(f"\n받음 {ok} · 건너뜀 {fail} → {OUT}")
    if ok == 0:
        sys.exit(1)


if __name__ == "__main__":
    main()
