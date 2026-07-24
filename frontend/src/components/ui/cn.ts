// Minimal class-name joiner (falsy entries dropped). No dependency — the design
// system's §1.8 dependency list does not include a classnames library.
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ')
}
