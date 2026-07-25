// Tag dictionary writes (11 §5) — the M3 tuning loop. create lives in
// useLinkDetail (the inspector's "add to dictionary" path); this file owns the
// tags-screen writes: PATCH (rename / aliases / facet) and DELETE.
//
// Both invalidate ['links'] and ['search'] as well as ['tags']: a facet change
// or a delete changes the chip color / chip set on every card that carries the
// tag, and those live in the link and search caches (11 §5(4)).

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import type { Tag, TagFacet } from '../lib/api/types'

export type CreateTagVars = {
  name: string
  aliases?: string[]
  /** omit → server creates the tag `neutral` (contract default, 11 §5(4)) */
  facet?: TagFacet
}

/**
 * POST /api/v1/tags — the tags-screen "+ 태그 추가" form, which unlike the
 * inspector's name-only create (useCreateTag in useLinkDetail) can set aliases
 * and facet at birth. A duplicate name is a 400 the caller surfaces inline.
 */
export function useCreateDictionaryTag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ name, aliases, facet }: CreateTagVars): Promise<Tag> => {
      const body: CreateTagVars = { name }
      if (aliases !== undefined) body.aliases = aliases
      if (facet !== undefined) body.facet = facet
      const { data, error } = await api.POST('/api/v1/tags', { body })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tags'] })
    },
  })
}

export type UpdateTagVars = {
  id: number
  /** only the passed keys are sent — the contract replaces exactly those (11 §5) */
  name?: string
  aliases?: string[]
  facet?: TagFacet
  /**
   * The dictionary Tag to write into the ['tags'] cache immediately, so a facet
   * change recolors this tag's chips before the server answers (11 §5(4)). The
   * caller builds it (it already holds the row's current Tag).
   */
  optimistic?: Tag
}

/**
 * PATCH /api/v1/tags/{id}. Optimistic on the ['tags'] cache and rolled back on
 * error (the caller shows the toast). Only the keys that were passed go in the
 * body — an aliases-only edit never sends `facet`, so it cannot silently reset
 * the tag's classification.
 */
export function useUpdateTag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, name, aliases, facet }: UpdateTagVars): Promise<Tag> => {
      const body: { name?: string; aliases?: string[]; facet?: TagFacet } = {}
      if (name !== undefined) body.name = name
      if (aliases !== undefined) body.aliases = aliases
      if (facet !== undefined) body.facet = facet
      const { data, error } = await api.PATCH('/api/v1/tags/{id}', {
        params: { path: { id } },
        body,
      })
      if (error || !data) throw error ?? new Error('empty response')
      return data
    },
    onMutate: async (vars) => {
      if (!vars.optimistic) return { prev: undefined }
      await qc.cancelQueries({ queryKey: ['tags'] })
      const prev = qc.getQueryData<Tag[]>(['tags'])
      if (prev) {
        qc.setQueryData<Tag[]>(
          ['tags'],
          prev.map((t) => (t.id === vars.id ? (vars.optimistic as Tag) : t)),
        )
      }
      return { prev }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(['tags'], ctx.prev)
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['tags'] })
      void qc.invalidateQueries({ queryKey: ['links'] })
      void qc.invalidateQueries({ queryKey: ['search'] })
    },
  })
}

/**
 * DELETE /api/v1/tags/{id}. No optimistic removal and no undo (11 §5(5)): the
 * contract CASCADE-deletes the link_tags rows and there is no restore endpoint,
 * so the row leaves only after the server confirms. Invalidates the link/search
 * caches because the tag vanishes from every card it was on.
 */
export function useDeleteTag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const { error } = await api.DELETE('/api/v1/tags/{id}', {
        params: { path: { id } },
      })
      if (error) throw error
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['tags'] })
      void qc.invalidateQueries({ queryKey: ['links'] })
      void qc.invalidateQueries({ queryKey: ['search'] })
    },
  })
}
