import { useEffect } from 'react'
import { Link, Outlet, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { Button, Icon } from '../components/ui'
import { ThemeToggle } from '../components/ThemeToggle'
import { ApiKeyBanner } from '../components/ApiKeyBanner'
import { OfflineBar } from '../components/OfflineBar'
import { KeyboardShortcuts } from '../components/KeyboardShortcuts'
import { LinkInspector } from './LinkInspector'
import { reportNetworkError, reportNetworkOk } from '../lib/offline'

// Nav is the 4 screen links only (§1.1). "저장" is the accent primary button on
// the right, not a nav link; the detail inspector is an overlay, not a route tab.
const NAV = [
  { to: '/', label: '목록' },
  { to: '/search', label: '검색' },
  { to: '/tags', label: '태그' },
  { to: '/settings', label: '설정' },
] as const

export function RootLayout() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  // Offline escalation (§1.6): a network-level fetch rejection surfaces in the
  // query cache as a thrown TypeError ("Failed to fetch"). HTTP 4xx/5xx throw the
  // contract Error object instead, so they do not false-trigger. Any success
  // clears the escalation. Coming back online revalidates the active queries
  // (§1.6 — current screen only; no toast, the bar vanishing is the signal).
  useEffect(() => {
    const unsub = queryClient.getQueryCache().subscribe((event) => {
      if (event.type !== 'updated') return
      if (event.action.type === 'error') {
        if (event.action.error instanceof TypeError) reportNetworkError()
      } else if (event.action.type === 'success') {
        reportNetworkOk()
      }
    })
    const onOnline = () => queryClient.invalidateQueries()
    window.addEventListener('online', onOnline)
    return () => {
      unsub()
      window.removeEventListener('online', onOnline)
    }
  }, [queryClient])

  return (
    <>
      {/* skip link — first tab stop, revealed only on keyboard focus (§7.2) */}
      <a
        href="#main-content"
        className="sr-only rounded-control bg-elevated px-12 py-8 text-body text-fg-1 shadow-panel focus-visible:not-sr-only focus-visible:absolute focus-visible:left-16 focus-visible:top-8 focus-visible:z-(--z-header)"
      >
        본문으로 건너뛰기
      </a>

      <div className="min-h-full">
        {/* Header + top banners pinned together (§1.1 / §1.4 / §1.6). */}
        <div className="sticky top-0 z-(--z-header)">
          <header className="glass border-b border-line-1">
            <div className="mx-auto flex h-(--size-header) max-w-(--w-page) items-center gap-16 px-(--gutter)">
              {/* wordmark: the separator dot is the ONLY accent glyph in the
                  chrome — the brand solid stays reserved for the 4 places in
                  §2.1.4. Hidden < 560. */}
              <Link to="/" className="hidden text-title text-fg-1 sm:block">
                Push<span className="text-accent">·</span>Point
              </Link>

              <nav className="flex items-center gap-2 text-body">
                {NAV.map((n) => (
                  <Link
                    key={n.to}
                    to={n.to}
                    activeOptions={{ exact: n.to === '/' }}
                    activeProps={{ 'aria-current': 'page' }}
                    className="rounded-control px-12 py-6 text-fg-2 transition-colors duration-(--dur-out) ease-ui hover:bg-hover data-[status=active]:bg-accent-tint data-[status=active]:font-medium data-[status=active]:text-accent"
                  >
                    {n.label}
                  </Link>
                ))}
              </nav>

              <div className="ml-auto flex items-center gap-8">
                {/* "저장" opens the composer over the list (§0 / §2). Icon (+) < 560. */}
                <Button
                  variant="primary"
                  aria-label="링크 저장"
                  onClick={() => navigate({ to: '/save' })}
                >
                  <Icon icon={Plus} size={16} className="sm:hidden" />
                  <span className="hidden sm:inline">저장</span>
                </Button>
                <ThemeToggle />
              </div>
            </div>
          </header>

          <ApiKeyBanner />
          <OfflineBar />
        </div>

        <main
          id="main-content"
          className="mx-auto max-w-(--w-page) px-(--gutter) pb-80 pt-24"
        >
          <Outlet />
        </main>
      </div>

      {/* The ?link inspector overlay, mounted once above the routed screens: the
          list/save set ?link on row-open and this reads it (§11 0 / §6). The
          /links/$id deep-link route renders its own inspector via the Outlet. */}
      <LinkInspector />

      {/* Global keyboard contract (§1.2) + the `?` overlay. */}
      <KeyboardShortcuts />
    </>
  )
}
