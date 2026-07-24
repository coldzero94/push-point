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
  const { tag, status, link } = route.useSearch()
  const navigate = useNavigate()

  const query = useLinks({ tag, status })
  const { data, isPending, isError, error, hasNextPage, isFetchingNextPage, fetchNextPage } = query
  const tagsQuery = useTags()
  const retry = useRetryLink()
  const del = useDeleteLink()
  const create = useCreateLink()
  const toast = useToast()
  const queryClient = useQueryClient()

  const facetOf = makeFacetResolver(tagsQuery.data)
  const links = data?.pages.flatMap((p) => p.links) ?? []
  const groups = groupByTime(links)
  const hasFilter = Boolean(tag || status)
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
          message: '삭제했습니다.',
          action: {
            label: '되돌리기 — 다시 수집됩니다',
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
    links,
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
            <span className="truncate">검색하거나 이동합니다</span>
            <kbd className="ml-auto rounded-control bg-hover px-6 py-2 font-mono text-label text-fg-2">
              /
            </kbd>
          </Link>
          <StatusFilter value={status} onChange={setStatus} />
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
              다시 시도
            </Button>
          </div>
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
                      facetOf={facetOf}
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
            title={status === 'failed' && !tag ? '실패한 링크가 없습니다' : '조건에 맞는 링크가 없습니다'}
            description={
              status === 'failed' && !tag
                ? '모든 링크가 정상 처리되었습니다.'
                : '선택한 태그·상태 조합에 해당하는 항목이 없습니다.'
            }
            action={
              <Button variant="secondary" onClick={clearFilters}>
                필터 해제
              </Button>
            }
          />
        ) : (
          <EmptyState
            title="아직 모아둔 것이 없습니다"
            description="URL을 붙여넣으면 제목과 태그가 자동으로 채워집니다."
            action={
              <Link
                to="/save"
                className="inline-flex h-32 items-center rounded-control bg-accent px-12 text-label text-on-accent hover:bg-accent-hover"
              >
                링크 저장하기
              </Link>
            }
          />
        )
      ) : null}

      {/* IntersectionObserver sentinel + fallback button when it fails. */}
      <div ref={sentinel} className="h-2" />
      {hasNextPage && !isFetchingNextPage ? (
        <Button variant="secondary" className="mx-auto" onClick={() => void fetchNextPage()}>
          더 보기
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
      <span className="font-mono text-label text-fg-3">{count}건</span>
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

