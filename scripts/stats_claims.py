#!/usr/bin/env python3
"""docs/v2/ko/14-STATS-REDESIGN.md §1의 수치를 다시 낸다.

문서가 결정 네 건(D1~D4)을 이 숫자들 위에 세우므로, 숫자를 고칠 일이 생기면 근거부터
다시 돌린다 — 그게 가능하려면 생성기가 저장소에 있어야 한다. 처음에는 "재현 가능한
시뮬레이션에서 나왔다"고 써 놓고 스크립트를 커밋하지 않았다(2026-07-29 리뷰 지적).

    python3 scripts/stats_claims.py

의존성 없음. 난수를 쓰는 항목은 시드를 고정하므로 같은 값이 다시 나온다.
"""
import random, math, statistics, collections, datetime

WEEKDAYS = ['일','월','화','수','목','금','토']  # index 0 = Sunday, matches Date.getDay()
W = 30  # by_day window guaranteed by contract

# ---------------------------------------------------------------- 1. window structure
def slot_counts(today: datetime.date):
    """How many times each weekday appears in the 30-day window ending `today`."""
    c = [0]*7
    for i in range(W):
        d = today - datetime.timedelta(days=i)
        # python weekday(): Mon=0..Sun=6 ; JS getDay(): Sun=0..Sat=6
        c[(d.weekday()+1) % 7] += 1
    return c

print("=== 1. 30-day window is not a whole number of weeks (30 = 4*7 + 2) ===")
base = datetime.date(2026,7,29)
for k in range(7):
    t = base + datetime.timedelta(days=k)
    c = slot_counts(t)
    five = [WEEKDAYS[i] for i,n in enumerate(c) if n==5]
    print(f"  today={t} ({WEEKDAYS[(t.weekday()+1)%7]}) slots={c}  5-slot weekdays={five}")

# ---------------------------------------------------------------- 2. rhythm.ts ports
def streak(by_day):
    i = len(by_day)-1
    if i < 0: return 0
    if by_day[i] == 0: i -= 1
    n = 0
    while i >= 0 and by_day[i] > 0:
        n += 1; i -= 1
    return n

def week_over_week(by_day, total_links):
    if len(by_day) < 14: return None
    in_window = sum(by_day)
    first_active = next((i for i,c in enumerate(by_day) if c>0), -1)
    hist = 0 if first_active < 0 else len(by_day)-first_active
    if hist < 14 and total_links <= in_window: return None
    return sum(by_day[-7:]) - sum(by_day[-14:-7])

def weekday_counts(by_day, today):
    c = [0]*7
    for i, n in enumerate(by_day):
        d = today - datetime.timedelta(days=(len(by_day)-1-i))
        c[(d.weekday()+1)%7] += n
    return c

def peak_weekday(by_day, today):
    c = weekday_counts(by_day, today)
    p = max(c)
    return None if p == 0 else c.index(p)   # indexOf == firstIndex(of:) -> lowest index wins

# ---------------------------------------------------------------- 3. deterministic user
print("\n=== 2. A PERFECTLY REGULAR user (exactly 2 saves every single day) ===")
today = datetime.date(2026,7,29)
prev = None
for k in range(10):
    t = today + datetime.timedelta(days=k)
    by = [2]*30
    idx = peak_weekday(by, t)
    c = weekday_counts(by, t)
    flip = '' if prev is None else ('  <-- FLIPPED' if idx != prev else '  (same)')
    print(f"  {t} ({WEEKDAYS[(t.weekday()+1)%7]}): counts={c} -> claims '{WEEKDAYS[idx]}요일에 가장 많이'{flip}")
    prev = idx

# ---------------------------------------------------------------- 4. Monte Carlo
MODELS = {
    "A 1-3/day, never idle (the DoD rate)": lambda r: r.choice([1,2,3]),
    "B 0-3/day uniform (idle days allowed)": lambda r: r.choice([0,1,2,3]),
    "C bursty: 60% idle, else 1-5":          lambda r: 0 if r.random()<0.6 else r.randint(1,5),
}

