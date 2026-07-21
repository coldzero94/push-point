import { useEffect, useRef } from 'react'
import { getRouteApi, Link } from '@tanstack/react-router'
import { useLinks } from '../hooks/useLinks'
import { LinkCard } from '../components/LinkCard'
import { errorMessage } from '../lib/api/client'

// Typed search params (?tag=&status=) live in the URL — read them from the route.
const route = getRouteApi('/')

export function ListScreen() {
  const { tag, status } = route.useSearch()
  const query = useLinks({ tag, status })
  const {
    data,
    isPending,
    isError,
    error,
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
  } = query

  // Auto-load the next page when the sentinel scrolls into view.
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

  const links = data?.pages.flatMap((p) => p.links) ?? []
  const hasFilter = Boolean(tag || status)

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">목록</h1>
        <Link
          to="/save"
          className="rounded-md bg-neutral-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-neutral-700 dark:bg-neutral-100 dark:text-neutral-900 dark:hover:bg-neutral-300"
        >
          저장
        </Link>
      </div>

      {hasFilter && (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          {tag && (
            <span className="rounded-full bg-neutral-100 px-2 py-0.5 dark:bg-neutral-800">
              태그: {tag}
            </span>
          )}
          {status && (
            <span className="rounded-full bg-neutral-100 px-2 py-0.5 dark:bg-neutral-800">
              상태: {status}
            </span>
          )}
          <Link to="/" search={{}} className="text-neutral-500 underline">
            필터 해제
          </Link>
        </div>
      )}

      {isPending && <p className="text-sm text-neutral-500">불러오는 중…</p>}
      {isError && <p className="text-sm text-red-600">{errorMessage(error)}</p>}
      {!isPending && !isError && links.length === 0 && (
        <p className="text-sm text-neutral-500">
          저장된 링크가 없습니다.{' '}
          <Link to="/save" className="underline">
            첫 링크 저장하기
          </Link>
          .
        </p>
      )}

      <div className="space-y-3">
        {links.map((link) => (
          <LinkCard key={link.id} link={link} />
        ))}
      </div>

      <div ref={sentinel} className="h-8" />
      {isFetchingNextPage && <p className="text-center text-sm text-neutral-500">더 불러오는 중…</p>}
      {hasNextPage && !isFetchingNextPage && (
        <button
          type="button"
          onClick={() => void fetchNextPage()}
          className="mx-auto block rounded-md border border-neutral-300 px-4 py-2 text-sm hover:bg-neutral-100 dark:border-neutral-700 dark:hover:bg-neutral-800"
        >
          더 보기
        </button>
      )}
    </section>
  )
}
