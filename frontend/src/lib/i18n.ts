// UI strings, in one place, in two languages.
//
// The app was Korean-only. That was defensible while it was a personal tool, but
// the repository is public and the landing page is English, and an English page
// showing a Korean app is incoherent — that is what prompted this.
//
// **Comments stay Korean.** CLAUDE.md draws the line at what the author reads
// (documents, comments) versus what a user reads (screens); only the second moves.
//
// The dictionary lives in `strings.ts` next door, keyed the same way `site/copy.js`
// is, and for the same reason: two hand-maintained sides drift, and this repo has
// been bitten three times by exactly that (the streak rule, the cover pattern, a
// hand-copied iOS golden). `just web-i18n-check` fails on a key that only one
// locale has, on a key used in source but absent from the dictionary, and on a key
// nobody uses.

import { STRINGS } from './strings'

export type Lang = 'ko' | 'en'
export const LANGS: Lang[] = ['ko', 'en']

const STORAGE = 'pushpoint.lang'
const EVENT = 'pushpoint:lang'

function detect(): Lang {
  try {
    const saved = localStorage.getItem(STORAGE)
    if (saved === 'ko' || saved === 'en') return saved
  } catch {
    // private mode — fall through to the browser's preference
  }
  return typeof navigator !== 'undefined' && navigator.language?.startsWith('ko') ? 'ko' : 'en'
}

let current: Lang = detect()

export function getLang(): Lang {
  return current
}

export function setLang(lang: Lang): void {
  if (current === lang) return
  current = lang
  try {
    localStorage.setItem(STORAGE, lang)
  } catch {
    // ignore — the choice still applies for this session
  }
  // DOM이 없는 곳에서도 불린다 — vitest는 node 환경에서 돌고, 공용 픽스처 테스트가
  // 한국어 쪽을 검증하려면 로케일을 고정해야 한다. 없는 전역을 만지면 거기서 죽는다.
  if (typeof document !== 'undefined') document.documentElement.lang = lang
  if (typeof window !== 'undefined') window.dispatchEvent(new Event(EVENT))
}

// useSyncExternalStore subscribe, matching auth.ts: the custom event for this
// tab's own switch, `storage` for a switch made in another tab.
export function subscribeLang(onChange: () => void): () => void {
  if (typeof window === 'undefined') return () => {}
  const onStorage = (e: StorageEvent) => {
    if (e.key === null || e.key === STORAGE) {
      current = detect()
      onChange()
    }
  }
  window.addEventListener(EVENT, onChange)
  window.addEventListener('storage', onStorage)
  return () => {
    window.removeEventListener(EVENT, onChange)
    window.removeEventListener('storage', onStorage)
  }
}

/**
 * Look up a string.
 *
 * `{name}` placeholders are filled from `params`. When `params.count` is present
 * the value may carry two forms separated by `|` — `'1 link|{count} links'` — and
 * the second is used for anything but 1. Korean has no plural agreement, so its
 * side normally has no `|` at all and the same string serves both; that asymmetry
 * is the reason plurals are a property of the *value* and not of the key.
 *
 * A missing key returns the key itself rather than an empty string: a visible
 * `list.empty` on screen is a bug report, and a blank line is not.
 */
export function t(key: string, params?: Record<string, string | number>): string {
  const table = STRINGS[current] ?? STRINGS.ko
  let value = table[key]
  if (value === undefined) {
    if (import.meta.env?.DEV) console.warn('i18n: missing key', key, current)
    return key
  }
  if (value.includes('|')) {
    const [one, other] = value.split('|')
    value = params?.count === 1 ? one : other
  }
  if (!params) return value
  return value.replace(/\{(\w+)\}/g, (whole, name) =>
    params[name] === undefined ? whole : String(params[name]),
  )
}
