import type { LinkStatus } from '../lib/api/types'

const STYLES: Record<LinkStatus, string> = {
  pending: 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300',
  scraping: 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300',
  tagging: 'bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300',
  done: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300',
  failed: 'bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300',
}

const LABELS: Record<LinkStatus, string> = {
  pending: '대기',
  scraping: '수집 중',
  tagging: '태깅 중',
  done: '완료',
  failed: '실패',
}

export function StatusBadge({ status }: { status: LinkStatus }) {
  return (
    <span
      className={`inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium ${STYLES[status]}`}
    >
      {LABELS[status]}
    </span>
  )
}
