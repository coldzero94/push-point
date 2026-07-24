// Tags (screen 4) — P1 (§9). Dictionary CRUD + alias editing + the facet
// 4-choice select land with the M3 tuning loop (aliases are the cheapest lever
// on tagging accuracy). Coming-soon inside the new shell until then.
import { EmptyState } from '../components/ui'

export function TagsScreen() {
  return (
    <section className="mx-auto max-w-(--w-content) pt-16">
      <EmptyState
        title="태그 사전은 준비 중입니다"
        description="사전 편집 · 별칭 · 분류(facet) 관리가 M3 튜닝 루프와 함께 열립니다. (P1)"
      />
    </section>
  )
}