def run(model, days=400, trials=400, seed=7):
    r = random.Random(seed)
    flips = collections.Counter(); obs = collections.Counter()
    wow_vals = []; peak_hist = collections.Counter(); streak_flips = 0; steps = 0
    active_flips = 0; week_flips = 0
    for _ in range(trials):
        series = [model(r) for _ in range(days)]
        start = datetime.date(2026,1,1)
        prev = None
        for end in range(60, days):          # after 60 days of history
            by = series[end-29:end+1]
            t = start + datetime.timedelta(days=end)
            total = sum(series[:end+1])
            cur = {
                'week':   sum(by[-7:]),
                'wow':    week_over_week(by, total),
                'peak':   peak_weekday(by, t),
                'streak': streak(by),
                'active': sum(1 for c in by if c>0),
            }
            if cur['wow'] is not None: wow_vals.append(cur['wow'])
            if cur['peak'] is not None: peak_hist[cur['peak']] += 1
            if prev is not None:
                steps += 1
                if cur['week']   != prev['week']:   week_flips += 1
                # wow: the *word* shown (많아요/적어요/같은 수) — sign, not magnitude
                s1 = None if prev['wow'] is None else (0 if prev['wow']==0 else (1 if prev['wow']>0 else -1))
                s2 = None if cur['wow']  is None else (0 if cur['wow']==0  else (1 if cur['wow']>0  else -1))
                if s1 != s2: flips['wow_direction'] += 1
                if prev['wow'] != cur['wow']: flips['wow_number'] += 1
                if prev['peak'] != cur['peak']: flips['peak_weekday'] += 1
                if prev['streak'] != cur['streak']: streak_flips += 1
                if prev['active'] != cur['active']: active_flips += 1
            prev = cur
    return dict(steps=steps, flips=flips, wow_vals=wow_vals, peak_hist=peak_hist,
                streak_flips=streak_flips, active_flips=active_flips, week_flips=week_flips)

print("\n=== 3. Day-to-day flip rate of each claim (Monte Carlo, 400 trials x 340 days) ===")
for name, m in MODELS.items():
    r = run(m)
    n = r['steps']
    wv = r['wow_vals']
    print(f"\n-- {name}")
    print(f"   '이번 주에 N개'          changes {100*r['week_flips']/n:5.1f}% of consecutive days")
    print(f"   '지난주보다 N개' (number) changes {100*r['flips']['wow_number']/n:5.1f}%")
    print(f"   '지난주보다 …' (direction word 많/적/같) changes {100*r['flips']['wow_direction']/n:5.1f}%")
    print(f"   'X요일에 가장 많이'       changes {100*r['flips']['peak_weekday']/n:5.1f}%")
    print(f"   '{'{'}N{'}'}일 연속'               changes {100*r['streak_flips']/n:5.1f}%")
    print(f"   '{'{'}N{'}'}일 저장' (활동일)       changes {100*r['active_flips']/n:5.1f}%")
    print(f"   week-over-week delta: mean {statistics.mean(wv):+.2f}, sd {statistics.pstdev(wv):.2f}, "
          f"mean|delta| {statistics.mean(abs(v) for v in wv):.2f}, "
          f"P(delta==0)={100*sum(1 for v in wv if v==0)/len(wv):.1f}%")
    tot = sum(r['peak_hist'].values())
    dist = ' '.join(f"{WEEKDAYS[i]}{100*r['peak_hist'][i]/tot:.0f}%" for i in range(7))
    print(f"   which weekday gets named:  {dist}   (uniform truth = 14.3% each)")

# ---------------------------------------------------------------- 5. weekday significance
print("\n=== 4. When does a 'busiest weekday' beat chance? (multinomial max test) ===")
def max_null(N, trials=200000, seed=3):
    r = random.Random(seed); out=[]
    for _ in range(trials):
        c=[0]*7
        for _ in range(N): c[r.randrange(7)] += 1
        out.append(max(c))
    return out

