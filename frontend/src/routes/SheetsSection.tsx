import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '../lib/api/client'
import { getLang, t } from '../lib/i18n'
import { Button } from '../components/ui/Button'

/**
 * 스프레드시트 내보내기 — 연결 상태와 "지금 동기화" 버튼.
 *
 * **연결도 여기서 한다**(2026-08-06). 예전에는 "터미널에서 `just sheets-setup`" 한 줄이
 * 전부였는데, 그러면 시작하려면 터미널을 열 줄 알아야 한다 — 그 자체가 벽이고, 실제로
 * 그 CLI는 표준입력이 없는 환경에서 **다섯 단계를 다 안내해 놓고 마지막에 죽는다.**
 *
 * 구글이 강제하는 셋(붙여넣기·배포·승인)은 그대로 남는다. 없앨 수 있는 것 — 터미널,
 * 클립보드 운, 표준입력 — 만 없앴다. Apps Script 편집기가 모바일에 없어서 **이 한 번은
 * 컴퓨터가 필요하다**는 것도 화면이 말한다. 모르고 폰에서 시도하는 것이 가장 나쁘다.
 *
 * 마지막 결과를 항상 보여준다. 버튼만 있고 결과가 없으면 눌렀을 때 됐는지 알 수 없고,
 * 실패는 특히 조용하다 — 시트는 다른 탭에 있으니 화면을 봐서는 모른다.
 */
export function SheetsSection() {
  const queryClient = useQueryClient()

  const status = useQuery({
    queryKey: ['sheets'],
    queryFn: async () => {
      const { data, error } = await api.GET('/api/v1/sheets', {})
      if (error) throw error
      return data
    },
  })

  const sync = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST('/api/v1/sheets/sync', {})
      if (error) throw error
      return data
    },
    onSuccess: (data) => queryClient.setQueryData(['sheets'], data),
  })

  const s = status.data

  return (
    <div className="space-y-12">
      <h2 className="text-title text-fg-1">{t('sheets.title')}</h2>

      {status.isPending ? (
        <p className="text-body text-fg-2">{t('sheets.checking')}</p>
      ) : !s?.connected ? (
        <>
          {/* 강조가 문장 한가운데 있으면 앞뒤 조각을 이어 붙이게 되고, 그 조립은 어순이
              다른 언어에서 무너진다. 그래서 강조를 문장 하나로 떼어 세 키로 나눴다 —
              각 조각이 그 자체로 완결된 문장이라 따로 옮길 수 있다. */}
          <p className="text-body text-fg-2">
            {t('sheets.exportIntro')} <strong className="text-fg-1">{t('sheets.notBackup')}</strong>{' '}
            {t('sheets.backupAdvice')}
          </p>
          <Connect onDone={(data) => queryClient.setQueryData(['sheets'], data)} />
        </>
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-16">
            <div className="space-y-4">
              <p className="text-body text-fg-1">{t('sheets.connected')}</p>
              <p className="text-meta text-fg-2">{lastRun(s)}</p>
            </div>
            <div className="flex gap-8">
              {s.sheet_url ? (
                <Button variant="secondary" onClick={() => window.open(s.sheet_url!, '_blank', 'noopener')}>
                  {t('sheets.openSheet')}
                </Button>
              ) : null}
              <Button onClick={() => sync.mutate()} disabled={sync.isPending}>
                {sync.isPending ? t('sheets.syncing') : t('sheets.syncNow')}
              </Button>
            </div>
          </div>

          {/* 실패는 그대로 보여준다. 시트는 다른 탭에 있어 화면만 봐서는 모르고,
              사유를 삼키면 사용자가 무엇을 고쳐야 할지 알 수 없다. */}
          {s.last_error ? (
            <p className="text-meta text-danger" role="status">
              {t('sheets.lastSyncFailed', { error: s.last_error })}
            </p>
          ) : null}
          {sync.isError ? (
            <p className="text-meta text-danger" role="status">
              {t('sheets.syncRequestFailed')}
            </p>
          ) : null}

          {/* 위 문단과 같은 이유로 강조가 문장 하나를 통째로 감싼다. */}
          <p className="text-meta text-fg-3">
            <strong className="text-fg-2">{t('sheets.rewriteWhole')}</strong>{' '}
            {t('sheets.rewriteWhy')}
          </p>
        </>
      )}
    </div>
  )
}

/** 마지막 실행을 한 문장으로. 한 번도 안 했으면 그 사실을 그대로 말한다. */
function lastRun(s: { last_sync_at?: number | null; last_rows?: number; last_error?: string }): string {
  if (!s.last_sync_at) return t('sheets.neverSynced')
  // 날짜 형식도 화면에 보이는 문자열이다 — 'ko-KR'로 고정하면 영어 화면에 "오후"가 섞인다.
  const when = new Date(s.last_sync_at * 1000).toLocaleString(
    getLang() === 'ko' ? 'ko-KR' : 'en-US',
    {
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    },
  )
  if (s.last_error) return t('sheets.lastRunFailed', { when })
  return t('sheets.lastRunOk', { when, count: s.last_rows ?? 0 })
}

