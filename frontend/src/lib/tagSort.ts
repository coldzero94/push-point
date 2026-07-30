import type { Tag } from './api/types'

export type TagSortKey = 'count' | 'name' | 'recent'

/**
 * 태그 사전의 정렬. **`TagsScreen`에서 꺼내 온 이유는 테스트다** — 화면 안에 있으면
 * jsdom 없이 검증할 수 없고, 이 프로젝트의 웹 테스트는 `src/lib/`의 순수 로직만 돈다.
 *
 * `recent`는 **한 번도 안 붙은 태그를 맨 뒤**에 둔다.
 *
 * `?? -1`은 의도를 적은 것이지 동작을 바꾸는 것이 아니다 — JS는 산술에서 `null`을 0으로
 * 강제하므로 없어도 같은 결과다(실제 epoch가 0일 수 없으니 0도 맨 뒤로 간다). 처음엔
 * "null을 0으로 접으면 정반대가 된다"고 적었는데, 변이 검증에서 그 변이가 빠져나가며
 * **주장이 코드보다 셌다는 것이 드러났다.** 실제로 고정된 것은 정렬 방향과 null의 위치이고,
 * 그 둘은 변이로 확인했다.
 */
export function sortTags(list: readonly Tag[], sort: TagSortKey): Tag[] {
  return [...list].sort((a, b) => {
    if (sort === 'name') return a.name.localeCompare(b.name)
    if (sort === 'count') return b.link_count - a.link_count || a.name.localeCompare(b.name)
    const av = a.last_saved_at ?? -1
    const bv = b.last_saved_at ?? -1
    return bv - av || a.name.localeCompare(b.name)
  })
}