print("  N = total saves inside the 30-day window; H0 = each save equally likely on any weekday")
print("  m* = smallest max-bucket count with P(max >= m* | H0) <= 0.05")
for N in [14, 21, 30, 45, 60, 90, 120, 200, 300, 500]:
    s = sorted(max_null(N), reverse=True)
    mstar = s[int(0.05*len(s))]
    exp = N/7
    print(f"   N={N:4d}  E[bucket]={exp:5.1f}  m*={mstar:4d}  "
          f"m*/E = {mstar/exp:.2f}x  -> peak must be {100*(mstar/exp-1):.0f}% above flat to mean anything")

print("\n  Power: how big must the true preference be to be detectable at 5%?")
def power(N, p_fav, trials=20000, seed=5):
    r = random.Random(seed)
    s = sorted(max_null(N, 40000, seed=11), reverse=True)
    mstar = s[int(0.05*len(s))]
    hits = 0
    other = (1-p_fav)/6
    for _ in range(trials):
        c=[0]*7
        for _ in range(N):
            x = r.random()
            if x < p_fav: c[0]+=1
            else: c[1+int((x-p_fav)/other)%6]+=1
        if c[0] >= mstar and c.index(max(c))==0: hits+=1
    return hits/trials, mstar

for N in [60, 120, 200]:
    for p in [0.20, 0.25, 0.30, 0.40]:
        pw, ms = power(N, p)
        print(f"   N={N:4d}  true P(fav weekday)={p:.2f} (flat=0.143)  m*={ms}  power={100*pw:5.1f}%")

# ---------------------------------------------------------------- 6. WoW arithmetic
print("\n=== 5. Week-over-week: how wide must the window be? ===")
for label, (mu, var) in {
    "1-3/day uniform":  (2.0, 2/3),
    "0-3/day uniform":  (1.5, 5/4),
}.items():
    print(f"  {label}: daily mean {mu}, var {var:.3f}")
    for k in [7, 14, 21, 28, 30]:
        sd_delta = math.sqrt(2*k*var)
        thresh = 1.96*sd_delta
        rel = thresh/(mu*k)
        print(f"    window {k:2d}d: sd(delta)={sd_delta:.2f}, 95% noise band = +-{thresh:.1f} links "
              f"-> smallest real change detectable = {100*rel:.0f}%")
import json, random, collections, datetime, statistics, math
WEEKDAYS = ['일','월','화','수','목','금','토']
import os
ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..') + '/'

tags = json.load(open(ROOT+'nlu/dictionary/tags.json'))
facet = {t['name']: t['facet'] for t in tags}
FACETS = ['craft','media','life','neutral']
LABEL = {'craft':'만드는 것','media':'형식','life':'일 바깥','neutral':'분류 없음'}

# realistic tag draws: sample whole links from the golden sets (real curated labels)
pool = []
for nm in ['dev','test','wild']:
    for l in open(ROOT+'nlu/golden/%s.jsonl'%nm):
        if l.strip(): pool.append(json.loads(l)['expected_tags'])
print(f"link pool for sampling: {len(pool)} real labelled links, "
      f"mean {statistics.mean(len(p) for p in pool):.2f} tags/link")

def dominant(counter):
    """rhythm.ts dominantFacet: max over ALL facets in TAG_FACETS order, then suppress neutral."""
    tot = collections.Counter()
    for name,c in counter.items(): tot[facet.get(name,'neutral')] += c
    best=None
    for f in FACETS:
        if tot[f] > (0 if best is None else tot[best]): best=f
    return None if best=='neutral' else best

print("\n=== 6. '주로 X에 관심이 갔고' — by_tag is ALL-TIME cumulative (no date filter in the SQL) ===")
r=random.Random(11)
for trial in range(3):
    cnt=collections.Counter(); seq=[]
    for i in range(1,201):
        for t in r.choice(pool): cnt[t]+=1
        seq.append(dominant(cnt))
    flips=sum(1 for a,b in zip(seq,seq[1:]) if a!=b)
    # settle point = last index where it changed
    last=max((i for i in range(1,len(seq)) if seq[i]!=seq[i-1]), default=0)
    print(f"  trial {trial}: {flips} changes over 200 links; last change at link #{last+1}; "
          f"final = '{LABEL[seq[-1]]}'")

