// LinkCard — the board's atomic unit (§10 4.4). Replaces LinkRow.
//
// Layout (every slot's height is fixed at mount, so the board never reflows as
// the worker fills a card in — CLS 0):
//   [ cover 16:9 ][ title 2줄 ][ description 2줄 ][ chips ][ domain · time ]
// The 3px StatusRail (S1) runs down the leading edge and shows NOTHING when the
// link is done. The cover is the thumbnail when there is one and a GeneratedCover
// (R4) when there is not — never a grey box, never a bare initial.
//
// S2 (채워지는 카드): each slot carries `.fill-step` + `data-in`, driven purely by
// whether its value has arrived. No timers — the poller updates the link and the
// slots transition in on their own, in the order the worker finishes. A card that
// is already complete on first render sets data-in="true" in its initial DOM, so
// it paints solid with no transition.
//
// Click model (§11 3(4)): the card opens the INSPECTOR. A stretched trigger
// button covers it; the title anchor, chips and actions sit above it via
// position:relative so they keep their own clicks.

import { useMemo } from 'react'
import { ExternalLink, Play, StickyNote } from 'lucide-react'
import { Button, Chip, GeneratedCover, Icon, Skeleton, StatusRail } from './ui'
import { dominantFacet, sortLinkTags } from '../lib/tags/facet'
import type { TagFacet } from '../lib/tags/facet'
import type { Link, LinkTag } from '../lib/api/types'
import { linkDisplayTitle } from '../lib/api/types'
import { formatAbsoluteTime, formatRelativeTime, toIso } from '../lib/time'

/**
 * The board's column ladder, declared once so every board (list, save) and every
 * skeleton agree about the grid. Column count is a CONTAINER query, not a
 * viewport one (§10 2.3) — the board narrows when the inspector opens without
 * the window ever resizing. The measuring `@container` belongs to the caller.
 */
export const BOARD_GRID = 'grid grid-cols-1 gap-16 @board-sm:grid-cols-2 @board-md:grid-cols-3'

export type LinkCardProps = {
  link: Link
  /** resolves a LinkTag's facet from the tag-dictionary cache (makeFacetResolver) */
  facetOf: (tag: Pick<LinkTag, 'id'>) => TagFacet
  /** this card is the one currently open in the inspector (?link === id) */
  selected: boolean
  /** the active ?tag filter — a matching chip renders fill 2 (solid) */
  activeTag?: string
  /** open the inspector for this link */
  onOpen: (id: number) => void
  /** toggle the ?tag filter (a card chip is display-styled but still filters, §11 3(4)) */
  onTagClick: (name: string) => void
  /** re-enqueue a failed link's jobs (POST /links/{id}/retry) */
  onRetry?: (link: Link) => void
}

