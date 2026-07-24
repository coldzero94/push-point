// Button — §4.1.
//
// 4 variants (primary = accent solid / secondary = line-control outline /
// ghost / danger = achromatic-bordered danger text). 2 sizes (sm 24px pointer-
// only, md 32px). There is no `danger-solid`: danger is never a fill (§2.1.4-b).
// Focus ring comes from the global :focus-visible rule; motion is sealed by the
// global reduced-motion rule.

import { forwardRef } from 'react'
import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { Loader2 } from 'lucide-react'
import { Icon } from './Icon'
import { cn } from './cn'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger'
type Size = 'sm' | 'md'

const VARIANT: Record<Variant, string> = {
  // hover: enter 0ms / leave --dur-out (asymmetric, §4.1)
  primary: 'bg-accent text-on-accent hover:bg-accent-hover',
  secondary: 'bg-surface text-fg-1 border border-line-control hover:bg-hover',
  ghost: 'text-fg-2 hover:bg-hover',
  danger: 'text-danger border border-danger hover:bg-hover',
}

const SIZE: Record<Size, string> = {
  sm: 'h-24 px-8 gap-4', // pointer-env only (§4.1 / §7.5)
  md: 'h-32 px-12 gap-6',
}

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant
  size?: Size
  /** keeps the label + inserts a 16px spinner on the left, width fixed, aria-busy */
  loading?: boolean
  children?: ReactNode
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', loading = false, disabled, className, children, ...rest },
  ref,
) {
  const isDisabled = disabled || loading
  return (
    <button
      ref={ref}
      // active: no extra variant, no transform (§4.1)
      disabled={isDisabled}
      aria-disabled={isDisabled || undefined}
      aria-busy={loading || undefined}
      className={cn(
        'inline-flex select-none items-center justify-center rounded-control text-label',
        'transition-colors duration-(--dur-out) ease-ui',
        'disabled:pointer-events-none disabled:opacity-45',
        VARIANT[variant],
        SIZE[size],
        className,
      )}
      {...rest}
    >
      {loading ? <Icon icon={Loader2} size={16} className="animate-spin" /> : null}
      {children}
    </button>
  )
})
