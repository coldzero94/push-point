// Tag combobox (11 §6(4), §1.8). Radix has no Combobox primitive, so we own the
// ARIA combobox pattern on top of Popover: role="combobox" input with
// aria-expanded / aria-controls / aria-activedescendant, a role="listbox" of
// role="option" rows, ↑↓ to move the active option, Enter to commit, Esc to
// close. The filter is one case-insensitive substring match — no fuzzy library.
//
// A typed name that is not in the dictionary offers a `warn` row: PATCH would
// 400 on an unknown tag, so we route through POST /tags first (11 §6(4)).

import { forwardRef, useId, useImperativeHandle, useMemo, useRef, useState } from 'react'
import * as Popover from '@radix-ui/react-popover'
import { Plus } from 'lucide-react'
import type { Tag } from '../../lib/api/types'
import { Icon, Input } from '../ui'
import { cn } from '../ui/cn'

export type TagComboboxHandle = { openAndFocus: () => void }

export type TagComboboxProps = {
  /** dictionary (GET /api/v1/tags) — options + exact-match check */
  tags: readonly Tag[]
  /** names already on the link, excluded from options (case-insensitive) */
  attachedNames: readonly string[]
  /** attach an existing dictionary tag by name (optimistic PATCH upstream) */
  onAdd: (name: string) => void
  /** create in the dictionary (POST /tags) then attach (PATCH) */
  onCreateAndAttach: (name: string) => void
  /** POST /tags in flight — disables the create row */
  creating?: boolean
  disabled?: boolean
}

export const TagCombobox = forwardRef<TagComboboxHandle, TagComboboxProps>(function TagCombobox(
  { tags, attachedNames, onAdd, onCreateAndAttach, creating = false, disabled = false },
  ref,
) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listId = useId()
  const optionId = (i: number) => `${listId}-opt-${i}`

  useImperativeHandle(ref, () => ({
    openAndFocus: () => setOpen(true),
  }))

  const attached = useMemo(
    () => new Set(attachedNames.map((n) => n.toLowerCase())),
    [attachedNames],
  )
  const q = query.trim().toLowerCase()

  const options = useMemo(() => {
    return tags
      .filter((t) => !attached.has(t.name.toLowerCase()) && t.name.toLowerCase().includes(q))
      .sort((a, b) => a.name.localeCompare(b.name))
      .slice(0, 8)
  }, [tags, attached, q])

  const exactExists = q !== '' && tags.some((t) => t.name.toLowerCase() === q)
  const showCreate = q !== '' && !exactExists
  // combined item count: options first, then the optional create row.
  const itemCount = options.length + (showCreate ? 1 : 0)
  const createIndex = options.length

  function reopenFresh(nextOpen: boolean) {
    setOpen(nextOpen)
    if (nextOpen) {
      setQuery('')
      setActive(0)
    }
  }

  function commit(index: number) {
    if (index < options.length) {
      onAdd(options[index].name)
      setQuery('')
      setActive(0)
      inputRef.current?.focus()
    } else if (showCreate && index === createIndex && !creating) {
      onCreateAndAttach(query.trim())
    }
  }

  function onInputKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((i) => (itemCount === 0 ? 0 : Math.min(itemCount - 1, i + 1)))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((i) => Math.max(0, i - 1))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (itemCount > 0) commit(Math.min(active, itemCount - 1))
    }
    // Esc / Tab handled by Radix Popover.
  }

  return (
    <Popover.Root open={open} onOpenChange={reopenFresh}>
      <Popover.Trigger asChild>
        <button
          type="button"
          disabled={disabled}
          className={cn(
            'inline-flex h-24 select-none items-center gap-4 rounded-chip border border-line-control px-8',
            'text-label text-fg-2 transition-colors duration-(--dur-out) ease-ui hover:bg-hover',
            'disabled:pointer-events-none disabled:opacity-45',
          )}
        >
          <Icon icon={Plus} size={16} />
          태그 추가
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="start"
          sideOffset={6}
          onOpenAutoFocus={(e) => {
            e.preventDefault()
            inputRef.current?.focus()
          }}
          // width has no token; a 260px combobox is a component dimension, kept
          // inline rather than inventing a --w-* token for one popover.
          style={{ width: '260px' }}
          className="rounded-panel bg-elevated p-8 shadow-panel"
        >
          <Input
            ref={inputRef}
            variant="text"
            role="combobox"
            aria-expanded={open}
            aria-controls={listId}
            aria-activedescendant={itemCount > 0 ? optionId(Math.min(active, itemCount - 1)) : undefined}
            aria-autocomplete="list"
            aria-label="태그 검색"
            spellCheck={false}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
              setActive(0)
            }}
            onKeyDown={onInputKeyDown}
            placeholder="태그 이름"
          />
          <ul id={listId} role="listbox" aria-label="태그 후보" className="mt-8 flex flex-col">
            {options.map((t, i) => (
              <li
                key={t.id}
                id={optionId(i)}
                role="option"
                aria-selected={i === active}
                onMouseEnter={() => setActive(i)}
                onClick={() => commit(i)}
                className={cn(
                  'flex cursor-pointer items-center rounded-control px-8 py-4 text-body text-fg-1',
                  i === active && 'bg-hover',
                )}
              >
                {t.name}
              </li>
            ))}

            {showCreate ? (
              <li
                id={optionId(createIndex)}
                role="option"
                aria-selected={active === createIndex}
                onMouseEnter={() => setActive(createIndex)}
                onClick={() => commit(createIndex)}
                className={cn(
                  'mt-4 flex cursor-pointer items-center gap-6 rounded-control px-8 py-4',
                  'text-meta text-warn',
                  active === createIndex && 'bg-warn-tint',
                  creating && 'pointer-events-none opacity-45',
                )}
              >
                <span className="font-sans text-fg-1">{query.trim()}</span>
                <span>은 사전에 없습니다 · 추가하고 붙이기</span>
              </li>
            ) : null}

            {itemCount === 0 && !showCreate ? (
              <li className="px-8 py-4 text-meta text-fg-3" aria-disabled>
                이름을 입력해 태그를 찾으세요
              </li>
            ) : null}
          </ul>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
})
