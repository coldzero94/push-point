# Client Parity

> Push-Point v2.1 — last updated: 2026-07-27

**The rule for deciding which client a new feature goes into.** Not a list of screens — a decision procedure.

This document exists because two features had already diverged — stats were iOS-only, dictionary
management was web-only. Worse than the divergence itself was **having no rule for judging whether
something had diverged**. `.claude/rules/frontend.md` had it written down that "only the share
sheet's two-second save is iOS-specific", and in reality that premise was already broken with
nothing in place to notice.

---

## 0. First: the two clients differ in more than their screens

| | iOS | Web |
|---|---|---|
| **Where the data lives** | Inside the app (self-contained). Home-server mode is **designed only, not working** | The very server that serves the page |
| Server configuration | None — there is nothing to configure | **None** — it is the same origin |
| Auth | Not needed, it is in-process | API key (settings screen, localStorage) |
| Save entry point | Share sheet | URL field · bookmarklet · extension |

On iOS, `Backend.swift` **brings up an in-process server** through gomobile. The same `internal/app`
wiring serves the same contract, so the screen code does not distinguish the two modes — the only
thing that changes is `serverURL`. Which means **the contract guarantees feature parity, and it
guarantees nothing about UX parity.**

> **Neither client has a server-address field.** The first edition of this document (2026-07-27)
> explained axis ③ with the example "the web requires an address and an API key to be entered", and
> **that example was wrong** — `lib/api/client.ts` calls `createClient({ baseUrl: '' })`, so every
> request is a same-origin relative path, and "copy server address" on the settings screen is not an
> input, it is **the clipboard handing you something to paste into the browser extension**. The Vite
> proxy makes the origin identical in development, the embedded SPA does it in production. An
> example that was wrong while explaining an axis means the axis itself has to be looked at again,
> so §1 ③ was rewritten.

---

## 1. The decision procedure — a new feature is one of three

Before a new feature goes in, **decide which axis it is on first, and write that reasoning into the PR.**

### ① Working with the archive — do it **on both sides** or defer it **on both sides**

List · search · detail · tagging a link · delete · stats.

The two clients are **two windows onto the same archive**, so when they split here the user has to
remember "where did I do that again?". That memory cost is worth more than any one feature.

**If only one side can be built, do not build it — defer it.** Stats are the example: the web spec
had already defined a stats section on 2026-07-22 and deferred it to P2, and on 07-26 iOS built **a
different screen** against the same contract first (`StatsView.swift`, first commit fa18e5e).

The period they were actually split was **one day**. The first edition of this document wrote "it
went on for three months" here, and that **was a fabrication** — the web client itself was created
on 07-21, so three months is arithmetically impossible. The one place that violated "no unmeasured
numbers" (CLAUDE.md) happened to be the only piece of evidence this rule had.

**The point is that one day was already problem enough.** It was not that it stayed short and so
never set; it was noticed by accident before it could set. Had it not been noticed, nothing was in
place to catch it.

### ② Entry and context — **each at its own best**, they do not need to match

Share sheet (iOS) · URL field and bookmarklet (web) · widget (iOS) · keyboard shortcuts (web).

**Matching here is actively worse.** Imitating a share sheet on the web, or handing iOS a URL field,
throws away what each platform is good at. This axis is not "what do you do" but "how do you get in".

### ③ What the runtime environment decides — **state the condition**

Browser extension integration · Google Sheets authorization · API key entry · a forty-two row editing table.

These look like ①, but **the thing that makes the feature possible exists only in that environment**.
The extension only exists in a browser; auth has no purpose on an in-process server. Here you write
down "why the other side does not have it" **as a conditional sentence** — "I didn't build it on the
phone" is not ③, it is a deferred ①.

When the call is hard, split on this question: **can you write a sentence saying what this feature
would do on the other client?** If you cannot, it is ③; if you can and it simply was not built, it is ①.

---

## 2. The current decision table

| Feature | Axis | iOS | Web | Notes |
|---|---|---|---|---|
| List · filter | ① | ✅ | ✅ | |
| Search | ① | ✅ | ✅ | |
| Detail · edit | ① | ✅ | ✅ | |
| Tagging a link | ① | ✅ | ✅ | Picked from the dictionary (not free text) |
| Stats: paragraph · streak · rhythm across 30 days | ① | ✅ | ✅ | **added to the web 2026-07-27** |
| Stats: day-of-week pattern | ① | ✅ | ✅ | **added to the web 2026-07-28** |
| **Stats: failed-link CTA** | ① | ✅ | ❌ | **the remaining hole — below** |
| Save: share sheet | ② | ✅ | — | OS feature |
| Save: URL field · bookmarklet | ② | — | ✅ | |
| Keyboard shortcuts | ② | — | ✅ | |
| Widget | ② | planned | — | |
| Browser extension integration | ③ | — | ✅ | The extension only exists in a browser |
| API key entry | ③ | — | ✅ | An in-process server has no auth |
| **Dictionary CRUD** | **③** | — | ✅ | **web-only is the right call — reasoning below** |
| Spreadsheet export | ③ | — | ✅ | The Google authorization is walked by `just sheets-setup` (terminal). The web only shows status and a sync button |

