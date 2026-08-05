# 14. Stats Screen Redesign Plan

> Push-Point v2.1 — last updated: 2026-07-29

Written 2026-07-29 · Status: plan (not yet built) · Target iOS `StatsView` + the web rhythm section = **axis ①** (13 §1)

---

## 0. Summary

This redesign is **mostly subtraction.**

The screen says four things to the person reading it. **Two of them are not supported by the
data**, one carries a label that lies, and the last one **puts a permanent fact in the slot
reserved for a claim about recency.** All of that was confirmed by measurement, and the
arithmetic is written out below exactly as it came.

What survives the subtraction is five things: **streak · active days · the 30 day bars · the
tag inventory · the total.** All five are **counts of facts** rather than inferences, so they
are true regardless of sample size, and when they move, something really did happen. This
project already knew both of those — when the M6 completion verdict was handed to
`scripts/streak.sh`, what it picked was exactly streak and active days (08 §4).

The empty space is not backfilled with a new chart. **The sentence is raised to be the
protagonist of the screen and everything else drops beneath it as that sentence's evidence.**
This is not a new idea; 11 §8(3-1) settled it already, and the current implementation is a
sentence laid on top of a dashboard. That mismatch is what the user meant by "very awkward".

---

## 1. What is not true (measured)

### 1.1 "The weekday you save most on" holds at no save rate whatsoever

**30 days = 4 weeks + a 2-day tail.** So only two weekdays get 5 slots inside the window and
the other five get 4. Those two weekdays are **always today and yesterday**, and they rotate
by one slot a day.

Feed it a user who saves **exactly 2-a-day and never misses**:

```
2026-07-29 (수) counts=[8,8,10,10,8,8,8] → "화요일에 가장 많이"
2026-07-30 (목) counts=[8,8,8,10,10,8,8] → "수요일에 가장 많이"   뒤집힘
2026-07-31 (금) counts=[8,8,8,8,10,10,8] → "목요일에 가장 많이"   뒤집힘
2026-08-01 (토) counts=[8,8,8,8,8,10,10] → "금요일에 가장 많이"   뒤집힘
```

**A user whose variance is 0 is told a different answer every day.** The maximum is a 100%
tie, and ties get broken toward Sunday by `indexOf`/`firstIndex(of:)`. At real saving volumes
(1~3 a day) the weekday that gets named is **"today or yesterday" 72.2% of the time** (chance
would be 28.6%), and **27.0%** of renders are decided by the tie-breaking rule rather than by
the data. — the first draft wrote those two as 85.9% and 27.2%. Re-running them while
committing the generator did not produce them, and **they were corrected to what the script
prints.** 85.9% (=6/7) was the figure for a "perfectly regular user", and it had been mixed
in as though it belonged to the 1~3-a-day user.

The significance side is closed too. To reject H0 (weekday makes no difference) with a
multinomial maximum test you need:

| N in the window | Expected slot | Max needed | vs. flat |
|---|---|---|---|
| 30 | 4.3 | 9 | +110% |
| 60 | 8.6 | 16 | +87% |
| 120 | 17.1 | 27 | +57% |

Actually detecting a 2x preference needs **N ≥ 120**, and **the 30 day window structurally
caps N at 30 × rate (30~90).** So this claim cannot reach its own significance bar without
widening the window to a 60~120-day span. An outside standard says the same — Washington
State's small-number reporting standard treats **counts of 16 or below as carrying relative
standard error above 25%** and asks for a caution flag, and every weekday bar sits under that.

### 1.2 "N more than last week" fires even when behaviour has not changed

With daily saves X ~ U{1,2,3} (mean 2, variance 2/3), the standard deviation of the weekly sum
is 2.16 and **the standard deviation of the difference is 3.06**. So **the mean |difference|
is 2.41 even when the behaviour change is 0.** In a week when nothing at all happened, the
screen habitually says "2~3 more/fewer than last week".

The 95% noise band, on a base of 14, is **±6.0**, so the smallest real change a seven against
seven comparison can distinguish is **43%**. And this **does not get better by waiting** — the
window is a fixed 7 days on each side, so it stays put however large the archive grows.
Widening it to thirty against thirty brings it to 21%, and that is a different feature.

Measured day to day flipping: **the direction word changes 31.7% of the time** — the arrow
turns around once every three days.

### 1.3 "N this week" has the number right and the label false

`links_this_week` is the sum of the last seven slots of `by_day` — that is, **a rolling 7 days
ending today**. It is not "this week" on the calendar. On every day that is not Sunday the
label disagrees with the fact.

