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

/**
 * domain → geometry. Pure, and deliberately colorless (§5.4).
 *
 * **The shifts must be `>>>`, not `>>`.** `hashDomain` returns a full uint32, but
 * JavaScript's `>>` coerces to int32 first, so any hash with the high bit set —
 * about half of all domains — comes out negative. `news.ycombinator.com` hashes
 * to 2307374702 and produced `step = -4`, which threw
 * `Failed to execute 'arc' … The radius provided (-4) is negative` and took the
 * entire list screen down with it (2026-08-03). Swift has no such coercion:
 * `CoverPattern.swift` shifts a `UInt32`, so iOS was right and the web was drawing
 * different covers for half the domains it had ever seen.
 *
 * `testdata/cover-cases.json` pins both sides against the same numbers.
 */
export function coverPattern(domain: string): CoverPattern {
  const seed = hashDomain(domain)
  return {
    kind: KINDS[seed % KINDS.length] ?? 'hatch',
    rotate: ((seed >>> 4) % 5) - 2,
    step: 12 + ((seed >>> 8) % 5) * 4,
    variant: (seed >>> 12) % 5,
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
const LINE_WIDTH = 1.25

/** One primitive. `arc` is the lower half only (π → 2π), which is what contour draws. */
export type CoverOp =
  | { op: 'line'; x1: number; y1: number; x2: number; y2: number }
  | { op: 'dot'; cx: number; cy: number; r: number }
  | { op: 'arc'; cx: number; cy: number; r: number }
  | { op: 'rect'; x: number; y: number; w: number; h: number }

export interface CoverGeometry {
  alpha: number
  lineWidth: number
  /** Degrees, applied about the centre of the box before anything is drawn. */
  rotate: number
  mode: 'stroke' | 'fill'
  ops: CoverOp[]
}

/**
 * The drawing, as data.
 *
 * **This exists so iOS can be held to it.** The two clients agreed on the pattern
 * *parameters* (`testdata/cover-cases.json`) while drawing four completely different
 * pictures from them — web hatched diagonally where iOS drew verticals, web dotted a
 * lattice where iOS ruled a grid, web arced half circles from below the frame where
 * iOS drew whole ellipses through the middle. Every one of those passed both test
 * suites, because nothing compared the *marks*. `testdata/cover-ops.json` does now.
 *
 * `w`/`h` are CSS pixels; the caller has already applied device pixel ratio.
 */
export function coverGeometry(pattern: CoverPattern, w: number, h: number): CoverGeometry {
  const { kind, rotate, step, variant } = pattern
  // Draw past the edges so no corner is left bare once the box is rotated.
  const reach = Math.hypot(w, h)
  const ops: CoverOp[] = []

  if (kind === 'hatch') {
    for (let x = -reach; x < reach * 2; x += step) {
      ops.push({ op: 'line', x1: x, y1: -reach, x2: x + h + reach, y2: reach * 2 })
    }
  } else if (kind === 'lattice') {
    const r = Math.max(1.4, step / 8)
    for (let y = step / 2; y < h + step; y += step) {
      // every other row offsets by half a step — a lattice, not a grid
      const offset = (Math.floor(y / step) % 2) * (step / 2)
      for (let x = step / 2; x < w + step; x += step) {
        ops.push({ op: 'dot', cx: x + offset, cy: y, r })
      }
    }
  } else if (kind === 'contour') {
    const cx = w * (0.18 + variant * 0.16)
    const cy = h * 1.02
    for (let r = step; r < reach; r += step) {
      ops.push({ op: 'arc', cx, cy, r })
    }
  } else if (kind === 'stack') {
    const s = step * 1.4
    for (let i = 0; i * s < w + h; i++) {
      ops.push({ op: 'rect', x: i * s - h, y: i * s * 0.55, w: s * 0.62, h: h * 2 })
    }
  } else {
    // Exhaustiveness: adding a CoverPatternKind fails the build here (as it
    // already does at ALPHA), instead of silently rendering as `stack`.
    const _never: never = kind
    void _never
  }

  return {
    alpha: ALPHA[kind],
    lineWidth: LINE_WIDTH,
    rotate,
    mode: kind === 'lattice' || kind === 'stack' ? 'fill' : 'stroke',
    ops,
  }
}

/** Paint the geometry. Kept thin on purpose — the shape decisions all live above. */
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

  const g = coverGeometry(pattern, w, h)
  ctx.save()
  ctx.globalAlpha = g.alpha
  ctx.strokeStyle = colors.stroke
  ctx.fillStyle = colors.stroke
  ctx.lineWidth = g.lineWidth
  ctx.translate(w / 2, h / 2)
  ctx.rotate((g.rotate * Math.PI) / 180)
  ctx.translate(-w / 2, -h / 2)

  if (g.mode === 'stroke') ctx.beginPath()
  for (const o of g.ops) {
    switch (o.op) {
      case 'line':
        ctx.moveTo(o.x1, o.y1)
        ctx.lineTo(o.x2, o.y2)
        break
      case 'dot':
        ctx.beginPath()
        ctx.arc(o.cx, o.cy, o.r, 0, Math.PI * 2)
        ctx.fill()
        break
      case 'arc':
        ctx.beginPath()
        ctx.arc(o.cx, o.cy, o.r, Math.PI, Math.PI * 2)
        ctx.stroke()
        break
      case 'rect':
        ctx.fillRect(o.x, o.y, o.w, o.h)
        break
    }
  }
  if (g.mode === 'stroke' && g.ops[0]?.op === 'line') ctx.stroke()
  ctx.restore()
}
