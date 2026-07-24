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
  /** selection wins over failed — it is the user's current intent (§4.7) */
  selected?: boolean
  className?: string
}

export function StatusRail({ status, selected = false, className }: StatusRailProps) {
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
  const announce = failed || inProgress ? STATUS_LABEL[status] : ''

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
