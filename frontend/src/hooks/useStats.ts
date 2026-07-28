import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import type { Stats } from '../lib/api/types'

// GET /api/v1/stats → Stats. Two consumers, one cache key: the tags header reads
// `total_links` (11 §5(3)), and the settings rhythm section reads `by_day` and
// `by_tag` (11 §8 (3-1)).
//
// `by_day` carries three guarantees the rhythm section depends on — exactly 30
// entries, ascending, last one is today. They live in `api/openapi.yaml`
// (Stats.by_day), not here, because iOS and `scripts/streak.sh` depend on them too.
export function useStats() {
  return useQuery({
    queryKey: ['stats'] as const,
    queryFn: async ({ signal }): Promise<Stats> => {
      const { data, error } = await api.GET('/api/v1/stats', { signal })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
  })
}