### Failed-link CTA — axis ①, and the web does not have it (2026-07-28)

The iOS stats tab has a row reading `N links failed to be collected` that taps through to that list
(`StatsView.needsAttention`). The judgement behind it is that a failed link is not a statistic but
**a to-do**, and that judgement is just as correct on the web. It is axis ①, so **both sides should
have it.**

The reason it is missing right now is the contract: `LinkPage` carries no total, so the web cannot
get the failure count cheaply (iOS counts it on another screen and hands it over). The fix is to put
`failed_links` into `Stats`, and that is a contract change, so it was not squeezed into this change set.

**Writing it down here is the point.** The first edition did not write this row at all and stamped
✅✅ on the stats row — a table built to watch for divergence recorded one divergence as closed.

### The case for dictionary CRUD being web-only (decided 2026-07-27)

Creating, renaming, deleting and facet-editing the tag dictionary does not go on the phone. It looks
like ①, but it is ③:

- **It cannot be undone.** Deletion is a `link_tags` CASCADE and there is no restore endpoint. It is
  the only screen that needs a confirmation dialog, and there is no reason to force that kind of
  operation onto a small screen while moving.
- **It is an editing table of some forty rows.** Sorting, inline editing and alias lists hang off it.
  You can build a worse version of it on a phone; you cannot build a better one.
- **The frequency is different.** Saving a link is daily; editing the dictionary is once every few months.
- **Parity does not break.** What iOS cannot do is **change** the dictionary; **using** the dictionary
  (tagging) happens on both sides. The user never runs into "where did I do that again?".

The condition that overturns this call: if real cases of wanting to fix the dictionary from the phone
actually pile up.

---

## 3. The same rule implemented in three places

Even with the same contract, **derived calculations end up being written again per client.** When
they diverge the screens say different things, and there is no basis for believing either one.

| Rule | Where it is implemented | Agreement check |
|---|---|---|
| **Save streak** | `ios/PushPoint/StatsView.swift` · `scripts/streak.sh` · `frontend/src/lib/rhythm.ts` | Web and shell both read `testdata/streak-cases.json` (iOS not yet) |
| ~~Week over week · dominant interest~~ | — | **deleted** (14 §D3·§D4) — the data did not hold it up |
| Relative time strings | `ios/Shared/RelativeTime.swift` · `frontend/src/lib/time.ts` | **Both sides read** `testdata/relative-time-cases.json` (2026-07-30) |
| Status labels | `ios/PushPoint/StatusAnnounce.swift` · `frontend/src/lib/statusAnnounce.ts` | **Both sides read `testdata/status-labels.json`, and both call the function** (2026-07-30) |
| facet labels | `ios/PushPoint/DesignSystem.swift` · `frontend/src/lib/tags/facet.ts` | **Both sides read** `testdata/facet-labels.json` (2026-07-30). This one is **a strong check** — `PP.Facet.label` is an enum rather than a view, so the test calls the function directly |

**This table now has no blanks left and no weak checks either.** All four rules share a fixture and
call the function directly. Status labels were the last one still shaped as a source scan; pulling
the labels out of the view turned it into "this input yields this output" — and the difference showed
up as a mutation: **a mutation that inverts the priority order leaves the strings exactly as they
were, so a source scan cannot catch it**, and only a check that calls the function does.

**Rule**: when you change something in this table, change **every implementation in the same change
set**. Same reason the backend interface rule (`.claude/rules/backend.md`) says "there is no commit
that changes only an interface".

### How this rule was first not kept (2026-07-28)

The first edition wrote this here: *"The save streak was confirmed to agree on 2026-07-27 by comparing
the web and `streak.sh` implementations across four cases."* The comparison did really happen, but
**the sentence was all that was left of it, so it could neither be re-run nor break when it turned
out to be wrong.** The same PR, one file over, was arguing that "a comment that disagrees with
shipped behaviour only fools the person reading it".

