import { getRouteApi, Link } from '@tanstack/react-router'

const route = getRouteApi('/links/$id')

// TODO(M-next): full detail screen.
//  - GET /api/v1/links/{id} → all fields + jobs summary + error
//  - retry (failed only) and delete via hooks/useLinkMutations (useRetryLink/useDeleteLink)
//  - link to edit (note/tags)
export function LinkDetailScreen() {
  const { id } = route.useParams()
  return (
    <section className="space-y-3">
      <h1 className="text-lg font-semibold">링크 상세</h1>
      <p className="text-sm text-neutral-500">
        스캐폴드 스텁입니다 (id {id}). 상세 필드·잡 요약·재시도/삭제·편집 연동이 후속 작업입니다.
      </p>
      <Link
        to="/links/$id/edit"
        params={{ id }}
        className="inline-block text-sm text-neutral-600 underline dark:text-neutral-300"
      >
        편집
      </Link>
    </section>
  )
}
