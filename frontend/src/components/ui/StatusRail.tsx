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
import { isProgress, statusAnnouncement } from '../../lib/statusAnnounce'
import { cn } from './cn'

// 라벨 규칙은 `lib/statusAnnounce.ts`에 있다 — 컴포넌트 안에 두면 검증할 수 없고,
// `aria-label`은 스크린샷에도 안 찍혀서 눈으로도 못 본다.

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
  const inProgress = isProgress(status)
  const failed = status === 'failed'

  // Branch order = priority: selected > failed > progress > done(transparent).
  const color = selected
    ? 'bg-accent'
    : failed
      ? 'bg-danger'
      : inProgress
        ? 'bg-rail-progress rail-pulse'
        : 'bg-transparent'

  // 색 단독 표현 금지 — 숨은 텍스트가 따라붙는다 (§4.7 / §7.1).
  const announce = statusAnnouncement(status, retryWaiting)

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
