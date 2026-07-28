// Rhythm — the derived numbers behind the stats screen (11 §8).
//
// Kept as pure functions, separate from the screen, for one reason: **these
// rules have three implementations and they must agree.** iOS computes them in
// `StatsView.swift`, `scripts/streak.sh` computes the streak to decide the
// 4-week goal, and this is the third. If the phone says "12 days" and the
// terminal says "11", there is no basis for deciding which to believe.
//
// Saying that is not the same as checking it, and the first version of this file
// only said it. `rhythm.test.ts` is the part that checks.
//
// ## What `by_day` guarantees, and why that removed most of this file
//
// The contract (`api/openapi.yaml`, Stats.by_day) guarantees **exactly 30
// entries, ascending, with the last one being today in server localtime**;
// days with no saves are present with `count: 0`.
//
// Before 2026-07-28 it did not. It was a raw `GROUP BY` result, so absent days
// had no row — and both clients indexed it positionally anyway. Five scattered
// saves rendered as five adjacent bars: "saved every day this week" when the
// truth was "saved five times this month". Web packed them right, iOS packed
// them left, so the same payload drew opposite pictures.
//
// The fix went in the server rather than here, which is why this file no longer
// does any date arithmetic to find "today": **index 29 is today.** Counting the
// streak is walking backwards from the end. That also deletes a timezone bug —
// the dates are minted in *server* localtime and this code was matching them
// against a `Date` built in *browser* localtime.
//
// The one thing still parsed as a date is the weekday name, which genuinely
// needs a calendar.

import type { Stats, TagFacet } from './api/types'
import { TAG_FACETS } from './tags/facet'

export type Day = Stats['by_day'][number]

/** Days in the contract's window. Not a tuning knob — see the by_day guarantee. */
const WINDOW = 30
const WEEK = 7

/**
 * Consecutive days with at least one save, counting back from today.
 *
 * Starts at yesterday when today has nothing yet, so the number only falls when
 * a day is actually missed rather than at every midnight. A streak metric that
 * reads 0 for the first hours of every day is a metric nobody trusts.
 *
 * Capped by the window: a 45-day streak reports as 30. `cappedStreak` is how a
 * caller finds out, so the screen can say so instead of implying precision it
 * does not have (`scripts/streak.sh` already discloses this).
 */
export function streak(byDay: readonly Day[]): number {
  let i = byDay.length - 1
  if (i < 0) return 0
  // Today not saved yet does not break yesterday's streak.
  if ((byDay[i]?.count ?? 0) === 0) i -= 1

  let n = 0
  while (i >= 0 && (byDay[i]?.count ?? 0) > 0) {
    n += 1
    i -= 1
  }
  return n
}

/** True when the streak ran off the start of the window and is really unknown. */
export function cappedStreak(byDay: readonly Day[], days: number): boolean {
  return days > 0 && days >= byDay.length
}

/**
 * Saves in the last 7 days minus the 7 before that, or null when there is not
 * yet enough history to compare.
 *
 * Returning null rather than 0 matters: "no change" and "cannot tell yet" read
 * identically as a number and differently as a sentence.
 *
 * "Enough history" is **14 days of it**, which is not the same as 14 rows — the
 * window is always 30 rows now. Two ways to have it: the first save inside the
 * window is at least 14 days back, or there are links older than the window at
 * all (`total_links` exceeds what the window accounts for). The second clause is
 * what keeps a long-time user who took a three-week break from being treated as
 * a new account.
 */
export function weekOverWeek(s: Stats): number | null {
  const byDay = s.by_day
  if (byDay.length < 2 * WEEK) return null

  const inWindow = byDay.reduce((a, d) => a + d.count, 0)
  const firstActive = byDay.findIndex((d) => d.count > 0)
  const daysOfHistory = firstActive < 0 ? 0 : byDay.length - firstActive
  if (daysOfHistory < 2 * WEEK && s.total_links <= inWindow) return null

  const recent = byDay.slice(-WEEK).reduce((a, d) => a + d.count, 0)
  const prior = byDay.slice(-2 * WEEK, -WEEK).reduce((a, d) => a + d.count, 0)
  return recent - prior
}

/** Saves per weekday, index 0 = Sunday — matching `Date.getDay()`. */
export function weekdayCounts(byDay: readonly Day[]): number[] {
  const counts = new Array<number>(WEEK).fill(0)
  for (const d of byDay) {
    // Parse as local time. `new Date('2026-07-27')` is parsed as UTC and can
    // land on the previous weekday west of Greenwich, which would silently
    // shift the busiest day by one.
    const [y, m, day] = d.date.split('-').map(Number)
    if (!y || !m || !day) continue
    const idx = new Date(y, m - 1, day).getDay()
    counts[idx] = (counts[idx] ?? 0) + d.count
  }
  return counts
}

/** Days in the window that had at least one save. */
export function activeDays(byDay: readonly Day[]): number {
  return byDay.filter((d) => d.count > 0).length
}

/**
 * The facet the user's tags lean toward, or null when the screen should stay
 * quiet about it.
 *
 * **The order of the two steps is load-bearing and was wrong on the web.** iOS
 * takes the max across *all* facets and then suppresses the sentence if the
 * winner is `neutral`; the web filtered `neutral` out *first* and then took the
 * max. With `읽을거리`(neutral) at 100 and `개발`(craft) at 10, iOS says nothing
 * and the web said "주로 '만드는 것'에 관심이 갔고," — promoting a distant
 * second to a headline. Suppressing is right: if most tags carry no facet, the
 * honest answer is that the data does not show an interest.
 *
 * Ties break by `TAG_FACETS` order (craft → media → life → neutral), matching
 * Swift's `max(by:)` over `PP.Facet.allCases`, which keeps the first maximum.
 */
export function dominantFacet(
  byTag: readonly Stats['by_tag'][number][],
  facetOf: (name: string) => TagFacet,
): TagFacet | null {
  const totals = new Map<TagFacet, number>()
  for (const t of byTag) {
    const f = facetOf(t.name)
    totals.set(f, (totals.get(f) ?? 0) + t.count)
  }
  let best: TagFacet | null = null
  for (const f of TAG_FACETS) {
    if ((totals.get(f) ?? 0) > (best === null ? 0 : (totals.get(best) ?? 0))) best = f
  }
  return best === 'neutral' ? null : best
}

export { WINDOW as RHYTHM_WINDOW }
