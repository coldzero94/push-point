// List (screen 2) — the app home (§11 3). A card BOARD broken by a time spine,
// with a sticky toolbar: a search launcher + status dropdown, then a tag filter
// chip bar. `?tag` / `?status` are URL state; `?link` opens the inspector overlay
// (the contract with the inspector owner — §11 0).
//
// Two structural decisions live here:
//  - Column count comes from a CONTAINER query, not the viewport (§10 2.3): the
//    board's usable width changes when the inspector opens, and the board should
//    drop a column then even though the window never resized.
//  - Groups come from `timeGroup` over an already-DESC list (lib/time.ts), so the
//    spine costs one pass and can never emit the same group twice.
//
// The three states are skeleton cards (loading) / EmptyState (empty) / an inline
// error block (§11 3(5) — list errors are an inline block, NOT a toast).

import { useEffect, useRef, useState } from 'react'
import { getRouteApi, Link, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useResurfaced } from '../hooks/useResurfaced'
import { Search } from 'lucide-react'
import { useLinks } from '../hooks/useLinks'
import { useTags } from '../hooks/useTags'
import { useCreateLink, useDeleteLink, useRetryLink } from '../hooks/useLinkMutations'
import { startPolling } from '../hooks/useSaveLink'
import { useRowKeyboard } from '../hooks/useRowKeyboard'
import { requestInspectorFocus } from '../lib/keyboard/inspectorFocus'
import { BOARD_GRID, LinkCard, LinkCardSkeleton } from '../components/LinkCard'
import { StatusFilter } from '../components/StatusFilter'
import { Button, Chip, EmptyState, Icon, useToast } from '../components/ui'
import { makeFacetResolver } from '../lib/tags/facet'
import { errorMessage } from '../lib/api/client'
import { t } from '../lib/i18n'
import { boardView } from '../lib/board'
import { timeGroup } from '../lib/time'
import type { Link as LinkItem, LinkStatus, Tag } from '../lib/api/types'

const route = getRouteApi('/')

// §11 1.7: suppress the skeleton unless the request outlives 200ms (a flash of
// skeleton reads slower than none). Local p99 < 50ms, so this is usually false.
function useDelayed(active: boolean, ms = 200): boolean {
  const [on, setOn] = useState(false)
  useEffect(() => {
    if (!active) {
      setOn(false)
      return
    }
    const t = window.setTimeout(() => setOn(true), ms)
    return () => window.clearTimeout(t)
  }, [active, ms])
  return on
}

type TimeGroupedLinks = { key: string; label: string; links: LinkItem[] }

/** Walk a DESC-sorted list once, splitting it at every time-group boundary. */
function groupByTime(links: readonly LinkItem[]): TimeGroupedLinks[] {
  const groups: TimeGroupedLinks[] = []
  for (const link of links) {
    const { key, label } = timeGroup(link.created_at)
    const last = groups[groups.length - 1]
    if (last && last.key === key) last.links.push(link)
    else groups.push({ key, label, links: [link] })
  }
  return groups
}

