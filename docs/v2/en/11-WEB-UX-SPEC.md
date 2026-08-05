# Web UX Spec

> Push-Point v2.1 — last updated: 2026-07-25

This document is the **screen-by-screen UX spec for the web client (`frontend/`)**.

Three premises get nailed down first.

1. **A feature the contract does not have is not specified here.** Every field name, parameter and status code below is the real name from [api/openapi.yaml](../../../api/openapi.yaml). Where the contract makes something impossible, this document writes "we cannot" and writes down the alternative (§10, the contract-gap list).
2. **The web is a full-feature client on par with iOS.** The original sentence — "only the share sheet differs" — was shown on 2026-07-27 to be false (stats existed on iOS only, dictionary CRUD on the web only). The per-feature classification rules moved to [13-CLIENT-PARITY.md](13-CLIENT-PARITY.md).
3. **Concept rules R1/R2/R3 govern every screen.** R1: hue is identity (tag facet + brand) and fill is intervention (fill 0/1/2) / R2: machine data is mono / R3: state is the left rail, not a badge. R1 was revised on 2026-07-22 — the definitions and the settled values are owned by 10 §1.2 · §2.1.4 · §5-side, and this document records only how they apply on screen.

**Which of the two documents owns what (a value is never written in both places).**

| Source | Territory |
|---|---|
| [10-DESIGN-SYSTEM.md](10-DESIGN-SYSTEM.md) | color, type, spacing, radius, shadow, the z ladder, motion duration/easing tokens, component dimensions and variants (card slots, cover, inspector width and the rest of the layout constants, §2.3), **the toast visual spec** (position, simultaneous count, duration, marker, §4.10), accessibility criteria (§7) |
| **11-WEB-UX-SPEC.md** (this document) | per-screen layout, field mapping, states, responsive behaviour, **the keyboard shortcut table** (§1.2), **when a toast fires + error-code mapping** (§1.4), optimistic-update and polling rules, new dependencies (§1.8), implementation priority (§9), contract gaps (§10) |

The other document's territory is **referenced, never copied** (e.g. "for duration see 10 §4.10"). When the two disagree, the owner of that territory wins.

## 0. Screen list and route decisions

| # | Screen | Route | Form | Notes |
|---|---|---|---|---|
| 1 | Save | `/save` | a **composer** that opens over the list (not a separate page) | `?url=`/`?note=` prefill for the bookmarklet |
| 2 | List | `/` | the home of the app. A card board cut by the time spine | `?tag` `?status` URL state |
| 3 | Search | `/search` | the same card renderer as the list + a search toolbar | `?q` `?tag` `?from` `?to` URL state |
| 4 | Tags | `/tags` | tag dictionary management table | the only writable dictionary in the contract |
| 5 | Detail | `/links/$id` | **inspector** (≥1024 right panel / below that, a sheet) | opens over the list when entered by deep link |
| 6 | Tag edit | — | **folded into detail. No separate screen is built** | see the decision below |
| 7 | Settings | `/settings` | single-column form | API key, theme, connection check, stats |

### Decision: the edit screen folds into detail (`/links/$id/edit` retired)

4 grounds.

- **The contract is one PATCH.** `PATCH /api/v1/links/{id}` takes `note` and `tags` together, and `tags` is a **full replacement**. A separate screen manufactures four steps — read → move to the edit screen → save → come back — when the contract is done in a single request.
- **The response is the detail.** The 200 response of PATCH is an entire `LinkDetail`. The screen that would show the edit result is already the detail screen, so splitting them means building the same renderer twice.
- **It collides with optimistic update.** Tag toggling is immediate on principle (§8), and a model where you move to another screen and press a "Save" button removes the place an optimistic update would live.
- **Consistency with iOS.** iOS opens detail with `.sheet` + detents and edits inside it. The web alone carrying one more route violates the §7 "same information hierarchy" principle.

**Migration**: delete the existing `/links/$id/edit` route and redirect it to `/links/$id` (bookmark compatibility). Edit state is expressed by focus inside the inspector (`E`/`N` shortcuts), not by a route.

### Decision: save is a composer, not a page

A row appearing instantly at the top of the list (signature S2) is the whole of the save UX. Move to a dedicated save page and you **save on a screen where the result is invisible**, which kills the signature. So `/save` renders as "the list, with the composer open and the URL field focused". The `Save` link in the top bar and the `S` shortcut open the same state.

---

## 1. Global elements

### 1.1 Navigation: top bar settled, no left sidebar

```
+----------------------------------------------------------------------+
| Push-Point   목록  검색  태그  설정        [ 저장 ]  [테마]  [?]      |  sticky glass 헤더
+----------------------------------------------------------------------+
```

(Header and toolbar heights and the other dimensions are owned by the 10 §2.3 layout constants. Every numeric annotation in the diagram follows those same 10 §2.3-defined values.)

- There are only 5 screens (list/search/tags/settings, with detail as an overlay) and one user. A sidebar is duplicate navigation: it occupies desktop width permanently and gets thrown away on mobile.
- Only `Save` is an accent primary button; the other four are text links (the active item gets weight 500 + `fg-1`). Of the four places that use brand solid (10 §2.1.4), the only one in the top bar is that primary button.
- The `Cmd/Ctrl+K` command palette doubles as navigation, search and recent-tag jumping. Because the palette exists, no dropdown is built into the top bar.
- `< 560`: the four text nav items do not shrink into four icons — **they stay as they are, and no horizontal scroll is introduced either**. Instead the `Push-Point` wordmark is hidden and `Save` shrinks to an icon button (+). Why no bottom tab bar: the bottom tab bar is an iOS idiom, and on the web it collides with the address bar and with sheets.

### 1.2 Keyboard shortcut table — the single source

**This table is the entire keyboard contract of the app.** The 10 §7.3-side holds only the accessibility requirements (roving tabindex, focus-exposure rules, `aria-live`) and defers key assignment to this table. Inside an input field every single-character shortcut is disabled (`Esc` and `Cmd/Ctrl+Enter` excepted).

| Key | Scope | Action |
|---|---|---|
| `Cmd/Ctrl+K` | global | open the command palette (navigate + search + tag jump) |
| `/` | list · search | focus the search input (from the list it navigates to `/search` and focuses) |
| `S` | global | open the save composer and focus the URL field |
| `Cmd/Ctrl+V` | outside inputs | if the clipboard starts with `http(s)://`, **save immediately** (§1.5 optimistic save, undo toast) |
| `J` / `↓` | list · search | move the cursor to the next row (virtual-scroll position stays in sync) |
| `K` / `↑` | list · search | move the cursor to the previous row |
| `Enter` | list · search | open the inspector for the cursor row |
| `O` | card cursor · inspector | open `url` in a new tab — **this key is the only way to open the original** |
| `E` | card cursor · inspector | open the inspector and focus the **tag input** |
| `N` | card cursor · inspector | open the inspector and focus the **note input** |
| `R` | card cursor · inspector | retry (`POST /links/{id}/retry`), only when `status === 'failed'` |
| `Backspace` / `Delete` | row cursor · inspector | soft-delete the link (`DELETE /links/{id}`) + undo toast |
| `Cmd/Ctrl+Enter` | **inside a save form** (composer URL and note, inspector note, tag edit row) | **submit/save that form immediately** (no waiting for blur or a button click) |
| `Esc` | global | close/clear **one at a time**, in the order palette → composer → inspector → search text |
| `?` | global | the shortcut overlay that shows this very table |

- **`Cmd/Ctrl+Enter` is save-only.** Outside a form it does nothing — doubling it up with "open the original" produces the misfire where you are writing a note, try to save, and a new tab opens.
- **How `Cmd/Ctrl+V` instant save is implemented**: read `event.clipboardData.getData('text')` from a `document`-level `paste` event and **nothing else**. `navigator.clipboard.readText()` is **forbidden** — Safari raises a paste confirmation UI and Firefox refuses, which breaks the §1.5-stated premise of "0 input latency". No `keydown` handler that intercepts the key combination directly, either (the browser will not hand over the clipboard).
- **Multi-select (`X`, `Shift+↑↓`) and theme cycling (`T`) are not in this table.** The first is out of MVP because the contract has no batch endpoint, so selecting N items becomes N requests (§10-3); for the second, the 3-state segment on the settings screen (§8) is the only path.
- **There is one delete-confirmation policy.** Deleting a link runs **immediately, with no confirmation dialog, plus an undo toast** (there is a way back, §1.5). The confirmation dialog (`AlertDialog`) is used in **exactly one place: tag dictionary deletion** — because CASCADE cannot be undone (§5-4).
- The `?` overlay and this table are the only source for every shortcut. Shortcuts are not duplicated into tooltips.

### 1.3 Display fallback rules (shared by every screen — same as iOS)

