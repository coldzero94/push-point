// 상대 시각 규칙 — **공유 픽스처로 iOS와 대조한다.**
//
// 이 규칙은 두 구현이 있고, 갈라져도 **양쪽 다 정상 동작하는 것처럼 보인다.** streak에서
// 같은 문제를 겪고 `testdata/streak-cases.json`으로 막았는데, 그때 픽스처를 붙이자마자
// 갈라져 있던 규칙 셋이 드러났다(13 §3). 시각도 같은 부류라 같은 방식으로 고정한다.

import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { formatRelativeTime } from './time'

const FIXTURE = JSON.parse(
  readFileSync(fileURLToPath(new URL('../../../testdata/relative-time-cases.json', import.meta.url)), 'utf8'),
) as {
  now: string
  cases: { name: string; at: string; today: boolean; plain: string; dayStated: string }[]
}

const now = new Date(FIXTURE.now)

describe('formatRelativeTime — 공용 픽스처', () => {
  for (const c of FIXTURE.cases) {
    const epoch = Math.floor(new Date(c.at).getTime() / 1000)

    it(`${c.name} — 구간 없음`, () => {
      expect(formatRelativeTime(epoch, now, false)).toBe(c.plain)
    })

    it(`${c.name} — 구간이 날을 말함`, () => {
      expect(formatRelativeTime(epoch, now, true)).toBe(c.dayStated)
    })
  }
})

describe('dayStated는 오늘이 아닌 것만 바꾼다', () => {
  // 오늘 구간에서는 경과 시간이 맞는 정보다 — 머리글이 "오늘"이라고 해도
  // "3시간 전"은 반복이 아니라 하루 안에서의 위치를 말한다. 이 단언이 무너지면
  // 플래그가 의도보다 넓게 작동하는 것이고, 그러면 오늘 구간의 라벨이 조용히 바뀐다.
  it('오늘인 항목은 두 모드가 같다', () => {
    for (const c of FIXTURE.cases.filter((x) => x.today)) {
      expect(c.dayStated).toBe(c.plain)
    }
  })

  it('오늘이 아닌 항목은 반드시 달라진다 — 날짜가 하나뿐인 경우만 빼고', () => {
    for (const c of FIXTURE.cases.filter((x) => !x.today)) {
      // "7월 24일"처럼 이미 절대 날짜면 바꿀 것이 없다.
      if (/^\d+월 /.test(c.plain)) continue
      expect(c.dayStated).not.toBe(c.plain)
    }
  })
})
