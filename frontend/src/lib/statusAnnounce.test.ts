import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { statusAnnouncement } from './statusAnnounce'
import { setLang } from './i18n'

// 상태 라벨 픽스처(testdata/status-labels.json)는 iOS와 공유하는 **한국어** 기준값이다.
// node 환경에서는 navigator.language가 없어 i18n이 'en'으로 떨어지므로,
// 여기서 고정하지 않으면 영어 문구와 비교하게 된다. iOS가 영문화되면
// 픽스처가 두 언어를 갖게 되고, 그때 이 고정을 두 번 도는 것으로 바꾼다.
setLang('ko')

// 이 규칙은 **화면으로 검증할 수 없다.** 레일이 색과 펄스를 그대로 두고 `aria-label`만
// 바꾸므로 스크린샷에 안 찍힌다. 그래서 여기가 유일한 검증 자리다.
//
// 그리고 문구는 iOS `LinkCard.statusLabel`과 **글자까지 같아야 한다**(§8.1). 두 화면이
// 같은 상태를 다르게 부르면 사용자가 둘을 다른 일로 읽는다.
describe('statusAnnouncement', () => {
  it('done은 아무 말도 하지 않는다 — 레일이 aria-hidden이 되는 근거다', () => {
    expect(statusAnnouncement('done', false)).toBe('')
  })

  it('진행 중과 실패는 말한다 — 색 단독 표현 금지(§7.1)', () => {
    expect(statusAnnouncement('pending', false)).toBe('대기')
    expect(statusAnnouncement('scraping', false)).toBe('수집 중')
    expect(statusAnnouncement('tagging', false)).toBe('태깅 중')
    expect(statusAnnouncement('failed', false)).toBe('실패')
  })

  it('백오프는 **status보다 우선한다** — 그러지 않으면 "대기"로 읽힌다', () => {
    // status는 여전히 pending이다. 그대로 두면 큐에서 순서를 기다리는 것과
    // 실패해서 30×attempts초를 세는 것이 같은 단어가 된다.
    expect(statusAnnouncement('pending', true)).toBe('재시도 대기 중')
  })

  it('done이어도 백오프면 말한다 — 우선순위가 뒤집히면 조용해진다', () => {
    // 계약상 done + waiting은 안 나오지만, 우선순위를 뒤집는 변이를 여기서 잡는다:
    // status를 먼저 보면 done이 빈 문자열을 내고 백오프가 통째로 사라진다.
    expect(statusAnnouncement('done', true)).toBe('재시도 대기 중')
  })
})

// **iOS와 같은 단어인지 파일로 대조한다.** §8.1이 요구하는데 검사가 없었다 — 13 §3의
// 판정표가 "facet 라벨: 일치 검사 없음"으로 적어 둔 것과 같은 부류다. streak·상대 시각이
// 픽스처로 묶이자마자 갈라짐이 드러났으니, 이것도 주장으로 두지 않는다.
describe('상태 라벨은 iOS와 같은 파일을 읽는다', () => {
  const FIXTURE = JSON.parse(
    readFileSync(
      fileURLToPath(new URL('../../../testdata/status-labels.json', import.meta.url)),
      'utf8',
    ),
  ) as { labels: Record<string, string>; retryWaiting: string }

  it('다섯 상태가 픽스처와 같다', () => {
    for (const [status, want] of Object.entries(FIXTURE.labels)) {
      const got = statusAnnouncement(status as never, false)
      // done만 빈 문자열이다 — 레일이 아무 말도 안 하는 것이 그 상태의 표현이다.
      expect(got).toBe(status === 'done' ? '' : want)
    }
  })

  it('백오프 문구가 픽스처와 같다', () => {
    expect(statusAnnouncement('pending', true)).toBe(FIXTURE.retryWaiting)
  })
})
