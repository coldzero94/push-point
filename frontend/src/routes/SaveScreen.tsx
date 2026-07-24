// Save route (§0 / §2) — the composer opens over the list so the S2 fill is
// visible where the save lands (a save-only page would hide its own result,
// killing the signature). The composer inserts a filling row into the shared
// ['links'] cache; the live list below renders it — the SAME LinkRow the home
// list uses (no separate card) — and polling fills it in place.

import { useEffect, useRef } from 'react'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { SaveComposer } from '../components/SaveComposer'
import { LinkRow } from '../components/LinkRow'
import { useLinks } from '../hooks/useLinks'
import { useTags } from '../hooks/useTags'
import { useRetryLink } from '../hooks/useLinkMutations'
import { Button, EmptyState, Skeleton } from '../components/ui'
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
    <section className="mx-auto flex max-w-(--w-content) flex-col gap-16 pt-16">
      <SaveComposer initialUrl={url} initialNote={note} />

      {/* Loading: skeleton rows at the real row height (CLS 0). isPending must
          never render an empty state (§1.7 / §4.9). */}
      {isPending ? (
        <ul className="@container flex flex-col" aria-hidden>
          {Array.from({ length: 4 }).map((_, i) => (
            <li key={i} className="flex h-(--size-row-sm) items-center gap-12 pl-12 pr-16 sm:h-(--size-row)">
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
          ))}
        </ul>
      ) : isError ? (
        <div className="rounded-panel bg-surface p-20 text-body text-fg-2 shadow-ring">
          <p>{errorMessage(error)}</p>
          <div className="mt-12">
            <Button onClick={() => void refetch()}>다시 시도</Button>
          </div>
        </div>
      ) : links.length === 0 ? (
        <EmptyState
          title="저장된 링크가 없습니다"
          description="URL을 붙여넣으면 제목과 태그가 자동으로 채워집니다."
        />
      ) : (
        <ul className="@container flex flex-col">
          {links.map((l) => (
            <LinkRow
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

      <div ref={sentinel} className="h-2" />
      {hasNextPage && !isFetchingNextPage ? (
        <Button variant="secondary" className="mx-auto" onClick={() => void fetchNextPage()}>
          더 보기
        </Button>
      ) : null}
    </section>
  )
}
