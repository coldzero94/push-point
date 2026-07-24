// API key storage. Single-user app: the static PUSHPOINT_API_KEY is entered in
// Settings and kept in localStorage, then injected as a Bearer token by the
// openapi-fetch middleware (iOS Keychain parity — no server-side auth relaxation).
//
// getApiKey/subscribeApiKey together form a useSyncExternalStore-compatible
// store so UI that depends on the key (the missing-key banner) re-renders on
// save instead of showing a stale snapshot until the next navigation.

const API_KEY_STORAGE = 'pushpoint.apiKey'

// localStorage's own `storage` event fires only in *other* tabs, so writes from
// this tab are announced with this custom event.
const API_KEY_EVENT = 'pushpoint:apikey'

// A real 401 observed on an authenticated response (§1.4): the key is present
// but the server rejected it (wrong/revoked). The missing-key banner must ALSO
// show in this case — otherwise a stored-but-invalid key leaves the app silently
// unauthorized. Session-scoped module state (not localStorage): it is reset the
// moment a new key is saved, not persisted. Announced on its own event so the
// banner (useSyncExternalStore) re-renders when it flips.
const AUTH_FAILED_EVENT = 'pushpoint:authfailed'
let authFailed = false

export function getApiKey(): string | null {
  try {
    return localStorage.getItem(API_KEY_STORAGE)
  } catch {
    return null
  }
}

export function setApiKey(key: string): void {
  try {
    const trimmed = key.trim()
    if (trimmed) localStorage.setItem(API_KEY_STORAGE, trimmed)
    else localStorage.removeItem(API_KEY_STORAGE)
  } catch {
    // ignore (private mode / storage disabled)
  }
  // A new key supersedes any prior 401 — clear the flag so the banner drops and
  // the invalidated queries get a clean retry (§1.4 / §8(4)).
  setAuthFailed(false)
  window.dispatchEvent(new Event(API_KEY_EVENT))
}

export function hasApiKey(): boolean {
  return !!getApiKey()
}

// Observed-401 store. markAuthFailed is called by the client onResponse
// middleware; the banner reads isAuthFailed via useSyncExternalStore.
export function setAuthFailed(failed: boolean): void {
  if (authFailed === failed) return
  authFailed = failed
  window.dispatchEvent(new Event(AUTH_FAILED_EVENT))
}

export function isAuthFailed(): boolean {
  return authFailed
}

export function subscribeAuthFailed(onChange: () => void): () => void {
  window.addEventListener(AUTH_FAILED_EVENT, onChange)
  return () => window.removeEventListener(AUTH_FAILED_EVENT, onChange)
}

// useSyncExternalStore subscribe. Both sources matter: the custom event for this
// tab's saves, `storage` for a save made in another tab. Snapshots are strings /
// booleans read straight from localStorage, so they stay referentially stable.
export function subscribeApiKey(onChange: () => void): () => void {
  const onStorage = (e: StorageEvent) => {
    // key === null means the whole store was cleared.
    if (e.key === null || e.key === API_KEY_STORAGE) onChange()
  }
  window.addEventListener(API_KEY_EVENT, onChange)
  window.addEventListener('storage', onStorage)
  return () => {
    window.removeEventListener(API_KEY_EVENT, onChange)
    window.removeEventListener('storage', onStorage)
  }
}
