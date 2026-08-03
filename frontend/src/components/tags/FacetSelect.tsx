// Facet select (11 §5(4)) — the 4-choice classification control, the ONLY place
// a user sets a tag's color-by-facet, and they set it by MEANING, not by picking
// a color (there is no color picker — color is derived from facet, §10 5.2). Each
// option carries a mini swatch of that facet's ink so name and color meet on one
// line; a helper line under the trigger states the判정 criteria.

import { Check, ChevronDown } from 'lucide-react'
import * as Select from '@radix-ui/react-select'
import { Icon } from '../ui'
import { t } from '../../lib/i18n'
import { facetLabel, TAG_FACETS } from '../../lib/tags/facet'
import type { TagFacet } from '../../lib/api/types'

// facet → ink swatch background. Literal classes for Tailwind's scanner. neutral
// has no token of its own (§5.2) — it borrows fg-2, the "no color here" signal.
const SWATCH: Record<TagFacet, string> = {
  craft: 'bg-tag-craft-ink',
  media: 'bg-tag-media-ink',
  life: 'bg-tag-life-ink',
  neutral: 'bg-fg-2',
}

export function FacetSelect({
  value,
  onChange,
  disabled,
}: {
  value: TagFacet
  onChange: (facet: TagFacet) => void
  disabled?: boolean
}) {
  return (
    <Select.Root value={value} onValueChange={(v) => onChange(v as TagFacet)} disabled={disabled}>
      <Select.Trigger
        aria-label={t('tags.facetSelectLabel')}
        className="inline-flex h-32 min-w-[7.5rem] items-center gap-8 rounded-control border border-line-control bg-surface px-12 text-label text-fg-1 hover:bg-hover data-[state=open]:bg-hover disabled:pointer-events-none disabled:opacity-45"
      >
        <span className={`size-8 shrink-0 rounded-full ${SWATCH[value]}`} aria-hidden />
        <Select.Value />
        <Select.Icon className="ml-auto">
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
            {TAG_FACETS.map((f) => (
              <Select.Item
                key={f}
                value={f}
                className="relative flex h-32 cursor-default select-none items-center gap-8 rounded-control pl-8 pr-24 text-label text-fg-1 outline-none data-[highlighted]:bg-hover"
              >
                <span className={`size-8 shrink-0 rounded-full ${SWATCH[f]}`} aria-hidden />
                <Select.ItemText>{facetLabel(f)}</Select.ItemText>
                <Select.ItemIndicator className="absolute right-8 inline-flex items-center">
                  <Icon icon={Check} size={16} className="text-accent" />
                </Select.ItemIndicator>
              </Select.Item>
            ))}
          </Select.Viewport>
        </Select.Content>
      </Select.Portal>
    </Select.Root>
  )
}
