import { useSyncExternalStore } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import { hasApiKey, subscribeApiKey } from '../lib/auth'
import type { LinkPage, LinkStatus } from '../lib/api/types'

export interface LinksFilter {
  tag?: string
  status?: LinkStatus
}

// Infinite list of links (GET /api/v1/links). Keyset cursor pagination: the next
// page param is the previous page's next_cursor (null → no more pages). Wraps
// TanStack useInfiniteQuery directly over openapi-fetch (no openapi-react-query).
export function useLinks(filter: LinksFilter = {}) {
  // Gate on the key so no-key state shows the banner + empty state, not a 401
  // error card. Subscribed (not a one-shot read) so saving the key flips the
  // query on without a reload.
  const keyed = useSyncExternalStore(subscribeApiKey, hasApiKey)
  return useInfiniteQuery({
    enabled: keyed,
    queryKey: ['links', filter] as const,
    queryFn: async ({ pageParam, signal }): Promise<LinkPage> => {
      const { data, error } = await api.GET('/api/v1/links', {
        params: {
          query: {
            cursor: pageParam,
            tag: filter.tag,
            status: filter.status,
          },
        },
        signal,
      })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  })
}
