// Search (screen 3) — §11 4. Same LinkCard board as the list (search results are
// not a different information hierarchy), plus a toolbar (query + period + tag)
// and a result-meta line that names the search path (`mode`: fts / like) and the
// running loaded-count — never a total, because the contract has none (§11 4(3)).
//
// The query text is a LOCAL state debounced 120ms into `?q` (navigate replace, so
// typing doesn't flood history); the query itself reads `?q`, so back/forward and
// sharing work. Period is a preset key in the URL, expanded to `from` per request
// (lib/period.ts). Changing q/tag/period restarts the query with a fresh cursor
// (useSearch keys on all of them) — the list and search cursors are incompatible.

import { useEffect, useRef, useState } from 'react'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { Check, ChevronDown } from 'lucide-react'
import * as Select from '@radix-ui/react-select'
import { useSearch } from '../hooks/useSearch'
import { useTags } from '../hooks/useTags'
import { useRetryLink } from '../hooks/useLinkMutations'
import { BOARD_GRID, LinkCard, LinkCardSkeleton } from '../components/LinkCard'
import { Button, EmptyState, Icon, Input } from '../components/ui'
import { makeFacetResolver } from '../lib/tags/facet'
import { errorMessage } from '../lib/api/client'
import { t } from '../lib/i18n'
import { PERIOD_LABEL, periodFrom } from '../lib/period'
import type { PeriodKey } from '../lib/period'
import type { SearchResult, Tag } from '../lib/api/types'

const route = getRouteApi('/search')

// Sentinel for "전체" — radix Select reserves "" (StatusFilter uses the same trick).
const ALL = 'all'

export function SearchScreen() {
  const { q, tag, period, link } = route.useSearch()
  const navigate = useNavigate()

  // Local input state for a responsive field; `?q` is synced on a 120ms debounce.
  const [text, setText] = useState(q)
  const inputRef = useRef<HTMLInputElement>(null)

  // Keep the field in sync if `?q` changes from elsewhere (back/forward, a shared
  // link) without clobbering what the user is mid-typing.
  useEffect(() => {
    setText((prev) => (prev === q ? prev : q))
  }, [q])

  // Debounce text → ?q (replace: no history entry per keystroke, §11 4(4)).
  useEffect(() => {
    if (text === q) return
    const t = window.setTimeout(() => {
      void navigate({ to: '/search', replace: true, search: (prev) => ({ ...prev, q: text }) })
    }, 120)
    return () => window.clearTimeout(t)
  }, [text, q, navigate])

  const from = periodFrom(period)
  const query = useSearch({ q, tag, from })
  const { data, isPending, isFetching, isError, error, hasNextPage, isFetchingNextPage, fetchNextPage } = query
  const tagsQuery = useTags()
  const retry = useRetryLink()

  const facetOf = makeFacetResolver(tagsQuery.data)
  const results = data?.pages.flatMap((p) => p.links) ?? []
  const mode = data?.pages[0]?.mode
  const hasFilter = Boolean(tag || period)

  const setTag = (t?: string) =>
    void navigate({ to: '/search', search: (prev) => ({ ...prev, tag: t, link: undefined }) })
  const setPeriod = (p?: PeriodKey) =>
    void navigate({ to: '/search', search: (prev) => ({ ...prev, period: p, link: undefined }) })
  const clearFilters = () =>
    void navigate({ to: '/search', search: (prev) => ({ q: prev.q, link: prev.link }) })
  const openInspector = (id: number) =>
    void navigate({ to: '/search', search: (prev) => ({ ...prev, link: id }) })

  // Esc: clear the query first, then leave to the list (§11 4(4)).
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      if (text.length > 0) setText('')
      else void navigate({ to: '/' })
    }
  }

  // Auto-load the next page.
  const sentinel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinel.current
    if (!el) return
    const io = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) void fetchNextPage()
    })
    io.observe(el)
    return () => io.disconnect()
  }, [hasNextPage, isFetchingNextPage, fetchNextPage])

  const active = q.length > 0
  // Loading dims the meta line only, never blanks the results (§11 4(7)): the old
  // results stay put and are replaced in place so typing never flickers.
  const showResultSkeleton = active && isPending

  return (
    <section className="mx-auto flex max-w-(--w-content) flex-col gap-16 pt-16">
      {/* Toolbar — input, then period + tag. Left-aligned, shares the list's
          baseline (§11 4(6): never centered). */}
      <div className="flex flex-col gap-12">
        <div className="max-w-(--w-search-input)">
          <Input
            ref={inputRef}
            variant="search"
            autoFocus
            value={text}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKeyDown}
            onClear={() => setText('')}
            placeholder={t('search.placeholder')}
            aria-label={t('search.queryLabel')}
          />
        </div>

        <div className="flex flex-wrap items-center gap-8">
          <PeriodFilter value={period} onChange={setPeriod} />
          <TagFilter tags={tagsQuery.data} value={tag} onChange={setTag} />
          {hasFilter ? (
            <Button size="sm" variant="ghost" onClick={clearFilters}>
              {t('common.clearFilters')}
            </Button>
          ) : null}
        </div>

        {/* Result meta — mode + running count (§11 4(3)). Dims while fetching. */}
        {active ? (
          <p
            className="font-mono text-meta text-fg-3 transition-opacity duration-(--dur-out)"
            style={{ opacity: isFetching ? 0.55 : 1 }}
            aria-live="polite"
          >
            {mode
              ? mode === 'fts'
                ? t('search.metaFts', { count: results.length })
                : t('search.metaLike', { count: results.length })
              : t('search.searching')}
          </p>
        ) : null}
      </div>

      {/* Results / states. */}
      {!active ? (
        <EmptyState title={t('search.promptTitle')} description={t('search.promptDesc')} />
      ) : showResultSkeleton ? (
        <div className="@container">
          <ul className={BOARD_GRID} aria-hidden>
            {Array.from({ length: 6 }, (_, i) => (
              <LinkCardSkeleton key={i} />
            ))}
          </ul>
        </div>
      ) : isError ? (
        <div className="flex flex-col items-center gap-12 rounded-card bg-surface py-40 text-center shadow-ring">
          <p className="text-body text-fg-2">{errorMessage(error)}</p>
          <Button variant="secondary" onClick={() => void query.refetch()}>
            {t('common.tryAgain')}
          </Button>
        </div>
      ) : results.length === 0 ? (
        <EmptyState
          title={t('search.noResultsTitle', { q })}
          description={t('search.noResultsDesc')}
          action={
            hasFilter ? (
              <Button variant="secondary" onClick={clearFilters}>
                {t('common.clearFilters')}
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className="@container">
          <ul className={BOARD_GRID}>
            {results.map((r: SearchResult) => (
              <LinkCard
                key={r.id}
                link={r}
                resolveFacet={facetOf}
                selected={link === r.id}
                activeTag={tag}
                onOpen={openInspector}
                onTagClick={setTag}
                onRetry={(x) => retry.mutate(x.id)}
              />
            ))}
          </ul>
        </div>
      )}

      <div ref={sentinel} className="h-2" />
      {hasNextPage && !isFetchingNextPage ? (
        <Button variant="secondary" className="mx-auto" onClick={() => void fetchNextPage()}>
          {t('common.loadMore')}
        </Button>
      ) : null}
    </section>
  )
}