### 1.4 "Mostly you were drawn to 'X'" is true, but it is a permanent fact

`by_tag` has **no date condition** (`sqlite_search.go`). It is an all-time cumulative total.
Stability, measured with the real labels:

```
링크  10건: 문장이 바뀌는 날 13.20%
링크  50건: 1.70%
링크 100건: 0.47%
링크 300건: 0.00%
```

(This table too was written 1.54 / 0.29 / 0.02 / 0.00 in the first draft. The values above are
what came out of re-running it while committing the generator — **an order of magnitude
larger.** The conclusion does not change, but the strength of the evidence does.)

**At a 100-link archive it moves about once in a 200-day stretch.** The larger the archive
gets, the harder it sets. The problem is that this clause sits in the second position of a
paragraph that opened with "this week…", so **it reads as a claim about recency.**

iOS **has already made this exact call once.** Removing the facet-ratio donut, it wrote —
*"`by_tag` is cumulative over the whole period, so it has no time axis … what was left was
only a chart showing a barely-changing ratio, in internal taxonomy terms, reaching nowhere."*
(`StatsView.swift:298-304`) **The narrative sentence is still doing in one line what that
donut did.**

A side finding: the neutral-suppression branch of `dominantFacet` **never fires on the seed
dictionary alone** — the distribution of the forty-two tags is craft 19 / media 5 / life 18 /
**neutral 0**. It is not dead code, though: `neutral` is in the contract's `TagFacet` enum and
11 §8 defines it as one of the four options for facet selection, so **it switches on through
the normal path the moment a user-made tag comes first.** It also switches on when `GET /tags`
fails, and on that path a network error turns into **the sentence quietly disappearing.**

### 1.5 What is supported

| Item | Why it is safe |
|---|---|
| Streak | A count of facts, not an inference. If it moves, a day really was missed |
| Active days (of 30) | Same. It counts; it does not conclude |
| The 30 day bars | It shows data, it does not state a conclusion. `by_day`'s 30-slot density guarantee backs it |
| Tag groups | True as an **inventory**. It only has to not read as "lately" |
| Total | True. But see §2.2 |

---

## 2. Decisions

### D1. One sentence, and one time range, fixed

Today three clauses are strung together with conjunctions and **the time range changes inside
a single sentence** (this week → the last 30 days). The reader takes both to be about this
week.

The new sentence **speaks about the last 30 days only**, and uses just the two supported
numbers.

```
저장이 이어질 때   최근 30일 가운데 22일 저장했어요. 지금 8일째 이어가고 있어요.
연속이 끊겼을 때   최근 30일 가운데 22일 저장했어요. 마지막은 3일 전이에요.
아직 비었을 때     아직 아무것도 저장하지 않았어요.
```

- Both numbers are **counts of facts**, so they do not ride on sample size
- "Your last one was 3 days ago" **does not scold** — it states the break without demanding
  recovery. The failure mode self-tracking research keeps confirming is precisely the opposite
  (CHI 2016: 16.2% of activity-tracker users reported guilt after stopping, and the design
  implication was *"help resumption rather than notify about the absence of tracking"*)
- On hitting the 30 day cap it says "**30 days or more**", per the existing `cappedStreak`
  rule (H7)

### D2. Delete the weekday chart

The arithmetic above closed the impossibility. The options are delete or widen the window, and
**delete is the choice**:

- Widening the window to a 60~120-day span is a contract change, and the claim it buys is worth
  little — knowing "I save a lot on Tuesdays" **reaches nowhere.** It does not pass H3 (every
  item reaches somewhere)
- The question this screen answers is "is this habit alive", not "when do I do it"

### D3. Delete the week-over-week comparison

§1.2. Even widened, thirty against thirty tops out at 21%, and that is the re-entry of
"recent interests", which backlog §4.2 R4 already rejected. The verdict written then applies
unchanged — *"a gauge that always wobbles is a broken gauge, and in an app whose identity is
its dashboard that is worse than having none."*

`weekOverWeek` and its fixtures are deleted. The streak cases in `testdata/streak-cases.json`
stay.

### D4. Delete the facet sentence; the tag groups do that job

§1.4. Show an inventory in the inventory's place and it is true; promote it into the
sentence's place and it becomes a claim about recency. Under a group heading the tags that
justify it are actually attached, so the label never stands alone.

