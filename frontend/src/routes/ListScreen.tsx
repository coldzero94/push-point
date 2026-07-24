// List (screen 2) — the app home (§11 3). A single-column row list with a
// sticky toolbar: a search launcher + status dropdown, then a tag filter chip
// bar. `?tag` / `?status` are URL state; `?link` opens the inspector overlay
// (the contract with the inspector owner — §11 0). Rows render at a fixed height
// (CLS 0) via LinkRow, facet-resolved from the tags cache. The three states are
// Skeleton rows (loading) / EmptyState (empty) / an inline error block (§11 3(5)
// — list errors are an inline block, NOT a toast).

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
import { LinkRow } from '../components/LinkRow'
import { StatusFilter } from '../components/StatusFilter'
import { Button, Chip, EmptyState, Icon, Skeleton, useToast } from '../components/ui'
import { makeFacetResolver } from '../lib/tags/facet'
import { errorMessage } from '../lib/api/client'
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

  // Row-cursor Backspace/Delete — soft delete + undo toast (§1.2 / §1.5). No
  // restore endpoint, so undo re-POSTs the same url (row reopens as pending); the
  // label states that. Mirrors the inspector's onDelete.
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

  // Row-cursor keyboard (§1.2): J/K move the DOM focus between row buttons; the
  // rest act on the focused row. E/N park a focus intent then open the inspector.
  const listRef = useRef<HTMLUListElement>(null)
  useRowKeyboard({
    containerRef: listRef,
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
    <section className="mx-auto flex max-w-(--w-content) flex-col gap-16 pt-16">
      {/* Toolbar (sticky) — search launcher + status filter. */}
      <div className="sticky top-(--size-header) z-(--z-header) -mx-16 flex flex-col gap-12 bg-canvas px-16 py-8">
        <div className="flex items-center gap-12">
          <Link
            to="/search"
            className="flex h-32 min-w-0 flex-1 items-center gap-8 rounded-control border border-line-control bg-surface px-12 text-meta text-fg-3 hover:bg-hover"
          >
            <Icon icon={Search} size={16} />
            <span>검색하거나 이동합니다</span>
            <kbd className="ml-auto rounded-control bg-hover px-6 py-2 font-mono text-fg-2">/</kbd>
          </Link>
          <StatusFilter value={status} onChange={setStatus} />
        </div>

        {/* Tag filter chip bar (control chips; §11 3(4)). Horizontal scroll
            stays inside this container — the body never scrolls sideways. */}
        <TagFilterBar tags={tagsQuery.data} activeTag={tag} onToggle={toggleTag} />
      </div>

      <ul ref={listRef} className="@container flex flex-col">
        {/* Loading */}
        {isPending && showSkeleton
          ? Array.from({ length: 8 }, (_, i) => <SkeletonRow key={i} />)
          : null}

        {/* Error — inline block, not a toast (§11 3(5)). Already-loaded pages stay. */}
        {isError ? (
          <li className="flex flex-col items-center gap-12 rounded-panel bg-surface py-40 text-center shadow-ring">
            <p className="text-body text-fg-2">{errorMessage(error)}</p>
            <Button variant="secondary" onClick={() => void query.refetch()}>
              다시 시도
            </Button>
          </li>
        ) : null}

        {/* Rows */}
        {!isPending && !isError
          ? links.map((l: LinkItem) => (
              <LinkRow
                key={l.id}
                link={l}
                facetOf={facetOf}
                selected={link === l.id}
                activeTag={tag}
                onOpen={openInspector}
                onTagClick={toggleTag}
                onRetry={(x) => retry.mutate(x.id)}
              />
            ))
          : null}

        {/* Next-page loading */}
        {isFetchingNextPage ? (
          <>
            <SkeletonRow />
            <SkeletonRow />
          </>
        ) : null}
      </ul>

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
            title="저장된 링크가 없습니다"
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
      {/* P1: switch to @tanstack/react-virtual when rendered rows exceed 200 (§10 4.4). */}
    </section>
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

// A row-shaped skeleton at the EXACT final dimensions (CLS 0) — the rail slot is
// reserved but transparent; the rail's own pulse signals progress (§4.9).
function SkeletonRow() {
  return (
    <li className="flex h-(--size-row-sm) items-center gap-12 pl-12 pr-16 sm:h-(--size-row)">
      <span className="w-(--size-rail) shrink-0" aria-hidden />
      <Skeleton
        variant="thumb"
        className="h-(--size-thumb-sm) w-(--size-thumb-sm) shrink-0 sm:h-(--size-thumb) sm:w-(--size-thumb)"
      />
      <div className="flex min-w-0 flex-1 flex-col gap-6">
        <Skeleton variant="text" className="h-16 w-3/5" />
        <Skeleton variant="text" className="h-12 w-2/5" />
      </div>
    </li>
  )
}
