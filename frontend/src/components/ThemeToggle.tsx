import { useState } from 'react'
import { Moon, Sun } from 'lucide-react'
import { Icon } from './ui'
import { t } from '../lib/i18n'
import { effectiveDark, toggleTheme } from '../lib/theme'

export function ThemeToggle() {
  const [dark, setDark] = useState(effectiveDark())
  return (
    <button
      type="button"
      onClick={() => {
        toggleTheme()
        setDark(effectiveDark())
      }}
      aria-label={dark ? t('settings.switchToLight') : t('settings.switchToDark')}
      // ghost control, token-only. Header quick toggle; the 3-state segment lives
      // in Settings (§8). hover: enter 0ms / leave --dur-out (§4.1). The 32px box
      // clears the mouse 24×24 minimum; `relative hit-target` extends it to 44×44
      // on touch (§7.5).
      className="relative hit-target inline-flex h-32 w-32 items-center justify-center rounded-control text-fg-2 transition-colors duration-(--dur-out) ease-ui hover:bg-hover"
    >
      {dark ? <Icon icon={Sun} size={20} /> : <Icon icon={Moon} size={20} />}
    </button>
  )
}