Now `just web-test` and `just streak-selftest` both read the ten cases in `testdata/streak-cases.json`.
And **the moment that check went in, it exposed two other rules that had diverged**:

### List density — axis ② (recorded 2026-07-30)

iOS has two density modes, card and dense, and **the web has neither.** The axis it splits on is
**platform capability** — the web, where widening the window adds columns, already has a way to change
density, and the phone does not (10 §1.3).

**The absence of this row was itself the failure this document is on guard against.** The call lived
only in the body of PR #65 and was in neither the decision table nor 10 §8.5, so **a table built to
watch for divergence did not know about one** — the same shape as the cover aspect ratio nearly being
reverted for reading as a "spec violation" (10 §8.5).

**Even though it is ②, the awkward part gets written down.** The discriminator stated in the decision
procedure above asks *"can you write a sentence saying what this feature would do on the other client?
If you can and it simply was not built, it is ①"* — and a density toggle on the web is perfectly
writable as a sentence. The grounds for calling it ② are not "you cannot write it" but **"on the web,
window width already does that job"**, so if the web ever loses that handle, this call has to be
looked at again.

- **`weekOverWeek` was wrong on both sides.** Back when `by_day` was a GROUP BY result with no row
  for empty days, `slice(-7)` was not "the last 7 days" but "the last seven rows that had a save".
  Counting seven rows spread across a month as "this week" left the arithmetic inconsistent with
  `links_this_week` (a real 7 days) in the sentence right beside it.
- **The bars for the last 30 days were wrong on both sides, and in opposite directions.** Same
  row-indexing bug, but the web packed them to the right and iOS to the left — **two screens were
  drawing the same response as mirror images of each other.**
- **The order for choosing the interest sentence was different.** iOS picks the top one across
  everything and kills the sentence if that one is neutral, while the web removed neutral first and
  then picked the top. With `읽을거리` (neutral) at 100 items and `개발` (craft) at ten items, iOS says
  nothing and the web said "주로 '만드는 것'에 관심이 갔고". Which amounts to erasing an overwhelming
  winner and promoting a distant runner-up to the headline.

All three typechecked and built. **While "the three implementations agree" stood as the claim, two of
the three did agree — on the wrong answer.**

> **Postscript, 2026-07-29.** The first and third of those three were **deleted** afterwards. Once
> they had been made to agree, it was clear the data did not hold the calculation up (14 §D3·§D4) —
> `weekOverWeek` flipped its direction word every three days even when behaviour did not change at
> all, and the interest sentence put an all-time total with no date condition where a recency claim
> belonged. **Making things agree and asking whether they are right are different jobs.** The fixture
> only made all three produce the same answer; it never asked whether that answer was true. The bars
> over the last 30 days, which survive, are still the same on both sides and still true.

The third implementation (iOS) still does not read the fixture. `streak` is a `private func` inside a
SwiftUI view, so it is invisible to the test target, and pulling it out is left as separate work.

---

## 4. The asymmetry in verification — this is where the first edition was most wrong

The first edition wrote: *"iOS screens can be looked at and web screens cannot. The web cannot have a
browser attached on this development machine, and the frontend has no test runner at all."*

**Only the second half was true, and even that was something a day's work could make untrue.**

- **Web screens can be looked at.** The Maestro registered in `.mcp.json` supports `chromium` as a
  device — the very tool that same sentence cited as evidence two words earlier. Before concluding it
  could not be seen, one call to `list_devices` would have settled it.
- **The missing test runner was a fact, and now it exists.** `just web-test` (vitest). The cost of
  adopting it was one devDependency and three lines of CI — CI's web job was already running `npm ci`.

**Binding the two into one sentence was the shape of the error.** "I cannot see the screen" (a tooling
problem) and "I cannot run a pure function" (simply never built) are entirely different claims, but
stuck together the second deferral looked reasonable by leaning on the first constraint. `rhythm.ts`
was **a pure function with no DOM and no React** from the very start, and had nothing to do with a
browser.

### Current state

| | Automated check | Tool |
|---|---|---|
| Web — pure logic | ✅ | `just web-test` (vitest), CI |
| Web — screens | a human looks | It can be looked at with Maestro `chromium` (no automated gate yet) |
| iOS — pure logic | partial | `just ios-test` — streak is still inside a view, so it is invisible |
| iOS — screens | ✅ | `just ios-uitest`, Maestro, AXe |

**The real remaining asymmetry is the screen side.** iOS has a screen gate that runs in CI and the web
does not. CLAUDE.md's "a successful build is not evidence about the screen" still applies to the web
exactly as written.
