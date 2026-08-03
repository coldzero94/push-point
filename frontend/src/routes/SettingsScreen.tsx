// Settings — 11 §8. Single-column form (max-width --w-form): API key + 2-step
// connection check + 3-state theme. Stats + bookmarklet are P2 (§9) — not here.
//
// Connection check is deliberately two calls (§8(3), §11 findings): GET /healthz
// is auth-exempt (security: []), so a wrong key still returns 200 — it proves the
// server is alive but NOT that the key works. Step 2 hits the lightest
// authenticated endpoint (GET /api/v1/tags) to validate the Bearer, yielding
// exactly three phrases: valid / mismatch(401) / unreachable. Saving the key
// invalidates every query so results that failed with 401 under the old (or
// missing) key refetch (§8(4)).
//
// The check runs against the SAVED key — the client middleware reads only
// localStorage, never the field — so 확인 is gated behind save: a pasted-but-
// unsaved edit disables the button (with a "먼저 저장하세요" prompt) instead of
// validating a stale key and misreporting "키 불일치".

import { Fragment, useEffect, useRef, useState } from 'react'
import type { FormEvent, KeyboardEvent, ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { RhythmSection } from './RhythmSection'
import { SheetsSection } from './SheetsSection'
import { Eye, EyeOff } from 'lucide-react'
import { api, errorMessage } from '../lib/api/client'
import { Button, Icon, Input, cn } from '../components/ui'
import { getApiKey, hasApiKey, setApiKey } from '../lib/auth'
import { t } from '../lib/i18n'
import { getThemePref, setThemePref } from '../lib/theme'
import type { ThemePref } from '../lib/theme'

// 인라인 마크업(<code>, <strong>)이 섞인 문장. 앞뒤를 따로 번역해 이어 붙이면 어순이
// 다른 언어에서 무너지므로, 문장 하나를 통째로 사전에 두고 {이름} 자리에만 노드를 끼운다.
// 키가 아니라 t()의 결과를 받는다 — scripts/web_i18n_check.py는 리터럴 t('키') 호출만
// 사용으로 세므로, 여기서 키를 받으면 멀쩡히 쓰는 문장이 미사용으로 잡힌다.
function fill(text: string, slots: Record<string, ReactNode>): ReactNode[] {
  return text.split(/(\{\w+\})/g).map((part, i) => {
    const name = /^\{(\w+)\}$/.exec(part)?.[1]
    return name !== undefined && name in slots ? <Fragment key={i}>{slots[name]}</Fragment> : part
  })
}

// idle → (checking) → one terminal result. `error` carries a contract message;
// the other three map to the three fixed phrases in §8(3).
type CheckState = 'idle' | 'checking' | 'valid' | 'unauthorized' | 'unreachable' | 'error'

export function SettingsScreen() {
  const queryClient = useQueryClient()
  const inputRef = useRef<HTMLInputElement>(null)

  const [key, setKey] = useState(getApiKey() ?? '')
  const [savedKey, setSavedKey] = useState(getApiKey() ?? '')
  const [show, setShow] = useState(false)
  // 확장 설정값 복사 — 실제 클릭 컨텍스트라 clipboard API가 정상 동작한다.
  const [copied, setCopied] = useState('')

  const copy = async (value: string, label: string) => {
    if (!value) {
      setCopied(t('settings.copyEmpty', { label }))
      return
    }
    try {
      await navigator.clipboard.writeText(value)
      setCopied(t('settings.copied', { label }))
    } catch {
      // 클립보드가 막힌 환경(비보안 컨텍스트 등) — 값을 숨기지 않고 그대로 보여준다.
      setCopied(t('settings.copyFailed', { value }))
    }
  }
  const [saved, setSaved] = useState(false)

  const [state, setState] = useState<CheckState>('idle')
  const [errMsg, setErrMsg] = useState('')

  // The field diverges from the stored key (setApiKey trims, so compare trimmed).
  // While dirty, 확인 would validate the OLD key — so we disable it and ask to
  // save first, keeping the check honest (§8(4)).
  const dirty = key.trim() !== savedKey

  // Key not set (§8(5)): the global warn banner stays; here we just focus the
  // field so the first keystroke lands where it should.
  useEffect(() => {
    if (!hasApiKey()) inputRef.current?.focus()
  }, [])

  function onSave(e: FormEvent) {
    e.preventDefault()
    setApiKey(key)
    setSavedKey(key.trim()) // mirror the trimmed value setApiKey actually stored
    // The Bearer token changed: drop every cached result so queries that 401'd
    // under the old/missing key refetch (§8(4)), and clear any prior check
    // result — it was measured against the old key. Confirmation is an inline
    // "저장됨" for 2s — NOT a toast (§1.4-1: in-view change).
    setState('idle')
    setErrMsg('')
    void queryClient.invalidateQueries()
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  async function checkConnection() {
    if (dirty) return // save first — the middleware only sees the stored key
    setState('checking')
    setErrMsg('')

    // Step 1 — server liveness. Auth-exempt, so this only proves the process is up.
    try {
      const { data, error, response } = await api.GET('/healthz', {})
      if (error || !response.ok || data?.status !== 'ok') {
        setState('unreachable')
        return
      }
    } catch {
      setState('unreachable')
      return
    }

    // Step 2 — key validation via the lightest authenticated endpoint.
    try {
      const { data, error, response } = await api.GET('/api/v1/tags', {})
      if (response.status === 401) {
        setState('unauthorized')
        return
      }
      if (error || !response.ok || !data) {
        setState('error')
        setErrMsg(errorMessage(error))
        return
      }
      // Not a throwaway probe — seed the tag-dictionary cache the chips read (§8(3)).
      queryClient.setQueryData(['tags'], data)
      setState('valid')
    } catch {
      setState('unreachable')
    }
  }

  return (
    <section className="mx-auto max-w-(--w-form) space-y-32 py-8">
      <h1 className="text-head text-fg-1">{t('settings.title')}</h1>

      {/* ── 연결 ─────────────────────────────────────────────── */}
      <div className="space-y-12">
        <h2 className="text-title text-fg-1">{t('settings.connection')}</h2>

        <form onSubmit={onSave} className="space-y-8">
          <label htmlFor="apikey" className="block text-label text-fg-2">
            {t('settings.apiKey')}
          </label>
          <div className="flex items-start gap-8">
            <div className="relative flex-1">
              <Input
                ref={inputRef}
                id="apikey"
                type={show ? 'text' : 'password'}
                autoComplete="off"
                spellCheck={false}
                value={key}
                onChange={(e) => setKey(e.target.value)}
                placeholder="dev-key"
                className="pr-32 font-mono"
              />
              <button
                type="button"
                onClick={() => setShow((s) => !s)}
                aria-label={show ? t('settings.hideKey') : t('settings.showKey')}
                // 24×24 meets the mouse minimum; `hit-target` adds the touch
                // 44×44 ::before. Already `absolute`, so it anchors its own (§7.5).
                className="hit-target absolute right-8 top-1/2 flex h-24 w-24 -translate-y-1/2 items-center justify-center rounded-control text-fg-3 hover:bg-hover hover:text-fg-2"
              >
                {show ? <Icon icon={EyeOff} size={16} /> : <Icon icon={Eye} size={16} />}
              </button>
            </div>
            <Button type="submit" variant="primary">
              {t('common.save')}
            </Button>
            {saved ? (
              <span aria-live="polite" className="self-center text-meta text-fg-2">
                {t('common.saved')}
              </span>
            ) : null}
          </div>
          <p className="text-meta text-fg-3">
            {fill(t('settings.keyHint'), {
              env: <code className="font-mono text-fg-2">PUSHPOINT_API_KEY</code>,
              header: <code className="font-mono text-fg-2">Authorization: Bearer</code>,
            })}
          </p>
        </form>

        <div className="flex flex-wrap items-center gap-12">
          <Button
            type="button"
            variant="secondary"
            onClick={checkConnection}
            loading={state === 'checking'}
            disabled={dirty}
          >
            {state === 'checking' ? t('settings.checking') : t('settings.checkConnection')}
          </Button>
          {dirty ? (
            <p role="status" className="text-meta text-fg-3">
              {t('settings.saveKeyFirst')}
            </p>
          ) : (
            <ConnectionResult
              state={state}
              errMsg={errMsg}
              onReenter={() => inputRef.current?.focus()}
            />
          )}
        </div>
      </div>

      <div className="border-t border-line-2" />

      {/* ── 저장 도구 ─────────────────────────────────────────
          서버가 못 가져오는 페이지(SPA·봇 차단·로그인 벽)는 브라우저 확장이 이미 렌더된
          본문을 함께 보낸다. 여기서는 설치 안내와 확장에 넣을 값 복사만 제공한다 —
          키는 확장 저장소로 들어가고 웹페이지는 접근할 수 없다. */}
      <div className="space-y-12">
        <h2 className="text-title text-fg-1">{t('settings.saveTools')}</h2>
        <p className="text-body text-fg-2">
          {fill(t('settings.extensionIntro'), {
            body: <strong className="text-fg-1">{t('settings.extensionIntroBody')}</strong>,
          })}
        </p>
        <ol className="ml-16 list-decimal space-y-6 text-body text-fg-2 marker:text-fg-3">
          <li>
            {fill(t('settings.extensionStep1'), {
              url: <code className="font-mono text-meta text-fg-1">chrome://extensions</code>,
            })}
          </li>
          <li>
            {fill(t('settings.extensionStep2'), {
              dir: <code className="font-mono text-meta text-fg-1">extension/</code>,
            })}
          </li>
          <li>{t('settings.extensionStep3')}</li>
        </ol>
        <div className="flex flex-wrap gap-8">
          <Button
            variant="secondary"
            onClick={() => copy(window.location.origin, t('settings.serverAddress'))}
          >
            {t('settings.copyServerAddress')}
          </Button>
          <Button variant="secondary" onClick={() => copy(getApiKey() ?? '', t('settings.apiKey'))}>
            {t('settings.copyApiKey')}
          </Button>
        </div>
        {copied ? (
          <p className="text-meta text-fg-2" role="status">
            {copied}
          </p>
        ) : null}
      </div>

      {/* ── 리듬 ───────────────────────────────────────────────
          위쪽 구분선을 이 컴포넌트가 직접 그린다 — 401이면 섹션이 통째로 사라지는데,
          구분선이 밖에 있으면 빈 줄 두 개가 겹쳐 남는다. */}
      <RhythmSection />

      <div className="border-t border-line-2" />

      {/* ── 스프레드시트 ──────────────────────────────────────
          연결은 터미널(just sheets-setup)이 맡는다 — 구글 승인을 브라우저에서 밟아야 해서
          서버가 대신할 수 없다. 연결된 뒤로는 여기서 끝나야 한다. */}
      <SheetsSection />

      <div className="border-t border-line-2" />

      {/* ── 모양 ─────────────────────────────────────────────── */}
      <div className="space-y-12">
        <h2 className="text-title text-fg-1">{t('settings.appearance')}</h2>
        <div className="flex items-center justify-between gap-16">
          <span className="text-body text-fg-1">{t('settings.theme')}</span>
          <ThemeSegment />
        </div>
      </div>
    </section>
  )
}

// The three fixed phrases (§8(3)/(5)). Success is achromatic — no accent (R1);
// only the two failure phrases carry --danger, and the sentence always repeats
// the state so color never carries meaning alone (§2.1.5 / §7.1). The result
// text is the one thing that transitions (§8(7), --dur-2); reduced-motion is
// sealed globally.
function ConnectionResult({
  state,
  errMsg,
  onReenter,
}: {
  state: CheckState
  errMsg: string
  onReenter: () => void
}) {
  if (state === 'idle' || state === 'checking') return null

  const base = 'text-meta transition-colors duration-(--dur-2) ease-ui'

  if (state === 'valid') {
    return (
      <p role="status" className={cn(base, 'text-fg-1')}>
        {t('settings.checkValid')}
      </p>
    )
  }
  if (state === 'unauthorized') {
    return (
      <p role="status" className={cn(base, 'text-danger')}>
        {t('settings.checkUnauthorized')}
        <button
          type="button"
          onClick={onReenter}
          className="ml-8 rounded-control text-fg-1 underline underline-offset-2 hover:text-fg-2"
        >
          {t('settings.reenterKey')}
        </button>
      </p>
    )
  }
  if (state === 'unreachable') {
    return (
      <p role="status" className={cn(base, 'text-danger')}>
        {t('status.serverUnreachable')}
      </p>
    )
  }
  // state === 'error' — contract message verbatim (§8(3) step 2 "그 외").
  return (
    <p role="status" className={cn(base, 'text-danger')}>
      {errMsg}
    </p>
  )
}

// 라벨이 함수인 이유: 모듈 로드 시점에 t()를 부르면 그때 잡힌 언어가 그대로 굳어
// 언어를 바꿔도 세그먼트만 안 바뀐다. 렌더할 때마다 다시 뽑는다.
const THEME_OPTIONS: { value: ThemePref; label: () => string }[] = [
  { value: 'light', label: () => t('settings.themeLight') },
  { value: 'dark', label: () => t('settings.themeDark') },
  { value: 'system', label: () => t('settings.themeSystem') },
]

// 3-state theme segment (§8(4)/§2.1.6). `system` MUST be reachable — its absence
// is a bug, so all three are exposed. Selection uses neutral elevation (raised
// bg-surface thumb in a recessed bg-hover track), NOT accent fill — brand solid
// is reserved for the four places in §2.1.4, and a segment is not one of them.
// ARIA radiogroup with roving tabindex + arrow keys.
function ThemeSegment() {
  const [pref, setPref] = useState<ThemePref>(getThemePref())
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  function select(i: number) {
    const v = THEME_OPTIONS[i].value
    setThemePref(v)
    setPref(v)
    refs.current[i]?.focus()
  }

  function onKeyDown(e: KeyboardEvent, i: number) {
    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
      e.preventDefault()
      select((i + 1) % THEME_OPTIONS.length)
    } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
      e.preventDefault()
      select((i + THEME_OPTIONS.length - 1) % THEME_OPTIONS.length)
    }
  }

  return (
    <div
      role="radiogroup"
      aria-label={t('settings.theme')}
      className="inline-flex gap-2 rounded-control border border-line-control bg-hover p-2"
    >
      {THEME_OPTIONS.map((o, i) => {
        const checked = pref === o.value
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            onClick={() => select(i)}
            onKeyDown={(e) => onKeyDown(e, i)}
            className={cn(
              'rounded-control px-12 py-4 text-label transition-colors duration-(--dur-out) ease-ui',
              checked ? 'bg-surface text-fg-1 shadow-ring' : 'text-fg-2 hover:text-fg-1',
            )}
          >
            {o.label()}
          </button>
        )
      })}
    </div>
  )
}
