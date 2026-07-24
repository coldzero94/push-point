// Global keyboard handler + the `?` shortcuts overlay (11 §1.2 / §9 P0-7).
//
// This owns the truly-global keys: S (open the save composer), / (focus/enter
// search), and ? (toggle this overlay). The row-cursor keys (J/K/Enter/O/E/N/R/
// Backspace) live with the rows they act on (useRowKeyboard), and the form-scope
// Cmd/Ctrl+Enter is handled by each form (composer, inspector note, tag row) —
// so it does nothing outside a form, exactly as §1.2 requires. Keys not in the
// §1.2 table are never bound; the overlay renders that table 1:1 (SHORTCUTS).
//
// Single-char keys are inert while a text input is focused (isEditableTarget),
// and any modifier combo is ignored so browser and form shortcuts pass through.

import { useCallback, useEffect, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { useNavigate } from '@tanstack/react-router'
import { X } from 'lucide-react'
import { cn, Icon } from './ui'
import { SHORTCUTS, isEditableTarget } from '../lib/keyboard/shortcuts'

export function KeyboardShortcuts() {
  const navigate = useNavigate()
  const [overlayOpen, setOverlayOpen] = useState(false)

  // `/` focuses the search input if one is present, else routes to search and
  // lets it focus on mount (§1.2 — "목록에서는 검색으로 이동하며 포커스").
  const focusSearch = useCallback(() => {
    const input = document.querySelector<HTMLElement>('[data-search-input]')
    if (input) input.focus()
    else void navigate({ to: '/search' })
  }, [navigate])

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (isEditableTarget(e.target)) return
      // while the overlay is open only `?` (toggle-close) responds; Esc is Radix.
      if (overlayOpen) {
        if (e.key === '?') {
          e.preventDefault()
          setOverlayOpen(false)
        }
        return
      }
      switch (e.key) {
        case '?':
          e.preventDefault()
          setOverlayOpen(true)
          return
        case 's':
        case 'S':
          e.preventDefault()
          void navigate({ to: '/save' })
          return
        case '/':
          e.preventDefault()
          focusSearch()
          return
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [navigate, focusSearch, overlayOpen])

  return <ShortcutsOverlay open={overlayOpen} onOpenChange={setOverlayOpen} />
}

// ── the `?` overlay ───────────────────────────────────────────────────────
// A modal Dialog (Radix gives focus trap + Esc + focus return, 10 §7.4). Enter
// reuses the palette motion (opacity, --dur-1, ease-enter); reduced-motion is
// sealed globally. Renders SHORTCUTS 1:1 (10 §7.4).

function ShortcutsOverlay({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [shown, setShown] = useState(false)
  useEffect(() => {
    if (!open) {
      setShown(false)
      return
    }
    const r = requestAnimationFrame(() => setShown(true))
    return () => cancelAnimationFrame(r)
  }, [open])

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay
          className={cn(
            'fixed inset-0 z-(--z-overlay) bg-canvas/75 transition-opacity duration-(--dur-1) ease-enter',
            !shown && 'opacity-0',
          )}
        />
        <Dialog.Content
          aria-describedby={undefined}
          style={{ maxHeight: '85dvh' }}
          className={cn(
            'fixed left-1/2 top-1/2 z-(--z-palette) -translate-x-1/2 -translate-y-1/2',
            'w-full max-w-(--w-form) overflow-y-auto rounded-panel bg-elevated p-20 shadow-panel',
            'transition-opacity duration-(--dur-1) ease-enter',
            !shown && 'opacity-0',
          )}
        >
          <div className="flex items-start justify-between gap-12">
            <Dialog.Title className="text-head text-fg-1">키보드 단축키</Dialog.Title>
            <Dialog.Close asChild>
              <button
                type="button"
                aria-label="닫기"
                className="-mr-4 shrink-0 rounded-control p-4 text-fg-2 hover:bg-hover"
              >
                <Icon icon={X} size={20} />
              </button>
            </Dialog.Close>
          </div>

          <ul className="mt-16 flex flex-col gap-8">
            {SHORTCUTS.map((s) => (
              <li key={`${s.keys}-${s.action}`} className="flex items-start gap-12 border-t border-line-2 pt-8 first:border-t-0 first:pt-0">
                <span className="flex shrink-0 flex-wrap items-center gap-4 pt-2" style={{ minWidth: 'var(--size-thumb)' }}>
                  {s.keys.split(' ').map((tok, j) =>
                    tok === '+' || tok === '/' ? (
                      <span key={j} className="text-meta text-fg-3">
                        {tok}
                      </span>
                    ) : (
                      <kbd
                        key={j}
                        className="rounded-control bg-hover px-6 py-2 font-mono text-meta text-fg-2"
                      >
                        {tok}
                      </kbd>
                    ),
                  )}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="text-body text-fg-1">{s.action}</p>
                  <p className="text-meta text-fg-3">{s.scope}</p>
                </div>
              </li>
            ))}
          </ul>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
