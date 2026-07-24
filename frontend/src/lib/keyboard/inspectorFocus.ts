// A one-shot hand-off for the `E` / `N` shortcuts fired from the LIST row cursor
// (11 §1.2 — "인스펙터를 열고 태그/메모 입력에 포커스"). The list can only open the
// inspector by setting ?link; it cannot reach the inspector's inputs. So it
// parks the intent here, the inspector consumes it once its content is mounted,
// and clears it. Module-level (not context) because exactly one inspector is
// ever open and the value is read-once.

export type InspectorFocus = 'tags' | 'note'

let pending: InspectorFocus | null = null

export function requestInspectorFocus(target: InspectorFocus): void {
  pending = target
}

/** Read and clear the pending focus intent (returns null if none). */
export function consumeInspectorFocus(): InspectorFocus | null {
  const value = pending
  pending = null
  return value
}
