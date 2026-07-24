// Offline bar (§1.6). Top-fixed under the header when the server is unreachable
// (navigator.onLine === false or an escalated network-level fetch failure).
// Achromatic, no icon: the sentence carries the whole meaning. Not an aria-live
// region — the app keeps exactly one (the toast viewport, §4.10 / §7.3); the
// persistent bar plus aria-disabled write controls convey the state instead.

import { useOffline } from '../lib/offline'

export function OfflineBar() {
  const offline = useOffline()
  if (!offline) return null
  return (
    <div role="region" aria-label="연결 상태" className="border-b border-line-1 bg-hover">
      <div className="mx-auto max-w-(--w-page) px-(--gutter) py-8 text-meta text-fg-2">
        서버에 연결할 수 없습니다. 저장·수정이 비활성화됩니다.
      </div>
    </div>
  )
}
