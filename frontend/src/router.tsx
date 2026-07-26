import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import { z } from 'zod'
import { RootLayout } from './routes/RootLayout'
import { ListScreen } from './routes/ListScreen'
import { SaveScreen } from './routes/SaveScreen'
import { SearchScreen } from './routes/SearchScreen'
import { TagsScreen } from './routes/TagsScreen'
import { LinkDetailScreen } from './routes/LinkDetailScreen'
import { LinkEditScreen } from './routes/LinkEditScreen'
import { SettingsScreen } from './routes/SettingsScreen'

const rootRoute = createRootRoute({ component: RootLayout })

// Typed URL search params — the list filters (?tag, ?status) are URL state, and
// ?link opens the inspector overlay over the list (contract with the inspector
// owner — §11 0). ?link is a positive integer link id.
const listSearchSchema = z.object({
  tag: z.string().optional(),
  status: z.enum(['pending', 'scraping', 'tagging', 'done', 'failed']).optional(),
  // One-way flag, not a tri-state: a "already read" filter has no use. Absent
  // means everything, so the URL stays clean in the common case.
  unopened: z.coerce.boolean().optional(),
  link: z.coerce.number().int().positive().optional(),
})

const searchSearchSchema = z.object({
  q: z.string().optional().default(''),
  tag: z.string().optional(),
  // Period preset stored as a key, not raw from/to (§11 4(3)): a key stays valid
  // as "now" moves and survives sharing, and the screen expands it to the
  // contract's from/to at request time. Absent = 전체.
  period: z.enum(['7d', '30d', 'year']).optional(),
  // ?link opens the inspector overlay over search results (same overlay as list).
  link: z.coerce.number().int().positive().optional(),
})

// Bookmarklet prefill (§2(4)) — fills the composer, never auto-submits. ?link
// opens the inspector over the save screen's board (the board below the composer
// is the shared LinkCard renderer).
const saveSearchSchema = z.object({
  url: z.string().optional(),
  note: z.string().optional(),
  link: z.coerce.number().int().positive().optional(),
})

const listRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: ListScreen,
  validateSearch: listSearchSchema,
})

const saveRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/save',
  component: SaveScreen,
  validateSearch: saveSearchSchema,
})

const searchRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/search',
  component: SearchScreen,
  validateSearch: searchSearchSchema,
})

const tagsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/tags',
  component: TagsScreen,
})

const linkDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/links/$id',
  component: LinkDetailScreen,
})

const linkEditRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/links/$id/edit',
  component: LinkEditScreen,
})

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsScreen,
})

const routeTree = rootRoute.addChildren([
  listRoute,
  saveRoute,
  searchRoute,
  tagsRoute,
  linkDetailRoute,
  linkEditRoute,
  settingsRoute,
])

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
