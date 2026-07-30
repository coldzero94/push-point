// Tags (screen 4) — §11 5. The controlled dictionary (30–50 tags) that feeds the
// rule tagger, and the ONLY place a user sees and changes a tag's facet by
// meaning rather than by color (§10 5.5). A sortable table with inline edit; the
// one screen in the app that uses a confirm dialog (delete has no undo — CASCADE
// + no restore endpoint, §11 5(4)).
//
// Deliberately NOT grouped/sorted by facet (§11 5(4)): color already encodes
// facet, so sorting by it too would double-encode. Sort is link_count desc
// (default) or name.

import { useMemo, useState } from 'react'
import { formatRelativeTime } from '../lib/time'
import { sortTags, type TagSortKey } from '../lib/tagSort'
import { Link } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import * as Dialog from '@radix-ui/react-dialog'
import {
  Button,
  Chip,
  EmptyState,
  Icon,
  Input,
  Skeleton,
  cn,
  useToast,
} from '../components/ui'
import { FacetSelect } from '../components/tags/FacetSelect'
import { AliasInput } from '../components/tags/AliasInput'
import { useTags } from '../hooks/useTags'
import { useStats } from '../hooks/useStats'
import { useCreateDictionaryTag, useDeleteTag, useUpdateTag } from '../hooks/useTagMutations'
import { FACET_LABELS } from '../lib/tags/facet'
import { errorMessage, isErrorCode } from '../lib/api/client'
import { useOnline } from '../hooks/useOnline'
import type { Tag, TagFacet } from '../lib/api/types'



export function TagsScreen() {
  const tagsQuery = useTags()
  const stats = useStats()
  const online = useOnline()

  const [sort, setSort] = useState<TagSortKey>('count')
  const [creating, setCreating] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)

  const sorted = useMemo(() => sortTags(tagsQuery.data ?? [], sort), [tagsQuery.data, sort])

  if (tagsQuery.isPending) return <TagsSkeleton />

  if (tagsQuery.isError) {
    return (
      <section className="mx-auto max-w-(--w-content) pt-16">
        <div className="flex flex-col items-center gap-12 rounded-card bg-surface py-40 text-center shadow-ring">
          <p className="text-body text-fg-2">{errorMessage(tagsQuery.error)}</p>
          <Button variant="secondary" onClick={() => void tagsQuery.refetch()}>
            다시 시도
          </Button>
        </div>
      </section>
    )
  }

  const tags = tagsQuery.data
  const total = stats.data?.total_links

  return (
    <section className="mx-auto flex max-w-(--w-content) flex-col gap-16 pt-16">
      {/* Header — count + total (mono) + add. */}
      <div className="flex items-end justify-between gap-12">
        <div>
          <h1 className="text-head text-fg-1">태그 사전</h1>
          <p className="mt-4 font-mono text-meta text-fg-3">
            {tags.length}개{total != null ? ` · 링크 ${total.toLocaleString()}건` : ''}
          </p>
        </div>
        <Button
          variant="primary"
          disabled={!online || creating}
          onClick={() => {
            setCreating(true)
            setEditingId(null)
          }}
        >
          <Icon icon={Plus} size={16} />
          태그 추가
        </Button>
      </div>

      {/* Sort toggle — 2 keys only (§11 5(4)). */}
      <div className="flex items-center gap-6">
        <span className="text-label text-fg-3">정렬</span>
        <SortToggle value={sort} onChange={setSort} />
      </div>

      {tags.length === 0 && !creating ? (
        <EmptyState
          title="태그 사전이 비어 있습니다"
          description="자동 태깅은 사전에 있는 태그만 붙입니다."
          action={
            <Button variant="primary" disabled={!online} onClick={() => setCreating(true)}>
              <Icon icon={Plus} size={16} />
              태그 추가
            </Button>
          }
        />
      ) : (
        <ul className="flex flex-col rounded-card border border-line-1 bg-surface">
          {creating ? (
            <li className="border-b border-line-1 last:border-b-0">
              <CreateRow onDone={() => setCreating(false)} online={online} />
            </li>
          ) : null}
          {sorted.map((tag) => (
            <li key={tag.id} className="border-b border-line-1 last:border-b-0">
              <TagRow
                tag={tag}
                editing={editingId === tag.id}
                online={online}
                onEdit={() => {
                  setEditingId(tag.id)
                  setCreating(false)
                }}
                onDone={() => setEditingId(null)}
              />
            </li>
          ))}
        </ul>
      )}

      {/* aliases matter most for accuracy — say so once, under the list. */}
      <p className="text-meta text-fg-3">별칭은 규칙 태거가 문자열로 매칭하는 대상입니다.</p>
    </section>
  )
}

