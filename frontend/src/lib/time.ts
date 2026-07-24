// Relative-time formatting for the machine `created_at` field (R2 — mono display).
//
// The contract's time fields are integer unix epoch SECONDS (api.md), never
// date-time strings. The list shows a relative label (방금/N분 전/N시간 전/어제/
// M월 D일) and carries the absolute time in the row's `title`/`dateTime`
// attribute (§11 3(3)) — so the low-contrast `--fg-3` relative label always has
// the full value duplicated elsewhere (§10 2.1.3 fg-3 exception).

const MINUTE = 60
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

/** epoch seconds → Korean relative label. */
export function formatRelativeTime(epochSeconds: number, now: Date = new Date()): string {
  const then = new Date(epochSeconds * 1000)
  const diff = Math.floor((now.getTime() - then.getTime()) / 1000)

  if (diff < MINUTE) return '방금'
  if (diff < HOUR) return `${Math.floor(diff / MINUTE)}분 전`
  if (diff < DAY) return `${Math.floor(diff / HOUR)}시간 전`

  // Calendar-day comparison for "어제" (not a flat 48h window).
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const startOfThen = new Date(then.getFullYear(), then.getMonth(), then.getDate()).getTime()
  const dayGap = Math.round((startOfToday - startOfThen) / (DAY * 1000))
  if (dayGap === 1) return '어제'

  return `${then.getMonth() + 1}월 ${then.getDate()}일`
}

/** epoch seconds → full absolute label for the `title`/`dateTime` attribute. */
export function formatAbsoluteTime(epochSeconds: number): string {
  const d = new Date(epochSeconds * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}년 ${d.getMonth() + 1}월 ${d.getDate()}일 ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** ISO 8601 for the <time dateTime> machine attribute. */
export function toIso(epochSeconds: number): string {
  return new Date(epochSeconds * 1000).toISOString()
}
