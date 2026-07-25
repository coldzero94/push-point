// Inspector (11 §6 / 10 §4.11) — detail + edit merged into one surface (11 §0).
// ≥1024 a non-modal right panel, 560–1023 a bottom sheet, <560 fullscreen.
//
// This file exports two things:
//   • InspectorPanel — controlled ({ id, onClose }); the whole detail + inline
//     edit surface. Used directly by the /links/$id deep-link route.
//   • LinkInspector  — a URL-driven overlay that reads ?link and closes by
//     clearing it. The integration owner mounts it once (e.g. in RootLayout) and
//     the list sets ?link on row-click; nothing here hard-codes a route.
//
// Runtime dependency: a <ToastProvider> (components/ui) must wrap the app — the
// undo/error toasts live in the app-wide aria-live region (10 §4.10 / §7).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { getRouteApi, useNavigate, useSearch } from '@tanstack/react-router'
import { AlertTriangle, ArrowLeft, ExternalLink, X } from 'lucide-react'
import { Button, Chip, EmptyState, GeneratedCover, Icon, Skeleton, StatusRail, Textarea, cn, useToast } from '../components/ui'
import { TagCombobox } from '../components/inspector/TagCombobox'
import type { TagComboboxHandle } from '../components/inspector/TagCombobox'
import { useCreateTag, useLinkDetail, useTags, useUpdateLink } from '../hooks/useLinkDetail'
import { useCreateLink, useDeleteLink, useRetryLink } from '../hooks/useLinkMutations'
import { startPolling } from '../hooks/useSaveLink'
import { useQueryClient } from '@tanstack/react-query'
import { dominantFacet, makeFacetResolver } from '../lib/tags/facet'
import { linkDisplayTitle } from '../lib/api/types'
import type { LinkDetail, LinkTag } from '../lib/api/types'
import { errorMessage } from '../lib/api/client'
import { consumeInspectorFocus } from '../lib/keyboard/inspectorFocus'
import { formatCount, formatDateTime, formatDay, formatDuration } from '../lib/datetime'

// ── small hooks ───────────────────────────────────────────────────────────

function useMediaQuery(query: string): boolean {
  const [match, setMatch] = useState(() => window.matchMedia(query).matches)
  useEffect(() => {
    const mql = window.matchMedia(query)
    const on = () => setMatch(mql.matches)
    on()
    mql.addEventListener('change', on)
    return () => mql.removeEventListener('change', on)
  }, [query])
  return match
}

function useOnline(): boolean {
  const [online, setOnline] = useState(() => navigator.onLine)
  useEffect(() => {
    const on = () => setOnline(true)
    const off = () => setOnline(false)
    window.addEventListener('online', on)
    window.addEventListener('offline', off)
    return () => {
      window.removeEventListener('online', on)
      window.removeEventListener('offline', off)
    }
  }, [])
  return online
}

const prefersReduce = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches

// Enter/leave presence: keep mounted through the close animation (--dur-close),
// but unmount instantly under reduced-motion (§6.1 — JS motion checks the MQL).
function useEnterLeave(open: boolean): { mounted: boolean; shown: boolean } {
  const [mounted, setMounted] = useState(open)
  const [shown, setShown] = useState(false)
  const timer = useRef<number | undefined>(undefined)
  useEffect(() => {
    clearTimeout(timer.current)
    if (open) {
      setMounted(true)
      const r = requestAnimationFrame(() => setShown(true))
      return () => cancelAnimationFrame(r)
    }
    setShown(false)
    timer.current = window.setTimeout(() => setMounted(false), prefersReduce() ? 0 : 200)
    return () => clearTimeout(timer.current)
  }, [open])
  return { mounted, shown }
}

function readErrorCode(e: unknown): string | undefined {
  if (e && typeof e === 'object' && 'error' in e) {
    const err = (e as { error?: { code?: unknown } }).error
    if (err && typeof err.code === 'string') return err.code
  }
  return undefined
}

function isEditableTarget(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false
  return t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable
}

// primary-button utility string, reused by the "원문 열기" anchor (needs a real
// <a> for middle/cmd-click). Tokens only — no raw hex.
const PRIMARY_ANCHOR =
  'inline-flex h-32 select-none items-center gap-6 rounded-control px-12 text-label ' +
  'bg-accent text-on-accent transition-colors duration-(--dur-out) ease-ui hover:bg-accent-hover'

// ── controlled panel ──────────────────────────────────────────────────────

export type InspectorPanelProps = { id: number | null; onClose: () => void }