What gets deleted is one thing: **`dominantFacet` in `frontend/src/lib/rhythm.ts`.** Three
functions in the repo carry that name and **the other two must live** — the ones in
`frontend/src/lib/tags/facet.ts` and `ios/PushPoint/LinkCard.swift` decide the colour of
generated covers, and the cover-hash goldens pin them. And the iOS stats sentence has no
`dominantFacet` in it at all: it suppresses with `top.facet != .neutral` after
`groupedTags(s).max(by:)` (`StatsView.swift`). This is not "delete the same name in two
implementations" — **the web deletes a function, iOS deletes that expression.** The web/iOS
selection-order agreement 13 §3 aligned recently goes away with it, and **that is not a loss,
because the thing the agreement protected is what is going away.**

### D5. `일 바깥` → `세상과 일상`

`일 바깥` ("outside work") **defines itself as what it is not.** And the definition itself
already confesses it — the ground on which 10 §5.5 puts `productivity`/`career` among the
borderline cases is *"because it is not something you make"*.

The real reason the label is awkward is that **it holds two sets.** The 18-strong `life` set
splits into the world (news·politics·economy·finance·law·realestate·sports·science) and the
everyday (career·productivity·health·food·travel·education·culture·game). **Football is not
me, and a career is not outside work.** `세상과 일상` ("the world and the everyday") says that
duality as it is, with no negation.

`형식` stays — it is the only accurate one of the three, and the place where it used to break
disappears once it leaves the sentence (D4). `만드는 것` is not touched this time. It has the
problem that the label drops the "**used for**" of its own definition (golang is not something
you make, it is something you make with), but as a group heading the tags underneath hold it
up. **The swap costs two places in code plus nine in the docs** — no test asserts the Korean
labels, and the linter only looks at enum keys. No contract or migration impact.

There is no recorded argument defending these labels. What the commit that introduced them
(`6b9247f`) defends is **only the number of colours** (with three, the worst-case colour-blind
ΔE is 7.5; with five, 2.9), and a label change does not touch that measured conclusion.

### D6. Flip the structure to sentence-first

The order 11 §8(3-1) set (sentence → streak → rhythm → tags → total) is right. The
implementation never followed.

Apple's charting guidance says the same thing from the other side — *text attached to a chart
must **be informative when read on its own***, and for data with gaps use bars rather than a
line (a line exaggerates the holes; bars leave a zero sitting there without drama). At its
present density the 30 day strip is **word-sized in Tufte's sense**, so it finds its place as
a footnote to the sentence rather than as a panel.

**The empty space is not backfilled with a new chart.** The restraint rule (10 §1.4) forbids a
third signature, and count-ups, stagger and scroll reveals are §6.2-forbidden by name. What
should make this screen feel well made is **the density of its typesetting** — that is why
Feltron's
annual reports read like a portrait even though they are "nothing but plain facts and
numbers".

### D7. Kept, with the verdict deferred

- **The total**: by Ries's definition it is a textbook vanity metric (it only goes up and
  nothing follows from it), but 11 §8(3-1) already fixed its position at the very end, and in
  that position it does no harm. Kept
- **Streak**: the Goodhart pressure of the streak itself is **already recorded as known and
  accepted** in 08 §M6 and the comments in `streak.sh`. The agreed exposure runs to the streak
  plus a single goal line; growing it into badges, levels or notifications is a separate
  decision. This plan does not grow it
- **Resurfacing**: at 1~3-a-day volumes, the most interesting true output the archive can
  produce is not a ratio but **one forgotten item** — it needs no threshold, it cannot be
  noise, and it is the only pattern that gets better as the archive ages. But **this is a new
  feature, not a statistic.** It stays outside this plan's scope and goes over as a backlog
  candidate

---

## 3. What has to be cleared before starting

### B1. The current screen has never been drawn to spec (must come first)

`frontend/tailwind.css` wipes `--text-*` to `initial` and defines only **8** of them
(label/meta/card/body/title/spine/head/display), while `RhythmSection.tsx` uses a
`text-caption` that does not exist in **fourteen places** and `text-mono` in **four**.
Counting the settings and sheet sections as well, `text-caption` is in **twenty places**.

The result: the labels and counts of the rhythm section **have never once had the type scale
applied to them**, and although 10 §2.2.4 explicitly names the `Stats` numbers as mono
targets, **mono is not on them.** The code comment says mono; the screen does not.

This is the failure mode `.claude/rules/frontend.md` warns about by name — *"`h-64`, `gap-1`
and `rounded-xs` compile to **a class with no CSS**: no lint error, no type error, nothing on
screen."* The same file carries the scar of hitting `h-64` once and fixing it, and
`text-caption` survived right next to it.

