import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import type { Stats } from '../lib/api/types'

// GET /api/v1/stats → Stats. Used by the tags header for the total link count
// (11 §5(3)). The widget/chart use of by_tag/by_day is M6 — this only needs
// total_links today, but the whole payload is cached under one key so a later
// stats screen reuses it.
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
