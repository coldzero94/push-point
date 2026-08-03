import type { LinkStatus } from './api/types'
import { t } from './i18n'

/**
 * 상태 레일이 **색 말고 무엇으로 말하는가**.
 *
 * §4.7과 §7.1이 색 단독 표현을 금지하므로 모든 비-done 상태에는 숨은 텍스트가 따라붙는다.
 * 그 텍스트를 정하는 규칙이 여기 있다 — 화면 컴포넌트 안에 두면 검증할 수가 없다.
 * (`aria-label`은 스크린샷에 안 찍히므로 눈으로 볼 수도 없다. 그래서 더더욱 테스트다.)
 *
 * iOS `LinkCard.statusLabel`과 **같은 단어**를 써야 한다(§8.1). 두 화면이 같은 상태를
 * 다르게 부르면 사용자가 둘을 다른 일로 읽는다.
 */
// 값이 문자열이 아니라 함수인 이유는 둘이다. 모듈이 읽히는 순간 문자열을 굳히면 언어를
// 바꿔도 옛 낱말이 남고, 키를 `t(MAP[status])`로 조립하면 `scripts/web_i18n_check.py`가
// 호출을 못 본다. 다섯 갈래를 그대로 펴 두면 둘 다 없다.
const STATUS_LABEL: Record<LinkStatus, () => string> = {
  pending: () => t('status.pending'),
  scraping: () => t('status.scraping'),
  tagging: () => t('status.tagging'),
  done: () => t('status.done'),
  failed: () => t('status.failed'),
}

const PROGRESS: ReadonlySet<LinkStatus> = new Set(['pending', 'scraping', 'tagging'])

export function isProgress(status: LinkStatus): boolean {
  return PROGRESS.has(status)
}

/**
 * 레일이 읽어 줄 문장. `done`이면 빈 문자열이고, 그때 레일은 `aria-hidden`이 된다.
 *
 * `retryWaiting`이 **status보다 우선한다**: 백오프로 누워 있는 링크는 `status`가 여전히
 * `pending`이라 "대기"로 읽히는데, 그건 큐에서 순서를 기다리는 것과 구분되지 않는다.
 * 실제로는 실패해서 30×attempts초를 세는 중이고, 그 둘은 사용자에게 다른 일이다.
 */
export function statusAnnouncement(status: LinkStatus, retryWaiting: boolean): string {
  if (retryWaiting) return t('status.retryWaiting')
  return status === 'failed' || isProgress(status) ? STATUS_LABEL[status]() : ''
}