function SortToggle({ value, onChange }: { value: TagSortKey; onChange: (k: TagSortKey) => void }) {
  const opt = (k: TagSortKey, label: string) => (
    <button
      type="button"
      aria-pressed={value === k}
      onClick={() => onChange(k)}
      className={cn(
        'h-24 rounded-control px-8 text-label transition-colors duration-(--dur-out) ease-ui',
        value === k ? 'bg-accent-tint text-accent' : 'text-fg-2 hover:bg-hover',
      )}
    >
      {label}
    </button>
  )
  return (
    <div className="flex items-center gap-2">
      {opt('count', '링크 수')}
      {opt('recent', '최근 저장순')}
      {opt('name', '이름')}
    </div>
  )
}

// A single dictionary row. Collapsed: name (→ filter), facet chip, alias mono
// chips, count, 편집. Expanded: the inline edit form.
function TagRow({
  tag,
  editing,
  online,
  onEdit,
  onDone,
}: {
  tag: Tag
  editing: boolean
  online: boolean
  onEdit: () => void
  onDone: () => void
}) {
  if (editing) return <EditForm tag={tag} online={online} onDone={onDone} />

  return (
    <div className="flex items-center gap-12 px-16 py-12">
      <Link
        to="/"
        search={{ tag: tag.name }}
        className="min-w-0 shrink-0 truncate text-title text-fg-1 hover:underline"
      >
        {tag.name}
      </Link>
      <Chip facet={tag.facet} role="readonly" source="manual">
        {FACET_LABELS[tag.facet]}
      </Chip>
      <div className="flex min-w-0 flex-1 flex-wrap gap-4 overflow-hidden">
        {tag.aliases.map((a) => (
          <span key={a} className="rounded-chip bg-hover px-8 py-2 font-mono text-label text-fg-3">
            {a}
          </span>
        ))}
      </div>
      {/* **신선도 한 조각.** 두 달 전에 끊긴 주제와 이번 주 주제가 같은 자리에 섞여 있는
          것이 통증이었고, 그건 추세가 아니라 태그당 날짜 하나로 답해진다(12 §4.2).
          화살표·상승/하락·카운트업 없음 — 그 셋을 붙이면 백로그가 "요즘 관심"을 기각한
          자리로 되돌아간다. */}
      <span className="shrink-0 font-mono text-meta tabular-nums text-fg-3">
        {tag.last_saved_at ? formatRelativeTime(tag.last_saved_at) : ''}
      </span>
      <span className="shrink-0 font-mono text-meta tabular-nums text-fg-2">{tag.link_count}</span>
      <Button size="sm" variant="ghost" disabled={!online} onClick={onEdit}>
        편집
      </Button>
    </div>
  )
}

// Shared edit surface for both create and edit. `existing` null = create.
function EditForm({
  tag,
  online,
  onDone,
}: {
  tag: Tag
  online: boolean
  onDone: () => void
}) {
  const toast = useToast()
  const update = useUpdateTag()
  const del = useDeleteTag()

  const [name, setName] = useState(tag.name)
  const [facet, setFacet] = useState<TagFacet>(tag.facet)
  const [aliases, setAliases] = useState<string[]>(tag.aliases)
  const [nameError, setNameError] = useState<string | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)

  const busy = update.isPending || del.isPending || !online

  const save = () => {
    const trimmed = name.trim()
    if (!trimmed) {
      setNameError('이름을 입력하세요.')
      return
    }
    update.mutate(
      {
        id: tag.id,
        name: trimmed,
        aliases,
        facet,
        optimistic: { ...tag, name: trimmed, aliases, facet },
      },
      {
        onSuccess: () => onDone(),
        onError: (err) => {
          if (isErrorCode(err, 'invalid_input')) setNameError('이미 있는 이름입니다.')
          else toast.show({ variant: 'error', message: errorMessage(err) })
        },
      },
    )
  }

  const remove = () =>
    del.mutate(tag.id, {
      onSuccess: () => {
        setConfirmOpen(false)
        onDone()
        toast.show({ variant: 'success', message: `"${tag.name}"을(를) 삭제했습니다.` })
      },
      onError: (err) => {
        setConfirmOpen(false)
        toast.show({ variant: 'error', message: errorMessage(err) })
      },
    })

  return (
    <div className="flex flex-col gap-12 bg-hover/40 px-16 py-16">
      <div className="flex flex-col gap-12 sm:flex-row sm:items-start">
        <div className="flex-1">
          <label className="mb-6 block text-label text-fg-2">이름</label>
          <Input
            value={name}
            autoFocus
            onChange={(e) => {
              setName(e.target.value)
              if (nameError) setNameError(null)
            }}
            invalid={nameError != null}
            errorMessage={nameError ?? undefined}
            aria-label="태그 이름"
          />
        </div>
        <div>
          <label className="mb-6 block text-label text-fg-2">분류</label>
          <FacetSelect value={facet} onChange={setFacet} disabled={busy} />
        </div>
      </div>

      <div>
        <label className="mb-6 block text-label text-fg-2">별칭</label>
        <AliasInput value={aliases} onChange={setAliases} disabled={busy} />
      </div>

      <div className="flex items-center gap-8">
        <Button variant="primary" onClick={save} loading={update.isPending} disabled={busy}>
          저장
        </Button>
        <Button variant="secondary" onClick={onDone} disabled={busy}>
          취소
        </Button>
        <Button
          variant="danger"
          className="ml-auto"
          disabled={busy}
          onClick={() => setConfirmOpen(true)}
        >
          삭제
        </Button>
      </div>

      <DeleteConfirm
        open={confirmOpen}
        tag={tag}
        pending={del.isPending}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={remove}
      />
    </div>
  )
}

