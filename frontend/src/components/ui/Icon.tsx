// Icon — the single lucide adapter (§1.3: strokeWidth 1.5, sizes 16 · 20 only).
//
// Every lucide glyph in the app renders through this wrapper so the two design
// invariants — stroke weight 1.5 and the two-size scale — are enforced in ONE
// place instead of being repeated (and forgotten) at each call site. `size` is
// typed to the union so a stray 18/24 is a compile error, and `strokeWidth` is
// applied last so it can never be overridden. Icons are decorative next to their
// labels, so `aria-hidden` defaults to true (overridable via rest).

import type { LucideIcon, LucideProps } from 'lucide-react'

export type IconProps = Omit<LucideProps, 'size' | 'strokeWidth'> & {
  icon: LucideIcon
  /** the only two icon sizes in the system (§1.3) */
  size?: 16 | 20
}

export function Icon({ icon: Glyph, size = 16, ...rest }: IconProps) {
  return <Glyph size={size} aria-hidden {...rest} strokeWidth={1.5} />
}
