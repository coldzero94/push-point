import { useState } from 'react'
import { Moon, Sun } from 'lucide-react'
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
      aria-label={dark ? '라이트 모드로 전환' : '다크 모드로 전환'}
      className="rounded-md p-2 text-neutral-600 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
    >
      {dark ? <Sun size={18} aria-hidden /> : <Moon size={18} aria-hidden />}
    </button>
  )
}