| Situation (as the contract expresses it) | Display |
|---|---|
| `title === ''` | `domain` → if that is empty too, `url` |
| `thumb_url === null` | `domain[0].toUpperCase()` + a `bg-hover` background. **No color hashing** |
| `description === ''` | hide the slot itself in the inspector (no blank line) |
| `author`/`lang`/`error === ''` | **hide that entire row** (never leave the label alone) |
| `confidence === null` (= `source: 'manual'`) | leave the confidence slot empty. Never print it as `0` |
| `jobs.tag`/`jobs.thumb` **field absent** | "not yet" (`fg-3`, `—`). Render it as a **different state** from `failed` |
| `jobs.thumb === 'failed'` + `status === 'done'` | **a normal combination.** Do not show the link as failed |
| `rank === null` (search `mode: 'like'`) | leave the rank slot empty |
| `content_type === 'video'` | a play icon at the bottom-right of the thumbnail (16px — the smaller of the two 10 §1.3-defined sizes). The other three (`article`/`post`/`other`) get **no marker** |

### 1.4 When a toast fires + error-code mapping

> **The visual spec is not here.** Position, how many show at once, duration, marker color and the enter/exit transitions are all owned by **10 §4.10 Toast**. This section fixes only "which variant fires for which event".

1. **When it fires**: only when the result happened **outside the current screen**. An action whose effect is visible immediately on screen (tag toggle, note save, filter applied) gets no toast.
2. **Composition**: one line of text + at most 1 action. No icon, no emoji.
3. **Event → variant mapping**:

| Event | Variant (10 §4.10) | Copy · action |
|---|---|---|
| duplicate save (`200 duplicate:true`) | `warn` | "You already saved this link." · `Open` → `/links/{id}` |
| link deleted (`204`) | `undo` | "Deleted." · `Undo — it will be collected again` (§1.5) |
| `Cmd/Ctrl+V` instant save succeeded | `undo` | "Saved." · `Undo` (= `DELETE`) |
| tag dictionary deletion (`204`) | `success` | "Deleted `개발`." · no action (no way back — §5-5) |
| server error (`500 internal` — declared in the contract) | `error` | `error.message` · `Retry` |
| optimistic mutation rolled back (`4xx`) | `error` | `error.message` · no action (the input stays as it was) |

