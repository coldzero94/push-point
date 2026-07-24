// 3-state theme (light / dark / system) — §2.1.6 / §3.
//
// Preference (choice) and result (resolved) are kept separate. The 3-state
// preference lives ONLY in localStorage; <html> always carries exactly one
// resolved class (`.light` or `.dark`) — never both, never none. Leaving the
// class off when `system` is chosen would let :root's light values apply under
// OS dark, so the resolved class is always stamped.
//
// The render-blocking script in index.html stamps the resolved class before
// first paint; this module keeps the same rule alive at runtime and follows the
// OS `change` event only while the preference is `system`.

const THEME_STORAGE = 'pushpoint.theme'
const MEDIA = '(prefers-color-scheme: dark)'

export type ThemePref = 'light' | 'dark' | 'system'

function safeGet(): string | null {
  try {
    return localStorage.getItem(THEME_STORAGE)
  } catch {
    return null
  }
}

export function getThemePref(): ThemePref {
  const v = safeGet()
  return v === 'light' || v === 'dark' ? v : 'system'
}

// A single retained MQL object so a `change` listener stays live across the
// session (real-time reaction to the OS setting while preference is `system`).
const mql = window.matchMedia(MEDIA)

export function effectiveDark(): boolean {
  const pref = getThemePref()
  return pref === 'dark' || (pref === 'system' && mql.matches)
}

// Subscribers to the RESOLVED theme. Generated covers (§10 4.5) are painted to a
// canvas from the live `--tag-*` values, so unlike CSS they cannot re-resolve
// themselves when the theme flips — they have to be told. One store here beats a
// MutationObserver per card.
const listeners = new Set<() => void>()

/** useSyncExternalStore subscribe. Snapshot is `effectiveDark`. */
export function subscribeTheme(onChange: () => void): () => void {
  listeners.add(onChange)
  return () => {
    listeners.delete(onChange)
  }
}

// Resolved class is REPLACED (never a both-cleared state): exactly one of
// `.light` / `.dark` is present after every call.
function applyResolvedClass(): void {
  const dark = effectiveDark()
  const root = document.documentElement
  root.classList.toggle('dark', dark)
  root.classList.toggle('light', !dark)
  for (const l of listeners) l()
}

export function setThemePref(pref: ThemePref): void {
  try {
    if (pref === 'system') localStorage.removeItem(THEME_STORAGE)
    else localStorage.setItem(THEME_STORAGE, pref)
  } catch {
    // ignore (private mode / disabled storage)
  }
  applyResolvedClass()
}

export function toggleTheme(): void {
  setThemePref(effectiveDark() ? 'light' : 'dark')
}

// Call once on boot. Reconciles the resolved class (the inline script already
// set it) and keeps following the OS setting while the preference is `system`.
export function initTheme(): void {
  applyResolvedClass()
  mql.addEventListener('change', () => {
    if (getThemePref() === 'system') applyResolvedClass()
  })
}
