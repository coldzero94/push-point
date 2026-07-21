import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from '@tanstack/react-router'
import { z } from 'zod'
import { useCreateLink } from '../hooks/useLinkMutations'
import { errorMessage } from '../lib/api/client'

// useState + Zod (no form library). URL is validated client-side before POST.
const saveSchema = z.object({
  url: z
    .string()
    .trim()
    .min(1, 'URL을 입력하세요.')
    .refine((v) => /^https?:\/\//i.test(v), 'http:// 또는 https:// 로 시작하는 URL이어야 합니다.'),
  note: z.string().trim().optional(),
})

type SaveResult = { id: number; duplicate: boolean }

export function SaveScreen() {
  const [url, setUrl] = useState('')
  const [note, setNote] = useState('')
  const [validationError, setValidationError] = useState<string | null>(null)
  const [result, setResult] = useState<SaveResult | null>(null)
  const create = useCreateLink()

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    setResult(null)
    const parsed = saveSchema.safeParse({ url, note })
    if (!parsed.success) {
      setValidationError(parsed.error.issues[0]?.message ?? '입력이 올바르지 않습니다.')
      return
    }
    setValidationError(null)
    create.mutate(
      { url: parsed.data.url, note: parsed.data.note || undefined },
      {
        onSuccess: (r) => {
          setResult(r)
          setUrl('')
          setNote('')
        },
      },
    )
  }

  return (
    <section className="space-y-4">
      <h1 className="text-lg font-semibold">링크 저장</h1>
      <p className="text-sm text-neutral-500">
        URL을 저장하면 서버가 백그라운드에서 제목·설명·썸네일을 수집하고 태깅합니다.
      </p>

      <form onSubmit={onSubmit} className="space-y-3">
        <div>
          <label htmlFor="url" className="mb-1 block text-sm font-medium">
            URL
          </label>
          <input
            id="url"
            type="text"
            inputMode="url"
            autoFocus
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://…"
            className="w-full rounded-md border border-neutral-300 bg-transparent px-3 py-2 text-sm outline-none focus:border-neutral-500 dark:border-neutral-700"
          />
        </div>

        <div>
          <label htmlFor="note" className="mb-1 block text-sm font-medium">
            메모 <span className="font-normal text-neutral-400">(선택)</span>
          </label>
          <input
            id="note"
            type="text"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="나중에 왜 저장했는지…"
            className="w-full rounded-md border border-neutral-300 bg-transparent px-3 py-2 text-sm outline-none focus:border-neutral-500 dark:border-neutral-700"
          />
        </div>

        {validationError && <p className="text-sm text-red-600">{validationError}</p>}
        {create.isError && (
          <p className="text-sm text-red-600">{errorMessage(create.error)}</p>
        )}

        <button
          type="submit"
          disabled={create.isPending}
          className="rounded-md bg-neutral-900 px-4 py-2 text-sm font-medium text-white hover:bg-neutral-700 disabled:opacity-50 dark:bg-neutral-100 dark:text-neutral-900 dark:hover:bg-neutral-300"
        >
          {create.isPending ? '저장 중…' : '저장'}
        </button>
      </form>

      {result && (
        <div
          className={
            result.duplicate
              ? 'rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-200'
              : 'rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200'
          }
        >
          {result.duplicate ? (
            <>이미 저장된 링크입니다 (id {result.id}). 새로 만들지 않았습니다.</>
          ) : (
            <>저장했습니다 (id {result.id}). 백그라운드에서 수집·태깅됩니다.</>
          )}{' '}
          <Link to="/links/$id" params={{ id: String(result.id) }} className="font-medium underline">
            상세 보기
          </Link>
        </div>
      )}
    </section>
  )
}
