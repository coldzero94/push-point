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
  link: z.coerce.number().int().positive().optional(),
})

const searchSearchSchema = z.object({
  q: z.string().optional().default(''),
  tag: z.string().optional(),
  // ?link opens the inspector overlay over search results (same overlay as list).
  link: z.coerce.number().int().positive().optional(),
})

// Bookmarklet prefill (§2(4)) — fills the composer, never auto-submits. ?link
// opens the inspector over the save screen's list (the row list below the
// composer is the shared LinkRow renderer).
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
