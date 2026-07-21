import { ImageOff } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import type { Link as LinkItem } from '../lib/api/types'
import { linkDisplayTitle } from '../lib/api/types'
import { StatusBadge } from './StatusBadge'

// A single list item. Title falls back domain → url when the server returns an
// empty title (og/title absent) — the empty-cell guard is the client's job.
export function LinkCard({ link }: { link: LinkItem }) {
  const title = linkDisplayTitle(link)
  return (
    <article className="flex gap-4 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <div className="h-20 w-32 shrink-0 overflow-hidden rounded-md bg-neutral-100 dark:bg-neutral-800">
        {link.thumb_url ? (
          <img
            src={link.thumb_url}
            alt=""
            loading="lazy"
            className="h-full w-full object-cover"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-neutral-400">
            <ImageOff size={20} aria-hidden />
          </div>
        )}
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex items-start justify-between gap-2">
          <Link
            to="/links/$id"
            params={{ id: String(link.id) }}
            className="truncate font-medium hover:underline"
          >
            {title}
          </Link>
          <StatusBadge status={link.status} />
        </div>

        <div className="mt-0.5 flex items-center gap-2 text-sm text-neutral-500">
          <span className="truncate">{link.domain}</span>
          <a
            href={link.url}
            target="_blank"
            rel="noreferrer"
            className="shrink-0 text-xs hover:underline"
          >
            원문
          </a>
        </div>

        {link.description && (
          <p className="mt-1 line-clamp-2 text-sm text-neutral-600 dark:text-neutral-400">
            {link.description}
          </p>
        )}

        {link.tags.length > 0 && (
          <ul className="mt-2 flex flex-wrap gap-1">
            {link.tags.map((t) => (
              <li
                key={t.id}
                className="rounded bg-neutral-100 px-1.5 py-0.5 text-xs text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300"
              >
                #{t.name}
              </li>
            ))}
          </ul>
        )}

        {link.note && (
          <p className="mt-2 text-sm text-neutral-500">
            <span className="font-medium">메모</span> {link.note}
          </p>
        )}
      </div>
    </article>
  )
}
