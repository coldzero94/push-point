import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import type { Tag } from '../lib/api/types'

// GET /api/v1/tags → the whole controlled dictionary (Tag[] with link_count +
// facet). This is the cache the list/search rows read to resolve a LinkTag's
// facet by id — `facet` ships ONLY here, not on `LinkTag` (a contract decision
// to avoid a facet string per link×tag, §10). A cache miss is never guessed;
// `makeFacetResolver` falls back to `neutral` (lib/tags/facet.ts).
export function useTags() {
  return useQuery({
    queryKey: ['tags'] as const,
    queryFn: async ({ signal }): Promise<Tag[]> => {
      const { data, error } = await api.GET('/api/v1/tags', { signal })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
  })
}