export function ListScreen() {
  const { tag, status, unopened, link } = route.useSearch()
  const navigate = useNavigate()

  const query = useLinks({ tag, status, unopened })
  const { data, isPending, isError, error, hasNextPage, isFetchingNextPage, fetchNextPage } = query
  const tagsQuery = useTags()
  const retry = useRetryLink()
  const del = useDeleteLink()
  const create = useCreateLink()
  const toast = useToast()
  const queryClient = useQueryClient()
  const resurfaced = useResurfaced()

  const facetOf = makeFacetResolver(tagsQuery.data)
  const links = data?.pages.flatMap((p) => p.links) ?? []
  // All three narrowing controls, not two. `unopened` was missing, and it matters more
  // than it looks now that the card is a move rather than a copy: filter to unopened and
  // one of those links silently leaves the time-ordered list for a section of its own, so
  // the list the user asked for is no longer complete. It also decides the empty state
  // below — with `?unopened=true` and no matches, "you have not saved anything yet" was
  // a lie about an archive that is full.
  const hasFilter = Boolean(tag || status || unopened)
  // Both rules — the card is a move, and a narrowed view gets no card — live in
  // `lib/board.ts` because iOS implements the same two. A shared fixture pins the
  // OUTPUT of both (testdata/resurface-board-cases.json); when this was written
  // inline here, the web checked `!tag` and iOS checked "no filter" and the two
  // screens looked fine apart. Call the function — a rule the app does not run is
  // a rule the fixture is not measuring.
  const { card: resurfacedCard, board: boardLinks } = boardView({
    links,
    resurfaced: resurfaced.data,
    filtered: hasFilter,
  })
  const groups = groupByTime(boardLinks)
  const showSkeleton = useDelayed(isPending)

  const openInspector = (id: number) =>
    void navigate({ to: '/', search: (prev) => ({ ...prev, link: id }) })
  const closeInspector = () =>
    void navigate({ to: '/', search: (prev) => ({ ...prev, link: undefined }) })
  const toggleTag = (name: string) =>
    void navigate({
      to: '/',
      search: (prev) => ({ ...prev, tag: prev.tag === name ? undefined : name, link: undefined }),
    })
  const setStatus = (s?: LinkStatus) =>
    void navigate({ to: '/', search: (prev) => ({ ...prev, status: s, link: undefined }) })
  const clearFilters = () =>
    void navigate({ to: '/', search: (prev) => ({ link: prev.link }) })

  const toastErr = (e: unknown) => toast.show({ variant: 'error', message: errorMessage(e) })

  // Card-cursor Backspace/Delete — soft delete + undo toast (§1.2 / §1.5). No
  // restore endpoint, so undo re-POSTs the same url (the card reopens as pending);
  // the label states that. Mirrors the inspector's onDelete.
  const undoDelete = (url: string, note: string) =>
    create.mutate(
      { url, note: note || undefined },
      {
        onError: toastErr,
        // Parity with the default save path (useSaveLink): a fresh 201 comes back
        // pending, so poll it to done to fill title/tags/thumb (§1.5) — the toast
        // promised "다시 수집됩니다". A 200 duplicate is already terminal.
        onSuccess: (res) => {
          if (!res.duplicate) startPolling(queryClient, res.id)
        },
      },
    )

  const deleteLink = (l: LinkItem) => {
    const { url, note } = l
    del.mutate(l.id, {
      onError: toastErr,
      onSuccess: () => {
        if (link === l.id) closeInspector()
        toast.show({
          variant: 'undo',
          message: t('common.deleted'),
          action: {
            label: t('common.undoRecollect'),
            onClick: () => undoDelete(url, note),
          },
        })
      },
    })
  }

  // Card-cursor keyboard (§1.2): J/K move the DOM focus between card buttons; the
  // rest act on the focused card. E/N park a focus intent then open the inspector.
  const boardRef = useRef<HTMLDivElement>(null)
  useRowKeyboard({
    containerRef: boardRef,
    // The forgotten-link card is a row on this board, so the keyboard has to know it. It is
    // not always inside `links`: it is picked from the whole archive and may sit past the
    // loaded pages, and then `links.find` misses. J/K would move focus onto it and every
    // action key — delete, retry, edit, note — would do nothing while the focus ring said
    // otherwise.
    links: resurfacedCard ? [resurfacedCard, ...links.filter((l) => l.id !== resurfacedCard.id)] : links,
    inspectorOpen: link != null,
    onOpen: openInspector,
    onOpenWithFocus: (id, focus) => {
      requestInspectorFocus(focus)
      openInspector(id)
    },
    onRetry: (l) => retry.mutate(l.id, { onError: toastErr }),
    onDelete: deleteLink,
  })

  // Auto-load the next page when the sentinel scrolls into view.
  const sentinel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinel.current
    if (!el) return
    const io = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) void fetchNextPage()
    })
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  return (
    <section className="flex flex-col gap-20">
      {/* Toolbar (sticky) — search launcher + status filter. */}
      <div className="sticky top-(--size-header) z-(--z-header) -mx-16 flex flex-col gap-12 bg-canvas px-16 pb-12 pt-8">
        <div className="flex items-center gap-12">
          <Link
            to="/search"
            className="flex h-40 min-w-0 flex-1 items-center gap-8 rounded-control border border-line-control bg-surface px-16 text-body text-fg-3 transition-colors duration-(--dur-out) ease-ui hover:bg-hover"
          >
            <Icon icon={Search} size={16} />
            <span className="truncate">{t('search.placeholder')}</span>
            <kbd className="ml-auto rounded-control bg-hover px-6 py-2 font-mono text-label text-fg-2">
              /
            </kbd>
          </Link>
          <StatusFilter value={status} onChange={setStatus} />
          {/* 안 연 것 — 컬럼 하나가 만드는 유일한 화면 변화다. 배지도 딤도 없다:
              이건 지표가 아니라 링크별 사실이고, 카드에 표시하면 "안 읽었다"는
              판정처럼 읽힌다(그 신호는 푸시포인트를 통과한 열람만 잡으므로
              구조적으로 과소집계다). */}
          <Button
            variant={unopened ? 'primary' : 'secondary'}
            onClick={() =>
              void navigate({
                to: '/',
                search: (prev) => ({ ...prev, unopened: unopened ? undefined : true, link: undefined }),
              })
            }
          >
            {t('list.unopened')}
          </Button>
        </div>

        {/* Tag filter chip bar (control chips; §11 3(4)). Horizontal scroll
            stays inside this container — the body never scrolls sideways. */}
        <TagFilterBar tags={tagsQuery.data} activeTag={tag} onToggle={toggleTag} />
      </div>

      {/* The element the board's column query measures. LinkCard declares no
          container of its own, so this is the only measuring element. */}
      <div ref={boardRef} className="@container">
        {/* Loading */}
        {isPending && showSkeleton ? (
          <ul className={BOARD_GRID}>
            {Array.from({ length: 6 }, (_, i) => (
              <LinkCardSkeleton key={i} />
            ))}
          </ul>
        ) : null}

        {/* Error — inline block, not a toast (§11 3(5)). Already-loaded pages stay. */}
        {isError ? (
          <div className="flex flex-col items-center gap-12 rounded-card bg-surface py-40 text-center shadow-ring">
            <p className="text-body text-fg-2">{errorMessage(error)}</p>
            <Button variant="secondary" onClick={() => void query.refetch()}>
              {t('common.tryAgain')}
            </Button>
          </div>
        ) : null}

        {/* 오늘의 한 건 — 잊고 있던 링크 하나. 후보가 없으면 서버가 204를 주고 아무것도
            그리지 않는다. **빈 자리를 만들지 않는 것이 규칙이다** — "오늘은 없습니다"는
            매일 보면 무시하게 되는 칸이고, 그러면 진짜 있는 날에도 안 보인다.

            필터나 검색이 걸린 화면에서는 숨긴다. 사용자가 좁혀 놓은 결과 맨 위에 그와
            무관한 카드를 끼우면 그건 되살리기가 아니라 방해다. **태그만이 아니라 상태
            필터도 마찬가지다** — 처음에는 `!tag`만 봤는데, 그러면 "실패"로 좁힌 목록
            맨 위에 멀쩡한 링크 한 장이 얹힌다. 좁혔다는 사실은 `hasFilter`가 안다. */}
        {!isPending && !isError && resurfacedCard ? (
          <section className="mb-32">
            <TimeSpine label={t('resurface.spine')} count={1} />
            <ul className={BOARD_GRID}>
              <LinkCard
                link={resurfacedCard}
                resolveFacet={facetOf}
                selected={link === resurfacedCard.id}
                activeTag={tag}
                onOpen={openInspector}
                onTagClick={toggleTag}
                onRetry={(x) => retry.mutate(x.id)}
              />
            </ul>
          </section>
        ) : null}

        {/* Board — one section per time group: spine header, then cards. */}
        {!isPending && !isError
          ? groups.map((group) => (
              <section key={group.key} className="mb-32 last:mb-0">
                <TimeSpine label={group.label} count={group.links.length} />
                <ul className={BOARD_GRID}>
                  {group.links.map((l) => (
                    <LinkCard
                      key={l.id}
                      link={l}
                      resolveFacet={facetOf}
                      selected={link === l.id}
                      activeTag={tag}
                      onOpen={openInspector}
                      onTagClick={toggleTag}
                      onRetry={(x) => retry.mutate(x.id)}
                    />
                  ))}
                </ul>
              </section>
            ))
          : null}

        {/* Next-page loading — appended under the last spine, not as a new group. */}
        {isFetchingNextPage ? (
          <ul className={`${BOARD_GRID} mt-16`}>
            <LinkCardSkeleton />
            <LinkCardSkeleton />
            <LinkCardSkeleton />
          </ul>
        ) : null}
      </div>

      {/* Empty states — never rendered while isPending (§4.8). */}
      {!isPending && !isError && links.length === 0 ? (
        hasFilter ? (
          <EmptyState
            title={
              status === 'failed' && !tag ? t('list.emptyFailedTitle') : t('list.emptyFilterTitle')
            }
            description={
              status === 'failed' && !tag
                ? t('list.emptyFailedDesc')
                : t('list.emptyFilterDesc')
            }
            action={
              <Button variant="secondary" onClick={clearFilters}>
                {t('common.clearFilters')}
              </Button>
            }
          />
        ) : (
          <EmptyState
            title={t('list.emptyTitle')}
            description={t('list.emptyDesc')}
            action={
              <Link
                to="/save"
                className="inline-flex h-32 items-center rounded-control bg-accent px-12 text-label text-on-accent hover:bg-accent-hover"
              >
                {t('list.saveCta')}
              </Link>
            }
          />
        )
      ) : null}

      {/* IntersectionObserver sentinel + fallback button when it fails. */}
      <div ref={sentinel} className="h-2" />
      {hasNextPage && !isFetchingNextPage ? (
        <Button variant="secondary" className="mx-auto" onClick={() => void fetchNextPage()}>
          {t('common.loadMore')}
        </Button>
      ) : null}
      {/* P1: switch to @tanstack/react-virtual when rendered cards exceed 200 (§10 4.4). */}
    </section>
  )
}

