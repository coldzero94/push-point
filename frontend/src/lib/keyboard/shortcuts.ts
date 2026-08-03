// The keyboard contract (11 §1.2) as data — the single source both the global
// handler and the `?` overlay read, so the overlay renders the table 1:1 (10
// §7.4: "`?` 오버레이는 11 §1.2 표를 그대로 렌더한다"). Do NOT bind keys that are
// not in this table.
//
// The P0 subset that is actually wired today (§9 P0-7): S, /, J/K, Enter, O, E,
// N, R, Backspace, Cmd/Ctrl+Enter, Esc, ?. Cmd/Ctrl+K (command palette, P1) and
// Cmd/Ctrl+V (instant save, P2) are listed in the table for parity but are not
// bound yet — the overlay still shows them so the contract stays 1:1.

export type Shortcut = {
  /** key column, space-separated so `+` (combo) and `/` (alternatives) render as
   *  separators between chips (e.g. 'Cmd/Ctrl + K', 'J / ↓') */
  keys: string
  /** i18n key for the scope column (11 §1.2) — resolved at render, not here, so
   *  a language switch repaints the overlay */
  scope: string
  /** i18n key for the action column (11 §1.2) */
  action: string
}

export const SHORTCUTS: readonly Shortcut[] = [
  { keys: 'Cmd/Ctrl + K', scope: 'shortcuts.scopeGlobal', action: 'shortcuts.actionPalette' },
  { keys: '/', scope: 'shortcuts.scopeListSearch', action: 'shortcuts.actionFocusSearch' },
  { keys: 'S', scope: 'shortcuts.scopeGlobal', action: 'shortcuts.actionOpenComposer' },
  { keys: 'Cmd/Ctrl + V', scope: 'shortcuts.scopeOutsideInput', action: 'shortcuts.actionPasteSave' },
  { keys: 'J / ↓', scope: 'shortcuts.scopeListSearch', action: 'shortcuts.actionNextRow' },
  { keys: 'K / ↑', scope: 'shortcuts.scopeListSearch', action: 'shortcuts.actionPrevRow' },
  { keys: 'Enter', scope: 'shortcuts.scopeListSearch', action: 'shortcuts.actionOpenInspector' },
  { keys: 'O', scope: 'shortcuts.scopeRowInspector', action: 'shortcuts.actionOpenOriginal' },
  { keys: 'E', scope: 'shortcuts.scopeRowInspector', action: 'shortcuts.actionEditTags' },
  { keys: 'N', scope: 'shortcuts.scopeRowInspector', action: 'shortcuts.actionEditNote' },
  { keys: 'R', scope: 'shortcuts.scopeRowInspector', action: 'shortcuts.actionRetry' },
  { keys: 'Backspace / Delete', scope: 'shortcuts.scopeRowInspector', action: 'shortcuts.actionDelete' },
  { keys: 'Cmd/Ctrl + Enter', scope: 'shortcuts.scopeSaveForm', action: 'shortcuts.actionSubmitForm' },
  { keys: 'Esc', scope: 'shortcuts.scopeGlobal', action: 'shortcuts.actionEscape' },
  { keys: '?', scope: 'shortcuts.scopeGlobal', action: 'shortcuts.actionToggleOverlay' },
]

/** True when focus is in a text input surface — single-char shortcuts are inert
 *  there (11 §1.2). Esc / Cmd-Ctrl+Enter are handled by the forms themselves. */
export function isEditableTarget(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false
  return t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable
}
