// Input / Textarea — §4.2.
//
// The border IS the only visual signal of a control boundary (input background
// equals the page surface), so it is `--line-control` (WCAG 1.4.11, 3:1) and
// never a decorative hairline. Focus uses the global :focus-visible outline
// ring — never outline-none + a border-color swap. `invalid` adds a danger
// border + aria-invalid + a 12px danger message.

import { forwardRef, useId } from 'react'
import type { InputHTMLAttributes, TextareaHTMLAttributes } from 'react'
import { Search, X } from 'lucide-react'
import { Icon } from './Icon'
import { cn } from './cn'

const FIELD_BASE =
  'w-full rounded-control bg-surface text-body text-fg-1 placeholder:text-fg-3 ' +
  'border transition-colors duration-(--dur-out) ease-ui ' +
  'hover:bg-hover disabled:pointer-events-none disabled:opacity-45 ' +
  'read-only:bg-hover'

type InputVariant = 'text' | 'url' | 'search'

export type InputProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'size'> & {
  variant?: InputVariant
  invalid?: boolean
  /** rendered below the field as a 12px danger message + wired to aria-describedby */
  errorMessage?: string
  /** search variant only: shows a clear button while the field has a value */
  onClear?: () => void
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { variant = 'text', invalid, errorMessage, onClear, className, id, value, ...rest },
  ref,
) {
  const autoId = useId()
  const fieldId = id ?? autoId
  const errId = `${fieldId}-err`
  const showClear = variant === 'search' && onClear != null && !!value

  const field = (
    <input
      ref={ref}
      id={fieldId}
      value={value}
      aria-invalid={invalid || undefined}
      aria-describedby={invalid && errorMessage ? errId : undefined}
      inputMode={variant === 'url' ? 'url' : undefined}
      spellCheck={variant === 'url' ? false : undefined}
      className={cn(
        FIELD_BASE,
        'h-32 px-12',
        variant === 'url' && 'font-mono',
        variant === 'search' && 'pl-32', // room for the 16px leading icon + gap
        showClear && 'pr-32',
        invalid ? 'border-danger' : 'border-line-control',
        className,
      )}
      {...rest}
    />
  )

  if (variant !== 'search' && !errorMessage) return field

  return (
    <div className="w-full">
      {variant === 'search' ? (
        <div className="relative">
          <Icon
            icon={Search}
            size={16}
            className="pointer-events-none absolute left-12 top-1/2 -translate-y-1/2 text-fg-3"
          />
          {field}
          {showClear ? (
            <button
              type="button"
              onClick={onClear}
              aria-label="검색어 지우기"
              // hit-target (§7.5): the ~20px clear button needs mouse ≥24×24 and
              // touch 44×44. Already `absolute`, so it anchors its own ::before.
              className="hit-target absolute right-8 top-1/2 flex -translate-y-1/2 items-center justify-center rounded-control p-2 text-fg-3 hover:bg-hover"
            >
              <Icon icon={X} size={16} />
            </button>
          ) : null}
        </div>
      ) : (
        field
      )}
      {invalid && errorMessage ? (
        <p id={errId} className="mt-8 text-meta text-danger">
          {errorMessage}
        </p>
      ) : null}
    </div>
  )
})

export type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & {
  invalid?: boolean
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { invalid, className, style, ...rest },
  ref,
) {
  return (
    <textarea
      ref={ref}
      aria-invalid={invalid || undefined}
      // min-height 72px + field-sizing:content (§4.2). 72px has no spacing token.
      style={{ minHeight: '72px', ...style }}
      className={cn(
        FIELD_BASE,
        'px-12 py-8 field-sizing-content',
        invalid ? 'border-danger' : 'border-line-control',
        className,
      )}
      {...rest}
    />
  )
})
