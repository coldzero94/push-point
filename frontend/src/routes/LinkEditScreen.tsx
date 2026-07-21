import { getRouteApi } from '@tanstack/react-router'

const route = getRouteApi('/links/$id/edit')

// TODO(M-next): edit screen.
//  - PATCH /api/v1/links/{id} with { note?, tags? } (tags = full replace)
//  - note field (useState) + tag multi-select from GET /api/v1/tags
export function LinkEditScreen() {
  const { id } = route.useParams()
  return (
    <section className="space-y-3">
      <h1 className="text-lg font-semibold">링크 편집</h1>
      <p className="text-sm text-neutral-500">
        스캐폴드 스텁입니다 (id {id}). 메모·태그(전체 교체) 편집이 후속 작업입니다.
      </p>
    </section>
  )
}
