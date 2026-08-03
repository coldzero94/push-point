// LinkCard — the board's atomic unit (§10 4.4). Replaces LinkRow.
//
// Layout (every slot's height is RESERVED at mount — aspect-ratio + min-height +
// 2-line clamp, so the worker filling a card in does not reflow the board, CLS 0.
// The one caveat: the chips slot reserves a single row, so three chips wrapping
// to a second row at a narrow column can still grow it):
//   [ cover 16:9 ][ title 2줄 ][ description 2줄 ][ chips ][ domain · time ]
// The 2px StatusRail (S1) runs down the leading edge and shows NOTHING when the
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

import { useMemo, useState } from 'react'
import { ExternalLink, Play, StickyNote } from 'lucide-react'
import { Button, Chip, GeneratedCover, Icon, Skeleton, StatusRail } from './ui'
import { sortLinkTags } from '../lib/tags/facet'
import type { TagFacet } from '../lib/tags/facet'
import type { Link, LinkTag } from '../lib/api/types'
import { linkDisplayTitle } from '../lib/api/types'
import { t } from '../lib/i18n'
import { formatAbsoluteTime, formatRelativeTime, toIso } from '../lib/time'
import { markOpened } from '../lib/api/markOpened'

/**
 * The board's column ladder, declared once so every board (list, save) and every
 * skeleton agree about the grid. Column count is a CONTAINER query, not a
 * viewport one (§10 2.3) — the board narrows when the inspector opens without
 * the window ever resizing. The measuring `@container` belongs to the caller.
 */
export const BOARD_GRID = 'grid grid-cols-1 gap-16 @board-sm:grid-cols-2 @board-md:grid-cols-3'

export type LinkCardProps = {
  link: Link
  /**
   * Resolves a LinkTag's facet from the tag-dictionary cache
   * (makeFacetResolver, keyed by tag id). Named `resolveFacet` rather than
   * `facetOf` to avoid colliding with `facet.ts`'s `facetOf(tag)`, which reads a
   * dictionary Tag's `.facet` field directly — a different function.
   */
  resolveFacet: (tag: Pick<LinkTag, 'id'>) => TagFacet
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
  resolveFacet,
  selected,
  activeTag,
  onOpen,
  onTagClick,
  onRetry,
}: LinkCardProps) {
  const title = linkDisplayTitle(link)
  const tags = useMemo(() => sortLinkTags(link.tags), [link.tags])
  const failed = link.status === 'failed'
  // The dominant tag is the first in chip order (manual first, then confidence).
  // `tags` is already that order, so read it directly instead of re-sorting via
  // dominantFacet (which the inspector uses where no sorted list exists).
  const coverFacet: TagFacet = tags[0] ? resolveFacet(tags[0]) : 'neutral'

  // S2 gates. The cover is "settled" once the pipeline is terminal — a done link
  // with no thumb_url is not still waiting, it simply has a generated cover.
  // 썸네일 전송이 실패했는가. 실패하면 생성 커버로 떨어진다 — 화면은 정상이고,
  // 사실은 로그에 남는다.
  const [thumbFailed, setThumbFailed] = useState(false)
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
          aria-label={t('common.openDetail', { title })}
          onClick={() => onOpen(link.id)}
          className="absolute inset-0 scroll-mt-80 rounded-card focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
        />

        {/* S1 rail — leading edge, full card height, 2px (§10 4.7). */}
        <span className="pointer-events-none absolute inset-y-0 left-0 z-10 flex">
          <StatusRail
            status={link.status}
            selected={selected}
            retryWaiting={link.retry_state === 'waiting'}
          />
        </span>

        {/* Cover. aspect-ratio (not a pixel height) is what keeps the slot
            reserved at every column width. `pointer-events-none`: the cover and
            the whole content column below are later positioned/transformed
            siblings of the stretched trigger, so without this they paint OVER it
            and swallow the click. Everything is click-through to the trigger
            except the interactive leaves, which re-enable pointer events. */}
        <div className="pointer-events-none relative aspect-[16/9] w-full shrink-0 overflow-hidden bg-hover">
          <div className="fill-step absolute inset-0" data-in={coverSettled}>
            {link.thumb_url && !thumbFailed ? (
              <img
                src={link.thumb_url}
                alt=""
                loading="lazy"
                decoding="async"
                className="thumb-img h-full w-full object-cover"
                // **`onError`가 없으면 R4가 깨진다.** 실패한 `<img>`는 `alt=""`라 아무것도
                // 그리지 않고 컨테이너의 `bg-hover`가 드러난다 — 회색 상자다. R4가 이름으로
                // 금지한 것이고(§4.5), 로그도 없어서 아무도 모른다.
                //
                // iOS는 같은 커밋(#76)에서 폴백과 로그를 둘 다 얻었는데 웹은 둘 다 없었다.
                // 서버가 없는 파일을 광고하지 않게 고쳤으니 파일 부재 경로는 줄었지만,
                // 전송 실패·0바이트 JPEG·목록과 이미지 요청 사이의 삭제는 그대로 남는다.
                onError={() => {
                  setThumbFailed(true)
                  // 화면은 바뀌지 않는다(생성 커버는 정상 표시다) — 대신 흔적을 남긴다.
                  console.error(
                    `[push-point] thumb load failed link=${link.id} url=${link.thumb_url}`,
                  )
                }}
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
              <span className="sr-only">{t('common.video')}</span>
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
              onClick={(e) => {
                e.stopPropagation()
                markOpened(link.id)
              }}
              className="fill-step clamp-2 relative text-title text-fg-1 hover:underline"
              data-in={titleFilled}
              inert={titleFilled ? undefined : true}
            >
              {title}
            </a>
            {link.note.trim() ? (
              <span className="shrink-0 pt-2 text-fg-3" title={t('common.hasNote')}>
                <Icon icon={StickyNote} size={16} />
                <span className="sr-only">{t('common.hasNote')}</span>
              </span>
            ) : null}
          </div>

          {/* Description — the field the old row list never showed but the
              contract already returns (200 chars in the list response, §11 3(3)).
              On a failed link this slot carries the failure sentence instead. */}
          <p
            className="fill-step clamp-2 min-h-(--size-card-desc) text-card text-fg-2"
            data-in={descFilled}
            inert={descFilled ? undefined : true}
          >
            {/* 실패 사유를 여기 쓴다 — 예전에는 모든 실패가 "수집하지 못했습니다." 한 문장
                이었고, 무엇이 잘못됐는지 보려면 링크마다 상세를 열어야 했다. 계약이 사유를
                목록으로 올렸다(`Link.error`). iOS의 failureLabel과 같은 폴백을 쓴다. */}
            {failed && !hasDesc
              ? link.error || t('common.collectFailed')
              : link.description}
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
                facet={resolveFacet(t)}
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
                  {t('common.retry')}
                </Button>
              ) : null}
              <a
                href={link.url}
                target="_blank"
                  rel="noreferrer"
                onClick={(e) => {
                  e.stopPropagation()
                  markOpened(link.id)
                }}
                aria-label={t('common.openOriginal')}
                className="flex h-24 items-center justify-center rounded-control px-6 text-fg-2 opacity-0 transition-opacity duration-(--dur-out) ease-ui hover:bg-hover focus-visible:opacity-100 group-hover:opacity-100 [@media(hover:none)]:opacity-100"
              >
                <Icon icon={ExternalLink} size={16} />
                <span className="sr-only">{t('common.openOriginal')}</span>
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