# flip rate per *day* at 2 links/day once the archive is grown
print("\n  flip rate per consecutive day, by archive size (2 saves/day):")
for base in [10, 20, 50, 100, 300, 1000]:
    flips=0; n=0
    for trial in range(300):
        rr=random.Random(1000+trial)
        cnt=collections.Counter()
        for _ in range(base):
            for t in rr.choice(pool): cnt[t]+=1
        prev=dominant(cnt)
        for _ in range(60):                    # 60 more days
            for _ in range(2):
                for t in rr.choice(pool): cnt[t]+=1
            cur=dominant(cnt); n+=1
            if cur!=prev: flips+=1
            prev=cur
    print(f"    archive {base:4d} links: {100*flips/n:5.2f}% of days the facet sentence changes")

print("\n  facet totals in the real labelled data (what the sentence would say):")
cnt=collections.Counter()
for p in pool:
    for t in p: cnt[t]+=1
tot=collections.Counter()
for name,c in cnt.items(): tot[facet.get(name,'neutral')]+=c
s=sum(tot.values())
for f,c in tot.most_common(): print(f"    {LABEL[f]:8s} {c:4d}  {100*c/s:5.1f}%")
print(f"    -> margin between 1st and 2nd = {100*(tot.most_common()[0][1]-tot.most_common()[1][1])/s:.1f}pp")
print(f"    dictionary has {sum(1 for t in tags if t['facet']=='neutral')} neutral tags out of {len(tags)}"
      f"  => the 'stay quiet' branch is unreachable with the shipped dictionary")

# ---------------------------------------------------------------- window-edge bias
print("\n=== 7. Is the named weekday tracking behaviour, or the window edge? ===")
def weekday_counts(by, today):
    c=[0]*7
    for i,n in enumerate(by):
        d=today-datetime.timedelta(days=(len(by)-1-i)); c[(d.weekday()+1)%7]+=n
    return c

for label, draw in [("exactly 2/day (perfectly regular)", lambda r: 2),
                    ("1-3/day uniform",                   lambda r: r.choice([1,2,3])),
                    ("0-3/day uniform",                   lambda r: r.choice([0,1,2,3]))]:
    r=random.Random(4); hit_today=0; hit_yest=0; n=0; ties=0
    for trial in range(3000):
        by=[draw(r) for _ in range(30)]
        today=datetime.date(2026,1,1)+datetime.timedelta(days=r.randrange(400))
        c=weekday_counts(by,today); p=max(c)
        if p==0: continue
        idx=c.index(p); n+=1
        if c.count(p)>1: ties+=1
        tw=(today.weekday()+1)%7; yw=((today-datetime.timedelta(days=1)).weekday()+1)%7
        if idx==tw: hit_today+=1
        if idx==yw: hit_yest+=1
    print(f"  {label}:")
    print(f"    named weekday == today's weekday   : {100*hit_today/n:5.1f}%  (chance 14.3%)")
    print(f"    named weekday == yesterday's weekday: {100*hit_yest/n:5.1f}%  (chance 14.3%)")
    print(f"    named weekday is one of those two  : {100*(hit_today+hit_yest)/n:5.1f}%  (chance 28.6%)")
    print(f"    ties for the max (broken by index, Sun first): {100*ties/n:5.1f}% of renders")

# ---------------------------------------------------------------- days-to-threshold
print("\n=== 8. Days of use needed, at the project's stated rate ===")
for rate in [1,2,3]:
    print(f"  at {rate} saves/day:")
    print(f"    30-day window holds N = {30*rate} saves  (weekday claim is capped here forever)")
    for N,desc in [(120,'2.0x weekday preference detectable at ~97% power'),
                   (200,'1.4x weekday preference detectable at only ~46% power')]:
        print(f"    to accumulate N={N} ({desc}): {math.ceil(N/rate)} days = {N/rate/30:.1f} months of window")


