import type { components } from './schema'

// Convenience aliases over the generated contract types. Do not hand-edit
// schema.d.ts — regenerate with `just web-gen` when api/openapi.yaml changes.
export type Link = components['schemas']['Link']
export type LinkDetail = components['schemas']['LinkDetail']
export type LinkTag = components['schemas']['LinkTag']
export type LinkPage = components['schemas']['LinkPage']
export type LinkInput = components['schemas']['LinkInput']
export type LinkUpdateInput = components['schemas']['LinkUpdateInput']
export type LinkStatus = components['schemas']['LinkStatus']
export type ContentType = components['schemas']['ContentType']
export type SearchResult = components['schemas']['SearchResult']
export type SearchPage = components['schemas']['SearchPage']
export type Tag = components['schemas']['Tag']
export type Stats = components['schemas']['Stats']

export const LINK_STATUSES: readonly LinkStatus[] = [
  'pending',
  'scraping',
  'tagging',
  'done',
  'failed',
]

// Empty-cell guard (iOS parity): server may return an empty title when og/title
// are absent — fall back to domain, then url.
export function linkDisplayTitle(link: Pick<Link, 'title' | 'domain' | 'url'>): string {
  return link.title.trim() || link.domain.trim() || link.url
}
