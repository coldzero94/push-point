// rhythm.ts의 파생 계산 검증.
//
// 이 파일이 존재하는 이유는 `rhythm.ts` 헤더가 자기 존재 이유로 적어 둔 것과 같다 —
// 같은 규칙의 구현이 셋이고 셋이 같은 답을 내야 한다. 그 주장이 처음에는 docs의 한
// 문장("네 케이스로 대조해 일치를 확인했다")으로만 있었고, 다시 돌려볼 수도 틀렸을 때
// 깨질 수도 없었다. 여기가 그 문장을 대체한다.
//
// streak 케이스는 `testdata/streak-cases.json`에서 읽는다 — `scripts/streak.sh --self-test`가
// 읽는 것과 **같은 파일**이라, 두 구현이 갈라지면 둘 중 하나가 빨개진다.

import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  activeDays,
  cappedStreak,
  dominantFacet,
  streak,
  weekOverWeek,
  weekdayCounts,
  type Day,
} from './rhythm'
import type { Stats, TagFacet } from './api/types'

const CASES = JSON.parse(
  readFileSync(fileURLToPath(new URL('../../../testdata/streak-cases.json', import.meta.url)), 'utf8'),
) as { cases: { name: string; counts: number[]; streak: number; capped: boolean }[] }

/**
 * 계약 그대로의 by_day를 만든다 — 정확히 30칸, 오름차순, **마지막이 오늘**.
 *
 * 날짜는 고정값에서 뽑는다. `new Date()`를 쓰면 자정을 넘길 때 테스트가 깜빡이고,
 * 그런 테스트는 결국 꺼진다.
 */
function windowOf(counts: number[], lastDay = '2026-07-27'): Day[] {
  const end = new Date(`${lastDay}T00:00:00`)
  return counts.map((count, i) => {
    const d = new Date(end)
    d.setDate(d.getDate() - (counts.length - 1 - i))
    const iso = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    return { date: iso, count }
  })
}

function statsOf(counts: number[], extra: Partial<Stats> = {}): Stats {
  const by_day = windowOf(counts)
  const inWindow = counts.reduce((a, c) => a + c, 0)
  return {
    total_links: inWindow,
    links_this_week: counts.slice(-7).reduce((a, c) => a + c, 0),
    by_tag: [],
    by_day,
    ...extra,
  }
}

describe('streak — 공용 픽스처', () => {
  for (const c of CASES.cases) {
    it(c.name, () => {
      const byDay = windowOf(c.counts)
      const got = streak(byDay)
      expect(got).toBe(c.streak)
      expect(cappedStreak(byDay, got)).toBe(c.capped)
    })
  }

  it('빈 배열에서도 죽지 않는다', () => {
    expect(streak([])).toBe(0)
    expect(cappedStreak([], 0)).toBe(false)
  })
})

describe('weekOverWeek', () => {
  // 이 함수는 2026-07-28까지 **틀려 있었다**. by_day가 GROUP BY 결과라 빈 날의 행이
  // 없던 시절, slice(-7)은 "최근 7일"이 아니라 "저장이 있던 마지막 7행"이었다. 20일에
  // 걸친 7행을 "이번 주"로 세고 있었다. 서버가 창을 채우게 바꿔서 고쳤고, 아래 첫
  // 케이스가 그 회귀를 막는다.
  it('최근 7칸 - 그 앞 7칸이다 (행이 아니라 날)', () => {
    // 29일 전부터 11일치 하루 1개 → 그 뒤 15일 공백 → 오늘 3개.
    const counts = Array(30).fill(0)
    for (let i = 0; i < 11; i++) counts[i] = 1
    counts[29] = 3
    // 최근 7칸(23..29) = 3, 그 앞 7칸(16..22) = 0 → +3.
    // 행 기준으로 세면 최근 7행 = 6개 앞선 7행 = 5개 → +1이 나왔었다.
    expect(weekOverWeek(statsOf(counts))).toBe(3)
  })

  it('같으면 0이고 null이 아니다', () => {
    const counts = Array(30).fill(0)
    for (let i = 16; i < 30; i++) counts[i] = 1
    // 히스토리가 14일이라 비교는 성립한다. 최근 7 = 7, 앞 7 = 7.
    expect(weekOverWeek(statsOf(counts))).toBe(0)
  })

  it('히스토리가 14일 미만이면 null — 0과 다르다', () => {
    const counts = Array(30).fill(0)
    counts[27] = 2
    counts[29] = 1
    expect(weekOverWeek(statsOf(counts))).toBeNull()
  })

  it('창보다 오래된 링크가 있으면 비교한다 — 오래 쉰 사용자를 신규로 보지 않는다', () => {
    const counts = Array(30).fill(0)
    counts[29] = 5
    // 창 안 합계는 5인데 전체가 400 → 창 밖에 히스토리가 있다.
    expect(weekOverWeek(statsOf(counts, { total_links: 400 }))).toBe(5)
    // 같은 데이터인데 전체가 창 합계와 같으면(=신규) 비교하지 않는다.
    expect(weekOverWeek(statsOf(counts))).toBeNull()
  })
})

