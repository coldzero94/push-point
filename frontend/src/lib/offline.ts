// Network availability store (§1.6). Mirrors the useSyncExternalStore shape of
// auth.ts so any write control can subscribe with the useOffline() hook and
// disable itself (aria-disabled) when the server is unreachable — reads are
// never blocked.
//
// Two signals feed it:
//  1. navigator.onLine + the window 'online'/'offline' events (the OS baseline).
//  2. reportNetworkError()/reportNetworkOk() — the shell escalates a network-
//     level fetch rejection (TypeError, no response) to offline even while the OS
//     still reports online (captive / half-broken network), and clears it on the
//     next successful request. HTTP 4xx/5xx do NOT escalate: the query fns throw
//     the contract Error object, not a TypeError.

import { useSyncExternalStore } from 'react'

let networkFailed = false
const listeners = new Set<() => void>()

function emit(): void {
  for (const l of listeners) l()
}

export function isOffline(): boolean {
  const osOffline = typeof navigator !== 'undefined' && navigator.onLine === false
  return osOffline || networkFailed
}

// Called by the shell when a request rejects at the network level (no response).
export function reportNetworkError(): void {
  if (!networkFailed) {
    networkFailed = true
    emit()
  }
}

// Called by the shell when any request succeeds — clears an escalated failure.
export function reportNetworkOk(): void {
  if (networkFailed) {
    networkFailed = false
    emit()
  }
}

export function subscribeOffline(onChange: () => void): () => void {
  const onOnline = () => {
    // A real OS 'online' event supersedes an escalated fetch failure.
    networkFailed = false
    onChange()
  }
  const onOffline = () => onChange()
  listeners.add(onChange)
  window.addEventListener('online', onOnline)
  window.addEventListener('offline', onOffline)
  return () => {
    listeners.delete(onChange)
    window.removeEventListener('online', onOnline)
    window.removeEventListener('offline', onOffline)
  }
}

// Hook for write controls across every screen: `const offline = useOffline()`.
export function useOffline(): boolean {
  return useSyncExternalStore(subscribeOffline, isOffline, () => false)
}
