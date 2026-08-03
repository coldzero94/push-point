// Epoch-seconds formatters for machine fields (R2 — these render in `--font-mono`
// at the call site). The contract stores every time as integer unix epoch
// seconds (api.md), never an ISO string, so the input is always a number.
//
// Absolute values only: the inspector meta section shows `2026-07-21 14:03` /
// `2026-06-30`, not relative time (11 §6(2)). The list's relative "방금/3시간 전"
// display is a separate concern owned by the row renderer.

import { t } from './i18n'

function pad2(n: number): string {
  return n < 10 ? `0${n}` : String(n)
}

/** `YYYY-MM-DD HH:MM` in the viewer's local timezone (저장 시각). */
export function formatDateTime(epochSeconds: number): string {
  const d = new Date(epochSeconds * 1000)
  return (
    `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ` +
    `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
  )
}

/** `YYYY-MM-DD` in local time (발행일 — no clock component). */
export function formatDay(epochSeconds: number): string {
  const d = new Date(epochSeconds * 1000)
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`
}

/** `12분 04초` / `1시간 05분` from a duration in seconds. */
export function formatDuration(sec: number): string {
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  return h > 0 ? t('time.durationHm', { h, m: pad2(m) }) : t('time.durationMs', { m, s: pad2(s) })
}

// Comma-grouped so counts read as `2,140`. Fixed 'en-US' grouping keeps the
// separator a comma regardless of the OS locale (tabular-nums at the call site).
const GROUPED = new Intl.NumberFormat('en-US')

/** `2,140` — thousands-separated integer for counts (word_count 등). */
export function formatCount(n: number): string {
  return GROUPED.format(n)
}
