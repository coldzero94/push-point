// TODO(M-next): tag dictionary screen.
//  - GET /api/v1/tags → list with link_count; create/update/delete (POST/PATCH/DELETE)
//  - clicking a tag navigates to `/` with ?tag=<name> (drives the list filter)
export function TagsScreen() {
  return (
    <section className="space-y-3">
      <h1 className="text-lg font-semibold">태그</h1>
      <p className="text-sm text-neutral-500">
        스캐폴드 스텁입니다. 태그 사전 조회·생성·수정·삭제와 목록 태그 필터 연동이 후속 작업입니다.
      </p>
    </section>
  )
}
