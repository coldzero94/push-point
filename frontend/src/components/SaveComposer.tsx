// Save composer (§2) — the URL + optional note form that opens over the list.
// Submitting inserts a filling row at the top of the list instantly (S2) via
// useSaveLink; this component owns only the input surface, keyboard, and result
// feedback. It is a standalone component so the list route can mount it at the
// top (the router's /save renders it through SaveScreen).
//
// Client validation is a single http(s):// prefix check — every other verdict
// is the server's (invalid_input). Keyboard: Enter or Cmd/Ctrl+Enter submits
// (from the note field too, §1.2), Esc closes to the list. ?url/?note prefill
// from a bookmarklet fills the fields but never auto-submits (§2(4)).

import { useCallback, useRef, useState, useSyncExternalStore } from 'react'
import type { FormEvent, KeyboardEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { z } from 'zod'
import { Button, Input, Textarea, useToast } from './ui'
import { useSaveLink } from '../hooks/useSaveLink'
import { errorMessage } from '../lib/api/client'
import type { ApiError } from '../lib/api/client'
import { hasApiKey, subscribeApiKey } from '../lib/auth'
import { t } from '../lib/i18n'

// http(s):// prefix only — the server owns every other verdict (§2(4)).
// 스키마를 함수로 두는 이유는 메시지 때문이다 — 모듈 로드 시점에 굳으면 언어를 바꿔도
// 그대로 남는다.
const urlSchema = () =>
  z
    .string()
    .trim()
    .min(1, t('save.urlRequired'))
    .refine((v) => /^https?:\/\//i.test(v), t('save.urlScheme'))

/** contract error.code (the 4 codes are all there is, §1.4). */
function errorCode(err: unknown): string | undefined {
  if (err && typeof err === 'object' && 'error' in err) {
    return (err as ApiError).error?.code
  }
  return undefined
}

export type SaveComposerProps = {
  /** ?url bookmarklet prefill (fills, never auto-submits — §2(4)) */
  initialUrl?: string
  /** ?note bookmarklet prefill */
  initialNote?: string
}

export function SaveComposer({ initialUrl = '', initialNote = '' }: SaveComposerProps) {
  const navigate = useNavigate()
  const toast = useToast()
  const save = useSaveLink()

  const [url, setUrl] = useState(initialUrl)
  const [note, setNote] = useState(initialNote)
  const [inlineError, setInlineError] = useState<string | null>(null)
  const urlRef = useRef<HTMLInputElement>(null)

  // Re-render when the key is saved in Settings so the composer swaps between the
  // form and the missing-key prompt without a navigation (§2(5)).
  const keyPresent = useSyncExternalStore(subscribeApiKey, hasApiKey, () => false)

  const submit = useCallback(async () => {
    const parsed = urlSchema().safeParse(url)
    if (!parsed.success) {
      setInlineError(parsed.error.issues[0]?.message ?? t('common.invalidInput'))
      urlRef.current?.focus()
      return
    }
    setInlineError(null)
    const trimmedNote = note.trim()
    try {
      const out = await save.mutateAsync({
        url: parsed.data,
        note: trimmedNote || undefined,
      })
      // Success path: fields clear and the URL field is re-focused for the next
      // save. Created shows NO toast — the S2 row already shows the result (§1.4).
      setUrl('')
      setNote('')
      urlRef.current?.focus()
      if (out.kind === 'duplicate') {
        toast.show({
          variant: 'warn',
          message: t('save.duplicate'),
          action: {
            label: t('common.open'),
            onClick: () => navigate({ to: '/links/$id', params: { id: String(out.id) } }),
          },
        })
      }
    } catch (err) {
      const code = errorCode(err)
      if (code === 'invalid_input') {
        // 400 → inline under the URL field, fields kept (§1.4 / §2(5)).
        setInlineError(errorMessage(err))
      } else if (code === 'unauthorized') {
        setInlineError(t('save.keyRequired'))
      } else if (code === undefined) {
        // no error body = network failure — offline, no toast (§1.4 / §1.6).
        setInlineError(t('save.offline'))
      } else {
        // 500 internal → error toast + single retry action (§1.4).
        toast.show({
          variant: 'error',
          message: errorMessage(err),
          action: { label: t('common.tryAgain'), onClick: () => void submit() },
        })
      }
    }
  }, [url, note, save, toast, navigate])

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void submit()
  }

  // Cmd/Ctrl+Enter submits from anywhere in the form (incl. the note field);
  // Esc closes the composer back to the list (§1.2).
  const onKeyDown = (e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault()
      void submit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      void navigate({ to: '/' })
    }
  }

  // §2(5): no key → prompt to open Settings instead of the form. A present but
  // wrong key still surfaces as an inline unauthorized message on submit.
  if (!keyPresent) {
    return (
      <section className="rounded-panel bg-surface p-20 shadow-panel">
        <p className="text-body text-fg-2">{t('save.enterKeyInSettings')}</p>
        <div className="mt-12">
          <Button variant="primary" onClick={() => navigate({ to: '/settings' })}>
            {t('common.openSettings')}
          </Button>
        </div>
      </section>
    )
  }

  return (
    <form
      onSubmit={onSubmit}
      onKeyDown={onKeyDown}
      className="rounded-panel bg-surface p-20 shadow-panel"
    >
      <div className="flex flex-col gap-12 sm:flex-row sm:items-start">
        <div className="min-w-0 flex-1">
          <label htmlFor="composer-url" className="mb-8 block text-label text-fg-2">
            URL
          </label>
          <Input
            ref={urlRef}
            id="composer-url"
            variant="url"
            autoFocus
            value={url}
            onChange={(e) => {
              setUrl(e.target.value)
              if (inlineError) setInlineError(null)
            }}
            placeholder="https://…"
            invalid={inlineError != null}
            errorMessage={inlineError ?? undefined}
            aria-label={t('save.urlAria')}
          />
        </div>
        <div className="sm:pt-24">
          <Button variant="primary" type="submit" disabled={url.trim().length === 0}>
            {t('common.save')}
          </Button>
        </div>
      </div>

      <div className="mt-12">
        <label htmlFor="composer-note" className="mb-8 block text-label text-fg-2">
          {t('common.note')} <span className="text-fg-3">{t('common.optional')}</span>
        </label>
        <Textarea
          id="composer-note"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder={t('save.notePlaceholder')}
          aria-label={t('save.noteAria')}
        />
      </div>
    </form>
  )
}
