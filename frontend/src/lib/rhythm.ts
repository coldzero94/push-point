// Rhythm — the derived numbers behind the stats screen (11 §6).
//
// Kept as pure functions, separate from the screen, for one reason: **the streak
// rule now has three implementations and they must agree.** iOS computes it in
// `StatsView.swift`, `scripts/streak.sh` computes it to decide the 4-week goal,
// and this is the third. If the phone says "12 days" and the terminal says "11",
// there is no basis for deciding which to believe.
//
// Two rules carry that agreement and are restated here so a reader does not have
// to open the Swift file:
//
//   1. **A day with no saves has no row in `by_day`** — it is a GROUP BY result.
//      So the count asks a set of dates whether it contains a cursor, rather than
//      filling absent days with zero.
//   2. **Not having saved *today* does not break yesterday's streak.** A number
//      that drops to 0 just after midnight is a number nobody trusts.
//
// Everything here derives from the `by_day` the contract already returns (30
// days). Nothing new is asked of the server: a value the client can derive is
// better derived.

import type { Stats } from './api/types'

type Day = Stats['by_day'][number]

/** `YYYY-MM-DD` in local time — the same shape `by_day.date` uses. */
function isoDay(d: Date): string {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

/**
 * Consecutive days with at least one save, counting back from today.
 *
 * Starts at yesterday when today has nothing yet (rule 2 above), so the number
 * only falls when a day is actually missed rather than at every midnight.
 */
export function streak(byDay: readonly Day[], today = new Date()): number {
  const saved = new Set(byDay.filter((d) => d.count > 0).map((d) => d.date))
  if (saved.size === 0) return 0

  const cursor = new Date(today)
  if (!saved.has(isoDay(cursor))) cursor.setDate(cursor.getDate() - 1)

  let n = 0
  while (saved.has(isoDay(cursor))) {
    n += 1
    cursor.setDate(cursor.getDate() - 1)
  }
  return n
}

/**
 * Saves in the last 7 days minus the 7 before that, or null when `by_day` is too
 * short to compare.
 *
 * Returning null rather than 0 matters: "no change" and "cannot tell yet" read
 * identically as a number and differently as a sentence.
 */
export function weekOverWeek(byDay: readonly Day[]): number | null {
  if (byDay.length < 14) return null
  const recent = byDay.slice(-7).reduce((a, d) => a + d.count, 0)
  const prior = byDay.slice(-14, -7).reduce((a, d) => a + d.count, 0)
  return recent - prior
}

/** Saves per weekday, index 0 = Sunday — matching `Date.getDay()`. */
export function weekdayCounts(byDay: readonly Day[]): number[] {
  const counts = new Array(7).fill(0) as number[]
  for (const d of byDay) {
    // Parse as local time. `new Date('2026-07-27')` is parsed as UTC and can
    // land on the previous weekday west of Greenwich, which would silently
    // shift the busiest day by one.
    const [y, m, day] = d.date.split('-').map(Number)
    if (!y || !m || !day) continue
    counts[new Date(y, m - 1, day).getDay()] += d.count
  }
  return counts
}

/** Days in the window that had at least one save. */
export function activeDays(byDay: readonly Day[]): number {
  return byDay.filter((d) => d.count > 0).length
}
