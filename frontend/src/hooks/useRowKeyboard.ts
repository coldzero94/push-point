// Row-cursor keyboard (11 §1.2, "목록·검색" + "행 커서" scopes). The cursor IS
// DOM focus: J/K move focus between the row trigger buttons (each tagged
// data-link-id), which gives a visible focus ring and scroll-into-view for free
// and keeps the roving model honest (10 §7.3). Enter is left to the button's
// native activation (it opens the inspector). O/E/N/R/Backspace act on the row
// whose button currently holds focus; when the inspector is already open it owns
// those keys (its own Dialog.Content handler), so we defer.
//
// Single-char keys are inert while a text input is focused (isEditableTarget),
// and modifier combos are ignored so Cmd/Ctrl+Enter (form save) and browser
// shortcuts pass through untouched.

import { useEffect, useRef } from 'react'
import type { RefObject } from 'react'
import { isEditableTarget } from '../lib/keyboard/shortcuts'
import type { InspectorFocus } from '../lib/keyboard/inspectorFocus'
import type { Link } from '../lib/api/types'

export type RowKeyboardOptions = {
  containerRef: RefObject<HTMLElement | null>
  links: readonly Link[]
  /** the inspector overlay is open (?link set) — it owns the action keys then */
  inspectorOpen: boolean
  onOpen: (id: number) => void
  onOpenWithFocus: (id: number, focus: InspectorFocus) => void
  onRetry: (link: Link) => void
  onDelete: (link: Link) => void
}

export function useRowKeyboard(opts: RowKeyboardOptions): void {
  // keep the listener stable across renders (links/cursor change often) by
  // reading the latest options through a ref.
  const latest = useRef(opts)
  latest.current = opts

  useEffect(() => {
    function rowButtons(): HTMLButtonElement[] {
      const c = latest.current.containerRef.current
      if (!c) return []
      return Array.from(c.querySelectorAll<HTMLButtonElement>('[data-link-id]'))
    }

    function onKeyDown(e: KeyboardEvent) {
      if (isEditableTarget(e.target)) return
      if (e.metaKey || e.ctrlKey || e.altKey) return

      const buttons = rowButtons()
      if (buttons.length === 0) return
      const active = document.activeElement
      const idx = active instanceof HTMLElement ? buttons.indexOf(active as HTMLButtonElement) : -1
      const focus = (next: number) => buttons[Math.max(0, Math.min(buttons.length - 1, next))]?.focus()

      switch (e.key) {
        case 'j':
        case 'J':
        case 'ArrowDown':
          e.preventDefault()
          focus(idx < 0 ? 0 : idx + 1)
          return
        case 'k':
        case 'K':
        case 'ArrowUp':
          e.preventDefault()
          focus(idx < 0 ? buttons.length - 1 : idx - 1)
          return
      }

      // Action keys require a cursor (a focused row) and defer to the inspector
      // when it is open (§1.2 — "행 커서·인스펙터" is owned by whichever holds focus).
      if (idx < 0 || latest.current.inspectorOpen) return
      const id = Number(buttons[idx].dataset.linkId)
      const link = latest.current.links.find((l) => l.id === id)
      if (!link) return

      switch (e.key) {
        case 'o':
        case 'O':
          e.preventDefault()
          window.open(link.url, '_blank', 'noopener,noreferrer')
          return
        case 'e':
        case 'E':
          e.preventDefault()
          latest.current.onOpenWithFocus(id, 'tags')
          return
        case 'n':
        case 'N':
          e.preventDefault()
          latest.current.onOpenWithFocus(id, 'note')
          return
        case 'r':
        case 'R':
          if (link.status === 'failed') {
            e.preventDefault()
            latest.current.onRetry(link)
          }
          return
        case 'Backspace':
        case 'Delete':
          e.preventDefault()
          latest.current.onDelete(link)
          return
      }
    }

    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])
}
