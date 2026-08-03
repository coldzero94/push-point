import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api/client'
import { getLang, t } from '../lib/i18n'
import { Button } from '../components/ui/Button'

/**
 * 스프레드시트 내보내기 — 연결 상태와 "지금 동기화" 버튼.
 *
 * **연결 자체는 여기서 못 한다.** 구글 승인을 브라우저에서 밟아야 하고 그건 서버가 대신할
 * 수 없어서, 연결은 터미널의 `just sheets-setup`이 안내한다. 대신 **연결된 뒤로는 터미널을
 * 열 이유가 없어야 한다** — 그게 이 화면이 있는 이유다. 버튼이 없으면 "시트에서 본다"는
 * 습관이 터미널 습관에 묶인다.
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
          <p className="text-body text-fg-2">{t('sheets.connectOnce')}</p>
          <pre className="overflow-x-auto rounded-control border border-line-2 bg-surface p-12 font-mono text-meta text-fg-1">
            just sheets-setup
          </pre>
          <p className="text-meta text-fg-3">{t('sheets.setupHint')}</p>
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