export function LinkCard({
  link,
  facetOf,
  selected,
  activeTag,
  onOpen,
  onTagClick,
  onRetry,
}: LinkCardProps) {
  const title = linkDisplayTitle(link)
  const tags = useMemo(() => sortLinkTags(link.tags), [link.tags])
  const failed = link.status === 'failed'
  const coverFacet = dominantFacet(link.tags, facetOf)

  // S2 gates. The cover is "settled" once the pipeline is terminal — a done link
  // with no thumb_url is not still waiting, it simply has a generated cover.
  const hasTitle = link.title.trim().length > 0
  const hasDesc = link.description.trim().length > 0
  const coverSettled = link.status === 'done' || failed
  // A slot counts as filled once its own value arrived OR the pipeline reached a
  // terminal state — a done link with an empty title is not still loading, it
  // simply has no title and falls back to its domain.
  const titleFilled = hasTitle || coverSettled
  const descFilled = hasDesc || coverSettled

  return (
    <li>
      <article
        className={[
          'group relative flex h-full flex-col overflow-hidden rounded-card bg-surface',
          selected ? 'shadow-lift ring-2 ring-accent' : 'shadow-card hover:shadow-lift',
          'transition-shadow duration-(--dur-1) ease-enter',
        ].join(' ')}
      >
        {/* Stretched inspector trigger — covers the whole card; interactive
            children below sit above it via position:relative (§11 3(4)). */}
        <button
          type="button"
          data-link-id={link.id}
          aria-label={`${title} 상세 열기`}
          onClick={() => onOpen(link.id)}
          className="absolute inset-0 scroll-mt-80 rounded-card focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        />

        {/* S1 rail — leading edge, full card height, 3px (§10 4.7). */}
        <span className="pointer-events-none absolute inset-y-0 left-0 z-10 flex">
          <StatusRail status={link.status} selected={selected} />
        </span>

        {/* Cover. aspect-ratio (not a pixel height) is what keeps the slot
            reserved at every column width. */}
        <div className="relative aspect-[16/9] w-full shrink-0 overflow-hidden bg-hover">
          <div className="fill-step absolute inset-0" data-in={coverSettled}>
            {link.thumb_url ? (
              <img
                src={link.thumb_url}
                alt=""
                loading="lazy"
                decoding="async"
                className="thumb-img h-full w-full object-cover"
              />
            ) : (
              <>
                <GeneratedCover domain={link.domain} facet={coverFacet} />
                {/* The wordmark rides the generated cover only — over a photo it
                    would be unreadable, and the meta line carries the domain
                    anyway. */}
                <span className="absolute bottom-8 left-12 font-mono text-meta text-fg-2">
                  {link.domain}
                </span>
              </>
            )}
          </div>
          {/* content_type: video is the only one that shows anything (§11 1.3). */}
          {link.content_type === 'video' ? (
            <span className="absolute right-8 top-8 flex items-center justify-center rounded-full bg-fg-1/70 p-4 text-fg-inverse">
              <Icon icon={Play} size={16} className="fill-current" />
              <span className="sr-only">동영상</span>
            </span>
          ) : null}
        </div>

        <div className="flex flex-1 flex-col gap-6 p-16">
          {/* Title — 2 lines, slot reserved even while empty.
              `inert` while unfilled: a slot at opacity 0 is still focusable and
              still announced, so without it a keyboard user tabs into an
              invisible link and a screen reader reads a card that is visually
              blank. `inert` removes it from both trees while keeping it mounted,
              which is what lets the fade actually run when the value lands. */}
          <div className="flex min-h-(--size-card-title) items-start gap-6">
            <a
              href={link.url}
              target="_blank"
              rel="noreferrer"
              onClick={(e) => e.stopPropagation()}
              className="fill-step clamp-2 relative text-title text-fg-1 hover:underline"
              data-in={titleFilled}
              inert={titleFilled ? undefined : true}
            >
              {title}
            </a>
            {link.note.trim() ? (
              <span className="shrink-0 pt-2 text-fg-3" title="메모 있음">
                <Icon icon={StickyNote} size={16} />
                <span className="sr-only">메모 있음</span>
              </span>
            ) : null}
          </div>

          {/* Description — the field the row design never used (§10 진단 1). On a
              failed link this slot carries the failure sentence instead. */}
          <p
            className="fill-step clamp-2 min-h-(--size-card-desc) text-card text-fg-2"
            data-in={descFilled}
            inert={descFilled ? undefined : true}
          >
            {failed && !hasDesc ? '수집하지 못했습니다.' : link.description}
          </p>

          {/* Chips — no container query any more: the column count already sets
              a predictable width, and chips wrap instead of being counted. */}
          <div
            className="fill-step relative flex min-h-24 flex-wrap gap-6"
            data-in={tags.length > 0}
            inert={tags.length > 0 ? undefined : true}
          >
            {tags.slice(0, 3).map((t) => (
              <Chip
                key={t.id}
                facet={facetOf(t)}
                role="readonly"
                source={t.source}
                selected={t.name === activeTag}
                onClick={() => onTagClick(t.name)}
              >
                {t.name}
              </Chip>
            ))}
          </div>

          {/* Meta + actions. R2: domain and time are machine data, so mono. */}
          <div className="relative mt-auto flex items-center gap-6 pt-4 font-mono text-meta text-fg-3">
            <span className="truncate">{link.domain}</span>
            <span aria-hidden>·</span>
            <time
              dateTime={toIso(link.created_at)}
              title={formatAbsoluteTime(link.created_at)}
              className="shrink-0"
            >
              {formatRelativeTime(link.created_at)}
            </time>

            <span className="ml-auto flex shrink-0 items-center gap-4">
              {failed && onRetry ? (
                <Button size="sm" variant="secondary" onClick={() => onRetry(link)}>
                  재시도
                </Button>
              ) : null}
              <a
                href={link.url}
                target="_blank"
                rel="noreferrer"
                onClick={(e) => e.stopPropagation()}
                aria-label="원문 열기"
                className="flex h-24 items-center justify-center rounded-control px-6 text-fg-2 opacity-0 transition-opacity duration-(--dur-out) ease-ui hover:bg-hover focus-visible:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100"
              >
                <Icon icon={ExternalLink} size={16} />
                <span className="sr-only">원문 열기</span>
              </a>
            </span>
          </div>
        </div>
      </article>
    </li>
  )
}

/**
 * A card-shaped skeleton at the EXACT final dimensions (CLS 0 — §4.9). The cover
 * slot is an aspect box rather than a pixel height, so it matches the real card
 * at every column width. It does not shimmer: the app's only infinite loop is the
 * progress rail.
 */
export function LinkCardSkeleton() {
  return (
    <li className="overflow-hidden rounded-card bg-surface shadow-card">
      {/* The cover slot is a plain block, not <Skeleton>: Skeleton carries
          `rounded-thumb`, and the card's own overflow-hidden already supplies the
          only corners this slot should have. */}
      <div className="aspect-[16/9] w-full bg-hover" aria-hidden />
      <div className="flex flex-col gap-6 p-16">
        <div className="flex min-h-(--size-card-title) flex-col gap-4">
          <Skeleton variant="text" className="h-16 w-4/5" />
          <Skeleton variant="text" className="h-16 w-3/5" />
        </div>
        <div className="min-h-(--size-card-desc)">
          <Skeleton variant="text" className="h-12 w-full" />
        </div>
        <div className="flex min-h-24 gap-6">
          <Skeleton variant="text" className="h-20 w-56" />
          <Skeleton variant="text" className="h-20 w-40" />
        </div>
        <Skeleton variant="text" className="mt-4 h-12 w-2/5" />
      </div>
    </li>
  )
}
