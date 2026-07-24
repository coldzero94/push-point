// Badge — §4.6.
//
// Badges are NOT used for status (R3 — status is a rail). Only two uses remain:
// `count` (tag/result counts) and `notice` (duplicate save, out-of-dictionary
// tag). `notice` must always accompany a sentence and never carries a tag name
// or sits beside a chip (§4.6). 18px has no spacing token → explicit height.

import type { ReactNode } from 'react'
import { cn } from './cn'

type BadgeVariant = 'count' | 'notice'

const VARIANT: Record<BadgeVariant, string> = {
  count: 'bg-hover text-fg-2 font-mono tabular-nums',
  notice: 'bg-warn-tint text-warn',
}

export type BadgeProps = {
  variant?: BadgeVariant
  children: ReactNode
  className?: string
}

export function Badge({ variant = 'count', children, className }: BadgeProps) {
  return (
    <span
      // height 18px (§4.6) — no 12-step token exists at 18px.
      style={{ height: '18px' }}
      className={cn(
        'inline-flex items-center rounded-chip px-6 text-label',
        VARIANT[variant],
        className,
      )}
    >
      {children}
    </span>
  )
}
