import { useSyncExternalStore } from 'react'
import { Link } from '@tanstack/react-router'
import { KeyRound } from 'lucide-react'
import { hasApiKey, subscribeApiKey } from '../lib/auth'

// Shown until an API key is configured. Every non-exempt endpoint needs the
// Bearer token, so without a key the app can only reach healthz/thumbs.
// Subscribed to the key store so saving in Settings hides this immediately.
export function ApiKeyBanner() {
  const configured = useSyncExternalStore(subscribeApiKey, hasApiKey)
  if (configured) return null
  return (
    <div className="flex items-center gap-2 border-b border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
      <KeyRound size={16} aria-hidden />
      <span>
        API 키가 설정되지 않았습니다.{' '}
        <Link to="/settings" className="font-medium underline">
          설정
        </Link>
        에서 입력하세요.
      </span>
    </div>
  )
}
