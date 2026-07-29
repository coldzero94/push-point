import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../lib/api/client'
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
      <h2 className="text-title text-fg-1">스프레드시트</h2>

      {status.isPending ? (
        <p className="text-body text-fg-2">확인 중…</p>
      ) : !s?.connected ? (
        <>
          <p className="text-body text-fg-2">
            저장한 링크를 Google 스프레드시트로 내보낼 수 있습니다. 시트에서 걸러 보거나 남에게
            보여줄 때 쓰는 자리이고, <strong className="text-fg-1">백업은 아닙니다</strong> —
            데이터는 SQLite 파일 하나라 그 파일을 복사하는 편이 더 완전합니다.
          </p>
          <p className="text-body text-fg-2">
            연결은 터미널에서 한 번만 하면 됩니다. 구글 승인을 브라우저에서 직접 밟아야 해서
            이 화면이 대신할 수 없습니다.
          </p>
          <pre className="overflow-x-auto rounded-control border border-line-2 bg-surface p-12 font-mono text-meta text-fg-1">
            just sheets-setup
          </pre>
          <p className="text-meta text-fg-3">
            스크립트를 클립보드에 넣고 브라우저를 열어 줍니다. 붙여넣고 배포한 뒤 URL만
            되돌려 주면 끝입니다.
          </p>
        </>
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-16">
            <div className="space-y-4">
              <p className="text-body text-fg-1">연결됨</p>
              <p className="text-meta text-fg-2">{lastRun(s)}</p>
            </div>
            <div className="flex gap-8">
              {s.sheet_url ? (
                <Button variant="secondary" onClick={() => window.open(s.sheet_url!, '_blank', 'noopener')}>
                  시트 열기
                </Button>
              ) : null}
              <Button onClick={() => sync.mutate()} disabled={sync.isPending}>
                {sync.isPending ? '보내는 중…' : '지금 동기화'}
              </Button>
            </div>
          </div>

          {/* 실패는 그대로 보여준다. 시트는 다른 탭에 있어 화면만 봐서는 모르고,
              사유를 삼키면 사용자가 무엇을 고쳐야 할지 알 수 없다. */}
          {s.last_error ? (
            <p className="text-meta text-danger" role="status">
              마지막 동기화 실패: {s.last_error}
            </p>
          ) : null}
          {sync.isError ? (
            <p className="text-meta text-danger" role="status">
              동기화 요청이 실패했습니다.
            </p>
          ) : null}

          <p className="text-meta text-fg-3">
            매번 시트를 <strong className="text-fg-2">통째로 다시 씁니다</strong> — 태그를 고치거나
            링크를 지운 것이 반영돼야 하기 때문입니다. 그래서 시트에 손으로 적은 것은 지워집니다.
            메모를 남기려면 다른 탭에 하세요.
          </p>
        </>
      )}
    </div>
  )
}

/** 마지막 실행을 한 문장으로. 한 번도 안 했으면 그 사실을 그대로 말한다. */
function lastRun(s: { last_sync_at?: number | null; last_rows?: number; last_error?: string }): string {
  if (!s.last_sync_at) return '아직 한 번도 보내지 않았습니다'
  const when = new Date(s.last_sync_at * 1000).toLocaleString('ko-KR', {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
  if (s.last_error) return `${when}에 시도했다가 실패`
  return `${when} · ${s.last_rows ?? 0}건`
}