**The "current state" screenshot of the redesign must not be used as the baseline.** Fix it
first, then look at the fixed screen.

### B2. Stale documents (so the redesign does not read as a regression)

- `12-BACKLOG.md` §4.2 still records "the web rhythm screen" as **rejected**, when it was built
  and shipped on 2026-07-27. The section that exists to stop re-litigation does not know its
  own conclusion was overturned
- `12 §3 A1`'s *"the judge lives only in the terminal"* was already retired as wrong by
  `streak.sh:27-30`
- The `11 §8` mockup wrote `상위 태그 … 상위 5개만`, but the implementation draws the whole
  facet grouping with no limit. The label differs too — `상위 태그` versus the screen's
  `무엇을 모았나`

### B3. All four were judged (2026-07-29)

| Mismatch | Verdict | Basis |
|---|---|---|
| The iOS stats heading is `spine` (serif) | **Revert the code** | §2.2.5 confines serif to the single time-spine and nails down that breaking it revives the ban on serif in body text. The web never used it, so reverting makes the two clients agree |
| iOS `.reveal(0…4)` sequential entry | **Revert the code** | It is not in the §6.1 table, and staggered entry of list items is §6.2-forbidden by name. The restraint clause also says "no choreography is added to other screen transitions or list entries". The web never had it |
| The Panel component exists nowhere | **Match the spec to the screen** | The original section carried no argument, and the two clients produced the same answer (canvas + rule lines) without consulting each other. `--bg-surface` is the link card's surface, so putting chrome on it collapses the distinction this system spends its budget on. Of the two specified variants, `interactive` never had a single place to be used (10 §4.5.1) |
| `display` has no live use | **Keep the token, correct the description** | It has a live consumer — the iOS detail screen title. The scale table **deliberately split the inspector title (`head`) from the detail screen title (`display`)**, and the web's drawer and iOS's pushed screen are not the same surface. Only "stats numbers" was removed (10 §2.2.2) |

One more thing came out of the judging: **the type slot of the stats sentence differs between
the two clients** (web `body` 15/400 vs iOS `head` 20/600). This is not a mismatch to fix but
**a legitimate deviation arising from different containers**, so it was entered into 10 §8.5 —
on iOS the sentence has a tab to itself and leads the screen, while on the web it is a section
inside settings and sits under a `title` heading.
Raising the web would make the sentence bigger than its own heading.

---

## 4. Scope and verification

**Parity axis ①** — stats is exactly the case 13 §1 designates with "if only one side can be
built, defer it", and this plan changes the sentence, the weekday chart, the comparison and
the labels on both sides at once.

**One rule, three implementations** (13 §3): streak lives in `StatsView.swift` ·
`scripts/streak.sh` · `rhythm.ts`. This plan does not change the streak calculation, so all
three stay as they are, but the `weekOverWeek` deletion (D3) and the `dominantFacet` deletion
(D4) are removed as the same change set in both implementations.

**Verification**

- `just web-test` — regression on the derived values that remain in `rhythm.ts`
  (streak·activeDays·groupedTags)
- `just streak-selftest` — the shared fixture stays
- `just ios-test` / `just ios-uitest`
- **The screen**: AXe screenshots for iOS, Maestro `chromium` for the web. A build is not
  evidence (CLAUDE.md)
- Copy changes are **not a mutation-testing target** — instead, confirm with a symbol grep
  that the deleted calculations really are never called, and write the result into the PR

**Measurement material**: `scripts/stats_claims.py`. With no dependencies,
`python3 scripts/stats_claims.py` reproduces the figures in §1. The first draft only said they
"came from a reproducible simulation" and did not commit the script; the review called that
out, and **running it for real failed to reproduce two of the items, so the document was the
side that got corrected** (§1.1, §1.4). A claim whose evidence is not committed is not
evidence.

---

## 5. What this plan does not do

- No new chart, gauge or heatmap is built (the restraint rule 10 §1.4, and §6.2)
- The contract is not changed. `Stats.failed_links`·`Tag.last_saved_at`·`by_domain`·
  `Link.error` are a separate bundle that 12 §5-5 records as **cheaper to ship as one PR**,
  and they are not mixed in here
- Streak exposure is not grown (no badges, levels or notifications — D7)
- Resurfacing is not implemented (handed to the backlog as a candidate — D7)
- The placement of the stats screen (an iOS tab vs a web settings section) is not judged.
  13 §1 requires feature parity but has never ruled on placement, and this plan's decisions
  are the same under either placement
