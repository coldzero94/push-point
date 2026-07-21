import { Link, Outlet } from '@tanstack/react-router'
import { ThemeToggle } from '../components/ThemeToggle'
import { ApiKeyBanner } from '../components/ApiKeyBanner'

const NAV = [
  { to: '/', label: '목록' },
  { to: '/save', label: '저장' },
  { to: '/search', label: '검색' },
  { to: '/tags', label: '태그' },
  { to: '/settings', label: '설정' },
] as const

export function RootLayout() {
  return (
    <div className="min-h-full">
      <header className="sticky top-0 z-10 border-b border-neutral-200 bg-white/80 backdrop-blur dark:border-neutral-800 dark:bg-neutral-950/80">
        <div className="mx-auto flex max-w-3xl items-center gap-1 px-4 py-2">
          <Link to="/" className="mr-2 font-semibold">
            Push-Point
          </Link>
          <nav className="flex items-center gap-1 text-sm">
            {NAV.map((n) => (
              <Link
                key={n.to}
                to={n.to}
                activeOptions={{ exact: n.to === '/' }}
                className="rounded-md px-2 py-1 text-neutral-600 hover:bg-neutral-100 data-[status=active]:font-medium data-[status=active]:text-neutral-900 dark:text-neutral-300 dark:hover:bg-neutral-800 dark:data-[status=active]:text-neutral-50"
              >
                {n.label}
              </Link>
            ))}
          </nav>
          <div className="ml-auto">
            <ThemeToggle />
          </div>
        </div>
      </header>

      <ApiKeyBanner />

      <main className="mx-auto max-w-3xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
