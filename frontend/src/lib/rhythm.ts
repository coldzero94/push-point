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
 * How many days ago the most recent save was, counting back from today, or
 * `null` when the 30-day window holds nothing.
 *
 * **Positional, like `streak`.** The contract guarantees `by_day` is exactly 30
 * dense entries ending at the server's local today, so walking backward from the
 * end is the whole calculation — no date arithmetic, and therefore no timezone
 * to get wrong. The clients used to index this array by date and both were
 * quietly wrong.
 *
 * This exists so the sentence can say something true when the streak is 0.
 * "0일 연속" is a number about nothing; "마지막은 3일 전이에요" is a fact, and it
 * reports the gap without asking the reader to close it.
 */
export function daysSinceLastSave(byDay: readonly Day[]): number | null {
  for (let i = byDay.length - 1; i >= 0; i--) {
    if (byDay[i].count > 0) return byDay.length - 1 - i
  }
  return null
}

/** Days in the window that had at least one save. */
export function activeDays(byDay: readonly Day[]): number {
  return byDay.filter((d) => d.count > 0).length
}


export { WINDOW as RHYTHM_WINDOW }

export type TagGroup = {
  facet: TagFacet
  total: number
  tags: readonly Stats['by_tag'][number][]
}

/**
 * 태그를 facet으로 묶는다 — iOS `StatsView.groupedTags`와 같은 규칙.
 *
 * **facet 순서는 `TAG_FACETS` 고정이고 개수로 재배치하지 않는다.** 개수순으로 묶음을
 * 흔들면 화면을 열 때마다 같은 태그가 다른 자리에 있어 위치 기억이 무너진다 — 이 목록의
 * 값어치가 "저번에 여기 있었지"에서 오기 때문이다.
 *
 * 묶음 **안**은 개수 내림차순이다. 묶음 사이의 순서와 달리 여기서는 "무엇을 많이 모았나"가
 * 곧 읽고 싶은 것이고, 태그 자체는 사전 순서에 아무 의미가 없다.
 *
 * 비어 있는 facet은 내보내지 않는다 — 0인 줄은 정보가 아니라 소음이다.
 */
export function groupedTags(
  byTag: readonly Stats['by_tag'][number][],
  facetOf: (name: string) => TagFacet,
): TagGroup[] {
  const bucket = new Map<TagFacet, Stats['by_tag'][number][]>()
  for (const t of byTag) {
    const f = facetOf(t.name)
    const list = bucket.get(f)
    if (list) list.push(t)
    else bucket.set(f, [t])
  }
  const out: TagGroup[] = []
  for (const facet of TAG_FACETS) {
    const tags = bucket.get(facet)
    if (!tags || tags.length === 0) continue
    out.push({
      facet,
      total: tags.reduce((a, t) => a + t.count, 0),
      tags: [...tags].sort((a, b) => b.count - a.count || a.name.localeCompare(b.name)),
    })
  }
  return out
}