export function InspectorPanel({ id, onClose }: InspectorPanelProps) {
  const isDesktop = useMediaQuery('(min-width: 1024px)') // ≥1024 → non-modal side panel
  const isTablet = useMediaQuery('(min-width: 560px)') // 560–1023 → sheet, <560 → fullscreen
  const online = useOnline()
  const toast = useToast()
  const qc = useQueryClient()

  // hold the last non-null id so content survives the close animation.
  const [displayId, setDisplayId] = useState(id)
  useEffect(() => {
    if (id != null) setDisplayId(id)
  }, [id])

  const { mounted, shown } = useEnterLeave(id != null)

  const detail = useLinkDetail(displayId)
  const tagsQuery = useTags()
  const update = useUpdateLink()
  const createTag = useCreateTag()
  const retry = useRetryLink()
  const del = useDeleteLink()
  const create = useCreateLink()

  const link = detail.data
  const facetOf = useMemo(() => makeFacetResolver(tagsQuery.data), [tagsQuery.data])
  const comboRef = useRef<TagComboboxHandle>(null)
  const noteRef = useRef<HTMLTextAreaElement>(null)

  // note draft — reset only when the link identity or server note changes, so a
  // background poll never clobbers what the user is typing (server note is stable
  // across polls; the user's own edits diverge and are left alone).
  const linkId = link?.id
  const serverNote = link?.note
  const [noteDraft, setNoteDraft] = useState('')
  useEffect(() => {
    if (serverNote !== undefined) setNoteDraft(serverNote)
  }, [linkId, serverNote])

  // When the inspector was opened by the row-cursor `E`/`N` shortcut, the list
  // parked a focus intent (§1.2 — "인스펙터를 열고 태그/메모 입력에 포커스"). Consume
  // it once the content for this link is present. consumeInspectorFocus is a
  // one-shot read (returns null after), so a re-run on a background poll is inert.
  useEffect(() => {
    if (!link) return
    const target = consumeInspectorFocus()
    if (target === 'tags') comboRef.current?.openAndFocus()
    else if (target === 'note') noteRef.current?.focus()
  }, [link])

  const toastErr = useCallback(
    (e: unknown) => toast.show({ variant: 'error', message: errorMessage(e) }),
    [toast],
  )

  const currentNames = link ? link.tags.map((t) => t.name) : []

  // full LinkTag[] for the optimistic cache: keep existing attachments as-is,
  // render new ones as manual/confidence-null with the dictionary id (facet).
  const buildOptimisticTags = useCallback(
    (names: string[]): LinkTag[] => {
      const byName = new Map((tagsQuery.data ?? []).map((t) => [t.name.toLowerCase(), t]))
      const existing = new Map((link?.tags ?? []).map((t) => [t.name.toLowerCase(), t]))
      return names.map((name): LinkTag => {
        const prev = existing.get(name.toLowerCase())
        if (prev) return prev
        const d = byName.get(name.toLowerCase())
        return { id: d?.id ?? -1, name, source: 'manual', confidence: null }
      })
    },
    [tagsQuery.data, link?.tags],
  )

  const replaceTags = useCallback(
    (names: string[]) => {
      if (!link) return
      update.mutate(
        { id: link.id, tags: names, optimisticTags: buildOptimisticTags(names) },
        { onError: toastErr },
      )
    },
    [link, update, buildOptimisticTags, toastErr],
  )

  const addTag = useCallback(
    (name: string) => {
      if (link) replaceTags([...link.tags.map((t) => t.name), name])
    },
    [link, replaceTags],
  )
  const removeTag = useCallback(
    (name: string) => {
      if (link) replaceTags(link.tags.map((t) => t.name).filter((n) => n !== name))
    },
    [link, replaceTags],
  )
  const createAndAttach = useCallback(
    (name: string) => {
      if (!link) return
      createTag.mutate(name, { onSuccess: () => addTag(name), onError: toastErr })
    },
    [link, createTag, addTag, toastErr],
  )

  const saveNote = useCallback(() => {
    if (!link || noteDraft === link.note) return
    update.mutate({ id: link.id, note: noteDraft }, { onError: toastErr })
  }, [link, noteDraft, update, toastErr])

  const onRetry = useCallback(() => {
    if (!link || link.status !== 'failed') return
    // optimistic: rail flips to progress immediately (11 §6(4)).
    qc.setQueryData<LinkDetail>(['link', link.id], (p) => (p ? { ...p, status: 'pending' } : p))
    retry.mutate(link.id, { onError: toastErr })
  }, [link, qc, retry, toastErr])

  const onDelete = useCallback(() => {
    if (!link) return
    const { url, note } = link
    del.mutate(link.id, {
      onError: toastErr,
      onSuccess: () => {
        onClose()
        toast.show({
          variant: 'undo',
          message: '삭제했습니다.',
          // no restore endpoint — re-POST the same url reopens the row (pending,
          // re-scraped). The label does not hide that (11 §1.5).
          action: {
            label: '되돌리기 — 다시 수집됩니다',
            onClick: () =>
              create.mutate(
                { url, note: note || undefined },
                {
                  onError: toastErr,
                  // Parity with the default save path (useSaveLink): poll the
                  // restored 201 to done so title/tags/thumb fill in (§1.5). A
                  // 200 duplicate is already terminal — nothing to poll.
                  onSuccess: (res) => {
                    if (!res.duplicate) startPolling(qc, res.id)
                  },
                },
              ),
          },
        })
      },
    })
  }, [link, del, create, qc, onClose, toast, toastErr])

  const openOriginal = useCallback(() => {
    if (link) window.open(link.url, '_blank', 'noopener,noreferrer')
  }, [link])

  // inspector-scoped shortcuts (11 §1.2). Single-char keys are inert inside an
  // input; Esc is handled by Radix, Cmd/Ctrl+Enter by the note field itself.
  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (isEditableTarget(e.target) || e.metaKey || e.ctrlKey || e.altKey) return
      switch (e.key) {
        case 'e':
        case 'E':
          e.preventDefault()
          comboRef.current?.openAndFocus()
          break
        case 'n':
        case 'N':
          e.preventDefault()
          noteRef.current?.focus()
          break
        case 'r':
        case 'R':
          if (link?.status === 'failed') {
            e.preventDefault()
            onRetry()
          }
          break
        case 'o':
        case 'O':
          e.preventDefault()
          openOriginal()
          break
        case 'Backspace':
        case 'Delete':
          e.preventDefault()
          onDelete()
          break
      }
    },
    [link?.status, onRetry, openOriginal, onDelete],
  )

  if (!mounted) return null

  const notFound = detail.isError && readErrorCode(detail.error) === 'not_found'
  const writesDisabled = !online || update.isPending

  // ── responsive container geometry ──
  const geometry = isDesktop
    ? // right-docked panel below the sticky header; list stays interactive
      cn('right-0 border-l border-line-2 shadow-panel', shown ? 'translate-x-0 opacity-100' : 'translate-x-12 opacity-0')
    : isTablet
      ? // bottom sheet 85dvh
        cn('inset-x-0 bottom-0 z-(--z-sheet) rounded-t-sheet shadow-sheet', shown ? 'translate-y-0' : 'translate-y-full')
      : // fullscreen
        cn('inset-0 z-(--z-sheet)', shown ? 'translate-y-0' : 'translate-y-full')
  const contentClass = cn(
    'fixed flex flex-col overflow-y-auto bg-elevated transition duration-(--dur-3) ease-enter',
    !shown && 'duration-(--dur-close) ease-ui',
    geometry,
  )
  const contentStyle = isDesktop
    ? { top: 'var(--size-header)', height: 'calc(100dvh - var(--size-header))', width: 'var(--w-inspector)' }
    : isTablet
      ? { height: '85dvh' }
      : undefined

  const title = link ? linkDisplayTitle(link) : ''

  return (
    <Dialog.Root open={mounted} onOpenChange={(o) => !o && onClose()} modal={!isDesktop}>
      <Dialog.Portal>
        {!isDesktop ? (
          <Dialog.Overlay
            className={cn(
              'fixed inset-0 z-(--z-overlay) bg-canvas/75 transition-opacity duration-(--dur-3) ease-enter',
              !shown && 'opacity-0 duration-(--dur-close)',
            )}
          />
        ) : null}
        <Dialog.Content
          aria-describedby={undefined}
          onKeyDown={onKeyDown}
          className={contentClass}
          style={contentStyle}
        >
          {/* leading status rail (S1) — 2px, same thickness as the row */}
          <div className="flex min-h-0 flex-1">
            {link ? <StatusRail status={link.status} /> : <span className="w-(--size-rail)" aria-hidden />}

            <div className="min-w-0 flex-1 p-20">
              {/* Cover header (R4) — full-bleed to the panel edges. Unlike the
                  old thumbnail slot this is never removed: a link with no
                  thumb_url gets a generated cover, so the panel opens with the
                  same anchor every time (§10 4.5). */}
              {link ? (
                <div className="relative -mx-20 -mt-20 mb-16 aspect-[16/9] overflow-hidden bg-hover">
                  {link.thumb_url ? (
                    <img
                      src={link.thumb_url}
                      alt=""
                      role="presentation"
                      loading="lazy"
                      decoding="async"
                      className="thumb-img h-full w-full object-cover"
                    />
                  ) : (
                    <>
                      <GeneratedCover
                        domain={link.domain}
                        facet={dominantFacet(link.tags, facetOf)}
                      />
                      <span className="absolute bottom-12 left-20 font-mono text-body text-fg-2">
                        {link.domain}
                      </span>
                    </>
                  )}
                </div>
              ) : null}

              {/* header: title + domain + close */}
              <div className="flex items-start gap-12">
                {!isTablet ? (
                  <Dialog.Close asChild>
                    <button type="button" aria-label="닫기" className="-ml-4 shrink-0 rounded-control p-4 text-fg-2 hover:bg-hover">
                      <Icon icon={ArrowLeft} size={20} />
                    </button>
                  </Dialog.Close>
                ) : null}
                <div className="min-w-0 flex-1">
                  <Dialog.Title asChild>
                    <h2 className="line-clamp-3 text-head text-fg-1">{title || <span className="text-fg-3">불러오는 중…</span>}</h2>
                  </Dialog.Title>
                  {link ? <p className="mt-4 truncate font-mono text-meta text-fg-3">{link.domain}</p> : null}
                </div>
                {isTablet ? (
                  <Dialog.Close asChild>
                    <button type="button" aria-label="닫기" className="shrink-0 rounded-control p-4 text-fg-2 hover:bg-hover">
                      <Icon icon={X} size={20} />
                    </button>
                  </Dialog.Close>
                ) : null}
              </div>

              {notFound ? (
                <EmptyState
                  className="py-40"
                  title="삭제되었거나 없는 링크입니다"
                  description="이 링크는 더 이상 존재하지 않습니다."
                  action={
                    <Dialog.Close asChild>
                      <Button variant="secondary">닫기</Button>
                    </Dialog.Close>
                  }
                />
              ) : !link ? (
                <InspectorSkeleton />
              ) : (
                <div className="mt-16 flex flex-col gap-16">
                  {/* actions */}
                  <div className="flex flex-wrap items-center gap-8">
                    <a href={link.url} target="_blank" rel="noreferrer" className={PRIMARY_ANCHOR}>
                      <Icon icon={ExternalLink} size={16} />
                      원문 열기
                    </a>
                    {link.status === 'failed' ? (
                      <Button variant="secondary" onClick={onRetry} disabled={writesDisabled} loading={retry.isPending}>
                        재시도
                      </Button>
                    ) : null}
                    <Button variant="danger" onClick={onDelete} disabled={writesDisabled} loading={del.isPending}>
                      삭제
                    </Button>
                  </div>

                  <Section label="태그">
                    <div className="flex flex-wrap items-center gap-6">
                      {link.tags.map((t) => (
                        <Chip
                          key={`${t.id}-${t.name}`}
                          facet={facetOf({ id: t.id })}
                          source={t.source}
                          role="readonly"
                          disabled={writesDisabled}
                          onRemove={() => removeTag(t.name)}
                        >
                          {t.name}
                        </Chip>
                      ))}
                      <TagCombobox
                        ref={comboRef}
                        tags={tagsQuery.data ?? []}
                        attachedNames={currentNames}
                        onAdd={addTag}
                        onCreateAndAttach={createAndAttach}
                        creating={createTag.isPending}
                        disabled={writesDisabled}
                      />
                    </div>
                    {link.tags.length > 0 ? (
                      <p className="mt-6 font-mono text-meta text-fg-3">
                        {link.tags
                          .map((t) => `${t.source} ${t.confidence != null ? t.confidence.toFixed(2) : '—'}`)
                          .join(' · ')}
                      </p>
                    ) : null}
                  </Section>

                  <Section label="메모">
                    <Textarea
                      ref={noteRef}
                      value={noteDraft}
                      disabled={!online}
                      onChange={(e) => setNoteDraft(e.target.value)}
                      onBlur={saveNote}
                      onKeyDown={(e) => {
                        if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                          e.preventDefault()
                          saveNote()
                        }
                      }}
                      placeholder="나중에 왜 저장했는지 적어 두세요"
                    />
                  </Section>

                  {link.description ? (
                    <Section label="설명">
                      <p className="whitespace-pre-line text-body text-fg-2">{link.description}</p>
                    </Section>
                  ) : null}

                  <Section label="메타">
                    <dl className="flex flex-col gap-6">
                      <MetaRow label="저장" value={formatDateTime(link.created_at)} mono />
                      {link.published_at != null ? <MetaRow label="발행" value={formatDay(link.published_at)} mono /> : null}
                      {link.author ? <MetaRow label="작성자" value={link.author} /> : null}
                      <MetaRow label="종류" value={link.content_type} />
                      {link.duration_sec != null || link.word_count != null ? (
                        <MetaRow
                          label="길이"
                          mono
                          value={[
                            link.duration_sec != null ? formatDuration(link.duration_sec) : null,
                            link.word_count != null ? `${formatCount(link.word_count)} 단어` : null,
                          ]
                            .filter(Boolean)
                            .join(' / ')}
                        />
                      ) : null}
                      {link.lang ? <MetaRow label="언어" value={link.lang} mono /> : null}
                    </dl>
                  </Section>

                  <Section label="잡">
                    <p className="font-mono text-meta">
                      {(
                        [
                          ['scrape', link.jobs.scrape],
                          ['tag', link.jobs.tag],
                          ['thumb', link.jobs.thumb],
                        ] as const
                      ).map(([kind, st], i) => (
                        <span key={kind} className={cn(i > 0 && 'ml-16')}>
                          <span className="text-fg-3">{kind} </span>
                          <span className={st === 'failed' ? 'text-danger' : st == null ? 'text-fg-3' : 'text-fg-2'}>
                            {st ?? '—'}
                          </span>
                        </span>
                      ))}
                    </p>
                  </Section>

                  {link.error ? (
                    <Section label="오류">
                      <p className="flex items-start gap-6 text-meta text-danger">
                        <Icon icon={AlertTriangle} size={16} className="mt-2 shrink-0" />
                        <span>{link.error}</span>
                      </p>
                    </Section>
                  ) : null}
                </div>
              )}
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

