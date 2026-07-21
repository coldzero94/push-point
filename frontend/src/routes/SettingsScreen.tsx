import { useState } from 'react'
import type { FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '../lib/api/client'
import { getApiKey, setApiKey } from '../lib/auth'

type Health = 'idle' | 'checking' | 'ok' | 'fail'

export function SettingsScreen() {
  const [key, setKey] = useState(getApiKey() ?? '')
  const [saved, setSaved] = useState(false)
  const [health, setHealth] = useState<Health>('idle')
  const [healthMsg, setHealthMsg] = useState('')
  const queryClient = useQueryClient()

  function onSave(e: FormEvent) {
    e.preventDefault()
    setApiKey(key)
    // The Bearer token changed: drop every cached result so queries that failed
    // with 401 under the old (or missing) key refetch.
    void queryClient.invalidateQueries()
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  async function checkConnection() {
    setHealth('checking')
    setHealthMsg('')
    try {
      const { data, error } = await api.GET('/healthz', {})
      if (error || data?.status !== 'ok') {
        setHealth('fail')
        setHealthMsg(error ? errorMessage(error) : '예상치 못한 응답')
        return
      }
      setHealth('ok')
    } catch (err) {
      setHealth('fail')
      setHealthMsg(errorMessage(err, '서버에 연결할 수 없습니다.'))
    }
  }

  return (
    <section className="space-y-4">
      <h1 className="text-lg font-semibold">설정</h1>

      <form onSubmit={onSave} className="space-y-3">
        <div>
          <label htmlFor="apikey" className="mb-1 block text-sm font-medium">
            API 키
          </label>
          <p className="mb-2 text-sm text-neutral-500">
            서버의 <code className="rounded bg-neutral-100 px-1 dark:bg-neutral-800">PUSHPOINT_API_KEY</code>{' '}
            값입니다. localStorage에 저장되어 모든 요청에 <code>Authorization: Bearer</code>로 붙습니다.
          </p>
          <input
            id="apikey"
            type="password"
            autoComplete="off"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder="dev-key"
            className="w-full rounded-md border border-neutral-300 bg-transparent px-3 py-2 text-sm outline-none focus:border-neutral-500 dark:border-neutral-700"
          />
        </div>

        <div className="flex items-center gap-3">
          <button
            type="submit"
            className="rounded-md bg-neutral-900 px-4 py-2 text-sm font-medium text-white hover:bg-neutral-700 dark:bg-neutral-100 dark:text-neutral-900 dark:hover:bg-neutral-300"
          >
            저장
          </button>
          <button
            type="button"
            onClick={checkConnection}
            className="rounded-md border border-neutral-300 px-4 py-2 text-sm hover:bg-neutral-100 dark:border-neutral-700 dark:hover:bg-neutral-800"
          >
            연결 확인
          </button>
          {saved && <span className="text-sm text-emerald-600">저장됨</span>}
        </div>
      </form>

      {health !== 'idle' && (
        <p className="text-sm">
          {health === 'checking' && <span className="text-neutral-500">확인 중…</span>}
          {health === 'ok' && <span className="text-emerald-600">서버 정상 (/healthz ok)</span>}
          {health === 'fail' && <span className="text-red-600">연결 실패: {healthMsg}</span>}
        </p>
      )}
    </section>
  )
}
