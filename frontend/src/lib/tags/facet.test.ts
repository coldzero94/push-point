import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { FACET_LABELS, TAG_FACETS } from './facet'

// facet 한글 라벨을 **iOS와 같은 파일로** 대조한다.
//
// 13 §3의 판정표에서 마지막까지 "일치 검사: 없음"으로 남아 있던 항목이다. 그 표의 다른
// 규칙들은 픽스처를 붙이자마자 실제 갈라짐이 드러났다 — streak은 셋 중 둘이 **틀린 답에서**
// 일치하고 있었고, 상대 시각은 픽스처 자신이 먼저 틀렸다. 주장으로 두지 않는다.
describe('facet 라벨은 iOS와 같은 파일을 읽는다', () => {
  const FIXTURE = JSON.parse(
    readFileSync(
      fileURLToPath(new URL('../../../../testdata/facet-labels.json', import.meta.url)),
      'utf8',
    ),
  ) as { labels: Record<string, string> }

  it('네 facet이 픽스처와 같다', () => {
    for (const facet of TAG_FACETS) {
      expect(FACET_LABELS[facet]).toBe(FIXTURE.labels[facet])
    }
  })

  it('픽스처와 enum의 개수가 같다 — 하나가 빠지면 그 facet만 검사 밖에 남는다', () => {
    expect(Object.keys(FIXTURE.labels).sort()).toEqual([...TAG_FACETS].sort())
  })
})
