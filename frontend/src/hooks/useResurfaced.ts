import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api/client'

/// 오늘의 한 건. 후보가 없으면 서버가 204를 주고, 그때는 `null`이다.
///
/// **staleTime이 긴 이유.** 서버가 하루 동안 같은 답을 주므로 자주 물어볼 이유가 없고,
/// 화면을 옮길 때마다 다시 부르면 같은 카드가 깜빡인다. 하루가 바뀌는 순간을 정확히
/// 잡으려 애쓰지 않는다 — 자정에 카드가 바뀌지 않아도 다음에 앱을 열 때 바뀐다.
export function useResurfaced() {
  return useQuery({
    queryKey: ['resurfaced'],
    staleTime: 60 * 60 * 1000,
    queryFn: async () => {
      const { data, response } = await api.GET('/api/v1/links/resurfaced', {})
      if (response.status === 204) return null
      return data ?? null
    },
  })
}
