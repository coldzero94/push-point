import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { boardView } from './board'

// `testdata/resurface-board-cases.json`은 iOS `BoardViewFixtureTests`와 **같은 파일**이다.
// 손으로 옮겨 적은 골든을 쓰다가 갈라진 전례가 이 저장소에 이미 둘 있다(커버 기하, streak).
//
// 고정하는 것이 **결과**(cardId·boardIds)인 것이 요점이다. 필터 입력을 고정하면 두
// 구현이 같은 입력을 받는다는 것만 확인되고, 그 입력으로 무엇을 그리는지는 안 본다.

type Case = {
  name: string
  links: number[]
  resurfaced: number | null
  filtered: boolean
  cardId: number | null
  boardIds: number[]
}

const fixture = JSON.parse(
  readFileSync(
    fileURLToPath(new URL('../../../testdata/resurface-board-cases.json', import.meta.url)),
    'utf8',
  ),
) as { cases: Case[] }

describe('boardView — 공유 픽스처', () => {
  it.each(fixture.cases)('$name', (c) => {
    const link = (id: number) => ({ id })
    const result = boardView({
      links: c.links.map(link),
      resurfaced: c.resurfaced === null ? null : link(c.resurfaced),
      filtered: c.filtered,
    })

    expect(result.card?.id ?? null).toBe(c.cardId)
    expect(result.board.map((l) => l.id)).toEqual(c.boardIds)
  })

  it('픽스처가 비어 있지 않다', () => {
    // 파일을 잘못 읽어 `cases`가 `undefined`면 it.each가 0건을 돌고 초록이 된다 —
    // 검사가 사라진 것이 통과로 보이는 자리라 따로 못 박는다.
    expect(fixture.cases.length).toBeGreaterThan(5)
  })

  it('보드는 링크를 잃지 않는다 — 카드로 간 한 건만 빠진다', () => {
    // 픽스처 값과 별개로 규칙 자체를 한 번 더 본다: 보드 + 카드 = 원래 목록.
    for (const c of fixture.cases) {
      const link = (id: number) => ({ id })
      const { card, board } = boardView({
        links: c.links.map(link),
        resurfaced: c.resurfaced === null ? null : link(c.resurfaced),
        filtered: c.filtered,
      })
      const shown = new Set(board.map((l) => l.id))
      if (card && c.links.includes(card.id)) shown.add(card.id)
      expect([...shown].sort()).toEqual([...c.links].sort())
    }
  })
})
