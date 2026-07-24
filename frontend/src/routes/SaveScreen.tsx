// Save route (§0 / §2) — the composer opens over the board so the S2 fill is
// visible where the save lands (a save-only page would hide its own result,
// killing the signature). The composer inserts a filling link into the shared
// ['links'] cache; the live board below renders it — the SAME LinkCard the home
// list uses — and polling fills it in place, slot by slot.

import { useEffect, useRef } from 'react'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { SaveComposer } from '../components/SaveComposer'
import { BOARD_GRID, LinkCard, LinkCardSkeleton } from '../components/LinkCard'
import { useLinks } from '../hooks/useLinks'
import { useTags } from '../hooks/useTags'
import { useRetryLink } from '../hooks/useLinkMutations'
import { Button, EmptyState } from '../components/ui'
import { makeFacetResolver } from '../lib/tags/facet'
import { errorMessage } from '../lib/api/client'

const route = getRouteApi('/save')

export function SaveScreen() {
  const { url, note, link } = route.useSearch()
  const navigate = useNavigate()
  const query = useLinks({})
  const {
    data,
    isPending,
    isError,
    error,
    refetch,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = query
  const tagsQuery = useTags()
  const retry = useRetryLink()

  const facetOf = makeFacetResolver(tagsQuery.data)
  const links = data?.pages.flatMap((p) => p.links) ?? []

  const openInspector = (id: number) =>
    void navigate({ to: '/save', search: (prev) => ({ ...prev, link: id }) })
  // A tag chip filters the archive — that lives on the home list, so jump there.
  const toggleTag = (name: string) => void navigate({ to: '/', search: { tag: name } })

  // Auto-load the next page when the sentinel scrolls into view (§3(3)).
  const sentinel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinel.current
    if (!el) return
    const io = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) {
        void fetchNextPage()
      }
    })
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  return (
    <section className="mx-auto flex max-w-(--w-content) flex-col gap-20 pt-16">
      <SaveComposer initialUrl={url} initialNote={note} />

      {/* No time spine here: the point of this screen is "what I just saved",
          and the composer already marks now. The board is flat and the newest
          card sits directly under the form. */}
      <div className="@container">
        {isPending ? (
          // Loading: skeleton cards at the real card dimensions (CLS 0).
          // isPending must never render an empty state (§1.7 / §4.9).
          <ul className={BOARD_GRID} aria-hidden>
            {Array.from({ length: 4 }).map((_, i) => (
              <LinkCardSkeleton key={i} />
            ))}
          </ul>
        ) : isError ? (
          <div className="rounded-card bg-surface p-20 text-body text-fg-2 shadow-ring">
            <p>{errorMessage(error)}</p>
            <div className="mt-12">
              <Button onClick={() => void refetch()}>다시 시도</Button>
            </div>
          </div>
        ) : links.length === 0 ? (
          <EmptyState
            title="아직 모아둔 것이 없습니다"
            description="URL을 붙여넣으면 제목과 태그가 자동으로 채워집니다."
          />
        ) : (
          <ul className={BOARD_GRID}>
            {links.map((l) => (
              <LinkCard
                key={l.id}
                link={l}
                facetOf={facetOf}
                selected={link === l.id}
                onOpen={openInspector}
                onTagClick={toggleTag}
                onRetry={(x) => retry.mutate(x.id)}
              />
            ))}
          </ul>
        )}
      </div>

      <div ref={sentinel} className="h-2" />
      {hasNextPage && !isFetchingNextPage ? (
        <Button variant="secondary" className="mx-auto" onClick={() => void fetchNextPage()}>
          더 보기
        </Button>
      ) : null}
    </section>
  )
}
