import { useSyncExternalStore } from 'react'
import { Link } from '@tanstack/react-router'
import { hasApiKey, isAuthFailed, subscribeApiKey, subscribeAuthFailed } from '../lib/auth'
import { t } from '../lib/i18n'

// Top-fixed banner shown when the app cannot authenticate (§1.4: unauthorized →
// banner, not a toast). Two triggers: no key configured, OR a key that the server
// actually rejected with 401 (wrong/revoked) — the latter is observed by the
// client middleware, so a stored-but-invalid key no longer fails silently. Every
// non-exempt endpoint needs the Bearer token, so unauthenticated the app can only
// reach healthz/thumbs. Both stores are useSyncExternalStore-backed, so saving a
// valid key in Settings (which clears both) hides this immediately.
// warn palette (§2.1.5: "API 키 미설정 배너"); not an aria-live region (§7.3).
export function ApiKeyBanner() {
  const configured = useSyncExternalStore(subscribeApiKey, hasApiKey)
  const authFailed = useSyncExternalStore(subscribeAuthFailed, isAuthFailed)
  if (configured && !authFailed) return null
  return (
    <div
      role="region"
      aria-label={t('status.apiKeyState')}
      className="border-b border-line-1 bg-warn-tint"
    >
      <div className="mx-auto flex max-w-(--w-page) items-center gap-8 px-(--gutter) py-8 text-meta text-warn">
        <span>{t('status.apiKeyRequired')}</span>
        <span aria-hidden>·</span>
        <Link to="/settings" className="font-medium underline underline-offset-2 hover:text-warn">
          {t('common.openSettings')}
        </Link>
      </div>
    </div>
  )
}