describe('weekdayCounts', () => {
  it('일요일이 0번이다', () => {
    // 2026-07-26은 일요일.
    expect(weekdayCounts([{ date: '2026-07-26', count: 3 }])).toEqual([3, 0, 0, 0, 0, 0, 0])
    // 2026-07-27은 월요일.
    expect(weekdayCounts([{ date: '2026-07-27', count: 2 }])).toEqual([0, 2, 0, 0, 0, 0, 0])
  })

  it('UTC가 아니라 로컬로 파싱한다', () => {
    // `new Date('2026-07-27')`은 UTC 자정으로 해석돼 그리니치 서쪽에서는 전날(일요일)로
    // 떨어진다. 그러면 가장 바쁜 요일이 통째로 하루씩 밀린다.
    const counts = weekdayCounts([{ date: '2026-07-27', count: 1 }])
    expect(counts.indexOf(1)).toBe(new Date(2026, 6, 27).getDay())
  })

  it('창 전체를 합산한다', () => {
    const byDay = windowOf(Array(30).fill(1))
    expect(weekdayCounts(byDay).reduce((a, c) => a + c, 0)).toBe(30)
  })
})

describe('activeDays', () => {
  it('0인 날은 세지 않는다 — 창은 30칸이지만 활동일은 그보다 적다', () => {
    const counts = Array(30).fill(0)
    counts[0] = 1
    counts[15] = 4
    counts[29] = 2
    expect(activeDays(windowOf(counts))).toBe(3)
  })
})

describe('dominantFacet', () => {
  const facetOf = (map: Record<string, TagFacet>) => (name: string) => map[name] ?? 'neutral'

  it('neutral이 1등이면 문장을 죽인다 (iOS와 같은 순서)', () => {
    // 이것이 2026-07-28까지 웹과 iOS가 갈라져 있던 지점이다. 웹은 neutral을 **먼저
    // 빼고** 최댓값을 골라서, 압도적인 neutral 뒤의 먼 2등을 헤드라인으로 올렸다.
    const byTag = [
      { name: '읽을거리', count: 100 },
      { name: '개발', count: 10 },
    ]
    expect(dominantFacet(byTag, facetOf({ 읽을거리: 'neutral', 개발: 'craft' }))).toBeNull()
  })

  it('neutral이 있어도 1등이 아니면 그 1등을 쓴다', () => {
    const byTag = [
      { name: '읽을거리', count: 5 },
      { name: '개발', count: 30 },
    ]
    expect(dominantFacet(byTag, facetOf({ 읽을거리: 'neutral', 개발: 'craft' }))).toBe('craft')
  })

  it('동점은 craft → media → life 순으로 깬다 (Swift allCases와 같다)', () => {
    const byTag = [
      { name: '영상', count: 20 },
      { name: '개발', count: 20 },
    ]
    expect(dominantFacet(byTag, facetOf({ 영상: 'media', 개발: 'craft' }))).toBe('craft')
  })

  it('태그가 없으면 null', () => {
    expect(dominantFacet([], facetOf({}))).toBeNull()
  })
})
