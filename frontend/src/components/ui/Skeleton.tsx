// Skeleton — §4.9.
//
// A block the EXACT size of the real content it stands in for (CLS 0). It does
// NOT move — no shimmer, no self-fade loop. The app's only infinite loop is the
// progress rail; several phase-shifted skeletons blinking would be noise, and
// the same row's rail already signals "in progress". The 200ms suppression rule
// (don't render at all if the response beats 200ms) is the caller's guard.

import { cn } from './cn'

type SkeletonVariant = 'thumb' | 'text' | 'block'

export type SkeletonProps = {
  variant?: SkeletonVariant
  /** token-based sizing utilities, e.g. `h-16 w-content` */
  className?: string
}

export function Skeleton({ variant = 'block', className }: SkeletonProps) {
  return (
    <div
      aria-hidden
      // text lines use a 4px radius (§4.9) — no radius token at 4px.
      style={variant === 'text' ? { borderRadius: '4px' } : undefined}
      className={cn('bg-hover', variant !== 'text' && 'rounded-thumb', className)}
    />
  )
}