/**
 * 연결 흐름 — 스크립트를 주고, 배포 URL을 받는다.
 *
 * **스크립트를 눌러야 받아온다.** 화면에 들어오자마자 부르면 아직 연결할 생각이 없는
 * 사람에게도 토큰이 만들어진다. 그 자체가 해롭진 않지만, "연결하기"를 누른 적 없는데
 * 서버 상태가 바뀌는 것은 설명하기 어려운 종류의 동작이다.
 *
 * **단계를 접지 않고 다 펼쳐 둔다.** 아코디언으로 감추면 지금 어디쯤인지가 사라지고,
 * 이 흐름은 브라우저 탭을 오가는 동안 자리를 잃기 쉽다.
 */
function Connect({ onDone }: { onDone: (data: unknown) => void }) {
  const [url, setUrl] = useState('')
  const [copied, setCopied] = useState(false)

  const script = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.GET('/api/v1/sheets/script', {})
      if (error) throw error
      return data
    },
  })

  const connect = useMutation({
    mutationFn: async (deployUrl: string) => {
      const { data, error } = await api.POST('/api/v1/sheets/connect', {
        body: { deploy_url: deployUrl },
      })
      if (error) throw error
      return data
    },
    onSuccess: onDone,
  })

  if (!script.data) {
    return (
      <div className="space-y-8">
        <p className="text-body text-fg-2">{t('sheets.connectIntro')}</p>
        {/* 컴퓨터가 필요하다는 사실을 **누르기 전에** 말한다. 폰에서 시작했다가 4단계에서
            막히는 것이 이 흐름의 최악이다 — 되돌아올 방법을 아무도 안내해 주지 않는다. */}
        <p className="text-meta text-fg-3">{t('sheets.needsDesktop')}</p>
        <Button onClick={() => script.mutate()} disabled={script.isPending}>
          {script.isPending ? t('sheets.preparing') : t('sheets.startConnect')}
        </Button>
        {script.isError ? (
          <p className="text-meta text-danger" role="status">{t('sheets.scriptFailed')}</p>
        ) : null}
      </div>
    )
  }

  return (
    <div className="space-y-12">
      <ol className="list-decimal space-y-8 pl-20 text-body text-fg-2 marker:text-fg-3">
        <li>
          {t('sheets.stepNewSheet')}{' '}
          <a className="text-accent underline" href="https://sheets.new" target="_blank" rel="noopener">
            sheets.new
          </a>
        </li>
        <li>{t('sheets.stepOpenEditor')}</li>
        <li>
          {t('sheets.stepPaste')}
          <div className="mt-8 space-y-8">
            <Button
              variant="secondary"
              onClick={() => {
                void navigator.clipboard.writeText(script.data.script).then(() => {
                  setCopied(true)
                  window.setTimeout(() => setCopied(false), 2000)
                })
              }}
            >
              {copied ? t('sheets.copied') : t('sheets.copyScript')}
            </Button>
            {/* 스크립트를 화면에도 둔다 — 클립보드가 막힌 브라우저가 있고, 무엇을
                붙여넣는지 볼 수 없는 채로 승인하라는 요구는 그 자체로 나쁘다. */}
            <details>
              <summary className="cursor-pointer text-meta text-fg-3">{t('sheets.showScript')}</summary>
              <pre className="mt-8 max-h-(--size-script) overflow-auto rounded-control border border-line-2 bg-surface p-12 font-mono text-meta text-fg-1">
                {script.data.script}
              </pre>
            </details>
          </div>
        </li>
        <li>{t('sheets.stepDeploy')}</li>
        <li>{t('sheets.stepApprove')}</li>
      </ol>

      <div className="space-y-8">
        <label className="block text-body text-fg-1" htmlFor="deploy-url">
          {t('sheets.deployUrlLabel')}
        </label>
        <input
          id="deploy-url"
          className="w-full rounded-control border border-line-control bg-surface px-12 py-8 font-mono text-meta text-fg-1"
          placeholder="https://script.google.com/macros/s/.../exec"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
        />
        <Button onClick={() => connect.mutate(url.trim())} disabled={!url.trim() || connect.isPending}>
          {connect.isPending ? t('sheets.connecting') : t('sheets.connect')}
        </Button>
        {/* 서버가 **찔러 보고** 실패한 사유를 그대로 보여준다. 가장 흔한 실패는 배포의
            액세스 권한이 «모든 사용자»가 아닌 경우이고, 그 문장이 응답에 들어 있다. */}
        {connect.isError ? (
          <div role="status" className="space-y-4">
            <p className="text-meta text-danger">{errorMessage(connect.error)}</p>
            {/* 조언은 **여기서** 붙인다. 서버가 붙이면 그 문장이 한 언어로 굳어서, 영어
                화면에 한국어가 섞인다 — 실제로 그렇게 나왔다. */}
            <p className="text-meta text-fg-3">{t('sheets.connectHint')}</p>
          </div>
        ) : null}
      </div>
    </div>
  )
}
