// Search (screen 3) — P1 (§9). The infinite hook (hooks/useSearch.ts) and the
// typed ?q/?tag params are wired at the contract level, but the full toolbar +
// results renderer land with M3, when /api/v1/search has depth (§9 P1). Until
// then the route stays reachable inside the shell as a coming-soon state so
// navigation and the `/` shortcut resolve somewhere consistent.
import { EmptyState } from '../components/ui'

export function SearchScreen() {
  return (
    <section className="mx-auto max-w-(--w-content) pt-16">
      <EmptyState
        title="검색은 준비 중입니다"
        description="M3에서 /api/v1/search가 켜지면 검색어 · 기간 · 태그 필터가 여기에서 열립니다. (P1)"
      />
    </section>
  )
}