// ── section + meta row ────────────────────────────────────────────────────

function Section({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <section className="border-t border-line-2 pt-16">
      <h3 className="mb-8 text-label text-fg-2">{label}</h3>
      {children}
    </section>
  )
}

function MetaRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-16">
      <dt className="shrink-0 text-meta text-fg-2">{label}</dt>
      <dd className={cn('min-w-0 truncate text-right text-meta text-fg-1', mono && 'font-mono')}>{value}</dd>
    </div>
  )
}

function InspectorSkeleton() {
  return (
    <div className="mt-16 flex flex-col gap-16" aria-hidden>
      {/* No cover skeleton: the cover header renders as soon as `link` exists,
          and this skeleton only shows while it does not. */}
      <div className="flex gap-8">
        <Skeleton className="h-32 w-1/3" />
        <Skeleton className="h-32 w-16" />
      </div>
      <Skeleton variant="text" className="h-16 w-full" />
      <Skeleton variant="text" className="h-16 w-1/2" />
      <Skeleton variant="text" className="h-16 w-3/4" />
    </div>
  )
}

// ── URL-driven overlay ────────────────────────────────────────────────────

// Reads ?link with a route-agnostic (strict:false) search read so it can be
// mounted once above the routed screens; closes by clearing `link` while
// preserving the rest of the search (tag/status/q). The list is responsible for
// setting ?link on row-click and for declaring `link` in its search schema.
export function LinkInspector() {
  const navigate = useNavigate()
  const search = useSearch({ strict: false }) as { link?: number | string }
  const raw = search.link
  const id = raw != null && Number(raw) > 0 ? Number(raw) : null

  const onClose = useCallback(() => {
    void navigate({
      to: '.',
      search: (prev) => {
        const next = { ...(prev as Record<string, unknown>) }
        delete next.link
        return next
      },
    })
  }, [navigate])

  return <InspectorPanel id={id} onClose={onClose} />
}

// ── deep-link route (/links/$id) ──────────────────────────────────────────

const detailRoute = getRouteApi('/links/$id')

// Bookmark/deep-link entry: render the inspector for the path id. Closing
// returns to the list (11 §6(4)).
export function LinkInspectorRoute() {
  const { id } = detailRoute.useParams()
  const navigate = useNavigate()
  const numId = Number(id)
  return (
    <InspectorPanel
      id={Number.isFinite(numId) && numId > 0 ? numId : null}
      onClose={() => void navigate({ to: '/' })}
    />
  )
}
