import { api } from './client'

/**
 * 이 링크를 열었다는 사실을 기록한다.
 *
 * **fire-and-forget이다.** 저장 경로 밖이라 p99 게이트와 무관하고, 한 번 놓친 열람
 * 기록이 아카이브를 손상시키지 않으므로 재시도 큐를 두지 않는다. 실패는 무시한다 —
 * 원문을 여는 흐름을 계측이 방해하면 본말이 전도된다.
 *
 * 호출 지점을 여기 하나로 모으는 이유: 링크를 여는 자리가 카드·키보드·인스펙터로
 * 여럿이라, 각자 부르면 한 곳을 빠뜨려도 아무도 모른다(컬럼이 조금 덜 찰 뿐이다).
 */
export function markOpened(id: number): void {
  void api.POST('/api/v1/links/{id}/open', { params: { path: { id } } }).catch(() => {})
}
