// Search period presets (11 §4(3)) — a preset key ⇄ a created_at lower bound.
//
// The URL stores the KEY (`?period=7d`), not raw epochs: a key stays meaningful
// as "now" advances and survives being shared, while a baked `from=...` would
// drift. The screen expands the key to the contract's `from` (unix seconds) at
// request time. Every preset is open-ended (no `to`) — "recent" means "since X,
// up to now".

import { t } from './i18n'

export type PeriodKey = '7d' | '30d' | 'year'

// Getters, not values. The object is built once at import time but the language
// can change at runtime, so each label has to be looked up when it is read
// rather than when the module loads. Property access keeps every call site
// (`PERIOD_LABEL['7d']`) exactly as it was.
export const PERIOD_LABEL: Record<PeriodKey | 'all', string> = {
  get all() {
    return t('common.all')
  },
  get '7d'() {
    return t('search.period7d')
  },
  get '30d'() {
    return t('common.last30Days')
  },
  get year() {
    return t('search.periodYear')
  },
}

/** Preset key → `from` in unix SECONDS (contract unit), or undefined for 전체. */
export function periodFrom(key: PeriodKey | undefined, now: Date = new Date()): number | undefined {
  if (!key) return undefined
  if (key === 'year') {
    return Math.floor(new Date(now.getFullYear(), 0, 1).getTime() / 1000)
  }
  const days = key === '7d' ? 7 : 30
  return Math.floor((now.getTime() - days * 24 * 60 * 60 * 1000) / 1000)
}
