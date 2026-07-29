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
  daysSinceLastSave,
  groupedTags,
  streak,
  type Day,
} from './rhythm'
import type { TagFacet } from './api/types'

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



describe('daysSinceLastSave', () => {
  it('오늘 저장했으면 0', () => {
    expect(daysSinceLastSave(windowOf([...Array(29).fill(0), 1]))).toBe(0)
  })

  it('마지막 칸이 비어 있으면 뒤에서부터 센다', () => {
    // ...1,0,0 → 마지막 저장은 2일 전
    expect(daysSinceLastSave(windowOf([...Array(27).fill(0), 1, 0, 0]))).toBe(2)
  })

  it('창 끝(30일 전)에만 저장이 있으면 29', () => {
    expect(daysSinceLastSave(windowOf([1, ...Array(29).fill(0)]))).toBe(29)
  })

  it('창이 통째로 비면 null — 0이 아니다', () => {
    // 0이면 "오늘 저장했다"가 되어 정반대 문장이 나온다.
    expect(daysSinceLastSave(windowOf(Array(30).fill(0)))).toBeNull()
  })

  it('중간에 저장이 여러 번 있어도 가장 최근 것만 본다', () => {
    expect(daysSinceLastSave(windowOf([...Array(10).fill(0), 3, ...Array(5).fill(0), 2, ...Array(13).fill(0)]))).toBe(13)
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


describe('groupedTags', () => {
  const facetOf = (map: Record<string, TagFacet>) => (n: string) => map[n] ?? 'neutral'

  it('facet 순서는 개수가 아니라 TAG_FACETS 고정이다', () => {
    // life가 훨씬 많아도 craft가 먼저다. 개수로 재배치하면 화면을 열 때마다 같은
    // 태그가 다른 자리에 있어 위치 기억이 무너진다.
    const g = groupedTags(
      [
        { name: '뉴스', count: 90 },
        { name: '개발', count: 3 },
      ],
      facetOf({ 뉴스: 'life', 개발: 'craft' }),
    )
    expect(g.map((x) => x.facet)).toEqual(['craft', 'life'])
  })

  it('묶음 안은 개수 내림차순, 동점은 이름순', () => {
    const g = groupedTags(
      [
        { name: 'swift', count: 2 },
        { name: 'golang', count: 9 },
        { name: 'ai', count: 2 },
      ],
      facetOf({ swift: 'craft', golang: 'craft', ai: 'craft' }),
    )
    expect(g[0]?.tags.map((t) => t.name)).toEqual(['golang', 'ai', 'swift'])
    expect(g[0]?.total).toBe(13)
  })

  it('빈 facet은 내보내지 않는다 — 0인 줄은 소음이다', () => {
    const g = groupedTags([{ name: '개발', count: 1 }], facetOf({ 개발: 'craft' }))
    expect(g).toHaveLength(1)
  })

  it('사전에 없는 태그는 neutral로 묶이고 사라지지 않는다', () => {
    const g = groupedTags([{ name: '처음보는것', count: 4 }], facetOf({}))
    expect(g).toEqual([
      { facet: 'neutral', total: 4, tags: [{ name: '처음보는것', count: 4 }] },
    ])
  })
})
