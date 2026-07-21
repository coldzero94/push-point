import { getRouteApi } from '@tanstack/react-router'

// Typed search params (?q=&tag=) are wired; results rendering is the follow-up.
const route = getRouteApi('/search')

// TODO(M-next): full search screen.
//  - input bound to ?q (typed search param), debounce → navigate({ search })
//  - render results with the existing `useSearch` infinite hook (hooks/useSearch.ts)
//  - show `mode` badge (fts vs like) and rank; reuse <LinkCard>
export function SearchScreen() {
  const { q } = route.useSearch()
  return (
    <section className="space-y-3">
      <h1 className="text-lg font-semibold">검색</h1>
      <p className="text-sm text-neutral-500">
        스캐폴드 스텁입니다. 무한스크롤 훅(<code>hooks/useSearch.ts</code>)은 준비돼 있고,
        입력 UI와 결과 렌더링(<code>mode</code>·rank 표시)이 후속 작업입니다.
      </p>
      {q && <p className="text-sm text-neutral-400">현재 q: {q}</p>}
    </section>
  )
}
