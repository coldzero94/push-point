// Tag facet system — §5 (design system) / §5.4 (web UX).
//
// Color belongs to 4 facets, not to individual tags (§5.1). This module is the
// ONLY place facet color is named, and it names *tokens*, never hex values.
// The web maps token names to Tailwind utilities; iOS maps the same names to
// Asset Catalog colors (§8.1). Neither hard-codes hex.

import type { LinkTag, Tag, TagFacet } from '../api/types'

// Re-exported so screens import the facet vocabulary from one place.
export type { TagFacet }

/** All facets in canonical order (craft-heavy palette — §5.5). */
export const TAG_FACETS: readonly TagFacet[] = ['craft', 'media', 'life', 'neutral']

/**
 * facet -> token names. The only place color appears, and it is names (not
 * values) — §5.2. `neutral` makes NO new token: it reuses `fg-2` / `hover`, so
 * the token name itself says "there is no color here".
 */
export const FACET_TOKENS: Record<TagFacet, { ink: string; tint: string }> = {
  craft: { ink: 'tag-craft-ink', tint: 'tag-craft-tint' },
  media: { ink: 'tag-media-ink', tint: 'tag-media-tint' },
  life: { ink: 'tag-life-ink', tint: 'tag-life-tint' },
  neutral: { ink: 'fg-2', tint: 'hover' }, // reuse existing tokens (§5.2)
}

/**
 * Korean UI labels — shared verbatim by web & iOS so the two clients never use
 * different words for the same facet (§5.5 / §8.1).
 */
export const FACET_LABELS: Record<TagFacet, string> = {
  craft: '만드는 것',
  media: '형식',
  life: '일 바깥',
  neutral: '분류 없음',
}

/**
 * facet of a dictionary Tag. The contract guarantees `Tag.facet`, so this is a
 * direct read; it exists so callers go through the facet vocabulary rather than
 * reaching into the raw field.
 */
export function facetOf(tag: Pick<Tag, 'facet'>): TagFacet {
  return tag.facet
}

/**
 * Resolve a LinkTag's facet from the tag-dictionary cache.
 *
 * `LinkTag` has no `facet` (a contract decision, not an omission — §10): the
 * list would otherwise ship a facet string per link×tag. The list/inspector
 * chips look the facet up by tag id in the `GET /api/v1/tags` payload the filter
 * bar already holds. A cache miss is NOT guessed — it renders `neutral`, which
 * is the exact fallback (a freshly created tag before the cache refreshes), not
 * a bug (§9 contract alignment).
 */
export function makeFacetResolver(
  tags: readonly Tag[] | undefined,
): (tag: Pick<LinkTag, 'id'>) => TagFacet {
  const byId = new Map<number, TagFacet>()
  for (const t of tags ?? []) byId.set(t.id, t.facet)
  return (tag) => byId.get(tag.id) ?? 'neutral'
}

/** `tag.source` from the contract (rules / embed / manual). */
type TagSource = LinkTag['source']

export type ChipInput = {
  facet: TagFacet
  /** matches the current ?tag filter (the contract's `tag` param is a single value) */
  selected: boolean
  /** null = filter bar (not a tag attached to a link) */
  source: TagSource | null
  /** filter-bar chip = control, row/inspector chip = readonly */
  role: 'control' | 'readonly'
  /** inside a selected row, `tint` collides with the row background (§4.3) */
  onSelectedRow: boolean
}

/** Token names only — never raw hex. Consumed by the Chip component. */
export type ChipStyle = { bg: string; fg: string; border: string }

/**
 * Chip style is fully determined by this pure function. hue = facet, fill =
 * state (R1). Branch order IS the priority — `selected` (fill 2) is evaluated
 * first, so a chip matching the active `?tag` keeps its solid fill even inside a
 * selected row; `onSelectedRow` only demotes the `manual` (fill 1) branch,
 * because `--bg-selected` equals the `craft` tint (§4.3 / §5.2).
 */
export function chipStyle(i: ChipInput): ChipStyle {
  const t = FACET_TOKENS[i.facet]
  if (i.selected) return { bg: t.ink, fg: 'surface', border: 'transparent' } // fill 2
  if (i.role === 'control') return { bg: t.tint, fg: t.ink, border: 'line-control' } // fill 1 + control border
  if (i.source === 'manual' && !i.onSelectedRow)
    return { bg: t.tint, fg: t.ink, border: 'transparent' } // fill 1
  return { bg: 'transparent', fg: t.ink, border: 'transparent' } // fill 0
}
