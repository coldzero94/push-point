// EmptyState — §4.8.
//
// A `text-head` title + a `text-body`/`fg-2` description + 0–1 action. 56px of
// vertical breathing room. No illustration, no mascot, no emoji. Never rendered
// while `isLoading` (§4.8 / §1.7) — that is the caller's guard.

import type { ReactNode } from 'react'
import { cn } from './cn'

export type EmptyStateProps = {
  title: string
  description?: string
  /** at most one action (e.g. a Button) */
  action?: ReactNode
  className?: string
}

export function EmptyState({ title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center py-56 text-center', className)}>
      <h2 className="text-head text-fg-1">{title}</h2>
      {description ? <p className="mt-8 text-body text-fg-2">{description}</p> : null}
      {action ? <div className="mt-24">{action}</div> : null}
    </div>
  )
}
