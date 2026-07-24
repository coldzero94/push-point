import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { router } from './router'
import { ToastProvider } from './components/ui'
import { isUnauthorized } from './lib/api/client'
import { initTheme } from './lib/theme'
import '../tailwind.css'

initTheme()

// Bootstrap (§3): reflect reduced-motion in real time, and seal transitions for
// the first paints so the SPA does not flash the whole list once on mount.
const reduceMotionMql = matchMedia('(prefers-reduced-motion: reduce)')
const applyReduceMotion = () =>
  document.documentElement.toggleAttribute('data-reduce-motion', reduceMotionMql.matches)
reduceMotionMql.addEventListener('change', applyReduceMotion)
applyReduceMotion()

document.documentElement.setAttribute('data-loading', '')
requestAnimationFrame(() =>
  requestAnimationFrame(() => document.documentElement.removeAttribute('data-loading')),
)

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      // §1.4: a 401 stops retrying that query — the banner is the recovery path,
      // not another round-trip. Everything else keeps the single retry.
      retry: (failureCount, error) => !isUnauthorized(error) && failureCount < 1,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      {/* One app-wide toast host — a single aria-live region (§1.8 / §4.10). */}
      <ToastProvider>
        <RouterProvider router={router} />
      </ToastProvider>
    </QueryClientProvider>
  </StrictMode>,
)
