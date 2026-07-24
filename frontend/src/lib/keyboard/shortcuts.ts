// The keyboard contract (11 §1.2) as data — the single source both the global
// handler and the `?` overlay read, so the overlay renders the table 1:1 (10
// §7.4: "`?` 오버레이는 11 §1.2 표를 그대로 렌더한다"). Do NOT bind keys that are
// not in this table.
//
// The P0 subset that is actually wired today (§9 P0-7): S, /, J/K, Enter, O, E,
// N, R, Backspace, Cmd/Ctrl+Enter, Esc, ?. Cmd/Ctrl+K (command palette, P1) and
// Cmd/Ctrl+V (instant save, P2) are listed in the table for parity but are not
// bound yet — the overlay still shows them so the contract stays 1:1.

export type Shortcut = {
  /** key column, space-separated so `+` (combo) and `/` (alternatives) render as
   *  separators between chips (e.g. 'Cmd/Ctrl + K', 'J / ↓') */
  keys: string
  /** scope column (11 §1.2) */
  scope: string
  /** action column (11 §1.2) */
  action: string
}

export const SHORTCUTS: readonly Shortcut[] = [
  { keys: 'Cmd/Ctrl + K', scope: '전역', action: '커맨드 팔레트 열기(이동 + 검색 + 태그 점프)' },
  { keys: '/', scope: '목록·검색', action: '검색 입력 포커스(목록에서는 검색으로 이동하며 포커스)' },
  { keys: 'S', scope: '전역', action: '저장 컴포저 열고 URL 필드 포커스' },
  { keys: 'Cmd/Ctrl + V', scope: '입력 밖', action: '클립보드가 http(s)://로 시작하면 즉시 저장(실행취소 토스트)' },
  { keys: 'J / ↓', scope: '목록·검색', action: '다음 행으로 커서 이동' },
  { keys: 'K / ↑', scope: '목록·검색', action: '이전 행으로 커서 이동' },
  { keys: 'Enter', scope: '목록·검색', action: '커서 행의 인스펙터 열기' },
  { keys: 'O', scope: '행 커서·인스펙터', action: '원문(url)을 새 탭으로 열기 — 원문 열기는 이 키 하나뿐이다' },
  { keys: 'E', scope: '행 커서·인스펙터', action: '인스펙터를 열고 태그 입력에 포커스' },
  { keys: 'N', scope: '행 커서·인스펙터', action: '인스펙터를 열고 메모 입력에 포커스' },
  { keys: 'R', scope: '행 커서·인스펙터', action: "status가 'failed'일 때만 재시도" },
  { keys: 'Backspace / Delete', scope: '행 커서·인스펙터', action: '링크 소프트 삭제 + 실행취소 토스트' },
  { keys: 'Cmd/Ctrl + Enter', scope: '저장 폼 안', action: '그 폼을 즉시 제출/저장(blur·버튼 클릭 대기 없이)' },
  { keys: 'Esc', scope: '전역', action: '팔레트 → 컴포저 → 인스펙터 → 검색어 순으로 하나씩 닫기/비우기' },
  { keys: '?', scope: '전역', action: '이 단축키 오버레이 열기/닫기' },
]

/** True when focus is in a text input surface — single-char shortcuts are inert
 *  there (11 §1.2). Esc / Cmd-Ctrl+Enter are handled by the forms themselves. */
export function isEditableTarget(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false
  return t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable
}