// Create row — a full-width edit form seeded to an empty neutral tag (§11 5(4):
// new tags are born neutral). Reuses the create hook rather than EditForm because
// there is no id yet and no delete.
function CreateRow({ onDone, online }: { onDone: () => void; online: boolean }) {
  const toast = useToast()
  const create = useCreateDictionaryTag()

  const [name, setName] = useState('')
  const [facet, setFacet] = useState<TagFacet>('neutral')
  const [aliases, setAliases] = useState<string[]>([])
  const [nameError, setNameError] = useState<string | null>(null)

  const busy = create.isPending || !online

  const submit = () => {
    const trimmed = name.trim()
    if (!trimmed) {
      setNameError('이름을 입력하세요.')
      return
    }
    create.mutate(
      { name: trimmed, aliases, facet },
      {
        onSuccess: () => onDone(),
        onError: (err) => {
          if (isErrorCode(err, 'invalid_input')) setNameError('이미 있는 이름입니다.')
          else toast.show({ variant: 'error', message: errorMessage(err) })
        },
      },
    )
  }

  return (
    <div className="flex flex-col gap-12 bg-accent-tint/40 px-16 py-16">
      <div className="flex flex-col gap-12 sm:flex-row sm:items-start">
        <div className="flex-1">
          <label className="mb-6 block text-label text-fg-2">이름</label>
          <Input
            value={name}
            autoFocus
            onChange={(e) => {
              setName(e.target.value)
              if (nameError) setNameError(null)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') submit()
            }}
            invalid={nameError != null}
            errorMessage={nameError ?? undefined}
            placeholder="새 태그 이름"
            aria-label="새 태그 이름"
          />
        </div>
        <div>
          <label className="mb-6 block text-label text-fg-2">분류</label>
          <FacetSelect value={facet} onChange={setFacet} disabled={busy} />
        </div>
      </div>
      <div>
        <label className="mb-6 block text-label text-fg-2">별칭 <span className="text-fg-3">(선택)</span></label>
        <AliasInput value={aliases} onChange={setAliases} disabled={busy} />
      </div>
      <div className="flex items-center gap-8">
        <Button variant="primary" onClick={submit} loading={create.isPending} disabled={busy}>
          추가
        </Button>
        <Button variant="secondary" onClick={onDone} disabled={busy}>
          취소
        </Button>
      </div>
    </div>
  )
}

// Delete confirm — the app's only confirm dialog (§11 5(4)). role="alertdialog"
// via the Dialog primitive; default focus lands on 취소 (autoFocus), and the
// message states the blast radius in numbers from Tag.link_count.
function DeleteConfirm({
  open,
  tag,
  pending,
  onCancel,
  onConfirm,
}: {
  open: boolean
  tag: Tag
  pending: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && onCancel()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-(--z-overlay) bg-canvas/75" />
        <Dialog.Content
          role="alertdialog"
          className="fixed left-1/2 top-1/2 z-(--z-sheet) w-full max-w-(--w-form) -translate-x-1/2 -translate-y-1/2 rounded-panel bg-elevated p-20 shadow-panel"
        >
          <Dialog.Title className="text-title text-fg-1">태그를 삭제할까요?</Dialog.Title>
          <Dialog.Description className="mt-8 text-body text-fg-2">
            <span className="font-medium text-fg-1">{tag.name}</span>을(를) 삭제하면 링크{' '}
            {tag.link_count.toLocaleString()}건에서 이 태그가 함께 제거됩니다. 되돌릴 수 없습니다.
          </Dialog.Description>
          <div className="mt-20 flex justify-end gap-8">
            <Button variant="secondary" autoFocus onClick={onCancel} disabled={pending}>
              취소
            </Button>
            <Button variant="danger" onClick={onConfirm} loading={pending}>
              삭제
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function TagsSkeleton() {
  return (
    <section className="mx-auto flex max-w-(--w-content) flex-col gap-16 pt-16" aria-hidden>
      <Skeleton variant="text" className="h-24 w-40" />
      <ul className="flex flex-col rounded-card border border-line-1 bg-surface">
        {Array.from({ length: 10 }, (_, i) => (
          <li key={i} className="flex items-center gap-12 border-b border-line-1 px-16 py-12 last:border-b-0">
            <Skeleton variant="text" className="h-16 w-80" />
            <Skeleton variant="text" className="h-20 w-56" />
            <Skeleton variant="text" className="h-12 flex-1" />
            <Skeleton variant="text" className="h-12 w-24" />
          </li>
        ))}
      </ul>
    </section>
  )
}
