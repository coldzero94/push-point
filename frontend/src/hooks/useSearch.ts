import { useInfiniteQuery } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import type { SearchPage } from '../lib/api/types'

export interface SearchParams {
  q: string
  tag?: string
}

// Infinite search (GET /api/v1/search). Same cursor shape as the list; server
// picks FTS5 (q >= 3 chars) or LIKE fallback and reports which via `mode`.
// Disabled until q is non-empty. Second of the two infinite-scroll hooks.
export function useSearch({ q, tag }: SearchParams) {
  return useInfiniteQuery({
    queryKey: ['search', { q, tag }] as const,
    enabled: q.length > 0,
    queryFn: async ({ pageParam, signal }): Promise<SearchPage> => {
      const { data, error } = await api.GET('/api/v1/search', {
        params: { query: { q, tag, cursor: pageParam } },
        signal,
      })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  })
}