4. **Error code → UI mapping** (the contract's four are all of them):

| `error.code` | HTTP | UI |
|---|---|---|
| `unauthorized` | 401 | not a toast. A **pinned top banner** ("An API key is required · Open settings") + stop retrying that query |
| `invalid_input` | 400 | an **inline** message on the input field (`error.message` verbatim). No toast |
| `not_found` | 404 | replace the inspector with a "this link was deleted or does not exist" state + drop that card from the list cache. No toast |
| `internal` | 500 | an `error` toast + 1 `Retry` action |
| network failure (no response) | — | no toast. Handled by the §1.6 offline bar |

**`500` is declared in the contract** — every path in `api/openapi.yaml` carries an `InternalError` response, 1:1-mapped onto `internal` in `Error.code`. Every status code in the table above is a real contract name (premise 1); the client branches on `error.code` and uses status codes only for logging and diagnosis.

5. **A success toast never uses the accent.** Success is achromatic — the places brand solid attaches to are fixed at the four in 10 §2.1.4, and a toast is not among them (R1). Only `warn` and `error` use color, and color never carries meaning by itself: the sentence always repeats the same information.

### 1.5 Optimistic update in the save flow

In the contract a save forks two ways: `POST /api/v1/links` **201 (new) / 200 (`duplicate: true`)**. The UI expresses that fork as it is.

```
제출 t=0ms      낙관적 행을 목록 최상단에 삽입
                { id: -tmp, url, domain(클라이언트 파싱), title:'', description:'',
                  content_type:'other', thumb_url:null, status:'pending',
                  tags:[], note, created_at: now }
                → 컴포저 즉시 비우고 닫힘. 입력 지연 0.

201 응답        temp id → 응답의 { id, status, created_at }로 치환 후
                GET /api/v1/links/{id} 1회로 행을 실값으로 교체
                (전체 목록 invalidate 금지 — 스크롤 위치와 다른 행이 흔들린다)

200 응답        duplicate:true → 낙관적 카드 제거, 경고 토스트
                "이미 저장한 링크입니다." [열기] → /links/{id} 인스펙터

4xx/5xx·네트워크 실패
                낙관적 카드 제거 + 컴포저를 입력값 그대로 다시 열고 인라인 에러
```

**Polling is the only way to track progress.** The contract has no SSE or WebSocket (§10-1). The rules:

- Target: only rows with `status ∈ {pending, scraping, tagging}` that are **inside the viewport**.
- Cadence: `GET /api/v1/links/{id}` once a second for ten rounds, then every three seconds for forty more, then stop (about two minutes in total). On `done`/`failed`, stop immediately.
- Pause when `document.visibilityState === 'hidden'`; run once immediately on return.
- A polling response replaces **that row only**. It never invalidates the whole list query.

Other optimistic mutations:

| Action | Endpoint | Optimistic effect | Rollback |
|---|---|---|---|
| tag toggle/add/remove | `PATCH /links/{id}` `{tags:[name…]}` (full replacement) | chip added/removed at once, additions shown as `source:'manual'`·`confidence:null` | restore the previous `tags` array + error toast |
| note save | `PATCH /links/{id}` `{note}` | text reflected immediately | restore the previous `note` |
| delete | `DELETE /links/{id}` | card removed at once (§1.7 card removal motion) | card restored |
| retry | `POST /links/{id}/retry` | rail switches to in-progress (pulse) at once, `status:'pending'` | restored to `failed` |

**The limits of delete-undo are stated out loud.** The contract has no restore endpoint. Instead, `POST /api/v1/links` means "re-saving a soft-deleted URL = restoring the same row (back to pending, note replaced, scrape re-enqueued)", so undo is implemented as a **re-POST with the same `url` and `note`**. The link does come back, but **`status` returns to `pending` and the scrape runs again**. The toast copy does not hide that → "Deleted. [Undo — it will be collected again]".

### 1.6 Offline and error global state

- **No offline save queue is built for the web.** Offline saving is the iOS Share Extension's job — the extension writes directly to the shared SQLite in the App Group, so a save completes with no network ([04-DATA-FLOW.md](04-DATA-FLOW.md) §7.4). Imitating it on the web would give the two clients different duplicate rules.
- When `navigator.onLine === false`, or a request fails at the network level, pin an **offline bar** (achromatic, no icon) under the top bar: "Cannot reach the server. Saving and editing are disabled."
- While offline: every write button gets `disabled` + `aria-disabled`. **Reading is not blocked** — lists and details still in the TanStack Query cache keep showing, and every row stays as it is (nothing gets greyed out).
- On coming back online, revalidate only the current screen's queries and show no toast (the user has already seen the bar disappear).

### 1.7 Loading and card transition rules (shared by every screen)

- **The 200ms rule**: if a request finishes within 200ms, no skeleton is rendered. The local server runs at a 50ms-p99, so most requests fall here.
- Split by purpose: first entry into a list = **skeleton rows** (at the final dimensions) / a single action = **spinner inside the button** / waiting on a scrape = **rail pulse** (no separate spinner).
- Never render an empty state while `isPending`.
- **CLS 0**: the card's cover aspect ratio, the title/body slot heights and the inspector width are all reserved in advance. The real dimensions are owned by 10 §2.3 · §4.4-side, and this document does not copy those values.

**Card insert/remove motion uses `transform`/`opacity` only** (the 10 §6.1-stated ban on animating `top`/`height`). The list, search and tags screens all share this rule.

| Event | Implementation |
|---|---|
| card insert (optimistic save S2) | the new card runs `opacity 0→1` + `translateY(-4px→0)` `--dur-2`(180ms) — 10 §6.1 "insertion and fill right after save (S2)" owns this. **The cards behind it FLIP `translateY`** `--dur-flip`(220ms) — the board height is already at its final value on the first frame. The slot-filling choreography after insertion is §3(7) |
| card remove (delete) | lift the target card out as a **`position: absolute` snapshot** so it leaves document flow at once, then `opacity 1→0` + `scaleY(1→.96)` `--dur-out`(120ms); on the same frame the cards behind it FLIP `translateY` `--dur-flip`(220ms) |
| inline edit expand | `grid-template-rows: 0fr → 1fr` `--dur-2`(180ms) + content `opacity`. `height` is never animated directly. **`grid-template-rows` interpolation is a 10 §6.1-registered approved exception, and the only thing it applies to is the edit row of the tag dictionary** |

- In the virtual-scroll range (§3-6), FLIP is **limited to rows inside the viewport**. No transform is applied to off-screen rows.
- `--dur-flip`(220ms) and `--dur-close`(200ms) are 10 §2.6-listed duration tokens. Every time value in this document sits inside that allowed set (120/160/180/200/220/260ms + the 2400ms rail pulse + toast duration), and **all three transitions above are registered in the 10 §6.1-usage table (six entries)** — this document never invents motion that is not in that table.

### 1.8 New dependencies this spec requires

Today `frontend/package.json` holds only React 19 / TanStack Router · Query / Tailwind v4 / openapi-fetch / lucide-react / zod. Implementing this spec needs the packages below **on top of that**. This table is the source for the adoption list, and no UI library outside it gets adopted.

| Package | Used for | Introduced at |
|---|---|---|
| `@radix-ui/react-dialog` | inspector/sheet (§6-6), command palette, the `?` shortcut overlay | P0 |
| `@radix-ui/react-popover` | search period and tag filters (§4-2), **the container for the tag combobox** (§6-4) | P0 |
| `@radix-ui/react-select` | the list status filter dropdown (§3-4), **the four-way facet choice on the tag management screen** (§5-4) | P0 |
| `@radix-ui/react-visually-hidden` | icon button labels, `aria-live` status sentences | P0 |
| `@radix-ui/react-alert-dialog` | **the one tag dictionary delete confirmation** (§5-4) | P1 |
| `@radix-ui/react-tooltip` | toolbar and card action tooltips | P1 |
| `@tanstack/react-virtual` | virtual scrolling once more than 200 rows render (§3-6) | P1 (after measuring) |

**The combobox and the command palette are hand-built.** Radix has **no** Combobox primitive (the list of what it does provide is 10 §4.11), and no separate library such as `cmdk` gets adopted either. Both are built from `Popover` (or `Dialog`) + our own input + a filtered list, implementing the ARIA combobox pattern ourselves (`role="combobox"` + `aria-expanded` + `aria-controls` + `aria-activedescendant`, with the list as `role="listbox"`/`role="option"`). The list filter is a single case-insensitive substring match — no fuzzy-matching library gets dragged in.

The toast is hand-built too — there is no reason to attach a dependency to a component that is one line of copy plus one action. The visual spec is 10 §4.10 and the accessibility roles (`role="status"` / `role="alert"`, 1 `aria-live` region) are 10 §7-owned; this document fixes only "which variant fires when" in §1.4-terms.

---

## 2. Screen 1 — Save

**(1) Purpose** — throw in a single URL with the least input possible, and let the result fill itself in at the top of the list.

**(2) Layout**

```
+----------------------------------------------------------------------+
| Push-Point   목록  검색  태그  설정        [ 저장 ]  [테마]  [?]      |
+----------------------------------------------------------------------+
|  +----------------------------------------------------------------+  |  컴포저 (목록 위 인라인)
|  | URL   https://…                                    [ 저장 ⏎ ]  |  |  --radius-panel, --sh-panel
|  | 메모  (선택) 왜 저장했는지                                     |  |
|  +----------------------------------------------------------------+  |
|                                                                      |
| ▎[■] ────────────    ░░░░░░░░░░                    ░░░ ░░░           |  ← 방금 만든 낙관적 행
| ▎    example.com · 방금                                              |     (스켈레톤 = S2 "채워지는 행")
| ──────────────────────────────────────────────────────────────────── |
| ▎[::] 이전에 저장한 링크 제목                       #개발 #영상      |
+----------------------------------------------------------------------+
```

**(3) API field mapping**

| UI | Contract |
|---|---|
| URL input | `LinkInput.url` (required) |
| note input | `LinkInput.note` (optional) |
| submit result (new) | `201 { id, status, created_at }` |
| submit result (duplicate) | `200 { id, duplicate: true }` |
| `domain` on the optimistic row | parsed client-side with `new URL(url).hostname` (once the server answers, the server value wins) |
| inline error | `400 error.message` verbatim |

**(4) Interaction**

- Opens with `S` → the URL field is focused automatically. `Enter` submits, `Cmd/Ctrl+Enter` submits immediately even from the note field, `Esc` closes (input is preserved for the session).
- Merely pasting into the URL field enables the submit button. Client-side validation is a single `http(s)://` prefix check — every other verdict is left to the server (`invalid_input`).
- `Cmd/Ctrl+V` outside an input → if the clipboard holds a URL, **save immediately** without opening the composer, and attach `Undo` (= `DELETE`) to the toast. The clipboard is read only from the `clipboardData` of a `document`-level `paste` event (§1.2 footnote — `navigator.clipboard.readText()` is forbidden).
- The note field is reachable only by `Tab`. The default save path is URL + Enter, two actions.
- Bookmarklet entry `/save?url=…&note=…` prefills the fields but **does not auto-submit** (no accidental fire).

**(5) By state**

| State | Presentation |
|---|---|
| loading (submitting) | none. The optimistic row is already inserted, so not even a button spinner shows |
| empty | not applicable (the composer always accepts input) |
| error 400 | a single inline `danger` line under the URL field. Composer stays, input stays |
| duplicate 200 | warn toast + `Open`. The composer closes (a duplicate is not a failure) |
| offline | the composer opens but the submit button is disabled + an inline "Cannot reach the server" |
| no key (401) | instead of the composer, "Enter an API key in settings · [Open settings]" |

**(6) Responsive**

| Width | Change |
|---|---|
| `< 560` | the composer becomes a sheet pinned to the top of the screen. The URL input takes the minimum touch-target size (10 §7.5) and the save button goes full width below the input |
| `560~1023` | an inline card at the top of the list. The save button sits inline to the right of the input |
| `≥ 1024` | the same + even with the inspector open, the composer is laid out only within the list column width |

**(7) Motion** — the composer's entrance and exit are **exactly the "command palette · composer" row of 10 §6.1-side**, and this document does not copy the properties or the timings (motion ownership belongs to 10-side — the ownership boundary table at the head of 10). Optimistic row insertion likewise follows the **§1.7 "row insert" rule**, and that rule is itself registered in the "row insertion and fill right after save (S2)" row of 10 §6.1-side. **Height is never animated** (10 §6.1).

**(8) iOS difference** — one-tap saving from the share sheet is the default path and the in-app URL form is the secondary one, so the composer exists only as a small detent of the `.sheet`.

---

## 3. Screen 2 — List

> **Revised 2026-07-25.** Row list → **card board + time spine**. The rationale and the rewritten exclusion list are 10 §1.3, the card spec is 10 §4.4, the generated cover is 10 §4.5.

**(1) Purpose** — skim recent saves top to bottom and let only "what is in progress" and "what failed" catch the eye. One thing has been added: **make what you collected recognisable from two lines of body text, not the title alone.**

**(2) Layout**

```
+--------------------------------------------------------------------------------+
| Push·Point   목록  검색  태그  설정              [ 저장 ]  [테마]  [?]          |  헤더 56px
+--------------------------------------------------------------------------------+
| [검색 / 또는 Cmd+K]                                    상태: 전체 ▾             |  툴바 sticky
| #개발 12  #영상 8  #디자인 5  #ai 3  …            (태그 없음)                   |  칩 바 1줄
+-----------------------------------------------------+--------------------------+
| 오늘  4건 ───────────────────────────────────────── |  인스펙터 (≥1024)        |  시간 척추 (serif)
| ┌──────────┐ ┌──────────┐ ┌──────────┐              |                          |
| │▎ 커버16:9 │ │▎ 커버16:9 │ │▎ 커버16:9 │              |                          |
| │ 제목 2줄  │ │ 제목 2줄  │ │(제목 빈칸)│              |                          |  pending: 레일 펄스
| │ 본문 2줄  │ │ 본문 2줄  │ │(본문 빈칸)│              |                          |
| │ #개발 #k8s│ │ #개발     │ │           │              |                          |
| │ 도메인·시각│ │ 도메인·시각│ │ 도메인·방금│              |                          |
| └──────────┘ └──────────┘ └──────────┘              |                          |
| 이번 주 11건 ─────────────────────────────────────── |                          |
+-----------------------------------------------------+--------------------------+
   ▲ 2px 상태 레일 (done은 투명 = 아무것도 없음)
```

The internals of a card (dimensions owned by 10 §2.3 · 10 §4.4):

```
[2px 레일 세로 관통]
[ 커버 aspect-[16/9] — thumb_url 또는 생성 커버(10 §4.5) ]
[ 제목  title/600, 2줄 클램프, 슬롯 40px 고정                 ]
[ 본문  card/400,  2줄 클램프, 슬롯 40px 고정                 ]
[ 태그 칩 ≤3, 줄바꿈 허용                                    ]
[ domain · 상대시각 (meta/mono, fg-3)          [hover 액션] ]
```

- **The column count is decided by the board container, not the viewport** — one column / `@board-sm`(460px) two columns / `@board-md`(760px) three columns. When the inspector opens, the viewport does not change but the board narrows (10 §2.3).
- **Every slot height is settled at mount time.** The place is held even when the value has not arrived, so the board does not shift while the worker fills it in (CLS 0).
- **The time spine** cuts `created_at` into `today / yesterday / this week (2~6 days ago) / month / year and month`. The list is already `created_at DESC`, so a single pass cutting at the boundaries is enough and the same group can never appear twice. The heading is **serif** (10 §2.2.5) and the count beside it is mono.

**(3) API field mapping** — `GET /api/v1/links` → `LinkPage.links[]` (`Link` schema)

| UI element | Field | Handling |
|---|---|---|
| status rail | `status` | `done`→transparent / `pending`·`scraping`·`tagging`→`rail-progress` pulse / `failed`→a solid `danger` line. The token's light and dark values are 10 §2.1.2, the pulse spec is 10 §4.7 |
| cover | `thumb_url` | if present, the image (`loading="lazy" decoding="async"`, `brightness(.92)` in dark). **If `null`, the generated cover** (10 §4.5) — not a grey box, not a domain initial |
| the cover's facet | first tag of `tags[]` | the first tag in chip sort order (= manual first, otherwise highest confidence). With no tags, `neutral` |
| play icon | `content_type` | shown for `'video'` only. Top-right of the cover |
| title | `title` | if the string is empty, `domain` → `url`. Two-line clamp |
| **body** | `description` | **Two-line clamp.** The contract already truncates the list response to 200-character text — no extra request. When it is `failed` and empty, the failure sentence goes into this slot |
| domain | `domain` | mono, `fg-3`. Also repeated as a wordmark on the generated cover |
| time | `created_at` | epoch seconds → relative form (just now / N minutes ago / N hours ago / yesterday / month and day). mono · `tabular-nums`, absolute time in the `title` attribute. **The time spine groups come from this field too** |
| tag chips | `tags[]` (`LinkTag`) | sort: `source==='manual'` first → `confidence` desc → `name`. The color is decided by 10 §5.2 `chipStyle` — **card chips are `role: 'readonly'`, `manual` is fill 1 (that tag's facet tint), everything else fill 0** |
| a chip's facet | **not on `LinkTag`** | resolved as `Map<tagId, facet>` from the `GET /api/v1/tags` cache. **A cache miss never guesses: `neutral`** (10 §5.2 · §9 contract consistency) |
| note-exists marker | `note !== ''` | a single icon. The content lives in the inspector |
| open the original | `url` | hover action + `O` |
| load more | `next_cursor` | `null` means the end. IntersectionObserver auto-loads |
| tag filter | query `tag` | 1:1 with the `?tag=` URL state |
| status filter | query `status` | all five `LinkStatus` values + all |
| page size | query `limit` | fixed at 50 (contract maximum 100, default 20) |

**What the list does not show**: `confidence` and the `note` body. (`description` has been shown since 2026-07-25 — the previous spec dropped it on the grounds that it "contributes most to density", and the reason that trade was reversed is the last item of 10 §4.4.)

**(4) Interaction**

- Clicking a card = opening the inspector. **Not opening the original** (that is the title link / the `Open` button / `O` / `Cmd+click`). Only the title is an `<a href={url}>`; the rest of the area is the inspector trigger.
- Right-hand actions appear on hover/focus only: `Open`. Under `@media (hover: none)` they are always visible. `Retry` on a `failed` card is always visible.
- On hover the card only raises its shadow (`--sh-card` → `--sh-lift`) plus at most `translateY(-2px)`. **No scale** — in a grid it overlaps the neighbouring card.
- The `J`/`K` cursor follows DOM order (= left→right, top→bottom). Cards carry `scroll-margin` so the sticky toolbar never hides them.
- Clicking a tag chip = toggling `?tag` (a single value — the contract has one `tag` parameter). No right-click menu on chips.
- **Chips in the chip bar (the filter bar) are controls; chips inside a card are display-only.** Only filter chips carry the `--line-control` border (WCAG 1.4.11 control boundary); card and inspector chips are `readonly` and have no border (10 §4.3 · §7.1). A selected filter chip is a solid fill of that tag's facet (fill 2), and the hue never changes.
- **Selection is shown by an accent ring, not by a background** (10 §4.4). The cover already occupies the card's background, so there is no place for `--bg-selected`. That is why the previous spec's rule — "inside a selected row a fill 1 chip drops to fill 0-level" — **does not fire on cards**: the `onSelectedRow` argument of `chipStyle` still exists, but cards always pass `false`.
- Choosing `failed` in the status dropdown (`@radix-ui/react-select`, §1.8) is the path for cleaning up failed links.
- Swipe: under `@media (hover:none)`, swiping a card left → delete, right → retry (`failed` only). Dragging past 60px commits; less than that returns with `--dur-out`(120ms) and no spring.

**(5) By state**

| State | Presentation |
|---|---|
| loading | 6 skeleton cards (pinned at the card dimensions — 10 §4.4). Not shown if the response arrives within 200ms |
| empty (no filter) | "Nothing collected yet." + a `Save a link` button (opens the composer). No illustration, no emoji |
| empty (with filter) | "No links match `#개발` + `failed`." + `Clear filters` |
| loading the next page | 3 skeleton cards at the bottom. They attach under the last group rather than creating a new time spine. A "Load more" button exists only as a fallback when IntersectionObserver fails |
| error | an achromatic block where the board is + `error.message` + `Retry`. Pages already loaded stay |
| offline | cached cards stay + the offline bar at the top. Infinite scroll stops |

**(6) Responsive**

| Width | Board columns | Chips | Inspector |
|---|---|---|---|
| `< 560` | one column | 3 max (wrapping) | full-screen sheet |
| `560~1023` | two columns | 3 max | bottom sheet `85dvh` |
| `≥ 1024` | three columns with the inspector closed / two with it open | 3 max | pinned right panel (width: 10 §2.3) |
| `≥ 1280` | three columns | 3 max | page max width + desktop gutters applied (10 §2.3) |

The column count is decided by **container queries** — the "columns" in the table above are a result, not a rule. The rules are only `@board-sm` 460px and `@board-md` 760px, and if opening or closing the inspector changes the board width, the columns change even though the viewport did not. **The "chips per row" that the previous spec decided by container query has been retired** — in a card the chips own their own line and wrap, so there is nothing to count. Past the 200-card render mark, switch to `@tanstack/react-virtual`.

**(7) Motion** — card hover enters at 0ms and leaves at `--dur-out`(120ms), shadow and `translateY` only. Card insertion and removal follow the §1.7 table exactly (**both `transform`/`opacity` only**, no height animation).

**The choreography of S2 (the card that fills in)** is the only staging on this screen (10 §1.4). Every slot carries `.fill-step` + `data-in`, and `data-in` **looks only at "has that value arrived"** — there is no timer, so the order is the order the worker actually finished in. The transition is `--dur-fill`(340ms) `--ease-enter`, moving `opacity` and a 4px `translateY` and nothing else. A card already complete on first render enters the DOM with `data-in="true"` and is painted with no transition (so the accident where every card fades in on entering the list is structurally impossible). The in-progress rail pulses at 2400ms (the only infinite loop in the app). Under `prefers-reduced-motion` the choreography is **removed**, not reduced. **No stagger entrances, no scroll reveals, no card lift.**

**(8) iOS difference** — `LazyVStack` + `Section(header:)` draws the time spine and `.swipeActions` replaces the hover actions. The header is `.navigationTitle(.large)`, so the toolbar chip bar attaches under the large title. The screen prototype is `ios/design/prototype.html`.

---

## 4. Screen 3 — Search

**(1) Purpose** — narrow the archive with the one word you remember, and do not hide which path (FTS5 / LIKE) found it.

**(2) Layout**

```
+--------------------------------------------------------------------------------+
| [🔍 검색어                                        ] [기간 ▾]  [태그 ▾]  [지우기]|  툴바
| FTS5 · bm25 정렬 · 불러온 20건                                                 |  결과 메타 (mono, fg-3)
+-----------------------------------------------------+--------------------------+
| ▎[::] Kubernetes 네트워킹 정리              #개발     |  인스펙터                |
| ▎     kubernetes.io · 3시간 전                       |                          |
| ─────────────────────────────────────────────────── |                          |
| ▎[::] k8s 트러블슈팅                        #개발     |                          |
+-----------------------------------------------------+--------------------------+
```

The row renderer is **exactly the same component** as the list's. Search results do not get a different card (same information hierarchy principle).

**(3) API field mapping** — `GET /api/v1/search` → `SearchPage`

| UI | Field / parameter |
|---|---|
| search input | query `q` (required, `minLength: 1`) |
| tag dropdown | query `tag` |
| period dropdown | the API takes queries `from` / `to` (epoch seconds). **The URL stores a preset key `?period` (7d/30d/year) and expands it into `from` at request time** (implemented 2026-07-25) — freezing an epoch into the URL drifts as "now" moves and breaks sharing. Presets: all / last 7 days / 30 days / this year, all open-ended (no `to`) |
| page | queries `cursor`, `limit`, response `next_cursor` |
| result meta label | response `mode` — `fts` → "FTS5 · bm25 order" / `like` → "LIKE fallback · newest first" |
| result meta count | **not a response field.** It is the client's running total of `links[]` lengths over the pages received so far, and the label must read "N loaded" |
| result rows | `links[]` (`SearchResult` = `Link` + `rank`) |
| rank | `rank` — not shown on the card (the information hierarchy stays the same as the list's). **Exposing it in an inspector diagnostics slot was deferred in the 2026-07-25 implementation — a contract gap:** `rank` exists only on `SearchResult` (a list item) and not on the `LinkDetail` the inspector reads, so the inspector cannot know the rank from its own query. Exposing it would mean carrying the search result's rank through navigation, and that is separate work. Carried over to the §10 contract-gap list |

**The UI reflects three contract facts.**
- **There is no total-count field.** `SearchPage` has only `mode`/`links`/`next_cursor` and `LinkPage` only `links`/`next_cursor` (cursor pagination, so the server never counts the total). So a **whole-result count such as "24 items" is never displayed** — the only number shown is "N loaded", and it keeps growing while `next_cursor !== null`. The list screen's empty and filter copy avoids counts for the same reason (§10-7).
- Search has **no `status` parameter.** So the search toolbar carries no status filter (list only).
- The list cursor and the search cursor are **not format-compatible.** When `q`/`tag`/`from`/`to` change, the cursor must be thrown away and the first page requested again (reuse gives 400).

**(4) Interaction**

- **No Enter needed.** Input debounce 120ms → `?q` is synced with `navigate({ search, replace: true })` (no history pollution).
- Fewer than three characters is not an error (contract: LIKE fallback). No "type at least three characters" notice is shown; it just searches. Instead the result meta shows `LIKE fallback · newest first`, telling you only that the ordering is different.
- `/` focuses; `Esc` goes 1) clear the query → 2) navigate to `/`.
- Row shortcuts `J`/`K`/`Enter`/`O` and the rest are identical to the list's.
- Match highlighting is **not done.** The client cannot reconstruct the FTS5 trigram match ranges exactly, so it would produce wrong emphasis (§10-4).

**(5) By state**

| State | Presentation |
|---|---|
| `q === ''` | empty the result area and show one line, "Type something to search." No recent-query storage |
| loading | 5 skeleton rows. The input is never locked (a re-request while typing aborts the previous one) |
| no results | "No links match `쿠버네티스`." + `Clear filters` if a filter is active |
| error 400 (cursor format, etc.) | throw the cursor away and re-request the first page automatically, one time; on failure, an error block |
| offline | input disabled + the last results kept |

**(6) Responsive** — `< 560`: the period and tag dropdowns drop to their own line under the input and the result meta goes below that. `≥ 1024`: input max width `--w-search-input` (the value is 10 §2.3), left-aligned (never centred — it must share the left baseline with the list so rows do not shift).

**(7) Motion** — replacing results has **no fade** (a flicker on every keystroke gets in the way of typing). The previous results stay in place and are substituted by the new ones; while loading, only the result-meta line drops its opacity to `.55`. Only filter chip add/remove uses `--dur-2`(180ms).

**(8) iOS difference** — `.searchable` replaces the search input. The remaining toolbar elements are **split one axis at a time** — `.searchScopes` is a single segmented-control row, which only holds for a 3~5-value set, and the tag dictionary has 30~50 entries (§5-1), so the two axes cannot share one scope bar.

| Web element | iOS placement |
|---|---|
| the 4 period presets (all / last 7 days / 30 days / this year) | `.searchScopes` — exactly four values, which fits a segmented control |
| tag filter (dictionary of 30~50) | a toolbar `Menu` **single-select picker** (the contract's `tag` parameter is one value). Not in the scope bar |
| result meta (`mode` + "N loaded") | split out as the **first section header** of the result list. `.font(.footnote.monospaced())` + the color matching `fg-3` |
| `Clear` | the system clear button of `.searchable` does the job (no separate button) |

The search screen is P1 on the web (§9) and **an explicit cut candidate for M4 on iOS** (08 §3 "M4 iOS" — if it slips, the tag filter substitutes and it carries over alongside M5). If it is cut, iOS carries rediscovery with the list's `tag` filter, and the web backfills the search demand in the meantime.

---

## 5. Screen 4 — Tags

**(1) Purpose** — work directly on the controlled dictionary (30~50 entries) that feeds automatic tagging, and see which tags actually get used. **This screen is the only path by which the user can inspect and change a facet (the basis of a tag's color) by means other than color** (10 §5.5, supporting measure 4).

**(2) Layout**

```
+--------------------------------------------------------------------------------+
| 태그 사전                                              [ + 태그 추가 ]           |
| 42개 · 링크 1,284건                                                            |  mono meta
+--------------------------------------------------------------------------------+
| 이름          분류        별칭(aliases)                          링크    액션   |
| ────────────────────────────────────────────────────────────────────────────── |
| 개발          만드는 것   dev  development  프로그래밍            312    [편집] |
| 쿠버네티스    만드는 것   kubernetes  k8s                          87    [편집] |
| 영상          형식        video                                    64    [편집] |
| 디자인        만드는 것   design                                    0    [편집] |  ← 0건도 여기선 보인다
| 여행          분류 없음   travel                                    3    [편집] |
+--------------------------------------------------------------------------------+
```

**(3) API field mapping**

| UI | Contract |
|---|---|
| row | `GET /api/v1/tags` → `Tag[]` |
| name | `Tag.name` (sans/500) |
| facet | `Tag.facet` (`TagFacet` = `craft`/`media`/`life`/`neutral`) — Korean labels `만드는 것`/`형식`/`세상과 일상`/`분류 없음` |
| aliases | `Tag.aliases[]` — **mono chips** (strings for machine matching, hence R2) |
| link count | `Tag.link_count` (mono, `tabular-nums`) — counting non-deleted links |
| header summary | `Tag[].length`, `GET /api/v1/stats` → `total_links` |
| create | `POST /api/v1/tags` `{name, aliases?, facet?}` — `name` required, case-insensitive UNIQUE (duplicate gives 400). Omitting `facet` means `neutral` |
| edit | `PATCH /api/v1/tags/{id}` `{name?, aliases?, facet?}` |
| delete | `DELETE /api/v1/tags/{id}` — `link_tags` goes with it via FK CASCADE |

**(4) Interaction**

- Clicking the name → navigate to `/?tag={name}` (the path that joins dictionary management to browsing).
- `Edit` is an inline expanding row (no separate dialog): name input + **facet select** + alias token input (`Enter`/comma commits a token, `Backspace` deletes the last one) + `Save`/`Cancel`/`Delete`.
- **The facet select is one four-way choice** (`@radix-ui/react-select`, §1.8). The options are fixed at `만드는 것(craft)` / `형식(media)` / `세상과 일상(life)` / `분류 없음(neutral)` and there is **neither free input nor a color picker** — color is not in the contract; the client maps facet to token (10 §5.2). One line under the select exposes the criteria: "craft = references used directly in the work / media = read, watch, listen / life = about me." A mini chip of that facet (fill 1) sits to the left of each option, joining color and name on the same line.
- **A new tag is born `neutral`.** The facet default on `+ Add tag` is `분류 없음`, and if the user does not choose, it saves as such (identical to the contract default).
- Facet changes are applied optimistically — the moment the select changes, that row's chip color changes and `PATCH {facet}` goes out. On failure, roll back to the previous value + error toast. After saving, invalidate the link query caches (`['links']`, `['search']`) (chip colors inside rows change).
- The screen says out loud that editing aliases is the cheapest contribution to accuracy — one line under the alias input: "Aliases are what the rule tagger matches as strings."
- Deleting **requires a confirmation dialog** (`AlertDialog`) — **the only place in the whole app that uses one** (§1.2, 10 §4.11). Unlike deleting a link, there is no undo here (no restore API + `link_tags` CASCADE). The copy states the blast radius as a number: "Deleting `개발` removes this tag from its 312-link attachments as well. This cannot be undone." The number is `Tag.link_count` verbatim. The delete button is **the 10 §4.1-danger variant** (transparent background + `--danger` text + a `1px --danger` border), cancel is `secondary`, and default focus is on cancel. **A filled danger variant does not exist** (10 §2.1.4-b).
- Only two sort toggles: `link count` desc (default) / `name`. **Never sort by facet** (10 §5.3 — if color governs position as well, the two channels duplicate each other). **Sorting by "times attached in the last 30 days" is not implemented, because the contract has no such data** (§10-2).
- After a tag is deleted or created, invalidate the link query caches (`['links']`, `['search']`) (chip display changes).

**(5) By state**

| State | Presentation |
|---|---|
| loading | 10 skeleton rows |
| empty | "The tag dictionary is empty. Automatic tagging only attaches tags that exist in the dictionary." + `+ Add tag` |
| create error (duplicate 400) | inline `warn` under the name input: "That name already exists." (case-insensitive UNIQUE) |
| facet change failed | put the select back to the previous value + an `error` toast. The row's chip color rolls back with it |
| delete succeeded | row removed + a toast ("Deleted `개발`.") — no undo (no restore API, and CASCADE cannot be reversed) |
| offline | reading stays; `+ Add`/`Edit`/`Delete` disabled |

**(6) Responsive** — `< 560`: the table becomes a card-style two-line list (line one: name + facet + `link_count`; line two: aliases with horizontal scroll), and the actions move into a `…` menu on the right of the row. `≥ 560`: a five-column table (name · facet · aliases · links · actions). `≥ 1024`: it keeps the list content width (10 §2.3) — there is no inspector on this screen, so it does not get any wider.

**(7) Motion** — the inline edit row expands with **`grid-template-rows: 0fr → 1fr`** `--dur-2`(180ms) plus the content `opacity` entering `--dur-out`(120ms) later. **`height` is never animated** (10 §6.1 — `top`/`height` forbidden). Row add/remove follows the §1.7 table exactly. Count numbers are **never animated**.

**(8) iOS difference** — deletion goes through `List` + `EditButton`, and alias editing is pushed into a separate `NavigationLink` detail (on a small screen an inline expansion destroys the touch targets).

---

## 6. Screen 5 — Detail = the inspector, with editing folded in

**(1) Purpose** — unfold the entire machine output for one link (meta, jobs, errors) and fix its tags and note right there.

**(2) Layout**

```
+----------------------------------------+
| Kubernetes 네트워킹 정리          [×]  |  head/600, 최대 3줄
| kubernetes.io                          |  mono, fg-3
|                                        |
| [       썸네일 16:9 (thumb_url)      ]  |  없으면 슬롯 자체 제거
|                                        |
| [ 원문 열기 ⏎ ]        [재시도] [삭제] |  primary / outline / outline
| ────────────────────────────────────── |
| 태그                                   |
| [#개발 ×] [#k8s ×] [+ 태그 추가]       |  fill 0/1 (10 §5.2), 보더 없음
|   rules 0.82 · rules 0.74 · manual —   |  mono, fg-3 (출처·신뢰도)
| ────────────────────────────────────── |
| 메모                                   |
| [ 나중에 다시 볼 것                  ] |  자동 저장(blur / Cmd+Enter)
| ────────────────────────────────────── |
| 설명                                   |
| 쿠버네티스 네트워크 모델은 …           |  body, description 전문
| ────────────────────────────────────── |
| 메타                                   |
| 저장       2026-07-21 14:03            |  좌: label / 우: mono
| 발행       2026-06-30                  |  published_at (null이면 행 제거)
| 작성자     조희찬                      |  author ('' 이면 행 제거)
| 종류       article                     |  content_type
| 길이       12분 04초 / 2,140 단어      |  duration_sec / word_count
| 언어       ko                          |  lang
| ────────────────────────────────────── |
| 잡                                     |
| scrape  done    tag  done    thumb  —  |  mono. 필드 부재 = "—"
| 오류    dial tcp: i/o timeout           |  error ('' 이면 섹션 제거)
+----------------------------------------+
```

**(3) API field mapping** — `GET /api/v1/links/{id}` → `LinkDetail` (`Link` + detail fields)

`summary` (the M5 extractive summary) is **inspector-only**: it is drawn as a "Summary" section **directly above** the "Description" section, with the newline-separated sentences each rendered as a `<p>` (joining sentences pulled from all over a document into one paragraph reads like "generated writing"). The summary is `text-fg-1` and the description `text-fg-2` — same typeface and size, differing only in brightness hierarchy, so "the sentences a machine picked" contrast with "the description the publisher wrote". **On an empty string the section itself is not drawn** (no skeleton, no "no summary" copy — the server returning an empty value as a quality guard is normal). Because the server blanks the summary when it is effectively the same as the description, the two being different is guaranteed whenever both are visible — which is why the client has no duplicate-detection logic. **It never goes on a card** (it is not in the list response).

| UI | Field | Notes |
|---|---|---|
| title | `title` | if the string is empty, `domain` → `url` |
| domain / original | `domain` / `url` | `url` gets `target="_blank" rel="noreferrer"` |
| thumbnail | `thumb_url` | if `null`, remove the slot (never leave a grey box) |
| tag chips | `tags[]`: `id`, `name`, `source`, `confidence` | `role: 'readonly'` — `manual` is fill 1 (that tag's facet tint), everything else fill 0. The facet is resolved from the `GET /api/v1/tags` cache (§3(3)). A `null` `confidence` renders as `—` |
| note | `note` | editing = `PATCH {note}` |
| description | `description` | the detail shows the full text, untruncated |
| meta | `published_at`, `author`, `content_type`, `duration_sec`, `word_count`, `lang` | fallback rules §1.3 |
| jobs | `jobs.scrape` (required), `jobs.tag`, `jobs.thumb` | **an absent field ≠ failed.** Absent is `—`, `failed` is `danger` text |
| error | `error` | on an empty string, remove the section entirely |
| status | `status` | shown as a `--size-rail` rail at the top left of the inspector (no badge). The thickness value and the four state colors are 10 §2.3 · §4.7 — **the same thickness as a row's** |
| retry | `POST /links/{id}/retry` → `202 {id, status}` | **exposed only when `status === 'failed'`.** Otherwise 400 |
| delete | `DELETE /links/{id}` → `204` | soft delete |
| tag candidates | `GET /api/v1/tags` → `Tag[]` | combobox options (filtered and displayed by `Tag.name`) |

**(4) Interaction — this is where editing is folded in**

- **Adding a tag**: `+ Add tag` or `E` → **a combobox we implemented ourselves**. Radix has no Combobox primitive, so we own the ARIA combobox pattern via `Popover` + a text input + a filtered list (§1.8 — `role="combobox"` + `aria-expanded`/`aria-controls`/`aria-activedescendant`, the list as `role="listbox"`/`role="option"`, `↑↓` to move the active option, `Enter` to commit, `Esc` to close). The options are the whole dictionary (`GET /tags`) and the input filters them by case-insensitive substring. Selection applies optimistically at once + `PATCH {tags: [...current names, new name]}`.
- **When a name that is not in the dictionary is typed**: the contract answers `PATCH` with a 400-error. So one `warn` item appears at the bottom of the combobox — "`머신러닝` is not in the dictionary. [Add it to the dictionary and attach]". Running it does `POST /api/v1/tags {name}` → and on success continues with `PATCH {tags}`. If the first of the two requests fails, the second is not sent and an inline error appears.
- **Removing a tag**: the chip's `×`, or `Backspace` with the chip focused. `PATCH` sends **the entire array of remaining names** (there is no partial-delete API). Removing everything = `tags: []` (full removal per the contract), which differs from `null`/omitted.
- **Note**: saved on blur or `Cmd/Ctrl+Enter` (§1.2 — immediate save inside a save form). If the value did not change, no request goes out. The input is never locked while saving.
- **Retry**: `R`. The moment it is pressed the rail switches to the in-progress pulse and the §1.5 polling starts. The button is an **achromatic outline** (10 §4.1 `secondary`) — the color-blind mitigation rule that chromatic ink and danger are never placed side by side as fill colors in the same component (10 §2.1.4-b; danger is mandatory, while parallel `warn` fill was softened to a recommendation by the 2026-07-24 hue re-selection).
- **Delete**: `Backspace`/`Delete`. **It runs immediately with no confirmation dialog** and is reversed by the `undo` toast (duration is 10 §4.10; the copy about the limits of undo is §1.5). The inspector closes and the cursor moves to the next row.
- Closes with `Esc`. Even when closed, the list scroll position and the `J`/`K` cursor are preserved.
- Entering the deep link `/links/123` directly: load the list behind it and render with the inspector open. Back = close the inspector.

**(5) By state**

| State | Presentation |
|---|---|
| loading | the panel skeleton is immediate (the width is reserved in advance — 10 §2.3), the inside is five skeleton blocks. Values already in the list row (`title`/`domain`/`tags`) are **filled from the cache first** and only the rest is skeleton |
| in progress | when `status` is not a terminal state, polling fills values into the meta and jobs sections. No spinner |
| failed (`status: 'failed'`) | rail `danger` at the top, the `error` section shown, the `Retry` button exposed |
| `jobs.thumb === 'failed'` + `status === 'done'` | **not shown as a failure.** `thumb failed` on the jobs line only; the link is fine |
| 404 | replace the panel content with "This link was deleted or does not exist." + `Close`, and drop that card from the list cache |
| error (500) | keep the content + an inline `Retry` bar at the top |
| offline | show cached values + disable tag/note input, retry and delete |

**(6) Responsive**

| Width | Form |
|---|---|
| `< 560` | full-screen sheet. `←` (close) at the top left, the action button row pinned to the bottom |
| `560~1023` | bottom sheet `85dvh`, with a drag handle; the list behind it is scroll-locked |
| `≥ 1024` | pinned right panel (panel width and list minimum width = 10 §2.3), the list is `flex: 1`. Background scrolling stays (it is not locked) |

**(7) Motion** — open `--dur-3`(260ms) `--ease-enter`, close `--dur-close`(200ms) `--ease-ui`. `visibility` transitions along with `step-end`/`step-start` so a closed panel is removed exactly from focus and hit testing. Chip add/remove `--dur-2`(180ms). The sheet uses `transform: translateY` only — **height animation forbidden** (10 §6.1).

**(8) iOS difference** — `.sheet` + `.presentationDetents([.medium, .large])` + a drag indicator replace the inspector, and delete/retry go into the trailing side of the bottom toolbar (within one-handed reach) rather than a top-right menu.

---

## 7. Screen 6 — Tag edit

**No separate screen is built.** Per the §0-decision, the editing UI lives entirely inside the inspector (§6-4). This section is a pointer for anyone looking for "where did editing go", and the one place where the three traps of the edit contract are collected.

| Trap | Contract fact | What the UI must honour |
|---|---|---|
| full replacement | `PATCH {tags}` **replaces this link's tags wholesale** | always send "the entire array of names currently visible on screen". Never send a delta |
| three-value distinction | `tags` omitted/`null` = keep, `[]` = remove everything | when saving only the note, **do not include the `tags` key at all** (sending an empty array wipes the tags) |
| dictionary enforcement | a tag name that does not exist gives 400 | allow free input, but check against the dictionary before submitting → if it is missing, steer into the `POST /tags` flow |

One more fact: a manual edit is recorded on the server as `link_tags(source='manual', confidence=NULL)` + `tag_feedback(added/removed)`, and that is the training data for M5 re-ranking. So marking only the `manual` chip with a one-step fill (that tag's facet tint) (R1) is not decoration but **a signal that training data is being made**. That mark cannot be undone from the UI — there is no API that turns `source` back into `rules`.

**The re-evaluation triggers are owned by 10 §5.2-side** — `neutral` chips over 40% and `craft` over 75%, those two. The older trigger, "if the `manual` share passes 30%, roll the tint back to inspector-only", has been **retired**: the one-step fill is that tag's own facet tint rather than the brand accent, so more `manual` does not bleed brand color.

---

## 8. Screen 7 — Settings

**(1) Purpose** — put the API key in, check the server is alive, and see **whether this habit is alive**.

It used to read "see how much has piled up", which runs head-on into (3-1) below deciding that "the pile is not this product's goal". If the purpose is a total, the screen becomes a total.

**(2) Layout**

```
+--------------------------------------------------------+
| 설정                                                   |
| ────────────────────────────────────────────────────── |
| 연결                                                   |
| API 키   [ •••••••••••••• ]              [ 저장 ]      |
|          서버의 PUSHPOINT_API_KEY. 이 브라우저의       |
|          localStorage에 저장됩니다.                    |
|          [ 연결 확인 ]   서버 정상 · 키 유효           |
| ────────────────────────────────────────────────────── |
| 저장 도구                                              |
| 북마클릿  [ Push-Point 저장 ]  ← 북마크바로 끌어놓기    |
| 확장      [ 서버 주소 복사 ]                           |
| ────────────────────────────────────────────────────── |
| 리듬                                                   |
|   최근 30일 가운데 18일 저장했어요.                    |  ← 문단이 먼저 온다
|   지금 12일째 이어가고 있어요.                         |
|                                                        |
|   12일 연속 — 4주(28일)까지 16일 남았어요              |  accent, 목표선
|                                                        |
|   최근 30일                              18일 저장     |
|   ▁▂▅▁▃▇▂▁▁▄▂▁▃▅▁▂▁▁▆▃▁▂▄▁▁▃▂▁▅▂                      |  by_day 30칸(계약 보장)
|   30일 전                                    오늘      |
|                                                        |
|   무엇을 모았나              누르면 그 목록으로        |
|    ● 만드는 것 14                                      |  facet 묶음, 개수 mono
|      dev 3 · backend 2 · …                             |
|   전체 1,284                                           |  총계는 맨 뒤 한 줄
| ────────────────────────────────────────────────────── |
| 스프레드시트                                           |
| ────────────────────────────────────────────────────── |
| 모양                                                   |
| 테마     ( 라이트 | 다크 | 시스템 )                    |  3-state 세그먼트
+--------------------------------------------------------+
```

The order above is the order `SettingsScreen.tsx` actually draws. The first mockup put appearance above rhythm and left the spreadsheet section out entirely, so the very commit that fixed this section failed to describe its own result.

**(3) API field mapping**

| UI | Contract |
|---|---|
| saving the API key | no server call. `localStorage` → the `Authorization: Bearer` on every request afterwards |
| connection check, step one (liveness) | `GET /healthz` (auth-exempt) → `{status: "ok"}` |
| connection check, step two (key validation) | one `GET /api/v1/tags` — the lightest endpoint that requires auth. A 200-response means the key is valid, `401 unauthorized` means it does not match |
| paragraph, streak, weekday | **derived on the client** from `by_day[]` and `links_this_week`. No new server field is demanded |
| the strip for the last 30 days | `by_day[]` = `{date: "YYYY-MM-DD", count}` (localtime) |
| top tags | `by_tag[]` = `{name, count}` — the top five only, clicking goes to `/?tag={name}` |
| total links | `total_links` — one line, last |

Per the contract `GET /api/v1/stats` can answer 401/404-class errors. A 401-response goes to the §1.4-key banner; a 404-response becomes the single line "Statistics are unavailable" and **the whole section is hidden** (no error block is left behind).

**(3-1) Why this section is not a parade of totals — revised 2026-07-27**

The original spec opened with the two numbers `1,284 / 37` set large. iOS was implemented first and built **a different screen** from the same contract, and the comment over there wrote down why: *"A dashboard puts numbers down and offloads the interpretation onto a person. Judging what changed and how means composing a sentence in your head every time, and that is the screen's job."*

The two clients were building different products from the same data, and **iOS was the one that was right**, so this spec is being aligned to it. The order flips: **paragraph → streak → rhythm → weekday → tags → total.** The total is last because it is the least useful number — the pile is not this product's goal.

The weekday bars were added on 2026-07-28. The bars over 30 days answer "how steady is this" but cannot answer "when", and that answer comes out of the dates in `by_day` with no server change.

The derived calculations (streak, week-over-week, weekday, dominant interest) live as pure functions in `frontend/src/lib/rhythm.ts` and are verified by `rhythm.test.ts`. That is because **the same rule has three implementations** — iOS `StatsView.swift`, `scripts/streak.sh`, and this one. When the three diverge, the phone says "12 days" and the terminal says "11 days" and there is no basis for believing either.

**The contract does most of that agreement for us.** `by_day` is a **30-slot array** padded through empty days, and **the last slot is today** (api/openapi.yaml). So all three implementations count from the back instead of guessing "today" from their own clock. Before that guarantee, on 2026-07-28, each one lined the dates up for itself and **the web and iOS were drawing the bars for 30 days in opposite directions.** One shared rule remains: **not having saved yet today does not break the streak up to yesterday** (nobody trusts a metric that drops to zero just after midnight).

Agreement is checked by having `just web-test` and `just streak-selftest` both read `testdata/streak-cases.json` — the full story is in [13 §3](13-CLIENT-PARITY.md).

**Putting the streak and the goal line on screen is a deliberate choice.** A streak of 4 weeks is this product's success criterion (08 §2 M6), so showing the days left to the goal creates the Goodhart pressure of "saving one meaningless link to keep the streak going". We knew that cost and chose motivation anyway — the same judgement is written in the head comment of `scripts/streak.sh`, and **the verdict itself is made by that script, not by the screen** (whatever has an exit code is the judge).

**The connection check has two stages — `/healthz` alone cannot validate a key.** In the contract `GET /healthz` is `security: []`, i.e. **exempt from auth**. It answers with a 200-response even when the key is wrong, so this call alone tells you nothing beyond "the server is fine". The purpose of this screen is to confirm the key actually works, so the sequence is split in two.

| Stage | Call | Verdict |
|---|---|---|
| 1 | `GET /healthz` | a 200-response confirms the server is alive → on to stage two. No response / network failure → sentence three immediately |
| 2 | `GET /api/v1/tags` (with Bearer) | 200 → sentence one / `401 unauthorized` → sentence two / anything else → a single `error.message` line |

There are exactly three result sentences.

1. **Server fine · key valid** — achromatic. Stage 1 passed and so did stage two.
2. **Server fine · key mismatch (401)** — one `danger` line + a `Re-enter key` link (focuses the input). The server is alive, so it does not say "connection failed".
3. **Cannot reach the server** — one `danger` line. The case where stage one got no response (§1.6, the same cause as the offline bar).

The stage-two response goes straight into the tag query cache (`['tags']`) — it is not a request thrown away for validation but data that actually gets used.

**(4) Interaction**

- Saving the key invalidates the whole cache with `queryClient.invalidateQueries()` (queries that 401'd under the old key run again). Save confirmation is a `Saved` next to the button for two seconds — not a toast (§1.4-1).
- The key input is `type="password"` + `autoComplete="off"`. 1 show/hide toggle on the right.
- `Check connection` is an explicit button and runs the two stages above once each, in order. It never polls automatically (needless traffic against a local server).
- Theme 3-state: `setThemePref('system')` currently being unreachable from the UI is a bug, so it is exposed as three segments.
- The bookmarklet is offered as a `javascript:` anchor, with a sentence beside it saying it is meant to be dragged. It includes the note that clicking it does nothing (browser policy).

**(5) By state**

| State | Presentation |
|---|---|
| no key set | keep a `warn` banner at the very top + auto-focus the key input. The stats section is hidden (a 401-response is a foregone conclusion) |
| checking | spinner in the button, label `Checking…` (the two stages are treated as one progress state) |
| check succeeded | `Server fine · key valid` (achromatic — success never uses color) |
| check failed (401) | one `danger` line, `Server fine · key mismatch (401)` + `Re-enter key` |
| check failed (no response) | one `danger` line, `Cannot reach the server` |
| rhythm loading | skeleton blocks (height reserved only). No count-up animation, and the skeleton does not move either (10 §4.9). It waits for the dictionary (`GET /tags`) too — drawing first would let the interest sentence cut in a beat later and make the paragraph grow while you are reading it |
| rhythm failed (401) | hide the whole section — the §1.4-key banner is already saying why |
| rhythm failed (anything else) | one `danger` line + `Retry`. **Do not hide it** — hiding 500s and timeouts too makes it indistinguishable from "a build with no rhythm section" |
| offline | only the connection-check button stays enabled (that is the offline diagnostic); everything else is disabled |

**(6) Responsive** — a single column at every width, max width `--w-form` (the value is 10 §2.3 — the width at which one line of a form reads well). The strip for 30 days and the weekday bars shrink their cell width rather than scrolling horizontally even at `< 560` (minimum cell width `--size-spark-min`, an explicit exception to the 10 §2.3-twelve-step scale). (The line that used to sit here, "the two stat numbers stack vertically", should have been deleted when (3-1) removed those two numbers.)

**(7) Motion** — theme switching gets **no** color transition (a full-screen crossfade feels slow and collides with the `data-loading` seal). Only the connection-check result text is replaced over `--dur-2`(180ms).

**(8) iOS difference** — in standalone mode there is no key for the user to enter (the app generates a random one per launch and hands it to its own in-process server). In home-server mode the key goes into the App Group's shared Keychain rather than localStorage, and **no theme toggle is offered** (HIG: avoid per-app appearance settings — an explicit exception for an otherwise equal client).

---

## 9. Implementation priority

3 criteria: (a) the paths used daily come first, (b) what the contract already implements comes first, (c) the signatures (S1 rail / S2 the row that fills in) go inside the MVP — pushed to later, they never go in at all.

### P0 — MVP (this alone has to be usable daily)

| Order | Item | Done when |
|---|---|---|
| 1 | the token layer (`@theme`, **including the 6 facet tokens**) + font stack + the `data-loading`/`data-reduce-motion` seal | 0 occurrences of `dark:` utilities or raw hex outside generated output (§sweep); the 10 §9-side CSS smoke check, contrast gate and color-blind gate all pass |
| 2 | the shell: top bar + offline bar + API key banner + one toast variant | all three paths — 401, offline, success — actually render |
| 3 | **the list screen**: fixed-height rows (10 §2.3), the status rail (S1), cursor infinite scroll, `?tag`/`?status` | scrolling holds on a 100k-row seed; all four rail states verified |
| 4 | **the save composer + the optimistic row (S2) + progress polling** | save → the row appears at once → title/tags/thumbnail fill in place |
| 5 | **the inspector (detail + tag/note editing folded in)** | the three `PATCH` cases — full replacement, `[]`, omitted — plus `retry`/`delete` + undo |
| 6 | settings: API key + connection check + theme 3-state | after saving the key, the queries that 401'd recover automatically |
| 7 | keyboard: `S` `/` `J/K` `Enter` `O` `E` `N` `R` `Backspace` `Cmd/Ctrl+Enter` `Esc` `?` | the `?` overlay is 1:1 with the §1.2 table (keys not in that table are not bound) |

Once P0 is done, **saving, rediscovery and tidying close on the web alone.** That is the point at which "the web client, first pass" is declared complete.

### P1 — straight after

| Item | Why |
|---|---|
| ~~**search screen**~~ **→ implemented (2026-07-25)**. `q` debounced 120ms → `?q` replace, `mode`(fts/like) + the "N loaded" meta, the `?period` preset, `?tag`, reusing the card board | built after confirming `/api/v1/search` actually works. Only exposing rank in the inspector is deferred (§4(3), contract gap) |
| ~~**tags screen**~~ **→ implemented (2026-07-25)**. dictionary table + inline editing (name, alias tokens, **the four-way facet choice**) + creation + delete confirmation dialog + two sort keys | aliases are the cheapest way to raise tagging accuracy. The facet select includes the optimistic update and the `['links']`·`['search']` invalidation |
| command palette (`Cmd+K`) | the precondition for taking navigation out of the top bar. Hand-built from `Dialog` + our own list (§1.8) |
| switch to virtual scrolling (past 200-card render) | measure it once real data has piled up, then put it in |

### P2 — when there is room

- ~~the stats section in settings~~ — **implemented 2026-07-27** (§8 (3-1)). It moved up from P2 because iOS built it first and the two clients diverged
- bookmarklet + `/save?url=` prefill
- touch swipe actions (delete/retry)
- `Cmd/Ctrl+V` instant save (via the `paste` event — §1.2)

### P3 — explicitly deferred

- multi-select + bulk delete/tag (no batch API — §10-3)
- search term highlighting (§10-4)
- tag sorting by "times attached in the last 30 days" (no data — §10-2)
- an offline save queue (that is iOS's role)
- view switcher, density toggle, theme presets (for a single user, options are a tax)

---

## 10. Contract-gap list (what cannot be done now, and why)

This section is the single answering place for "why isn't that feature there", and it is where you start when judging whether to grow the spec. To grow it, fix `api/openapi.yaml` first and let `just gen` + `just web-gen` follow.

**Tag color is not here — it is not a gap, it was solved.** The contract has the `TagFacet` enum and `Tag.facet` / `TagInput.facet`, and the client maps facet → token (10 §5.2). **`facet` being absent from `LinkTag` is a decision, not an omission** — the list screen already holds the whole `GET /api/v1/tags` for the filter bar, so `Map<tagId, facet>` solves it, and at the 100k-link target, shipping a facet string on every `LinkTag` grows the payload by as many tags as each link has. A new tag briefly rendering `neutral` on a cache miss is **the correct fallback**.

1. **There is no realtime progress notification.** With no SSE/WebSocket endpoint, the `pending → done` transition is known only by polling (§1.5). At a handful of links, polling is cheap enough. If saves pass a few dozen a day, or batch-import progress becomes necessary, that is when a stream becomes a candidate for the spec.
2. **There is no per-tag count for the last 30 days.** `Tag.link_count` is a lifetime total, and `Stats.by_day` is link counts, not per-tag. So `link_count` desc is the best available sort for chips and the dictionary. Having the client fetch every link and aggregate collides head-on with the 100k-link target, so it is not done.
3. **There is no batch API.** Delete and tag edits are per link, so multi-select becomes N requests. At single-user scale the gain is small, so it is deferred.
4. **There is no match-range information for search.** The response carries no snippet or offset, only `rank` (bm25). If the client guessed the trigram match ranges to highlight them, it would produce wrong emphasis — so it does not.
5. **There is no tag count scoped to the filtered result.** `link_count` is global, so "hide chips with 0-count inside the current filter" is impossible. Instead, **only tags whose global `link_count === 0` are hidden from the chip bar on the list screen** (the tag management screen must show 0-count tags too — they are exactly what dictionary management is for).
6. **There is no restore endpoint for a soft delete.** Undo is implemented as a re-POST of the same `url`, and `status` returns to `pending` (§1.5). The UI copy does not hide that loss.
7. **There is no total count.** `LinkPage` has `links`/`next_cursor` and `SearchPage` has `mode`/`links`/`next_cursor`, and `total` appears nowhere — not counting the total is exactly what makes keyset cursor pagination fast (a full COUNT collides with the 100k-row target). So neither the list nor search ever writes "N total", the only number shown is **the running total the client has loaded so far**, and the label says so ("N loaded"). If a total is genuinely needed, adding `total` to `LinkPage` and `SearchPage` in `api/openapi.yaml` is the precondition. (`Stats.total_links` on the settings screen is a separate thing — it is the lifetime archive total, not the count of a filter or search result.)
8. **The inspector cannot read the search `rank`.** `rank` (bm25) exists only on `SearchResult` (a list item) and not on the `LinkDetail` the inspector opens. So the "rank in an inspector diagnostics slot" of §4(3) is impossible from the inspector's own query — the rank held by the search result would have to be carried through navigation when the inspector opens, and that being separate work, it was deferred in the 2026-07-25 implementation. The card does not show rank anyway (same information hierarchy as the list), so there is no gap visible to the user.

---

## 11. Follow-up work

- Replace the stack sentence in `.claude/rules/frontend.md` and `frontend/README.md` — "shadcn/ui" → "we own the Radix primitives" — and align the §1.8-dependency table with that stack description.
- When the §1.8-packages are actually installed, pin the versions and commit them in `frontend/package.json` — this document is the source for **what goes in and why**, and `package.json` is the source for versions.
- Declare implementation complete only after `just fmt` / `just lint` / `just test` / `just gen-check` / `just web-gen-check` / `just web-build` all pass and the output is presented.
