// LinkRow — the list's atomic unit. A ROW, not a card (§10 4.4).
//
// Fixed height (--size-row 76px ≥560 / --size-row-sm 88px <560) so the list has
// CLS 0 — every slot's dimensions are pre-reserved (§11 1.7). Layout:
//   [2px rail][12][thumb][12][ title / domain·time ][ ≤3 chips ][ hover actions ][16]
// The leading-edge StatusRail (S1) replaces status badges; `done` shows nothing
// (§10 4.7). Chip count is decided by a CONTAINER query, not the viewport — the
// row's usable width changes as the inspector opens/closes (§10 2.3 / §11 3(6)).
//
// Click model (§11 3(4)): the row opens the INSPECTOR (not the original). A
// stretched trigger button covers the row; the title anchor, tag chips and hover
// actions sit above it (position:relative) so they keep their own clicks — the
// title/chips/actions are the only original-open + filter + action affordances.

import { useMemo } from 'react'
import { ExternalLink, Play, StickyNote, Tag as TagIcon } from 'lucide-react'
import { Button, Chip, Icon, StatusRail } from './ui'
import type { TagFacet } from '../lib/tags/facet'
import type { Link, LinkTag } from '../lib/api/types'
import { linkDisplayTitle } from '../lib/api/types'
import { formatAbsoluteTime, formatRelativeTime, toIso } from '../lib/time'

export type LinkRowProps = {
  link: Link
  /** resolves a LinkTag's facet from the tag-dictionary cache (makeFacetResolver) */
  facetOf: (tag: Pick<LinkTag, 'id'>) => TagFacet
  /** this row is the one currently open in the inspector (?link === id) */
  selected: boolean
  /** the active ?tag filter — a matching row chip renders fill 2 (solid) */
  activeTag?: string
  /** open the inspector for this link */
  onOpen: (id: number) => void
  /** toggle the ?tag filter (a row chip is display-styled but still filters, §11 3(4)) */
  onTagClick: (name: string) => void
  /** re-enqueue a failed link's jobs (POST /links/{id}/retry) */
  onRetry?: (link: Link) => void
}

// manual first → confidence desc → name (§11 3(3) tag chip ordering).
function sortTags(tags: readonly LinkTag[]): LinkTag[] {
  return [...tags].sort((a, b) => {
    const am = a.source === 'manual' ? 0 : 1
    const bm = b.source === 'manual' ? 0 : 1
    if (am !== bm) return am - bm
    const ac = a.confidence ?? -1
    const bc = b.confidence ?? -1
    if (ac !== bc) return bc - ac
    return a.name.localeCompare(b.name)
  })
}

export function LinkRow({
  link,
  facetOf,
  selected,
  activeTag,
  onOpen,
  onTagClick,
  onRetry,
}: LinkRowProps) {
  const title = linkDisplayTitle(link)
  const tags = useMemo(() => sortTags(link.tags), [link.tags])
  const failed = link.status === 'failed'
  const initial = (link.domain || link.url).trim().charAt(0).toUpperCase() || '·'

  return (
    <li
      className={[
        'group relative flex items-stretch overflow-hidden rounded-row',
        'h-(--size-row-sm) sm:h-(--size-row)',
        selected ? 'bg-selected' : 'hover:bg-hover',
        'transition-colors duration-(--dur-out) ease-ui',
      ].join(' ')}
    >
      {/* Stretched inspector trigger — covers the whole row; interactive
          children below sit above it via position:relative (§11 3(4)). */}
      <button
        type="button"
        data-link-id={link.id}
        aria-label={`${title} 상세 열기`}
        onClick={() => onOpen(link.id)}
        className="absolute inset-0 scroll-mt-80 rounded-row focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
      />

      {/* S1 rail — flush to the leading edge, full row height, 2px (§10 4.7). */}
      <StatusRail status={link.status} selected={selected} />

      <div className="flex min-w-0 flex-1 items-center gap-12 pl-12 pr-16">
        {/* Thumbnail (or domain-initial fallback — NO color hash, §10 4.4). */}
        <div className="relative h-(--size-thumb-sm) w-(--size-thumb-sm) shrink-0 overflow-hidden rounded-thumb bg-hover sm:h-(--size-thumb) sm:w-(--size-thumb)">
          {link.thumb_url ? (
            <img
              src={link.thumb_url}
              alt=""
              loading="lazy"
              decoding="async"
              className="thumb-img h-full w-full object-cover"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-head text-fg-3">
              {initial}
            </div>
          )}
          {/* content_type video → play glyph; the other 3 types show nothing (§11 1.3). */}
          {link.content_type === 'video' ? (
            <span className="absolute bottom-2 right-2 flex items-center justify-center rounded-full bg-fg-1/70 p-2 text-fg-inverse">
              <Icon icon={Play} size={16} className="fill-current" />
            </span>
          ) : null}
        </div>

        {/* Title + meta line. */}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-6">
            <a
              href={link.url}
              target="_blank"
              rel="noreferrer"
              onClick={(e) => e.stopPropagation()}
              className="relative truncate text-title text-fg-1 hover:underline"
            >
              {title}
            </a>
            {link.note.trim() ? (
              <span className="shrink-0 text-fg-3" title="메모 있음">
                <Icon icon={StickyNote} size={16} />
                <span className="sr-only">메모 있음</span>
              </span>
            ) : null}
          </div>
          <div className="mt-2 flex min-w-0 items-center gap-6 text-meta font-mono text-fg-3">
            <span className="truncate">{link.domain}</span>
            <span aria-hidden>·</span>
            <time
              dateTime={toIso(link.created_at)}
              title={formatAbsoluteTime(link.created_at)}
              className="shrink-0"
            >
              {formatRelativeTime(link.created_at)}
            </time>
          </div>
        </div>

        {/* Tag chips — count driven by the container query (§10 2.3). Below
            @row-sm show only a summary (icon + count): 24px chips + 44px touch
            targets don't fit on phones (§10 4.4). */}
        {tags.length > 0 ? (
          <div className="relative flex shrink-0 items-center gap-6">
            <span className="inline-flex items-center gap-4 text-label text-fg-3 @row-sm:hidden">
              <Icon icon={TagIcon} size={16} />
              <span className="tabular-nums">{tags.length}</span>
              <span className="sr-only">태그 {tags.length}개</span>
            </span>
            <div className="hidden items-center gap-6 @row-sm:flex">
              {tags.slice(0, 3).map((t, i) => (
                <Chip
                  key={t.id}
                  facet={facetOf(t)}
                  role="readonly"
                  source={t.source}
                  selected={t.name === activeTag}
                  onSelectedRow={selected}
                  onClick={() => onTagClick(t.name)}
                  className={i === 2 ? 'hidden @row-md:inline-flex' : undefined}
                >
                  {t.name}
                </Chip>
              ))}
            </div>
          </div>
        ) : null}

        {/* Right-side actions. Failed → an always-visible 재시도 (secondary,
            never a danger fill — §10 2.1.4-b). Open reveals on hover/focus, and
            is always shown on touch (@media hover:none, §10 4.4). */}
        <div className="relative flex shrink-0 items-center gap-4">
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
            className="flex h-24 items-center justify-center rounded-control px-8 text-fg-2 opacity-0 transition-opacity duration-(--dur-out) ease-ui hover:bg-hover focus-visible:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100"
          >
            <Icon icon={ExternalLink} size={16} />
            <span className="sr-only">원문 열기</span>
          </a>
        </div>
      </div>
    </li>
  )
}
