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
/**
 * `dayStated` — **화면이 이미 그 날을 말했는가.** 보드는 시간 척추로 끊으므로 "어제"
 * 구간의 카드가 다시 "어제"라고 적으면 머리글을 한 줄씩 되풀이한다. 검색 결과에는
 * 구간이 없어 그 라벨이 유일한 날짜 정보이므로 거기서는 그대로 둔다.
 *
 * iOS `Shared/RelativeTime.swift`와 **같은 다섯 갈래 + 같은 시각 형식**이어야 한다.
 */
export function formatRelativeTime(
  epochSeconds: number,
  now: Date = new Date(),
  dayStated = false,
): string {
  const then = new Date(epochSeconds * 1000)
  const diff = Math.floor((now.getTime() - then.getTime()) / 1000)

  // Calendar-day comparison (not a flat 48h window).
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const startOfThen = new Date(then.getFullYear(), then.getMonth(), then.getDate()).getTime()
  const dayGap = Math.round((startOfToday - startOfThen) / (DAY * 1000))

  // **머리글이 날을 말했으면 달력을 먼저 본다.** 경과 시간을 먼저 재면 24시간 미만인
  // 어제 항목이 "N시간 전"으로 빠져나가, 같은 "어제" 구간에 "9시간 전"과 "오전 9:05"가
  // 섞인다. iOS에서 실제로 그렇게 나왔고 **테스트는 통과했다** — 픽스처의 어제 케이스가
  // 전부 24시간을 넘겼기 때문이다. 화면을 보고서야 드러났다(2026-07-30).
  if (dayStated && dayGap > 0) {
    return dayGap === 1 ? timeOfDay(then) : monthDay(then)
  }

  if (diff < MINUTE) return '방금'
  if (diff < HOUR) return `${Math.floor(diff / MINUTE)}분 전`
  if (diff < DAY) return `${Math.floor(diff / HOUR)}시간 전`

  if (dayGap === 1) return '어제'

  return `${then.getMonth() + 1}월 ${then.getDate()}일`
}

/** epoch seconds → full absolute label for the `title`/`dateTime` attribute. */
/** "M월 D일". iOS `RelativeTime.monthDay`와 같은 형식이다. */
function monthDay(d: Date): string {
  return `${d.getMonth() + 1}월 ${d.getDate()}일`
}

/** 그 날 안에서의 시각. iOS `RelativeTime.timeOfDay`와 같은 형식이다. */
function timeOfDay(d: Date): string {
  const h = d.getHours()
  const period = h < 12 ? '오전' : '오후'
  const h12 = h % 12 === 0 ? 12 : h % 12
  return `${period} ${h12}:${String(d.getMinutes()).padStart(2, '0')}`
}

export function formatAbsoluteTime(epochSeconds: number): string {
  const d = new Date(epochSeconds * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}년 ${d.getMonth() + 1}월 ${d.getDate()}일 ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** ISO 8601 for the <time dateTime> machine attribute. */
export function toIso(epochSeconds: number): string {
  return new Date(epochSeconds * 1000).toISOString()
}

/**
 * Bucket a link into the board's time spine (§11 3(2)).
 *
 * People remember "the thing I saved Tuesday", not a cursor offset, so the board
 * is broken by time rather than paged. The list is already `created_at DESC`, so
 * walking it and starting a new group whenever `key` changes yields groups in
 * order with no sorting and no repeats.
 *
 * Buckets: 오늘 / 어제 / 이번 주(2–6일 전) / 월 / 연도+월. `key` is stable and
 * `label` is display-only — never key a group by its label (두 해의 "7월"이
 * 같은 그룹으로 합쳐진다).
 */
export function timeGroup(
  epochSeconds: number,
  now: Date = new Date(),
): { key: string; label: string } {
  const then = new Date(epochSeconds * 1000)
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  const startOfThen = new Date(then.getFullYear(), then.getMonth(), then.getDate()).getTime()
  const dayGap = Math.round((startOfToday - startOfThen) / (DAY * 1000))

  if (dayGap <= 0) return { key: 'today', label: '오늘' }
  if (dayGap === 1) return { key: 'yesterday', label: '어제' }
  if (dayGap <= 6) return { key: 'week', label: '이번 주' }

  const year = then.getFullYear()
  const month = then.getMonth() + 1
  return {
    key: `m-${year}-${month}`,
    label: year === now.getFullYear() ? `${month}월` : `${year}년 ${month}월`,
  }
}
