// Chip (tag / filter) — §4.3.
//
// The variant axis is `3 fill levels × 4 facets`, not `4 variants × facet` —
// hue is set by the tag's facet (§2.1.2), fill level encodes state (R1). The
// pure `chipStyle` function (facet.ts / §5.2) returns TOKEN NAMES; this file is
// the single place that maps those names to Tailwind utilities. The maps below
// are written as literals so Tailwind's static scanner emits every class.

import type { ReactNode } from 'react'
import { X } from 'lucide-react'
import { Icon } from './Icon'
import { chipStyle } from '../../lib/tags/facet'
import type { ChipInput, TagFacet } from '../../lib/tags/facet'
import { cn } from './cn'

// token name -> literal utility (scanner-safe; no dynamic `bg-${x}`)
const BG: Record<string, string> = {
  transparent: 'bg-transparent',
  'tag-craft-ink': 'bg-tag-craft-ink',
  'tag-media-ink': 'bg-tag-media-ink',
  'tag-life-ink': 'bg-tag-life-ink',
  'tag-craft-tint': 'bg-tag-craft-tint',
  'tag-media-tint': 'bg-tag-media-tint',
  'tag-life-tint': 'bg-tag-life-tint',
  'fg-2': 'bg-fg-2',
  hover: 'bg-hover',
}

const FG: Record<string, string> = {
  surface: 'text-surface',
  'tag-craft-ink': 'text-tag-craft-ink',
  'tag-media-ink': 'text-tag-media-ink',
  'tag-life-ink': 'text-tag-life-ink',
  'fg-2': 'text-fg-2',
}

// facet -> ink CSS variable, for the selected-chip count color-mix (§4.3).
const FACET_INK_VAR: Record<TagFacet, string> = {
  craft: '--tag-craft-ink',
  media: '--tag-media-ink',
  life: '--tag-life-ink',
  neutral: '--fg-2',
}

export type ChipProps = Partial<ChipInput> & {
  facet: TagFacet
  children: ReactNode
  /** filter chips only: attached count (mono, dimmed) */
  count?: number
  /** editable chips (inspector): renders a trailing × */
  onRemove?: () => void
  /** filter chips: toggles the ?tag filter */
  onClick?: () => void
  disabled?: boolean
  className?: string
  'aria-label'?: string
}

export function Chip({
  facet,
  children,
  count,
  onRemove,
  onClick,
  disabled,
  className,
  selected = false,
  source = null,
  role = 'readonly',
  onSelectedRow = false,
  ...rest
}: ChipProps) {
  const s = chipStyle({ facet, selected, source, role, onSelectedRow })
  const hasBorder = s.border !== 'transparent'
  // control chip hover: fill 1 background -> hover; fill 2 keeps its fill (§4.3)
  const hover = role === 'control' && !selected ? 'hover:bg-hover' : ''

  const base = cn(
    'inline-flex h-24 select-none items-center gap-6 rounded-chip px-8 text-label',
    BG[s.bg],
    FG[s.fg],
    hasBorder && `border ${s.border === 'line-control' ? 'border-line-control' : ''}`,
    hover,
    'transition-colors duration-(--dur-out) ease-ui',
    disabled && 'pointer-events-none opacity-45',
    className,
  )

  const inner = (
    <>
      <span>{children}</span>
      {count != null ? (
        <span
          className={cn('font-mono tabular-nums', !selected && 'text-fg-3')}
          // selected (fill 2): surface mixed toward the facet ink (§4.3)
          style={
            selected
              ? { color: `color-mix(in oklab, var(--bg-surface) 70%, var(${FACET_INK_VAR[facet]}))` }
              : undefined
          }
        >
          {count}
        </span>
      ) : null}
    </>
  )

  // Editable chip (inspector): label + trailing × as a nested button.
  if (onRemove) {
    return (
      <span className={base} {...rest}>
        {inner}
        <button
          type="button"
          onClick={onRemove}
          disabled={disabled}
          aria-label={`${typeof children === 'string' ? children : '태그'} 제거`}
          // hit-target (§7.5): mouse ≥24×24 (the ~16px glyph alone fails 2.5.8),
          // touch ::before to 44×44. `relative` anchors that pseudo-element.
          className="relative hit-target flex items-center justify-center rounded-chip text-current hover:opacity-70"
        >
          <Icon icon={X} size={16} />
        </button>
      </span>
    )
  }

  // Filter chip (control): a real button.
  if (onClick) {
    return (
      <button
        type="button"
        onClick={onClick}
        disabled={disabled}
        aria-pressed={role === 'control' ? selected : undefined}
        className={base}
        {...rest}
      >
        {inner}
      </button>
    )
  }

  // Display chip (readonly): no hover, no interaction.
  return (
    <span className={base} {...rest}>
      {inner}
    </span>
  )
}
