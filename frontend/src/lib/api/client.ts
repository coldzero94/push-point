import createClient from 'openapi-fetch'
import type { paths } from './schema'
import { getApiKey } from '../auth'

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
})

// Contract error body: components["schemas"]["Error"] = { error: { code, message } }.
export type ApiError = { error: { code: string; message: string } }

// Narrow an openapi-fetch error (or thrown value) to a display message.
export function errorMessage(err: unknown, fallback = '요청에 실패했습니다.'): string {
  if (err && typeof err === 'object' && 'error' in err) {
    const e = (err as ApiError).error
    if (e && typeof e.message === 'string') return e.message
  }
  if (err instanceof Error) return err.message
  return fallback
}
