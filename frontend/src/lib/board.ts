// What the board actually draws, once the forgotten-link card is taken into account.
//
// This is a rule implemented twice — here and in `FeedModel.boardView(filtered:)` on
// iOS — so it lives in a pure function that a shared fixture can call
// (`testdata/resurface-board-cases.json`). It has already diverged once: the web
// checked `!tag` while iOS checked "no filter at all", and nothing noticed because
// both screens looked fine on their own.
//
// Two rules, and both of them are quiet when broken:
//
//  1. The card is a MOVE, not a copy. Resurfacing picks from the whole archive and a
//     seven-day-old link is commonly still inside page one, so without this the same
//     card renders twice on one screen — certain on a small archive.
//
//  2. A narrowed view gets no card. Putting an unrelated link on top of a filtered
//     list is not resurfacing, it is interference. `unopened` counts as narrowing too:
//     the card is a move, so one of those links would silently leave the time-ordered
//     list the user asked to see complete.
//
// The tail matters as much as the contents. iOS pulls the next page when the last
// board card appears and has no "load more" fallback, so if the tail were computed
// from the raw list instead of this result, an archive could simply stop at fifty.

export type BoardInput<T extends { id: number }> = {
  links: readonly T[]
  /** The forgotten link, or null/undefined when the server had no candidate (204). */
  resurfaced: T | null | undefined
  /** Is the view narrowed by any control — tag, status or unopened. */
  filtered: boolean
}

export type BoardView<T extends { id: number }> = {
  /** The card above the board, or undefined when there is none to draw. */
  card: T | undefined
  /** The links the board itself draws, in order. */
  board: readonly T[]
}

export function boardView<T extends { id: number }>(input: BoardInput<T>): BoardView<T> {
  const card = input.filtered ? undefined : (input.resurfaced ?? undefined)
  if (!card) return { card: undefined, board: input.links }
  return { card, board: input.links.filter((l) => l.id !== card.id) }
}
