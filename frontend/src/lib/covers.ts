// Generated covers (R4) — §10 4.5.
//
// R4: "빈칸을 만들지 않는다 — 없으면 생성한다". `thumb: failed` + `status: done`
// is a NORMAL contract combination, so any design that leans on og:image degrades
// into a field of grey boxes. Instead every link without a thumbnail gets a cover
// drawn from what we DO have: the facet of its dominant tag, and its domain.
//
// The R1 boundary this module must not cross: **hue never comes from the hash**.
// The ground and stroke colors are the facet tint/ink tokens (§5.2) — the exact
// same two colors its chips use — so a craft link's cover is craft-colored no
// matter what its domain is. The hash picks GEOMETRY only (which of 4 patterns,
// its rotation, its density). That is what keeps §5.4's ban on hashed tag color
// intact: two links on the same domain look related because they share a
// pattern, not because a hash invented a color for them.

import type { TagFacet } from './tags/facet'
import { FACET_TOKENS } from './tags/facet'

/** The 4 pattern families. Names are descriptive, not decorative. */
export type CoverPatternKind = 'hatch' | 'lattice' | 'contour' | 'stack'

export type CoverPattern = {
  kind: CoverPatternKind
  /** −2..2 degrees — enough to read as "not aligned to the grid", not enough to wobble */
  rotate: number
  /** 12..28px — pattern density */
  step: number
  /** 0..4 — a second axis only `contour` reads (where it centers its arcs) */
  variant: number
}

const KINDS: readonly CoverPatternKind[] = ['hatch', 'lattice', 'contour', 'stack']

/**
 * FNV-1a. Stable across sessions and platforms (no Math.random, no Date) so the
 * same domain always draws the same cover — that stability is the point: the
 * cover becomes a recognizable mark for a source you save from often.
 */
export function hashDomain(domain: string): number {
  let h = 2166136261
  for (let i = 0; i < domain.length; i++) {
    h ^= domain.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

/** domain → geometry. Pure, and deliberately colorless (§5.4). */
export function coverPattern(domain: string): CoverPattern {
  const seed = hashDomain(domain)
  return {
    kind: KINDS[seed % KINDS.length] ?? 'hatch',
    rotate: ((seed >> 4) % 5) - 2,
    step: 12 + ((seed >> 8) % 5) * 4,
    variant: (seed >> 12) % 5,
  }
}

/**
 * facet token name → CSS custom property. `facet.ts` owns the token NAMES (it is
 * the only place facet color is named); this map is the canvas equivalent of
 * Chip.tsx's name→utility map, needed because canvas cannot read Tailwind
 * utilities — it needs the resolved value.
 */
const CSS_VAR: Record<string, string> = {
  'tag-craft-ink': '--tag-craft-ink',
  'tag-craft-tint': '--tag-craft-tint',
  'tag-media-ink': '--tag-media-ink',
  'tag-media-tint': '--tag-media-tint',
  'tag-life-ink': '--tag-life-ink',
  'tag-life-tint': '--tag-life-tint',
  'fg-2': '--fg-2',
  hover: '--bg-hover',
  'cover-craft': '--cover-craft',
  'cover-media': '--cover-media',
  'cover-life': '--cover-life',
  'cover-neutral': '--cover-neutral',
}

export type CoverColors = { ground: string; stroke: string }

/**
 * Resolve the facet's two colors from the live theme (light/dark aware).
 *
 * A resolved value can legitimately be `''` — a `CSS_VAR` value typo, a token
 * map that drifted from `FACET_TOKENS`, or (in dev) CSS not yet applied. Handing
 * `''` to `ctx.fillStyle` is silently ignored by the canvas API, which would
 * paint black or a stale color with no signal, so `read` never returns `''`: it
 * falls back to a neutral grey. The facet tint is ALSO set as the canvas's CSS
 * background (GeneratedCover), so a fully-failed paint still lands on the facet
 * color rather than this grey — the grey is the last-resort floor, not the
 * expected path.
 */
export function coverColors(facet: TagFacet, el: Element): CoverColors {
  const style = getComputedStyle(el)
  const t = FACET_TOKENS[facet]
  const read = (token: string) => style.getPropertyValue(CSS_VAR[token] ?? '--fg-2').trim() || '#808D86'
  // 바탕은 칩 tint가 아니라 **커버 전용 토큰**이다 — 요구가 다르다(§10 4.5.2).
  return { ground: read(t.cover), stroke: read(t.ink) }
}

/** Stroke alpha per pattern. `stack` fills rather than strokes, so it sits lower. */
const ALPHA: Record<CoverPatternKind, number> = {
  hatch: 0.16,
  lattice: 0.16,
  contour: 0.16,
  stack: 0.13,
}

/**
 * Draw one cover. `w`/`h` are CSS pixels — the caller has already applied the
 * devicePixelRatio transform.
 */
export function drawCover(
  ctx: CanvasRenderingContext2D,
  w: number,
  h: number,
  pattern: CoverPattern,
  colors: CoverColors,
): void {
  ctx.clearRect(0, 0, w, h)
  ctx.fillStyle = colors.ground
  ctx.fillRect(0, 0, w, h)

  const { kind, rotate, step, variant } = pattern
  ctx.save()
  ctx.globalAlpha = ALPHA[kind]
  ctx.strokeStyle = colors.stroke
  ctx.fillStyle = colors.stroke
  ctx.lineWidth = 1.25
  // Rotate about the center, then draw past the edges so no corner is left bare.
  ctx.translate(w / 2, h / 2)
  ctx.rotate((rotate * Math.PI) / 180)
  ctx.translate(-w / 2, -h / 2)
  const reach = Math.hypot(w, h)

  if (kind === 'hatch') {
    ctx.beginPath()
    for (let x = -reach; x < reach * 2; x += step) {
      ctx.moveTo(x, -reach)
      ctx.lineTo(x + h + reach, reach * 2)
    }
    ctx.stroke()
  } else if (kind === 'lattice') {
    const r = Math.max(1.4, step / 8)
    for (let y = step / 2; y < h + step; y += step) {
      // every other row offsets by half a step — a lattice, not a grid
      const offset = (Math.floor(y / step) % 2) * (step / 2)
      for (let x = step / 2; x < w + step; x += step) {
        ctx.beginPath()
        ctx.arc(x + offset, y, r, 0, Math.PI * 2)
        ctx.fill()
      }
    }
  } else if (kind === 'contour') {
    const cx = w * (0.18 + variant * 0.16)
    const cy = h * 1.02
    for (let r = step; r < reach; r += step) {
      ctx.beginPath()
      ctx.arc(cx, cy, r, Math.PI, Math.PI * 2)
      ctx.stroke()
    }
  } else if (kind === 'stack') {
    const s = step * 1.4
    for (let i = 0; i * s < w + h; i++) {
      ctx.fillRect(i * s - h, i * s * 0.55, s * 0.62, h * 2)
    }
  } else {
    // Exhaustiveness: adding a CoverPatternKind fails the build here (as it
    // already does at ALPHA), instead of silently rendering as `stack`.
    const _never: never = kind
    void _never
  }
  ctx.restore()
}