// Period preset dropdown — 4 fixed options (§11 4(3)). Radix Select, token-styled.
function PeriodFilter({
  value,
  onChange,
}: {
  value?: PeriodKey
  onChange: (p?: PeriodKey) => void
}) {
  return (
    <Select.Root
      value={value ?? ALL}
      onValueChange={(v) => onChange(v === ALL ? undefined : (v as PeriodKey))}
    >
      <Select.Trigger
        aria-label={t('search.periodFilter')}
        className="inline-flex h-32 items-center gap-6 rounded-control border border-line-control bg-surface px-12 text-label text-fg-1 hover:bg-hover data-[state=open]:bg-hover"
      >
        <span className="text-fg-3">{t('search.period')}</span>
        <Select.Value />
        <Select.Icon>
          <Icon icon={ChevronDown} size={16} className="text-fg-2" />
        </Select.Icon>
      </Select.Trigger>
      <Select.Portal>
        <Select.Content
          position="popper"
          sideOffset={4}
          className="z-(--z-popover) min-w-(--radix-select-trigger-width) overflow-hidden rounded-panel bg-elevated p-4 shadow-panel"
        >
          <Select.Viewport>
            <SelectItem value={ALL} label={PERIOD_LABEL.all} />
            <SelectItem value="7d" label={PERIOD_LABEL['7d']} />
            <SelectItem value="30d" label={PERIOD_LABEL['30d']} />
            <SelectItem value="year" label={PERIOD_LABEL.year} />
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}

// Tag dropdown — single value (the contract's `tag` is one value). Used tags
// first, then the rest; 전체 clears it.
function TagFilter({
  tags,
  value,
  onChange,
}: {
  tags: readonly Tag[] | undefined
  value?: string
  onChange: (t?: string) => void
}) {
  const sorted = tags ? [...tags].sort((a, b) => b.link_count - a.link_count) : []
  return (
    <Select.Root
      value={value ?? ALL}
      onValueChange={(v) => onChange(v === ALL ? undefined : v)}
    >
      <Select.Trigger
        aria-label={t('search.tagFilter')}
        className="inline-flex h-32 items-center gap-6 rounded-control border border-line-control bg-surface px-12 text-label text-fg-1 hover:bg-hover data-[state=open]:bg-hover"
      >
        <span className="text-fg-3">{t('search.tag')}</span>
        <Select.Value />
        <Select.Icon>
          <Icon icon={ChevronDown} size={16} className="text-fg-2" />
        </Select.Icon>
      </Select.Trigger>
      <Select.Portal>
        <Select.Content
          position="popper"
          sideOffset={4}
          className="z-(--z-popover) max-h-[16rem] min-w-(--radix-select-trigger-width) overflow-y-auto rounded-panel bg-elevated p-4 shadow-panel"
        >
          <Select.Viewport>
            <SelectItem value={ALL} label={t('common.all')} />
            {sorted.map((t) => (
              <SelectItem key={t.id} value={t.name} label={t.name} count={t.link_count} />
            ))}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}

function SelectItem({ value, label, count }: { value: string; label: string; count?: number }) {
  return (
    <Select.Item
      value={value}
      className="relative flex h-32 cursor-default select-none items-center gap-8 rounded-control pl-8 pr-24 text-label text-fg-1 outline-none data-[highlighted]:bg-hover"
    >
      <Select.ItemText>{label}</Select.ItemText>
      {count != null ? <span className="font-mono text-fg-3">{count}</span> : null}
      <Select.ItemIndicator className="absolute right-8 inline-flex items-center">
        <Icon icon={Check} size={16} className="text-accent" />
      </Select.ItemIndicator>
    </Select.Item>
  )
}
