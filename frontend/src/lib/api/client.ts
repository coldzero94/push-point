import createClient from 'openapi-fetch'
import type { paths } from './schema'
import { getApiKey, setAuthFailed } from '../auth'

// Single typed client over the openapi.yaml contract. baseUrl is empty so every
// request is a relative path — dev goes through the Vite proxy, prod hits the
// same-origin Go embed. Mirrors the iOS swift-openapi ClientMiddleware.
export const api = createClient<paths>({ baseUrl: '' })

// Inject the static API key (set in Settings, stored in localStorage) as a
// Bearer token on every request. healthz/thumbs are exempt server-side; sending
// the header there is harmless.
api.use({
  onRequest({ request }) {
    const key = getApiKey()
    if (key) request.headers.set('Authorization', `Bearer ${key}`)
    return request
  },
  // A real 401 on any authenticated endpoint means the stored key is wrong or
  // revoked. Raise the observed-401 flag so the missing-key banner also shows in
  // the "key present but invalid" case (§1.4). healthz/thumbs are auth-exempt and
  // never 401. The flag clears when a new key is saved (auth.setApiKey).
  onResponse({ response }) {
    if (response.status === 401) setAuthFailed(true)
    return response
  },
})

// Contract error body: components["schemas"]["Error"] = { error: { code, message } }.
export type ApiError = { error: { code: string; message: string } }

// True when the thrown openapi-fetch error body carries the given contract
// error.code. The 4 codes are the whole set (§1.4); callers match on them to
// route a failure (e.g. a duplicate-name 400 → inline `invalid_input` warn).
export function isErrorCode(err: unknown, code: string): boolean {
  return (
    !!err &&
    typeof err === 'object' &&
    'error' in err &&
    (err as ApiError).error?.code === code
  )
}

// The contract maps 401 ↔ error.code 'unauthorized' 1:1, so query hooks that
// throw the openapi-fetch error body carry it here. Used to stop TanStack Query
// from retrying an unauthorized request (§1.4: "해당 쿼리 재시도 중단").
export function isUnauthorized(err: unknown): boolean {
  return isErrorCode(err, 'unauthorized')
}

// Narrow an openapi-fetch error (or thrown value) to a display message.
export function errorMessage(err: unknown, fallback = '요청에 실패했습니다.'): string {
  if (err && typeof err === 'object' && 'error' in err) {
    const e = (err as ApiError).error
    if (e && typeof e.message === 'string') return e.message
  }
  if (err instanceof Error) return err.message
  return fallback
}
