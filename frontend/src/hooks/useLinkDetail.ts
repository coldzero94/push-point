// Inspector data layer (11 §6). Detail query + tag dictionary + the two writes
// the inspector owns (note/tags full-replace PATCH, and create-tag for the
// "not in the dictionary" flow). Retry/delete/undo reuse the existing mutation
// surface in useLinkMutations — this file does not duplicate them.
//
// Wraps TanStack Query directly over openapi-fetch (no openapi-react-query —
// .claude/rules/frontend.md). All types come from the generated contract.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import type { LinkDetail, LinkTag, LinkStatus, Tag } from '../lib/api/types'

const TERMINAL: ReadonlySet<LinkStatus> = new Set<LinkStatus>(['done', 'failed'])

/**
 * GET /api/v1/links/{id} → LinkDetail. Disabled when `id` is null (closed
 * inspector). While the link is not in a terminal state the query polls so the
 * meta/jobs sections fill in as the worker progresses (11 §6(5)); polling is the
 * only progress channel — the contract has no stream (§10-1). Background tabs do
 * not poll (React Query default: refetchIntervalInBackground false).
 */
export function useLinkDetail(id: number | null) {
  return useQuery({
    queryKey: ['link', id] as const,
    enabled: id != null,
    queryFn: async ({ signal }): Promise<LinkDetail> => {
      const { data, error } = await api.GET('/api/v1/links/{id}', {
        params: { path: { id: id as number } },
        signal,
      })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
    refetchInterval: (query) => (TERMINAL.has(query.state.data?.status ?? 'done') ? false : 1500),
  })
}

/**
 * GET /api/v1/tags → Tag[]. The dictionary drives the combobox options and the
 * facet resolver (a LinkTag carries no facet — §10). Shared key with the filter
 * bar so both read one cache entry.
 */
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

export type UpdateLinkVars = {
  id: number
  /** replaces note. Omit the key entirely for a tag-only edit. */
  note?: string
  /** FULL tag-name replacement (11 §7). Omit the key for a note-only edit — an
   *  empty array means "remove all tags", which is different from omitting. */
  tags?: string[]
  /** component-computed LinkTag[] (facet/source aware) applied to the cache
   *  optimistically, so the chips reflect the edit before the server answers. */
  optimisticTags?: LinkTag[]
}

/**
 * PATCH /api/v1/links/{id}. Optimistic: the detail cache updates immediately and
 * rolls back on error (the caller shows the error toast). The request body only
 * carries the keys that were passed — a note-only save never sends `tags`, so it
 * cannot wipe the link's tags (the 3-value trap, 11 §7).
 */
export function useUpdateLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, note, tags }: UpdateLinkVars): Promise<LinkDetail> => {
      const body: { note?: string; tags?: string[] } = {}
      if (note !== undefined) body.note = note
      if (tags !== undefined) body.tags = tags
      const { data, error } = await api.PATCH('/api/v1/links/{id}', {
        params: { path: { id } },
        body,
      })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
    onMutate: async (vars) => {
      await qc.cancelQueries({ queryKey: ['link', vars.id] })
      const prev = qc.getQueryData<LinkDetail>(['link', vars.id])
      if (prev) {
        qc.setQueryData<LinkDetail>(['link', vars.id], {
          ...prev,
          ...(vars.note !== undefined ? { note: vars.note } : {}),
          ...(vars.optimisticTags ? { tags: vars.optimisticTags } : {}),
        })
      }
      return { prev }
    },
    onError: (_err, vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(['link', vars.id], ctx.prev)
    },
    onSettled: (_data, _err, vars) => {
      void qc.invalidateQueries({ queryKey: ['link', vars.id] })
      void qc.invalidateQueries({ queryKey: ['links'] })
      void qc.invalidateQueries({ queryKey: ['search'] })
    },
  })
}

/**
 * POST /api/v1/tags — used only by the inspector's "add to the dictionary and
 * attach" path when a typed name is not in the dictionary (11 §6(4)). Created
 * tags are born `neutral` (contract default). Invalidates the dictionary so the
 * new tag's facet resolves on the next render.
 */
export function useCreateTag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (name: string): Promise<Tag> => {
      const { data, error } = await api.POST('/api/v1/tags', { body: { name } })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tags'] })
    },
  })
}
