// StatusFilter — the list-only status dropdown (§11 3(4), §1.8 P0). Built on
// @radix-ui/react-select for the accessible listbox pattern; styled with tokens
// only. Single value: 전체 (= no ?status) or one of the 5 LinkStatus values.
// Search has NO status param, so this control lives only on the list (§11 4(3)).

import { Check, ChevronDown } from 'lucide-react'
import * as Select from '@radix-ui/react-select'
import { Icon } from './ui'
import type { LinkStatus } from '../lib/api/types'
import { LINK_STATUSES } from '../lib/api/types'

const ALL = 'all' // radix reserves "" — use a sentinel for 전체

const STATUS_LABEL: Record<LinkStatus, string> = {
  pending: '대기',
  scraping: '수집 중',
  tagging: '태깅 중',
  done: '완료',
  failed: '실패',
}

export type StatusFilterProps = {
  value?: LinkStatus
  onChange: (status?: LinkStatus) => void
}

export function StatusFilter({ value, onChange }: StatusFilterProps) {
  return (
    <Select.Root
      value={value ?? ALL}
      onValueChange={(v) => onChange(v === ALL ? undefined : (v as LinkStatus))}
    >
      <Select.Trigger
        aria-label="상태 필터"
        className="inline-flex h-32 items-center gap-6 rounded-control border border-line-control bg-surface px-12 text-label text-fg-1 hover:bg-hover data-[state=open]:bg-hover"
      >
        <span className="text-fg-3">상태</span>
        <Select.Value />
        <Select.Icon>
          <Icon icon={ChevronDown} size={16} className="text-fg-2" />
        </Select.Icon>
      </Select.Trigger>

      <Select.Portal>
        <Select.Content
          position="popper"
          sideOffset={4}
          className="z-(--z-popover) min-w-(--radix-select-trigger-width) overflow-hidden rounded-panel bg-elevated p-4 shadow-panel"
        >
          <Select.Viewport>
            <StatusItem value={ALL} label="전체" />
            {LINK_STATUSES.map((s) => (
              <StatusItem key={s} value={s} label={STATUS_LABEL[s]} />
            ))}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}

function StatusItem({ value, label }: { value: string; label: string }) {
  return (
    <Select.Item
      value={value}
      className="relative flex h-32 cursor-default select-none items-center rounded-control pl-8 pr-24 text-label text-fg-1 outline-none data-[highlighted]:bg-hover"
    >
      <Select.ItemText>{label}</Select.ItemText>
      <Select.ItemIndicator className="absolute right-8 inline-flex items-center">
        <Icon icon={Check} size={16} className="text-accent" />
      </Select.ItemIndicator>
    </Select.Item>
  )
}
