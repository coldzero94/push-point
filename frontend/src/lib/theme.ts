// Dark mode: prefers-color-scheme as the default, overridable by a localStorage
// toggle. lib/theme applies the effective mode as a `.dark` class on <html>,
// which the Tailwind `@custom-variant dark` picks up.

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

function systemPrefersDark(): boolean {
  return window.matchMedia(MEDIA).matches
}

export function effectiveDark(): boolean {
  const pref = getThemePref()
  return pref === 'dark' || (pref === 'system' && systemPrefersDark())
}

function apply(): void {
  document.documentElement.classList.toggle('dark', effectiveDark())
}

export function setThemePref(pref: ThemePref): void {
  try {
    if (pref === 'system') localStorage.removeItem(THEME_STORAGE)
    else localStorage.setItem(THEME_STORAGE, pref)
  } catch {
    // ignore
  }
  apply()
}

export function toggleTheme(): void {
  setThemePref(effectiveDark() ? 'light' : 'dark')
}

// Call once on boot. Applies the current mode and keeps following the system
// setting while the user has not chosen an explicit override.
export function initTheme(): void {
  apply()
  window.matchMedia(MEDIA).addEventListener('change', () => {
    if (getThemePref() === 'system') apply()
  })
}
