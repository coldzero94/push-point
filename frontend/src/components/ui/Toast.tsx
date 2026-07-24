// Toast — §4.10 (visual spec) + §1.4 (when to show, error-code mapping is the
// caller's concern).
//
// Bottom-center, fixed. Exactly ONE at a time — a new toast REPLACES the
// previous (no stack, no queue). One sentence + at most one action; no icon, no
// emoji. success/undo are achromatic (success uses NO accent — R1); only warn
// and error carry a 2px left marker, and the sentence always repeats the state
// so color never carries meaning alone. 4000/8000ms are JS timer values (the
// two exceptions to the CSS duration token set, §2.6). role=status for
// success/warn/undo, role=alert for error, inside one aria-live region.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import type { ReactNode } from 'react'
import { cn } from './cn'

export type ToastVariant = 'success' | 'warn' | 'error' | 'undo'

export type ToastAction = { label: string; onClick: () => void }

export type ToastOptions = {
  variant?: ToastVariant
  message: string
  action?: ToastAction
}

type ActiveToast = Required<Pick<ToastOptions, 'variant' | 'message'>> & {
  action?: ToastAction
  id: number
}

type ToastApi = {
  show: (opts: ToastOptions) => void
  dismiss: () => void
}

// null = no auto-dismiss (error: closed manually via its action, or by the next
// toast). 4000 / 8000 are the §2.6 JS-timer exceptions.
const DURATION: Record<ToastVariant, number | null> = {
  success: 4000,
  warn: 4000,
  error: null,
  undo: 8000,
}

const LEAVE_MS = 120 // --dur-out, mirrored in JS for unmount timing

const MARKER: Partial<Record<ToastVariant, string>> = {
  warn: 'bg-warn',
  error: 'bg-danger',
}

const ToastContext = createContext<ToastApi | null>(null)

let counter = 0

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<ActiveToast | null>(null)

  const show = useCallback((opts: ToastOptions) => {
    counter += 1
    setToast({ variant: 'success', ...opts, id: counter })
  }, [])

  const dismiss = useCallback(() => setToast(null), [])

  const api = useMemo<ToastApi>(() => ({ show, dismiss }), [show, dismiss])

  return (
    <ToastContext.Provider value={api}>
      {children}
      <ToastViewport toast={toast} onRemove={dismiss} />
    </ToastContext.Provider>
  )
}

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within a ToastProvider')
  return ctx
}

function ToastViewport({ toast, onRemove }: { toast: ActiveToast | null; onRemove: () => void }) {
  const [shown, setShown] = useState(false)
  const [paused, setPaused] = useState(false)
  const leaveTimer = useRef<number | undefined>(undefined)

  const beginLeave = useCallback(() => {
    setShown(false)
    leaveTimer.current = window.setTimeout(onRemove, LEAVE_MS)
  }, [onRemove])

  // Enter: reset on every new toast id, then flip `shown` on the next frame.
  useEffect(() => {
    clearTimeout(leaveTimer.current)
    setShown(false)
    if (!toast) return
    const r = requestAnimationFrame(() => setShown(true))
    return () => cancelAnimationFrame(r)
    // `show` always creates a fresh object, so depending on `toast` is
    // equivalent to keying on its id (re-enter per new toast, reset on null).
  }, [toast])

  // Auto-dismiss (paused while hovered/focused).
  useEffect(() => {
    if (!toast || paused) return
    const ms = DURATION[toast.variant]
    if (ms == null) return
    const t = window.setTimeout(beginLeave, ms)
    return () => clearTimeout(t)
  }, [toast, paused, beginLeave])

  useEffect(() => () => clearTimeout(leaveTimer.current), [])

  return (
    <div
      className="pointer-events-none fixed inset-x-0 bottom-0 z-(--z-toast) flex justify-center"
      // lift above the mobile home indicator (§4.10)
      style={{ paddingBottom: 'calc(env(safe-area-inset-bottom) + var(--spacing-16))' }}
    >
      {/* one persistent aria-live region per app */}
      <div aria-live="polite" aria-atomic="true">
        {toast ? (
          <div
            role={toast.variant === 'error' ? 'alert' : 'status'}
            onMouseEnter={() => setPaused(true)}
            onMouseLeave={() => setPaused(false)}
            onFocus={() => setPaused(true)}
            onBlur={() => setPaused(false)}
            // max-width 360px (§4.10) — no width token at 360px.
            style={{ maxWidth: '360px' }}
            className={cn(
              'pointer-events-auto relative flex items-center gap-12 overflow-hidden',
              'rounded-panel bg-elevated px-16 py-12 shadow-panel',
              'transition duration-(--dur-2) ease-enter',
              shown ? 'translate-y-0 opacity-100' : 'translate-y-4 opacity-0',
            )}
          >
            {MARKER[toast.variant] ? (
              <span
                aria-hidden
                className={cn('absolute inset-y-0 left-0 w-(--size-rail)', MARKER[toast.variant])}
              />
            ) : null}
            <p className="text-body text-fg-1">{toast.message}</p>
            {toast.action ? (
              <button
                type="button"
                onClick={() => {
                  toast.action?.onClick()
                  beginLeave()
                }}
                className="shrink-0 rounded-control px-8 py-2 text-label text-fg-1 hover:bg-hover"
              >
                {toast.action.label}
              </button>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  )
}