# ---------------------------------------------------------------------------
# 아래 둘은 2026-07-29 리뷰가 "문서에 인용됐는데 재현 불가"라고 지적한 항목이다.
# 문서의 권위가 센 근거에 있으므로 생성기에 넣는다.
# ---------------------------------------------------------------------------

def named_weekday_is_recent(rate=(1, 2, 3), days=4000, seed=20260729):
    """호명되는 요일이 '오늘 또는 어제'인 비율, 그리고 동점이 답을 정하는 비율.

    30 = 4주 + 2일이라 오늘·어제 요일만 창에 5칸을 갖는다. 그래서 최댓값은 데이터가
    아니라 창의 모양이 정하는 경우가 많고, 동점일 때는 `indexOf`가 일요일 쪽으로 깬다.
    """
    import random
    rnd = random.Random(seed)
    counts_per_day = [rnd.choice(rate) for _ in range(days + 30)]

    recent = ties = 0
    for d in range(days):
        window = counts_per_day[d:d + 30]          # 마지막 칸이 '오늘'
        buckets = [0] * 7
        for i, c in enumerate(window):
            # 창의 마지막(i=29)이 오늘. 요일 인덱스는 오늘로부터 거슬러 센다.
            buckets[(d + i) % 7] += c
        peak = max(buckets)
        if buckets.count(peak) > 1:
            ties += 1
        idx = buckets.index(peak)                   # 클라이언트와 같은 동점 처리
        today_i, yest_i = (d + 29) % 7, (d + 28) % 7
        if idx in (today_i, yest_i):
            recent += 1
    return recent / days, ties / days


def facet_sentence_stability(seed=20260729):
    """지배 관심사 문장이 하루 사이에 바뀌는 비율 — 아카이브 크기별.

    `by_tag`에 날짜 조건이 없어 전 기간 누계이므로, 링크가 쌓일수록 1등이 굳는다.
    golden 실제 라벨의 facet 분포를 모집단으로 쓴다.
    """
    import json, os, random, collections
    root = os.path.join(os.path.dirname(__file__), "..")
    dic = json.load(open(os.path.join(root, "nlu/dictionary/tags.json")))
    tags = dic["tags"] if isinstance(dic, dict) else dic
    facet_of = {t["name"]: t.get("facet", "neutral") for t in tags}

    pool = []
    for name in ("dev", "test", "wild"):
        path = os.path.join(root, f"nlu/golden/{name}.jsonl")
        if not os.path.exists(path):
            continue
        for line in open(path):
            row = json.loads(line)
            for t in row.get("expected_tags", []):
                if t in facet_of:
                    pool.append(facet_of[t])
    if not pool:
        return []

    rnd = random.Random(seed)
    out = []
    for size in (10, 50, 100, 300):
        flips = trials = 0
        for _ in range(3000):
            drawn = collections.Counter(rnd.choice(pool) for _ in range(size))
            before = max(drawn, key=lambda f: (drawn[f], f))
            drawn[rnd.choice(pool)] += 1          # 하루치 저장 한 건
            after = max(drawn, key=lambda f: (drawn[f], f))
            flips += before != after
            trials += 1
        out.append((size, flips / trials))
    return out


if __name__ == "__main__":
    print("\n=== 호명 요일이 '오늘 또는 어제'인 비율 (하루 1~3건) ===")
    recent, ties = named_weekday_is_recent()
    print(f"  오늘/어제 요일이 호명됨  {recent:.1%}   (우연이면 2/7 = 28.6%)")
    print(f"  동점이 답을 정한 렌더    {ties:.1%}")

    print("\n=== 지배 관심사 문장이 하루 사이에 바뀌는 비율 ===")
    for size, p in facet_sentence_stability():
        print(f"  아카이브 {size:4}건: {p:.2%}")
