// Save flow engine — the optimistic row (S2) + progress polling behind the
// composer. §1.5 (optimistic update / polling) + §2 (save screen).
//
// The signature is behavioural, realised through the SHARED `['links']` query
// cache: submit inserts a filling row at the top instantly, the 201 response
// swaps the temp id for the real one, and polling fills title/tags/thumbnail in
// place. We NEVER invalidate the whole list (that shakes scroll position and
// every other row, §1.5) — every write targets one row via setQueriesData.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { InfiniteData, QueryClient } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import type { Link, LinkDetail, LinkInput, LinkPage } from '../lib/api/types'

type LinksData = InfiniteData<LinkPage, string | undefined>

export type SaveOutcome =
  | { kind: 'created'; id: number }
  | { kind: 'duplicate'; id: number }

// Temp ids are negative and monotonically decreasing so they never collide with
// server ids (positive) or with each other within a session.
let tempSeq = 0
const nextTempId = () => -(1_000_000_000_000 + ++tempSeq)

/**
 * The optimistic Link inserted at t=0 (§1.5). Only `domain` is real (parsed
 * client-side); title/description/tags/thumb are empty and get filled by the
 * server response + polling. `domain` loses to the server value once it arrives.
 */
function optimisticLink(id: number, url: string, note: string): Link {
  let domain = ''
  try {
    domain = new URL(url).hostname
  } catch {
    // client validation already required an http(s):// prefix; if URL() still
    // throws, leave domain empty and let the title fall back to the raw url.
  }
  return {
    id,
    url,
    domain,
    title: '',
    description: '',
    content_type: 'other',
    thumb_url: null,
    status: 'pending',
    tags: [],
    note,
    created_at: Math.floor(Date.now() / 1000),
  }
}

/** LinkDetail (poll response) narrowed to the list-row Link subset. */
function toLink(d: LinkDetail): Link {
  return {
    id: d.id,
    url: d.url,
    domain: d.domain,
    title: d.title,
    description: d.description,
    content_type: d.content_type,
    thumb_url: d.thumb_url,
    status: d.status,
    tags: d.tags,
    note: d.note,
    created_at: d.created_at,
  }
}

// ── one-row cache edits (prefix-match every ['links', filter] query) ──────────

function prependRow(qc: QueryClient, link: Link) {
  qc.setQueriesData<LinksData>({ queryKey: ['links'] }, (old) => {
    if (!old || old.pages.length === 0) return old
    const [first, ...rest] = old.pages
    return { ...old, pages: [{ ...first, links: [link, ...first.links] }, ...rest] }
  })
}

function patchRow(qc: QueryClient, id: number, next: (l: Link) => Link) {
  qc.setQueriesData<LinksData>({ queryKey: ['links'] }, (old) => {
    if (!old) return old
    return {
      ...old,
      pages: old.pages.map((p) => ({
        ...p,
        links: p.links.map((l) => (l.id === id ? next(l) : l)),
      })),
    }
  })
}

function removeRow(qc: QueryClient, id: number) {
  qc.setQueriesData<LinksData>({ queryKey: ['links'] }, (old) => {
    if (!old) return old
    return {
      ...old,
      pages: old.pages.map((p) => ({ ...p, links: p.links.filter((l) => l.id !== id) })),
    }
  })
}

// ── progress polling (§1.5) ───────────────────────────────────────────────────
//
// Module-scoped so it survives the composer unmounting (the user may navigate
// to the list while the row is still scraping). Schedule: GET /links/{id} at 1s
// ×10 then 3s ×40 (~2min total), stop immediately on done/failed, pause while
// the tab is hidden and fire once on return. Each tick replaces ONLY that row.

const MAX_FAST = 10 // 1s ticks
const MAX_SLOW = 40 // 3s ticks
const active = new Map<number, () => void>()

export function startPolling(qc: QueryClient, id: number) {
  active.get(id)?.() // cancel any prior poll for this id

  let attempt = 0
  let stopped = false
  let timer: number | undefined

  const cancel = () => {
    if (stopped) return
    stopped = true
    if (timer !== undefined) clearTimeout(timer)
    document.removeEventListener('visibilitychange', onVisibility)
    active.delete(id)
  }

  const schedule = () => {
    if (stopped) return
    if (attempt >= MAX_FAST + MAX_SLOW) {
      cancel()
      return
    }
    const delay = attempt < MAX_FAST ? 1000 : 3000
    timer = window.setTimeout(tick, delay)
  }

  const tick = async () => {
    if (stopped) return
    // Paused while hidden — keep a light heartbeat but don't spend a request or
    // count an attempt; visibilitychange fires an immediate tick on return.
    if (document.visibilityState === 'hidden') {
      schedule()
      return
    }
    attempt += 1
    try {
      const { data, error } = await api.GET('/api/v1/links/{id}', {
        params: { path: { id } },
      })
      if (!error && data) {
        patchRow(qc, id, () => toLink(data))
        qc.setQueryData(['link', id], data)
        if (data.status === 'done' || data.status === 'failed') {
          cancel()
          return
        }
      }
    } catch {
      // transient (offline/500): keep polling on the schedule below
    }
    schedule()
  }

  const onVisibility = () => {
    if (document.visibilityState === 'visible' && !stopped) {
      if (timer !== undefined) clearTimeout(timer)
      void tick()
    }
  }

  document.addEventListener('visibilitychange', onVisibility)
  schedule()
  active.set(id, cancel)
}

/**
 * Optimistic save. `mutateAsync` resolves with the outcome so the composer can
 * branch (201 created vs 200 duplicate) for its toasts; 4xx/5xx reject after the
 * optimistic row is rolled back.
 */
export function useSaveLink() {
  const qc = useQueryClient()
  return useMutation<SaveOutcome, unknown, LinkInput, { tempId: number }>({
    onMutate: (input) => {
      const tempId = nextTempId()
      prependRow(qc, optimisticLink(tempId, input.url, input.note ?? ''))
      return { tempId }
    },
    mutationFn: async (input) => {
      const { data, error, response } = await api.POST('/api/v1/links', { body: input })
      if (error || !data) throw error ?? new Error('empty response')
      // 200 = duplicate url_hash, 201 = newly saved (distinguished by status).
      return response.status === 200
        ? { kind: 'duplicate', id: data.id }
        : { kind: 'created', id: data.id }
    },
    onSuccess: (out, _input, ctx) => {
      if (out.kind === 'duplicate') {
        // not a failure — drop the optimistic row, the composer scrolls the user
        // to the existing one instead (§1.5 / §2(5)).
        removeRow(qc, ctx.tempId)
        return
      }
      // created: swap temp id -> real id in place, then let polling fill it.
      patchRow(qc, ctx.tempId, (l) => ({ ...l, id: out.id }))
      startPolling(qc, out.id)
    },
    onError: (_err, _input, ctx) => {
      if (ctx) removeRow(qc, ctx.tempId)
    },
  })
}
