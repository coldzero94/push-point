import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import type { LinkInput } from '../lib/api/types'

export interface CreateLinkResult {
  id: number
  // 201 = newly saved, 200 = duplicate (idempotent by url_hash).
  duplicate: boolean
}

// POST /api/v1/links. The contract returns 201 for a new save and 200 with
// duplicate:true for an existing url_hash — we distinguish by HTTP status.
export function useCreateLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: LinkInput): Promise<CreateLinkResult> => {
      const { data, error, response } = await api.POST('/api/v1/links', {
        body: input,
      })
      if (error || !data) throw error ?? new Error('empty response')
      return { id: data.id, duplicate: response.status === 200 }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['links'] })
    },
  })
}

// POST /api/v1/links/{id}/retry — re-enqueue a failed link's jobs. Used by the
// detail screen (M-next); kept here with createLink for the mutation surface.
export function useRetryLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { data, error } = await api.POST('/api/v1/links/{id}/retry', {
        params: { path: { id } },
      })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
    onSuccess: (_d, id) => {
      qc.invalidateQueries({ queryKey: ['links'] })
      qc.invalidateQueries({ queryKey: ['link', id] })
      qc.invalidateQueries({ queryKey: ['resurfaced'] })
    },
  })
}

// DELETE /api/v1/links/{id} — soft delete. Used by the detail screen (M-next).
export function useDeleteLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { error } = await api.DELETE('/api/v1/links/{id}', {
        params: { path: { id } },
      })
      if (error) throw error
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['links'] })
      // The forgotten-link card is a third list on the same screen and it caches for an
      // hour (useResurfaced), so it does not follow ['links']. Without this the card
      // survives its own link: delete it and the toast appears while the card stays,
      // and clicking it 404s. iOS blocks this in FeedModel.apply — the same rule has to
      // be written twice because the two clients hold the card in different places.
      qc.invalidateQueries({ queryKey: ['resurfaced'] })
    },
  })
}
