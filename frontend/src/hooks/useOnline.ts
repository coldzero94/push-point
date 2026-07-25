import { useEffect, useState } from 'react'

// navigator.onLine + the window online/offline events — the OS-level baseline.
// The tags screen disables its writes (+추가/편집/삭제) while offline (11 §5(5)).
// This is the coarse signal; the escalating fetch-failure detection (lib/offline)
// drives the app-wide offline bar and is a separate concern.
export function useOnline(): boolean {
  const [online, setOnline] = useState(() => navigator.onLine)
  useEffect(() => {
    const update = () => setOnline(navigator.onLine)
    window.addEventListener('online', update)
    window.addEventListener('offline', update)
    return () => {
      window.removeEventListener('online', update)
      window.removeEventListener('offline', update)
    }
  }, [])
  return online
}
