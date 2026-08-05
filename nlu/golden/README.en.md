# nlu/golden — the tagging-quality golden set

> Push-Point v2.1 — last updated: 2026-07-27

The dataset `just eval` reads to measure tagging accuracy. Committed as JSONL.

## Schema (one line = one link)

```json
{"url": "...", "snapshot": {"title": "...", "description": "...", "body_text": "...", "keywords": "...", "body_source": "server|client"}, "expected_tags": ["...", "..."]}
```

- eval makes **zero network calls** — the tagger's input is the snapshot fields and nothing else. Nothing re-fetches the URL, so the result reproduces no matter when it is run.
- The snapshot is **exactly the surface the runtime tagger sees** (`title`, `description`, `body_text`, `keywords`, with the domain derived from `url`). That is what removes train/serve skew — so golden is captured through the **production scrape path** (`pushpoint golden-capture`, go-trafilatura body extraction). It is never fetched from a richer source.
- `keywords` (the publisher's own classification) was **added on 2026-07-26.** It had been left out on the view that it is "a mostly dead signal on the modern web", but measuring found it present in 18 of dev's 62 entries and in 23 of test's 61. The grounds for putting it in the schema, though, are not how often it exists but its **measured contribution** (below).
  - **When the body is present** its contribution to Recall@3 is zero (+0.000 on both dev and test) — the body already says whatever the classification says.
  - **Only when the body is missing** does it earn its place: +0.065 on dev, +0.049 on test. And a missing body is exactly the situation this field exists for — a failed scrape, an SPA, a bot wall; that is, the client-capture path.
  - So eval reports the two **separately**. Measure only one and the field reads as either "useless" or "essential", whichever you happened to pick.
  - golden is heavy on developer documentation, so only about 30% of it carries a classification, but Korean news — which real use saves often — emits `article:section` almost every time. **So this measurement is a lower bound on the real-world gain.**
- `expected_tags` uses only names that exist in the tag dictionary (`nlu/dictionary/`).

## The split: dev 77 / test 84 / wild 28

- **dev.jsonl (77 entries)**: rules, thresholds and dictionary are tuned against this and nothing else.
  **Grown 62 → 77 on 2026-07-27** — see the "dev expansion" section below.
- **test.jsonl (84 entries)**: frozen. Gate verdicts (milestone entry/exit conditions) are taken on frozen test alone.
  **Grown 61 → 84 on 2026-07-27** — see the "test expansion" section below. Every earlier gate calculation was against 61 entries, so
  those figures are not compared directly with these.
- **wild.jsonl (28 entries)**: the real web outside developer blogs (social, commerce, app stores, communities). Same grade as
  dev, and not a gate. **It is reported separately, never merged into dev/test** — merged, the gap this set exists to
  expose disappears behind an average. Background in the "wild.jsonl" section below.

## The freeze rule

- Fixing the tagger to match the golden set is allowed; **the reverse — fixing the golden set to match the tagger's output — is forbidden**.
- The only exception for editing test.jsonl is correcting a label error (a wrongly assigned expected_tags), and the reason for the correction goes in the commit message.
- **wild.jsonl is the same grade as dev** — it may be inspected and fixed. It is not used for gate verdicts (see below).

## Metric definitions (`just eval` output)

- **hit**: predicted top-3 ∩ expected_tags ≥ 1
- **Recall@3**: hits / total entries
- **Three variants measured at once**: `full` (domain + title + description + body) / `no-body` (body excluded — the Δ the body contributes) / `baseline` (domain heuristics only — the Δ the rules contribute).
- Per-tag precision/recall (on `full`, top-3) and per-tag frequency in golden are printed as tables alongside.

## Set expansion (2026-07-25)

The original 100 entries contained **not one Naver Blog, Tistory or dev.to** — leave out the platforms this app's user
actually saves most and a scraper regression on them can never be caught. Adding 23 brought it to
123 (Naver 8, Tistory 7, dev.to 2, Hashnode, GitHub Pages, a personal domain, LINE, Danggeun), and
**all 23 got a body**. `expected_tags` was labelled from the content, independently of the tagger's output, and no
tuning was done on the addition, so this does not breach the freeze rule (fixing the tagger to match golden is allowed,
fixing golden to match the tagger is forbidden).

## wild.jsonl — the real-web set (first cut 2026-07-26, re-captured 2026-07-27, 28 entries)

**The 123 dev and test entries are overwhelmingly developer blogs and technical documentation.** Social/video is 0 entries in dev and
1 in test (1.6%). Yet what this app aims to save includes X, YouTube and communities (that is not a measured
fact but a **design assumption** — real usage has accumulated only three saves, too few to measure). Either way,
one thing is confirmed: `test 0.885` is a number produced by a developer-blog distribution.

This gap came out of how the set was built. Curation took "representative URLs that evenly cover the dictionary's 30 tags"
as its criterion, and what satisfies that criterion best is a developer blog. It is not that one axis was left out — **the
selection criterion let only one kind of web through.**

wild was built by putting 39 URLs through the production scrape path — 30 survived (29 distinct hosts),
7 never arrived because of bot blocking (429/403), 1 was a 404, and 1 was dropped because its label could not be
justified (below). Commerce, app stores, job boards, recipes, wikis, games, courses, maps, music, communities —
places that get saved but were absent from golden.

Measured at the time the set was built (before defect E was fixed):

| Set | Recall@3 full | no-body | baseline | Korean | English |
|---|---|---|---|---|---|
| dev (62)  | 0.952 | 0.871 | 0.355 | 1.000 | 0.912 |
| test (61) | 0.885 | 0.820 | 0.344 | 0.941 | 0.815 |
| **wild (30)** | **0.733** | 0.533 | 0.300 | 0.750 | 0.722 |

**-15.2pp against test.** And all 8 misses are **zero-scored on the correct tag** — meaning no amount of re-ranking gets past
0.733, and that the signal was not pushed down but is absent or wrong outright.

**After fixing five defects and re-capturing (2026-07-27):** dev 0.935 / test **0.902** / wild **0.821** (28 entries).
But wild's rise **came from a shrinking denominator** — the "Re-capture" section below separates it entry by entry.

### 0.733 has to be split into the tagger's share and the capture's share

**0.733 is not tagger quality.** In 4 of the 30 entries the snapshot's signal (title + description + body) does not reach 200 characters —
the scraper fetched **a wall, not a page** (imdb's `JavaScript is disabled` at 166 characters, Reddit's
verification hold at 37, and so on). Those entries are a miss forever, whatever is done to the tagger. The fix is not the tagger
but client capture.

```
신호 200자 미만 4건(맞힌 것 1) — 캡처가 벽·빈 응답을 물어온 몫이라 태거로 못 고친다.
  나머지 26건 기준 Recall@3=0.808 — 태거를 고쳐서 올릴 수 있는 상한이다.
```

So this is how to read it: **0.808 is as far as fixing the tagger can go, and the difference from 0.733 is the share that
belongs to the scraper and to client capture.** Report only one of the two numbers and a capture defect gets misattributed
to tagger quality — which is exactly what the first draft of this PR did, until review caught it.

The 200-character threshold is **an approximation and is used only as one.** There are walls longer than 200 characters and
genuinely short posts under it. The reason no attempt is made to judge wall-ness from content is that a bot-block page has
a plausible title too (`Reddit - Please wait for verification`), which would make the rule itself an inference that can be wrong.
The only thing counted is the character count.

### One entry dropped — its label could not be justified from the committed snapshot

`threads.net/@zuck/post/C8_hHqNRLwG` was labelled `food, life`, but **the body that justified it is not in the committed
file.** Saved through the API during exploration it came back with a 2,266-character body (a post about a Magok cafe);
fetched again with `golden-capture` it came back as a 101-character login wall. The label was written from the first and the
file ended up holding the second.

**That the same URL gives something different on each run** is itself a finding, but it does not license keeping a label
justified by a body that is not there (that would be precisely the violation of "fixing the tagger to match golden is allowed,
the reverse is forbidden"). So it was dropped. A capture that does not reproduce cannot go into golden.

### Five defect classes only this set exposes — recorded as found

> **All five were handled on 2026-07-27, which is why none of them reproduce against today's `wild.jsonl`.**
> The line numbers and quotations below are against the **pre-re-capture snapshot** (`wild.jsonl` as of PR #45) —
> pull that revision with `git log -- nlu/golden/wild.jsonl` and they check out exactly as written. What was fixed
> has to stay verifiable months from now, so the quotations were kept and stamped with their date rather than deleted. Each row is an
> observation **as of discovery (2026-07-26)**, and fixed rows are marked as fixed.

**Only what is inside the committed file counts as evidence.** The first draft of this table copied down what an exploration
session had shown, and review found three of its rows wrong — the URL differed between exploration and the committed
file (dribbble), the capture differed run to run (threads), the domain was not in the set at all (instagram). Evidence
that cannot be reproduced is evidence nobody can check months later, so it is not evidence.

In the per-tag table, the tags predicted while `golden=0` (`javascript`, `sports`, `realestate`, `book`)
are the fingerprint of a false positive.

| | Symptom | Reproduction (line numbers are in `wild.jsonl`) |
|---|---|---|
| A | **A bot-block page becomes the body** — **fixed 2026-07-27** | Before the fix: line 12, imdb, had an empty-string title and a 166-character body `"<h1>JavaScript is disabled</h1> … not a robot"`, and **`javascript`** landed in the top-3 — the wall turned its own name into a tag. Now the scrape fails with `ErrBlockedPage` and the reason is recorded in `links.error` |
| B | **A footer or boilerplate becomes the body** — **partly fixed (2026-07-27)** | Line 20, Melon (a 355-character body that is entirely the site footer), now has its body discarded → the `sports` false positive is gone. Line 8, Notion (`광고 차단기로 인해 동영상이…`), and line 21, the recipe (`동영상 조리순서`), were left alone because the contamination sits **in the middle of the body** — see "What was not done for B" below |
| C | **A verification or login wall becomes the title** — **fixed 2026-07-27** (the same fix as A) | Before the fix: line 28, Reddit `/r/LocalLLaMA/`, had the title `"Reddit - Please wait for verification"` and a 37-character body. Now the wall's metadata is not stored at all — a wall's title never appears in the list |
| E | **Korean matching looked only at the front of a word** — **fixed 2026-07-27** (section below) | Before the fix: Hangul matching was `strings.HasPrefix` and nothing else, so **`"대박식당처럼"` did not match `식당`.** Line 21, the kimchi-jjigae recipe, had `[video]` alone in its top-3 and `food` scored zero (`식당`, `요리` and `레시피` are all `food` aliases, and all three were buried inside a word). Now it is `[food video]` |
| F | **charset not handled** — **fixed 2026-07-27** | Before the fix: line 16, Naver News, had its EUC-KR page read as UTF-8, so the title was `���̹� ����`. `'네이버 뉴스'.encode('euc-kr').decode('utf-8','replace')` reproduces it down to the bytes. Fetch the same URL now and the title is `네이버 뉴스` with the body going 10,849 → 14,989 characters. **The snapshot is still the old one** — the re-capture happens in one pass once A, B and C are fixed too |

**E ran deepest.** It explains why Korean Recall was dev 1.000 and wild 0.750 —
Korean in developer blogs puts the technical term at the head of the word ("쿠버네티스 도입기"), while everyday Korean
buries it inside one ("대박식당처럼", "김진옥요리가좋다").

### Three observations from the capture session that this set does not reproduce

The following was seen during the 2026-07-26 capture session and **cannot be checked against `wild.jsonl`.** That is
why it is kept out of the table above. It is still worth recording.

- **The instagram adapter does not even make an HTTP request.** `backend/internal/scraper/adapters.go:264`
  returns `Metadata{ContentType:"post"}` as is (the code is still there to check). The result is a link with an empty
  title, body and tags sitting at `status=done`. **There is no instagram entry in the set** —
  nothing in the snapshot would justify a label, so none was added.
- **Seven blocked.** amazon, airbnb, behance, linkedin, figma, docs.google and coupang never got through, on
  429/403. That is why commerce is thin in this set.
- **The runtime and eval cut at different depths.** The YouTube entry's body carries the channel boilerplate `iOS(아이폰) 👉 apple.co/…`
  and `ios` scores 0.667, but **eval's top-3 does not show it, because it ranks fourth.** The runtime uses
  `topK = 5` (`classify.go:43`), so **it does get attached on the user's screen.** That is, B shows up more often in the
  actual product, and this table holds only its lower bound.

### Fixing defect E — Korean matching now looks at both ends of a word (2026-07-27)

`matchesKoSurface` (`backend/internal/tagger/match.go`) looks at both prefix and suffix.
The grounds are that **Korean compound nouns are head-final** — `대박식당` is modifier + **식당**,
`김치찌개` is 김치 + **찌개**; the semantic head comes last. Look only at the prefix and you catch `식당가`, where the head
leads, and miss the more common shape wholesale.

| Set | Before | After | Δ | Korean before → after |
|---|---|---|---|---|
| dev (62) | 0.952 | 0.935 | **−1.6pp** | 1.000 → 0.964 |
| **test (61)** | 0.885 | **0.902** | **+1.7pp** | 0.941 → **0.971** |
| **wild (30)** | 0.733 | **0.767** | **+3.4pp** | 0.750 → **0.833** |

By hits that is **2 improved, 1 regressed**.

- Improved: `wild:21` the kimchi-jjigae recipe (`대박식당처럼` → `식당` → **`food`**) and `test:53` `blog.naver.com/esteee_/224346723933` (→ `book`)
- Regressed: `dev:31` `nomadcoders.co/wetube` — `유튜브 클론코딩 … 강의`. `클론코딩` matched `코딩` (a `dev`
  alias), so **`dev` was newly attached and pushed the labelled `tutorial` out of the top three.**
  **This is not a mismatch** — `dev` is a correct tag for a coding course; it simply is not in the label.
  The label does not get edited to make the number look better (the forbidden direction of the freeze rule).

**But "2 against 1" understates the size of the change.** Only 3 entries changed hit status, yet
**11 had their top-3 itself change**, and the other 8 kept their hit while **giving up a labelled tag to a
new one.** hit@3 passes as long as a single correct tag survives, so it cannot see that substitution.

| Entry | Before → after | Where the new tag came from | Verdict |
|---|---|---|---|
| `wild:18` tistory | `[article life]` → `[article life food]` | 공주**맛집** → `맛집` | Correct (a restaurant post in the popular list) |
| `wild:4` the YouTube AI-in-government clip | `[ai security video]` → `[ai politics security]` | 범**정부** → `정부` | Reasonable (a government policy video) |
| `test:20` the Toss savings-rate post | `[finance startup article]` → `[finance startup game]` | 토**스팀** → `스팀` | **False positive** |
| `test:34` woowahan | `[backend data kubernetes]` → `[article backend data]` | — | Lost the labelled `kubernetes` |

**`토스팀` → `스팀` (Steam) is the representative false positive this change created.** It is a two-character alias
collision, the same root as "What remains broken" below.

### Why `strings.Contains` was not used

**Recall@3 is identical to `Contains` on all three sets.** But that number is the only thing that is identical —
compare the full output and they diverge:

| | prefix+suffix | Contains |
|---|---|---|
| dev boundary ties | 29/62 (47%) | 28/62 (45%) |
| dev `database` R | 0.67 | **0.33** |
| dev `dev` P | 0.38 | 0.40 |
| test `dev` P / R | 0.43 / 0.60 | 0.45 / 0.67 |

`Contains` also catches surfaces buried in the **middle** of a word, and those extra matches do not raise Recall while
in places like `database` they cut it. **Nothing gained, new risk taken** — so the narrower rule wins.

> The first draft of this PR wrote here that "across 153 entries the two rules produced **completely identical**
> results, and that is empirical proof of the head-final hypothesis". **Both claims were wrong.** What matched was
> Recall@3 alone, and surfaces buried only mid-word do exist. "Identical" was written after comparing a single
> Recall line, and the inference drawn from it inflated the conclusion beyond its evidence. Recorded here.

**What remains broken**: this rule amplifies bad aliases. `경기` (a sports alias) comes to match not only `경기도` but
`불경기`. That is a dictionary-side problem and needs a migration of its own (defect B).

`matchTail` (the phrase tail) was **deliberately left as prefix-only.** Applying the same rule moved no number at all
across the 153 entries, and without evidence of gain nothing gets widened.

### Defect B — what was fixed and what was not (2026-07-27)

**Fixed: a body that is nothing but the site footer gets discarded** (`backend/internal/scraper/boilerplate.go`).
The test has the same shape as the wall test — **marker count × length**. Across golden's 153 entries three bodies carry
two or more markers, and they are of completely different natures:

| | Body | Markers | |
|---|---|---|---|
| Melon track page | 355 chars | 7 | Entirely company info and terms |
| Tistory home | 514 chars | 4 | Navigation + today's popular posts |
| App Store (Toss) | 2,428 chars | 2 | **A real app description** |

Go by count alone and the App Store is caught; go by length alone and short genuine posts are. `≥3 markers AND <1000 chars`
leaves only the first two. Simulating the effect before the re-capture: **Recall@3 unchanged on all three sets**
(both still hit through title and description), the `sports` false-positive row gone, `food` precision 0.33 → 0.50,
`life` recall 0.67 → 0.33 (Tistory loses the tag it had been getting from the popular-post list).

#### Not done ① — cleaning up two-character aliases

When the defect E fix turned suffix matching on, the note said "short aliases become dangerous" — **measuring
largely refuted that worry.** An exhaustive audit of all 16 two-character surfaces in golden that match at the end of a word
found **15 correct and only 1 wrong**:

- Correct: `오사카여행`→travel · `클론코딩`→dev · `공주맛집`→food · `웹서버`→backend ·
  `기술면접`→career · `고금리`→economy · `구내식당`→food · `고등학교`→education, and so on
- Wrong: **`토스팀`→`스팀`→game** (test:20)

Delete `스팀` and Steam becomes uncatchable in Korean text; narrow the rule to "suffixes of three characters or more"
and all 15 above are lost at once. **That is trading 15 correct matches away to remove 1 false positive**, so it was not made.
`조직문화`→culture cannot be separated by any matching rule, because Korean `문화` genuinely carries two senses.

#### Not done ② — cutting boilerplate out of the middle of a body

Notion's `광고 차단기로 인해 동영상이 재생되지 않는 것 같습니다` and the recipe's `동영상 조리순서` both produce a
`video` false positive (wild `video` precision 0.40). But **it is a different kind of thing from the footer rule** —
the footer test is an all-or-nothing verdict that throws away the **whole** body, whereas this means excising a phrase from
inside one. Per-site phrase lists rot, and a wrong cut deletes real content. The recipe's `동영상 조리순서`
sits on a page that really does have a video, so `video` is not even badly wrong.

**The gain is two entries, the risk taken is "one day it cuts real body text"** — not until more evidence accumulates.

### Re-capture (2026-07-27) — measuring the fixes and site drift apart

After the five defects were fixed, `wild.jsonl` was fetched again through the production path. **A re-capture moves the
numbers by itself** — sites change in the meantime. To keep that out of our own effect, the same URLs were scraped
**once more at the same moment with the pre-fix code**, giving a three-way comparison (then-old-code / now-old-code / now-new-code).

**① Site drift: 8 of 30.** Nothing to do with us. vimeo's body shrank sharply, 953 → 206 characters
(a site redesign); the rest are minor.

**② Our fixes' effect: 4 entries, all of them intended. Zero unintended changes.**

| | Old code (now) | New code (now) | Defect |
|---|---|---|---|
| news.naver.com | `���̹� ����`, 10,850 chars | **`네이버 뉴스`, 15,045 chars** | F |
| tistory.com | body 544 chars (site footer) | **0** | B |
| melon.com | body 355 chars (site footer) | **0** | B |
| imdb · reddit | wall stored as body | **capture fails** | A · C |

#### imdb and Reddit dropped from the set — 30 → 28 entries

Capture now fails on both with `ErrBlockedPage`. The convention is that golden holds **only what came through the
production scrape path** ("How entries are collected", in this document), and that path now legitimately refuses them. Writing a snapshot
in by hand would break the convention, so it was not done.

**There is a way back — client capture.** The browser extension (`extension/src/extract.js`)
already sends `{url, title, description, body_text, keywords}`, the API accepts it, and the queue leaves a link at
`done` when `body_source='client'`. Verified on the server path: post a 472-character client body and you get
`status=done · body_source=client · tags dev,llm`.
**This is not breaking through a block; it is saving what the user is already looking at**, and imdb, Reddit and threads
all resolve through the one path. A snapshot fetched that way is eligible for golden.

#### How to read the numbers honestly

| | Before re-capture | After re-capture |
|---|---|---|
| Entries | 30 | 28 |
| hit | 23 | **23** |
| Recall@3 | 0.767 | 0.821 |

**The numerator is unchanged.** 0.767 → 0.821 **rose because the denominator shrank**, not because anything new was
got right. Entry by entry:

- **Naver News 0→1** — `[ai]` → `[news culture economy]`. The direct effect of the F fix.
- **Notion 1→0** — `[video tutorial]` → `[video]`. **Site drift** (body 866 → 904 characters).
  Scraping now with the old code gives the same thing, so it is not down to our fix.
- **Melon** `[culture sports]` → `[culture]`, **Tistory** `[article life food]` → `[article]`.
  Both keep their hit, but **the false positives are gone.**

That last line is the real gain of this work, and **Recall@3 cannot see it.** It shows up in the per-tag table,
in the tags predicted while `golden=0` — of the pre-fix `javascript`, `sports`, `realestate` and `book`,
**`javascript` and `sports` are gone.** `economy` and `politics` came in instead, the result of Naver News becoming
readable and bringing in the content of a ranking page that genuinely spans several fields. The label `[news]` may be
too narrow, but **labels do not get edited to make the numbers look better.**

## test expansion 61 → 84 (2026-07-27)

The starting point of this whole exercise was **"the frozen 61-entry test is skewed to developer blogs and is not
representative"**. With the five defects fixed and the re-capture done, that is what gets handled now.

### What was missing

Count the 61 entries against the dictionary's 42 tags and **12 of them never appear** — `sports` `football`
`politics` `economy` `realestate` `health` `food` `culture` `game` `education` `startup` `law`.
The top of the list is `article×27` `dev×15` `tutorial×14`, and there is **1 social/video entry and 0 commerce**.

### What they were selected on

**Tag coverage was not the criterion.** That is precisely the criterion that turned dev and test into 123
developer-blog entries ("How entries are collected", in this document). Selection went by **kind of domain**, letting tags follow —
communities, courses, film press, startup media, gaming webzines, sports, health, real estate, app stores, legal press,
travelogues, wikis, restaurant reviews. 30 URLs went through the production path, 28 captured, and 23 of those stayed:

- 2 were 404s (a wrong App Store ID, a deleted Brunch post)
- 1 had **a snapshot unrelated to the link** — `youtube.com/watch?v=jNQXAC9IVRw` had the right title, but a
  description belonging to an entirely different video. Dropped for the same reason as threads
- 1 was **a duplicate** — two Inven URLs with identical title and description
- 2 were surplus travel — six of them would have been 23%, so it was cut back to four

Tags filled: `realestate` `health` `food` `culture` `game` `education` `startup` `law`
`sports` `football`. `politics` and `economy` were **not forced in** — holding to the kind-of-domain
criterion, those two simply did not come up, and the moment a URL is picked to satisfy coverage the original mistake
repeats itself.

### The result — the numbers go down, and that is the point

| | 61 entries | 84 entries |
|---|---|---|
| Recall@3 | 0.902 | **0.881** |
| baseline (domain only) | 0.344 | **0.250** |
| Δrules | +0.557 | **+0.631** |
| Korean / English | 0.971 / 0.815 | 0.929 / 0.786 |

**−2.1pp.** The thesis of this whole exercise was that adding a representative batch makes the numbers go down, and it
came out exactly that way. The domain baseline falling further, 0.344 → 0.250, is the same story —
`nlu/dictionary/domains.json` is developer-blog-centric and barely covers the new domains.

### The grounds for saying it is uncontaminated

- Labels were written **from reading the captured content.** The tagger was never run on these 23 entries and no
  per-entry prediction was looked at. The table above is **the first and only measurement of this batch.**
- No tuning followed the addition. Dictionary, weights and matching rules are all untouched.
- The same grounds and the same convention as the 2026-07-25 expansion (23 entries added).

**Every earlier gate calculation is against 61 entries.** Figures like "54/61 = 0.885" and
"reranking ceiling 55/61" in `docs/v2/en/08` §M5 are records of that moment and are not compared directly with the 84-entry set.

### Why this set is not a gate

**It is kept at the same grade as dev.** Building it meant rummaging through the defects above entry by entry, so fixing
against it and then measuring against it would be marking your own work. Gate verdicts continue to be taken on frozen test.

**Growing test is itself necessary** — that the 61 entries are skewed to developer blogs and are not representative is the
conclusion of this measurement. But what goes in when it grows has to be a new batch **whose individual entries have not
been inspected**, and that batch gets drawn after these defects are fixed. Reverse the order and "fixed" becomes unmeasurable.

## Measurement record (2026-07-25 — after the body-extraction bug fix and the platform expansion)

`just eval` (tagging):

| Set | Recall@3 full | no-body (Δbody) | baseline, domain only (Δrules) | Korean | English |
|---|---|---|---|---|---|
| dev (62)  | 0.952 | 0.806 (+0.145) | 0.355 (+0.597) | **1.000** | 0.912 |
| test (61) | 0.918 | 0.770 (+0.148) | 0.344 (+0.574) | **1.000** | 0.815 |

> **The `no-body` in this table is today's `bare`.** When `keywords` entered the schema on 2026-07-26,
> `no-body` split into `no-body` (classification present) and `bare` (no classification either). Compare it against
> the table in the section above (dev 0.871) on the strength of a shared column name and **a changed definition reads as a +6.5pp improvement.**

`just eval-summary` (summarization, TextRank vs the lead-3 baseline):

| Set | Coverage | desc overlap | intra overlap | Tag retention | Determinism | Body extraction |
|---|---|---|---|---|---|---|
| dev  | 0.855 (lead 0.919) | **0.125** (lead 0.397) | 0.208 (lead 0.289) | **0.868** (lead 0.830) | 1.000 | 59/62 |
| test | 0.885 (lead 0.951) | **0.102** (lead 0.342) | 0.244 (lead 0.254) | 0.815 (lead 0.815) | 1.000 | 60/61 |

**④ Tag retention's denominator is the entries where both systems produced a summary.** Divide by the total instead and
guardless lead-3 has structurally higher coverage, so the side that has guards is penalised for having them (that is the axis
① measures). With the denominator corrected, TextRank leads on dev (0.868 vs 0.830).

Performance: `BenchmarkSummarize` on a 32KB body is **1.5ms per document** (645KB alloc) — it runs in the asynchronous tag job, so it has nothing to do with the save p99 gate.

**The previous measurement (morning of 2026-07-25, with the body-extraction bug in place)**: tagging dev 0.900 / test 0.880, summary coverage dev 0.740 / test 0.660, body extraction 78/100. Korean tech blogs (Kakaobank, Toss, Brunch, velog and the like) came back with a 0B body on `transform: short internal buffer`; passing trafilatura the DOM goquery had already parsed took **bodies obtained from 78 to 96**, Korean tagging Recall@3 from 0.895 to **1.000**, and Korean summary coverage from 0.44~0.58 to **0.90~0.92**. This bug failed no test at all and was found **while spot-checking by reading the content directly** — which is why the `body extraction` metric is in eval.

- **The rule tagger beats the domain baseline by +52pp.** That margin is an inflated way of putting it, though — a constant predictor that looks at no content, `{article, tutorial, dev}`, already scores test 0.721, and Phase A's real paired-sample advantage is **+16.4pp** (McNemar p=0.0063). The M5 entry gate was redefined on that basis (02-TECH-SPEC.md).
- **The body (body_text) raises Recall@3 by +12~14pp** — the measured gain from body extraction.
- Tuning room (observed on dev, for later): `data` (the alias `데이터` over-matches in Korean text) and `backend` have low precision, and low-frequency tags go undetected. Cleaning up dictionary aliases needs a new migration, so it is separate work.
- The 4 entries still without a body are the **medium.com (SPA + bot blocking)** family — no pure-Go path reaches them. The fix is not headless but **client capture** (a bookmarklet or Share Extension in which the user's logged-in browser sends the body along), and it is a roadmap item. Login walls and paywalled articles resolve through the same path.

## Is widening the domain map worth it — no (measured 2026-07-27)

Growing test to 84 entries dropped `baseline (domain only)` from **0.344 to 0.250**. It is true that the 40-entry domain map is
developer-blog-centric and does not cover the community, commerce and travel hosts that came in, and in fact
**53~64% of golden's hosts are absent from the mapping** (dev 33/62 · test 50/84 · wild 18/28).

**So "widen the domain map" looked like the next task — and measuring says otherwise.**

`baseline (domain only)` is how much the domain gets right **on its own.** The question that actually matters is different —
with title, description and body all present, how much does **the domain add on top**? That variant was not in eval, so
`no-domain` (full − domain) was added:

| | baseline (domain only) | **Δdomain (marginal contribution)** |
|---|---|---|
| dev (62) | 0.355 | **+0.048** (3 entries) |
| test (84) | 0.250 | **+0.024** (2 entries) |
| wild (28) | 0.321 | **+0.000** (0 entries) |

**More than half the hosts are missing from the mapping, and the marginal contribution is 2~3 entries — on wild it is exactly zero.**
It means the rest of the signal is already saying the same thing. That is the ceiling on what adding domain rules can buy,
and on top of it **choosing which domains to add by looking at golden's misses is fitting to golden**, which
takes on risk as well.

**This is why the two metrics have to be read together.** Look at baseline alone and it reads as "the domain map
collapsed" — which is how it was read. Only after measuring Δdomain did it become clear that the fall is **a fall in a
comparison construct**, not a loss in the predictor that ships.

## Reinforcing dictionary aliases — by taking dev's "zero-scored correct tags" apart (2026-07-27)

Most misses are **a correct tag scoring zero** (dev 3/4 · test 9/10 · wild 5/5). That makes it a vocabulary
problem rather than a ranking one, so dev's three zero-scored misses were taken apart one at a time with the real tokenizer.

| dev | Diagnosis | Kind |
|---|---|---|
| 48 `blog.cloudflare.com` DDoS | `security` **0 occurrences**; instead `ddos`×26 `mitigation`×12 `attack`×6 `firewall`×5 | **Dictionary gap** |
| 37 `jamesclear.com/habit-guide` | `habit`×11 is the centre of the piece, but the dictionary holds no such concept | **Dictionary gap** |
| 11 `etcd.io/docs/` | The snapshot is a version-list page (body 201 chars, prediction `[]`) | **golden quality** |

That `life` appears once in 37 and still scores zero is **by design** — the body-length-proportional requirement is
`1 + 4821/2000 = 3` occurrences, so a single one is filtered out. A 4,800-character piece should not be tagged on one appearance of "life".

### What went in and what was deliberately kept out

`security` + `ddos` `firewall` `방화벽` `vulnerability` `취약점` `encryption` `암호화`
`productivity` + `습관` `habit` `시간관리` (migration `0011`)

**What was kept out** looks conceptually right but produces false positives in this corpus:
`attack` and `공격` (the '공격수' of football coverage) · `mitigation` (climate mitigation) · `routine` (a 'routine' in code).

### The result

| Set | Before | After |
|---|---|---|
| dev (62) | 0.935 | **0.968** (+3.2pp, misses 4→2) |
| **test (84)** | 0.881 | **0.893** (+1.2pp, misses 10→9) |
| wild (28) | 0.821 | 0.821 |

Entry by entry that is **3 improved, 0 regressed**. And the third improvement matters —
**`test:48` is a different Cloudflare piece I had never once looked at** (`defending-the-internet`), and `security`
attached to it, turning it into a hit. The aliases were not fitted to the entries that had been diagnosed; **the frozen set
confirms they fit the concept.** That is the opposite direction from the precedent where a dictionary expansion cost frozen test 1.7pp (E1).

The two entries whose tags changed without their hit changing were improvements too: `dev:55`, a minimalist-living post, gave up
`travel` and gained `productivity` (`travel` had been a false positive), and `dev:6`, an Atomic Habits summary, newly got
the labelled `productivity` right. **Recall@3 sees neither.**

### What was left alone — `etcd.io/docs/`

The dictionary cannot fix it. The label `[database, opensource]` says **what etcd is**, while
the snapshot is a version-list page. At 201 characters of body it clears the thin-signal threshold (200 characters) by a hair, so it is not even flagged.
**The reason it was not fixed in the same pass** is that mixing a set change and a dictionary change into one commit makes it
impossible to tell which of them the measurement above is due to.

## dev is saturated (2026-07-27)

`etcd.io/docs/` was replaced with a page on the same site that has content. The label `[database, opensource]` is
unchanged — the label was not wrong; **the snapshot was a version-list page** and had nothing to give
the tagger (body 201 chars, prediction `[]`). It cleared the 200-character thin-signal threshold by a hair, so it was never flagged.

| | Before | After |
|---|---|---|
| URL | `etcd.io/docs/` | `etcd.io/docs/v3.5/learning/why/` |
| Body | 201 chars | 13,162 chars |
| Label | `[database, opensource]` | Unchanged |

**Result: dev 0.968 → 0.984, 1 miss, and 0 of those zero-scored.**

The one remaining miss is an **alphabetical tie** on `nomadcoders.co/wetube` (`dev`, `education` and `tutorial` in a
0.8333 three-way tie, recorded in PR #41). That is, **dev offers exactly one entry's worth of signal observable by fixing
rules or the dictionary**, and even that one is a tie-break — a spot that rearranges wholesale the moment weights are touched.

Ground (c) for cancelling the original plan in planning document 08 §M5 was "the tuning set is saturated — there are three
entries of observable signal", and **now there is one.** Creating tuning room again means growing dev, and nothing else.
test and wild are for verdicts and cannot be tuned against.

## dev expansion 62 → 77 (2026-07-27) — the only way out of saturation

The section directly above recorded dev as saturated (1 miss, and that one a tie). The only way to create tuning room is
to grow dev — test and wild are for verdicts and cannot be tuned against.

**dev carried exactly the same skew as test**: 12 of the 42 tags never appeared (`sports` `football`
`politics` `economy` `realestate` `health` `food` `culture` `game` `education` `startup` `law`),
0 social/video, 0 commerce, 0 community, 0 travel/food. The top of the list was `article×26` `tutorial×14` `dev×13`.

The grounds for this choice are that **a tuning set has to share the distribution of the set that renders verdicts.**
Steer with dev and judge with test, and if the two distributions differ the steering points somewhere else entirely. So selection used
**the same criterion as the test expansion (kind of domain)**.

25 URLs → 18 captured → **15 adopted**. The reasons for the drops are worth recording in themselves:

- **FM Korea, 5 entries** — all blocked with `430`. The community axis got that much thinner
- **MangoPlate, 2 entries** — `no such host`. **The service is gone** (founded in 2013, no DNS today)
- **Brunch, 1 entry** — the error page `brunch 서비스 접속이 원활하지 않습니다` (125 characters)
- **Wikipedia, 2 entries** — six would have made one host over-represented, so it was cut back (four in the end)

### The result — what surfaced is a ranking problem, not a vocabulary one

| | 62 entries | 77 entries |
|---|---|---|
| Recall@3 | 0.984 | 0.974 |
| Misses | 1 | 2 |
| **Zero-scored correct tag** | 0 | **0** |
| **Pushed down the ranking** | 1 | **2** |
| Reranking ceiling | 1.000 | **1.000 (+0.026)** |

**The 15 new entries opened no vocabulary gap at all** — zero-scored correct tags are still 0. It means the dictionary already
covers gaming, real estate, law, health and sports, and what is left is entirely a **ranking** problem.

Both misses are pure alphabetical ties:

- `nomadcoders.co/wetube` — `dev`, `education` and `tutorial` in a **0.8333 three-way tie**, with the correct `tutorial` ranked fourth
- `ruliweb.com` — `ai`, `book`, `culture`, `finance` and `game` **all five at exactly 0.75**, with the correct `game` ranked fifth

**The second is textbook.** The correct tag scores exactly what four wrong ones score and loses on alphabetical order alone.
It is a homepage, so several topics get mentioned once each, all clear the minimum count and all end up at the same score.

This is signal that can answer the very question M5 meant to ask — whether an ensemble is any use for reranking.
**It is the kind that adding vocabulary cannot produce**, and it is what growing dev actually bought.

## Tie-breaking moved from the alphabet to evidence (2026-07-27)

dev's two remaining misses were both pure alphabetical ties. Digging into the cause: **the cap was deciding the
ranking.**

Raw match counts per tag in `ruliweb.com`'s body (8,946 characters):

| Tag | Raw matches | After `capN` |
|---|---|---|
| **`game`** | **45** | 3 |
| `ai` | 8 | 3 |
| `book` | 7 | 3 |
| `culture` | 7 | 3 |
| `finance` | 4 | 3 |

**`game` occurs 45 times and scores the same as `finance`, which occurs 4.** `capN` is the device that stops keyword
stuffing from **inflating a score**, but when several tags hit the cap their scores all become equal and
**the cap ends up deciding who is above whom** — or rather, the alphabet ends up deciding. That was never the cap's job.

**How it was fixed**: ties are broken on the **uncapped score** (the sort in `classify.go`). It is the same weights and the same
structure with only `capN` removed — **a number that was already being computed and then thrown away.** The primary score is untouched,
so stuffing protection stands, and the order of a pair that is not tied can never change.

| Set | Before | After |
|---|---|---|
| dev (77) | 0.974 | **1.000** |
| test (84) | 0.893 | **0.905** |
| wild (28) | 0.821 | 0.821 |

**3 improved, 0 regressed.** But those 3 are all Recall@3 sees, and **65 of the 189 entries (34%) had their
top-3 change.** The user sees different tags on a third of their links:

- `ruliweb.com` — `[ai book culture]` → **`[game sports politics]`**
- `namu.wiki/직방` — `[dev game law]` → **`[realestate dev game]`**

The `boundary ties` metric keeps being reported. It is now split by evidence rather than the alphabet, but **the fact that
the primary score could not separate those positions is unchanged** (and when the secondary key ties too, it goes back to the alphabet).

### dev 1.000 is a warning, not an achievement

**Reranking room is exhausted on both dev and test.**

| | Misses | Zero-scored correct tag | Reranking ceiling |
|---|---|---|---|
| dev (77) | 0 | 0 | — |
| test (84) | 8 | **8** | **+0.000** |
| wild (28) | 5 | **5** | **+0.000** |

**In all three sets every remaining miss is a zero-scored correct tag.** No amount of reordering fixes them, and the
reranking ceiling that ground (b) of 08 §M5 spoke of is now genuinely zero. There is nothing an ensemble can win
through reranking — what is left is **promoting a zero-scored tag above the threshold**, and that is a different risk: a flood of
false positives.

And with dev at 1.000, **the tuning signal is back to zero.** One commit after growing it 62 → 77.
Touching the rules again means growing dev again, and this time **links selected the current way will not be
enough** — 15 went in and not one vocabulary gap came out.

## Search quality — `search.jsonl` + `just eval-search` (2026-07-27)

Tagging has a three-set evaluation; **search had nothing.** The performance targets table
says "search (FTS5, 10k links) < 30ms", and the repo did not even hold a command that measures it. Touch
search in that state and you get an unmeasured quality change and an unmeasured performance regression at once.

**The corpus is the 189 golden snapshots.** The reason no separate fixture was built is the same as for the tagging eval —
it is committed, so it reproduces regardless of when it runs, and its content is real pages fetched through the production scrape path.
It is loaded into a temporary DB with the migrations applied, so the `links_fts` index, bm25 ranking and the LIKE fallback all
run the same code as the runtime.

**None of the 25 queries types a title verbatim.** Copying a title is easy and only inflates the baseline. They are written
the way someone would actually type months later when looking for it again — remembering in Korean when the document is in English,
spacing it differently (`머신러닝` vs `머신 러닝`), forgetting the name and remembering only the content
(`웹 취약점 top 10` → OWASP), remembering only a phrase from the body.

### Baseline (**before** the AND retry was introduced)

| | |
|---|---|
| **hit@1** | **0.360** (9/25) |
| **MRR@10** | **0.413** |
| Reached the top 10 | 0.480 (12/25) |

**That is low. It is why this harness was built** — "search seems to work fine" was in fact this number.

### The biggest cause: a space is an AND

`ftsMatchQuery` (`sqlite_search.go`) joins the tokens with spaces, and **in FTS5 a space is an implicit AND**.
One word missing from the index makes **the whole result set zero**.

Most of the 12 not-found queries are this:

- `웹 취약점 top 10` → `top` and `10` are in the OWASP title, but `취약점` is not, so the whole thing dies
- `판다스 10분 입문` → the title is `10 minutes to pandas`, so `판다스` is not in the index
- `쿠버네티스 하드웨이` → the title is `kubernetes-the-hard-way`; the transliteration does not line up

**The second cause is the language boundary.** Queries remembered in Korean whose document is in English are wiped out —
`고랭 제네릭 언제 쓰나`, `기후 변화 증거`, `허깅페이스 트랜스포머 입문`. trigram is a character
n-gram and cannot jump across a language.

**The third is that the body is not in the index.** Two queries were put in **knowing they would fail** —
`브랜치 예측 실패 정렬 배열` (branch prediction fail, in the body) and `돼지고기 앞다리살 김치찌개`
(the ingredients are only in the body). That is the gap backlog item B2 addresses, and its worth can now be measured.

### Retry with OR when AND comes back empty (2026-07-27)

**It was not switched to OR across the board.** If AND returns results they stand; **only when it returns nothing** is the
question asked again with OR. That makes it impossible for the ranking of queries that already get found to shift — the intervention itself
is confined to the zero-result case.

| | Before | After |
|---|---|---|
| hit@1 | 0.360 | **0.440** |
| MRR@10 | 0.413 | **0.493** |
| Reached the top 10 | 0.480 | **0.560** |

Query by query that is **2 improved, 0 regressed**. The claimed property was confirmed by measurement — the 14 that already
worked kept even their ranking, and `etcd 다른 키값 저장소와 비교` and `웹 취약점 top 10` went from not-found to first.

### The LIKE fallback goes word by word too (2026-07-27)

When every token is under three characters the query cannot take FTS and falls to LIKE, and that pattern was searching for
**the whole query as one lump** — as in `%직방 다방 차이%`, the string had to be present **in one piece**. People type the
words they remember in order rather than the document's exact phrasing, so in that shape it almost never matches.

**This path gets used often in Korean.** A two-syllable word falls exactly inside the under-three-characters band, so
queries where everything is two characters, like `직방 다방 차이`, are common.

It was split word by word — **AND between words, and any one word may sit in any of the three fields.** `직방` in the title
and `비교` in the description still match even when scattered. That alone was not enough, though:

> The target title for `직방 다방 차이` is `직방, 다방, 네이버 부동산, 집토스 **뭐가 다를까?**`, so
> **the word `차이` is simply not there.** People remember by meaning and the document uses a different word.

So **the same convention as FTS** was applied — if the word-level AND comes back empty, retry with OR. The intervention is confined to the
zero-result case, so queries that already worked keep even their ranking.

| | AND retry only | + LIKE word split |
|---|---|---|
| hit@1 | 0.440 | 0.440 |
| MRR@10 | 0.493 | **0.507** |
| Reached the top 10 | 0.560 | **0.600** |

**1 improved, 0 regressed.** `직방 다방 차이` went from not-found to third. hit@1 not moving means it did not reach
first, and **that is an honest result** — the correct document does not contain `차이`, so its grounds for outranking the other
documents are that much weaker.

### The 10 still not found — there are three more causes

Take the ones the OR retry did not revive apart down to their tokens and they are **problems distinct from `①`**.

**(a) Tokens under three characters are discarded.** The rule comes from the trigram index requiring three characters or more, but in Korean
a two-syllable word is the key word:

| Query | Tokens left for FTS |
|---|---|
| `고랭 제네릭 언제 쓰나` | `제네릭` (고랭·언제·쓰나 discarded) |
| `머신러닝 강의` | `머신러닝` (강의 discarded) |
| `직방 다방 차이` | **None** — all three are two characters, so FTS is skipped entirely |

**(b) The LIKE fallback searches for the query as one lump.** — **fixed 2026-07-27** (section above).
When a query like `직방 다방 차이` cannot take FTS it falls to LIKE, and the pattern was `%직방 다방 차이%`, so **the string had to be
present in one piece**.

**(c) The language boundary.** Remembered in Korean, document in English, and the query is wiped out — trigram is a character n-gram and cannot cross
between `머신러닝` and `machine learning`. This axis cannot be solved by an indexing scheme.

The reason the three were not fixed together this time is that **the measurements would not separate.** (a) and (b) have to
be measured apart from each other to know what each one bought.

### The 10 remaining not-found queries, classified — eight-tenths are the language boundary (2026-07-27)

After the two cheap fixes (the FTS AND retry and the LIKE word split), the remaining 10 were classified **before** fixing
anything. "(a) rescue two-character tokens" had been pencilled in as the next candidate, and measuring overturned that judgement.

| Cause | Count |
|---|---|
| **Language boundary** — Korean query → English document | **8** |
| Only in the body | 1 |
| A two-character token discarded | 1 |

**(a) rescues only 1 of the 10** (`토스 리액트 네이티브` — the title is `토스가 꿈꾸는 React Native`, so
`토스` really is there but gets discarded for being two characters). And it is **the most complicated of the three** — trigram demands three
characters, so FTS and LIKE have to be mixed. Highest cost, smallest gain.

**(d) indexing the body** has a potential size of 2 out of 25 as well.

**No indexing scheme solves those 8.** trigram and LIKE are both character matching and cannot cross between `머신러닝` and
`machine learning`, or `쿠버네티스` and `kubernetes`. Growing the dictionary does not do it either —
the dictionary exists for the 42 tags, not to link arbitrary pairs of words.

#### M5 embeddings are worth more in search than in tagging

This is an observation that changes the plan. In tagging, **reranking room was confirmed to be zero on all three sets** —
every remaining miss has the correct tag genuinely at zero, so reordering wins nothing. For embeddings to earn their place
there they would have to **promote** a zero-scored tag, which takes on the opposite risk: a flood of false positives.

Search is different. **Those 8 not-found queries are exactly the problem a multilingual embedding is good at** —
linking things that mean the same and are written differently. And search carries no promotion risk: it adds candidates to a
query that returned nothing, so **there is structurally no room for it to get worse** (the same logic the two fixes above used).

That is, the discard criterion for M5 Phase 1 (the offline embedding spike) **must not be decided on tagging alone.**
The same model can be useless for tagging and worth having for search.

### The freeze rule is the same as for tagging

**Fixing search to match golden is allowed; fixing golden to match search results is forbidden.**
Rewriting a query into "something that currently gets found" to raise the number makes this set meaningless.

## Baselines and relative gates

- eval always measures the "domain heuristics only" baseline configuration alongside.
- The gates are relative conditions: entering M5 = Phase A is significant on a paired sample against the constant predictor (McNemar p<0.05); exiting M5 = the ensemble has 0 regressions and 5 or more improvements against Phase A. **One entry = 1.64pp**, and the smallest improvement distinguishable from chance is five entries, so the gates are written in entries rather than percentages.

## The client-capture path — `pushpoint golden-from-db` (M5 Phase 0 ③)

`golden-capture` fetches through the production **scrape** path. That path legitimately refuses bot walls and login walls
(`ErrBlockedPage`), so **a page the server cannot fetch structurally cannot enter golden.**
The body of that class of page (imdb, Reddit, threads and the like) can only be obtained by the user's **logged-in browser**, and
the extension (`extension/`) already sends it that way (`body_source='client'`).

- `pushpoint golden-from-db > candidates.jsonl` — pulls the saved links whose `body_source='client'` as
  golden candidates. `expected_tags` comes out **empty** (a label has to be written by a person reading the content,
  and the tags already on a link are tagger output, so copying them would breach the freeze rule). The current
  tags are emitted alongside in `_current_tags` for reference.
- Entries with a body under 200 characters are skipped (walls and empty captures can come in through the client path too).
- An entry goes into golden only after a person has filled in `expected_tags`. `dict-lint` rejects an empty label, so
  nothing gets committed unlabelled.

**Measurement**: when `body_source` is present, `just eval` reports **the client path separately** —
`클라이언트 캡처 N건 Recall@3=… · 서버 M건 …`. When there are no such entries it says so
**explicitly**: `클라이언트 캡처 0건 — … 아직 미측정(Phase 0 ③)`. The point is to stop silence being read as
"measured, no problem", and the fact that this path had never once been measured is itself the finding of Phase 0 ③.

**Still open**: golden holds 0 actual client-capture entries. It closes in this order — the user saves a
bot-blocked or login-walled page with the extension → `golden-from-db` → labelling → a second golden pass.
The server-side pipeline (API reception, the queue holding `done`, from-db extraction) is verified.

## How entries are collected — `pushpoint golden-capture`

- It runs an input TSV (one line of `url<TAB>tag,tag,tag`) through the **production scrape path** and emits JSONL with the snapshot filled in:
  `go run ./cmd/pushpoint golden-capture urls.tsv > out.jsonl`. This is what forces golden == runtime.
- URLs and `expected_tags`: real, representative URLs that evenly cover the dictionary's 30 tags are curated (Korean 40 / English 60), and
  `expected_tags` is labelled independently as **the dictionary tags that actually fit the content**, not as tagger output (the freeze rule).
- Stratified sampling that preserves the domain and content_type proportions.
- **Representativeness is not judged by "does it evenly cover the dictionary tags".** dev and test were built on that criterion and
  the result was 123 developer-blog entries (see the wild section). The kind of domain — whether social, commerce, app stores and communities
  are in there — is looked at alongside.
- **The URL has to exist.** The wild draft used made-up IDs like `airbnb.com/rooms/1234567`, and because
  a blocking site returns the same empty snapshot for a 404 as for a 403, **there was no way to tell it was fake.** The more a domain blocks,
  the more its existence needs checking.

## Measurement record

| Date | Dictionary | dev Recall@3 | test Recall@3 | Note |
|---|---|---|---|---|
| 2026-07-26 | 42 tags | 0.952 | **0.885** | Current |
| — | 30 tags | 0.952 | 0.902 | Same golden, dictionary alone reverted (isolating the variable) |

**A dictionary expansion produced a 1.7pp regression on frozen test, and that fact went unrecorded for three days.**
`fa18e5e` (#30) grew `nlu/dictionary/tags.json` from 30 to 42 tags (+450 lines) without re-running `just eval`.
dev stayed at 0.952, so had only dev been looked at nothing would appear to have happened — **this is the kind of regression
frozen test exists to catch, and test is in fact the only thing that moved.**

The cause has not been established. More tags means stiffer competition for the top three, so a new tag most likely pushed an
existing correct answer out of third place, but which tag pushed which entry was never counted.
So as not to write a guess down as a fact, the record stops here.

**The rule to take from this**: any change touching `nlu/dictionary/` attaches the `just eval` output to the PR.
A dictionary expansion looks like a pure addition, which is why nobody suspects a regression — and measuring says otherwise.

## Unicode normalization (2026-07-26)

`Normalize()` performs NFC composition first. The difference against not having it:

| Input | dev | test |
|---|---|---|
| NFC (the usual) | 0.952 | 0.885 |
| **NFD (no normalization)** | **0.710** | **0.689** |
| NFD (with normalization) | 0.952 | 0.885 |

Hangul writes "한" both as the single character U+D55C and as the three code points `ᄒ+ᅡ+ᆫ`. Identical to the eye, different
in bytes. Without normalization **a third of Korean tagging disappears with no error and no log** — only what was
saved that day is left with no tags. macOS filenames are NFD, and some web forms, the clipboard and the iOS share
path pass it straight through.

It sits in `Normalize()` because that is the only gate **both dictionary matching and `corpus_df` accumulation pass
through.** Normalize only one side and the accumulation key diverges from the lookup key, leaving df silently at zero.

## Two things the metrics could not see (2026-07-26, added to `just eval`)

These were structurally invisible for as long as `just eval` reported Recall@3 and nothing else.

Measured on **2026-07-27 (after the defect E fix)**, with the pre-fix figures in parentheses:

| | dev | test | wild |
|---|---|---|---|
| **Boundary ties** (third and fourth score the same, so they split on the tag name's alphabetical order) | 29/62 = 47% (30/62) | 21/61 = 34% (22/61) | 7/30 = 23% |
| Misses — **correct tag scored zero** | 3 (3) | 6 (6) | 7 |
| Misses — pushed down the ranking | **1** (0) | **0** (1) | 0 |
| **Reranking ceiling** | 0.952 **(+0.016)** | 0.902 **(+0.000)** | 0.767 (+0.000) |

**dev and test swapped places.** The defect E fix turned test's one pushed-down entry into a hit and thereby **exhausted**
its reranking room (+1.6pp → +0.000), while creating dev's first pushed-down entry
(`nomadcoders.co/wetube` — the "Fixing defect E" section below). It means which set still holds room for
reranking-class improvements flipped outright.

**Boundary ties** are what hit@3 cannot see in principle — anything inside the top three passes. Yet touch the weights
and this whole block rearranges while Recall@3 barely moves. That is why E1 (the dictionary expansion that cost frozen
test 1.7pp) stayed invisible for three days.

**Dissecting the misses** separates which improvements can work from which cannot. A zero-scored miss cannot be fixed by any
amount of reordering; the tag has to be **promoted** above the threshold, and that is a different risk: a flood of false positives.
**The side with nothing left to win from reranking is now test** (all 6 misses zero-scored) — reranking-class improvements
produce no signal at all on dev.

## B1 gate measurement (2026-07-26)

The condition for starting B1 (summaries into the FTS index) in [12-BACKLOG.md](../../docs/v2/en/12-BACKLOG.md) was **"at least 30% of
links gain one or more 3-grams that are in the summary only and not in title, description or tags"**.

| Item | Value |
|---|---|
| Scope | golden 123 entries (dev 62 + test 61) |
| No summary | 16 entries |
| **Links gaining a 3-gram found only in the summary** | **107 entries = 87.0%** |
| Gate at 30% | **Pass** |

The count is in 3-grams because the FTS5 trigram tokenizer indexes in that unit — the size of a new search surface only
means something measured in the unit the actual search uses. 87% means the summary really does bring in vocabulary that title and
description could not carry, **on most links**, and B1 has its grounds to start.

## Adding a field — `pushpoint golden-refill`

Once the tagger starts using a new field, a golden without that field **cannot measure the improvement** (Δ is always 0).
But re-fetching everything with `golden-capture` shakes `title` and `body_text` as well, because pages changed in the meantime, and
**the comparison with earlier measurements breaks** — which is especially unacceptable for test, a frozen set.

`golden-refill` re-fetches but fills **only the empty fields.** A field that already has a value is left alone:

```
go run ./cmd/pushpoint golden-refill nlu/golden/dev.jsonl > /tmp/dev.jsonl
```

- An entry whose fetch fails passes through unchanged — losing an entry because a URL died would silently shrink the set and make metric comparison impossible.
- When `keywords` was added on 2026-07-26 this is the tool that actually filled it, and the commit came **after confirming that not one other field had changed** (dev 18/62 and test 23/61 filled, 0 fetch failures).

## B1 start-gate measurement (2026-07-31)

This is the number 12 §4 demanded before any code — **"the share of links gaining one or more 3-grams that are in the
summary only and not in title, description, note or tags"** — and under 30% folds both B1 and B2.

```
just b1-gate

  dev    77건 · 요약 있음  66 (85.7%) · 새 3-gram 얻음  66 (85.7%)
  test   84건 · 요약 있음  71 (84.5%) · 새 3-gram 얻음  71 (84.5%)
  wild   28건 · 요약 있음  17 (60.7%) · 새 3-gram 얻음  17 (60.7%)

  전체 189건 · 요약 154 (81.5%) · 획득 154 (81.5%)   ← 게이트 30%
  획득한 링크의 3-gram 수: 중앙값 215, 최소 59, 최대 356
```

**It passes. But this number must not be read at face value.**

The 154 that gained and the 154 that have a summary are **the same number.** That is, a link with a summary gains a new
3-gram **without exception** — structurally inevitable, since the summary is extracted from `body_text`, which is not indexed.
So this gate looks like it measures "does the summary add value to the index" when in fact it
**reduces to "does a summary come out at all".** The part with discriminating power is not the trailing figure (81.5%) but
the 18.5% where `Summarize` returns an empty string.

The gate still serves its purpose — what was asked was "is there a new search surface", and the answer is yes.
It could have been no: had the summary been an implementation that repeats the description, the overlap would have been large enough to fail, and
the last of `Summarize`'s five guards is exactly what prevents that duplication.

**wild being low at 60.7% says more.** On the web outside developer blogs the body is thin or short of prose,
so no summary comes out at all. B1's value is smallest on that set, and that is the same kind of fact as the reason this project
keeps `wild` separate.

### An aside: size against the B2 candidate

Measuring the first 2KB of the body the same way (what B2 wants to index) gives **a median of 875 new 3-grams** —
about four times the summary's 215. By surface size alone B2 is bigger, but the summary is **selected sentences** while
the first 2KB is **a truncated lump** that includes navigation and boilerplate. Size is not search quality, so
this comparison does not rank them — it only records that the two candidates compete on different axes.