// Time spine header (§11 3(2)) — the one slot where serif is allowed (§10 2.2.5).
// The label is human ("오늘") and the count is machine, so R2 puts the two in
// different faces on the same line.
function TimeSpine({ label, count }: { label: string; count: number }) {
  return (
    <div className="mb-12 flex items-baseline gap-12">
      <h2 className="font-serif text-spine text-fg-1">{label}</h2>
      <span className="font-mono text-label text-fg-3">{t('list.groupCount', { count })}</span>
      <span className="h-px flex-1 bg-line-1" aria-hidden />
    </div>
  )
}

// Tag filter chip bar — dictionary tags with link_count > 0, most-used first.
// The active ?tag is always present even if its count dropped to 0.
function TagFilterBar({
  tags,
  activeTag,
  onToggle,
}: {
  tags: readonly Tag[] | undefined
  activeTag?: string
  onToggle: (name: string) => void
}) {
  if (!tags || tags.length === 0) return null
  const used = tags.filter((t) => t.link_count > 0).sort((a, b) => b.link_count - a.link_count)
  if (activeTag && !used.some((t) => t.name === activeTag)) {
    const active = tags.find((t) => t.name === activeTag)
    if (active) used.unshift(active)
  }
  if (used.length === 0) return null

  return (
    <div className="-mx-16 flex gap-6 overflow-x-auto px-16 pb-2">
      {used.map((t) => (
        <Chip
          key={t.id}
          facet={t.facet}
          role="control"
          source={null}
          selected={t.name === activeTag}
          count={t.link_count}
          onClick={() => onToggle(t.name)}
          className="shrink-0"
        >
          {t.name}
        </Chip>
      ))}
    </div>
  )
}

