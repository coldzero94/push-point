// Alias token input (11 §5(4)) — aliases are the strings the rule tagger matches
// on (R2 → mono), so editing them is the cheapest lever on tagging accuracy. A
// token editor: Enter or comma commits the typed text as a token, Backspace on an
// empty field removes the last token, and duplicates (case-insensitive) are
// ignored rather than added twice.

import { useState } from 'react'
import { X } from 'lucide-react'
import { Icon } from '../ui'
import type { KeyboardEvent } from 'react'

export function AliasInput({
  value,
  onChange,
  disabled,
}: {
  value: string[]
  onChange: (aliases: string[]) => void
  disabled?: boolean
}) {
  const [draft, setDraft] = useState('')

  const commit = () => {
    const t = draft.trim()
    if (!t) return
    if (!value.some((a) => a.toLowerCase() === t.toLowerCase())) onChange([...value, t])
    setDraft('')
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      commit()
    } else if (e.key === 'Backspace' && draft === '' && value.length > 0) {
      e.preventDefault()
      onChange(value.slice(0, -1))
    }
  }

  return (
    <div
      className="flex flex-wrap items-center gap-6 rounded-control border border-line-control bg-surface px-8 py-6 focus-within:outline focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-accent"
      // clicking anywhere in the field focuses the text input
      onClick={(e) => (e.currentTarget.querySelector('input') as HTMLInputElement | null)?.focus()}
    >
      {value.map((alias) => (
        <span
          key={alias}
          className="inline-flex h-24 items-center gap-4 rounded-chip bg-hover px-8 font-mono text-label text-fg-2"
        >
          {alias}
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              onChange(value.filter((a) => a !== alias))
            }}
            disabled={disabled}
            aria-label={`별칭 ${alias} 제거`}
            className="relative hit-target flex items-center justify-center rounded-chip text-fg-3 hover:text-fg-1"
          >
            <Icon icon={X} size={16} />
          </button>
        </span>
      ))}
      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={onKeyDown}
        onBlur={commit}
        disabled={disabled}
        placeholder={value.length === 0 ? '별칭 입력 후 Enter' : ''}
        aria-label="별칭 추가"
        className="min-w-[6rem] flex-1 bg-transparent font-mono text-meta text-fg-1 placeholder:text-fg-3 focus:outline-none disabled:opacity-45"
      />
    </div>
  )
}
