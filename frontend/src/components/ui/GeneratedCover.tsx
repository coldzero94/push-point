// GeneratedCover (R4) — §4.5.
//
// The cover a link gets when it has no thumbnail. It is NOT a placeholder for a
// missing image: `thumb: failed` + `status: done` is a normal terminal state, so
// for a large share of links this IS the final cover. It says what the machine
// decided (facet color) and where the link came from (the pattern is stable per
// domain) — nothing it is pretending to be a photograph of.
//
// Color comes from the facet tokens only; the domain hash picks geometry only
// (lib/covers.ts explains why that boundary is load-bearing for R1/§5.4).

import { useEffect, useMemo, useRef, useSyncExternalStore } from 'react'
import { coverColors, coverPattern, drawCover } from '../../lib/covers'
import { effectiveDark, subscribeTheme } from '../../lib/theme'
import type { TagFacet } from '../../lib/tags/facet'
import { cn } from './cn'

export type GeneratedCoverProps = {
  /** the link's domain — the only input to the pattern */
  domain: string
  /** facet of the dominant tag; `neutral` when the link has no tags yet */
  facet: TagFacet
  className?: string
}

export function GeneratedCover({ domain, facet, className }: GeneratedCoverProps) {
  const ref = useRef<HTMLCanvasElement>(null)
  const pattern = useMemo(() => coverPattern(domain), [domain])

  // Canvas pixels do not follow a CSS class change — repaint when the resolved
  // theme flips (lib/theme.ts owns the store).
  const dark = useSyncExternalStore(subscribeTheme, effectiveDark, () => false)

  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return

    const paint = () => {
      const { clientWidth: w, clientHeight: h } = canvas
      if (w === 0 || h === 0) return
      // Cap at 2× — a 3× repaint on every card costs more than it shows.
      const dpr = Math.min(window.devicePixelRatio || 1, 2)
      canvas.width = Math.round(w * dpr)
      canvas.height = Math.round(h * dpr)
      const ctx = canvas.getContext('2d')
      if (!ctx) return
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      drawCover(ctx, w, h, pattern, coverColors(facet, canvas))
    }

    paint()
    // The board reflows when the inspector opens/closes and when columns change;
    // a canvas does not rescale, it has to be redrawn at the new size.
    const ro = new ResizeObserver(paint)
    ro.observe(canvas)
    return () => ro.disconnect()
  }, [pattern, facet, dark])

  return (
    <canvas
      ref={ref}
      aria-hidden
      className={cn('block h-full w-full', className)}
    />
  )
}
