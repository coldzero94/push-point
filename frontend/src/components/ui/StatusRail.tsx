// StatusRail (S1) — §4.7.
//
// A single `--size-rail` (2px) vertical stroke at the leading edge replaces the
// old 5-color StatusBadge. `done` shows NOTHING — every remaining stroke means
// "something is happening or wrong". Thickness is the one rail token; hierarchy
// comes from position, never width. Colors are semantic tokens only. The pulse
// is the app's only infinite loop and becomes static under reduced-motion (the
// global rule handles `.rail-pulse`). Because color alone must not carry
// meaning, a visually-hidden status label accompanies every non-done state.

import type { LinkStatus } from '../../lib/api/types'
import { cn } from './cn'

// Korean status labels — shared verbatim with iOS (§8.1).
const STATUS_LABEL: Record<LinkStatus, string> = {
  pending: '대기',
  scraping: '수집 중',
  tagging: '태깅 중',
  done: '완료',
  failed: '실패',
}

const PROGRESS: ReadonlySet<LinkStatus> = new Set(['pending', 'scraping', 'tagging'])

export type StatusRailProps = {
  status: LinkStatus
  /**
   * 이 링크가 **백오프로 누워 있는가**(계약의 `retry_state === 'waiting'`).
   *
   * `status`는 여전히 `pending`이라 레일이 돌고 화면은 일하는 중이라고 말하는데, 실제로는
   * 기다리는 중이다 — 그 둘이 지금까지 똑같이 생겼다(12 §4.3). 색과 펄스는 그대로 두고
   * **말만 바꾼다**: 진행은 진행이고, 다른 것은 왜 멈춰 보이는지다.
   */
  retryWaiting?: boolean
  /** selection wins over failed — it is the user's current intent (§4.7) */
  selected?: boolean
  className?: string
}

export function StatusRail({
  status,
  selected = false,
  retryWaiting = false,
  className,
}: StatusRailProps) {
  const inProgress = PROGRESS.has(status)
  const failed = status === 'failed'

  // Branch order = priority: selected > failed > progress > done(transparent).
  const color = selected
    ? 'bg-accent'
    : failed
      ? 'bg-danger'
      : inProgress
        ? 'bg-rail-progress rail-pulse'
        : 'bg-transparent'

  // Announce non-done states in a channel that is not color (§4.7 / §7.1).
  const announce = retryWaiting
    ? '재시도 대기 중' // iOS `statusLabel`과 같은 단어 (§8.1)
    : failed || inProgress
      ? STATUS_LABEL[status]
      : ''

  return (
    <span
      aria-hidden={announce ? undefined : true}
      role={failed ? 'status' : undefined}
      className={cn('block h-full w-(--size-rail) shrink-0', color, className)}
    >
      {announce ? <span className="sr-only">{announce}</span> : null}
    </span>
  )
}
