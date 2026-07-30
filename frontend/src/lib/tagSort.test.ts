import { describe, expect, it } from 'vitest'
import { sortTags } from './tagSort'
import type { Tag } from './api/types'

const tag = (name: string, count: number, last: number | null): Tag => ({
  id: name.length,
  name,
  aliases: [],
  link_count: count,
  facet: 'neutral',
  last_saved_at: last,
})

describe('sortTags — recent', () => {
  it('최근 저장이 앞에 온다', () => {
    const out = sortTags([tag('old', 1, 100), tag('new', 1, 900)], 'recent')
    expect(out.map((t) => t.name)).toEqual(['new', 'old'])
  })

  it('한 번도 안 붙은 태그는 맨 뒤다 (이름 순서와 어긋나는 픽스처)', () => {
    // 이름을 일부러 반대로 뒀다 — 신선도 순서와 알파벳 순서가 같으면 이 단언이
    // 어느 쪽 때문에 통과했는지 알 수 없다.
    const out = sortTags([tag('aaa-never', 0, null), tag('zzz-recent', 1, 900)], 'recent')
    expect(out.map((t) => t.name)).toEqual(['zzz-recent', 'aaa-never'])
  })

  it('같은 시각이면 이름순으로 깬다 — 열 때마다 자리가 바뀌면 위치 기억이 무너진다', () => {
    const out = sortTags([tag('b', 1, 500), tag('a', 1, 500)], 'recent')
    expect(out.map((t) => t.name)).toEqual(['a', 'b'])
  })

  it('입력을 제자리에서 정렬하지 않는다', () => {
    const input = [tag('z', 1, 100), tag('a', 1, 900)]
    sortTags(input, 'recent')
    expect(input.map((t) => t.name)).toEqual(['z', 'a'])
  })
})
