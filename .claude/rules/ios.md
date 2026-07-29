---
paths:
  - "ios/**"
---

# iOS workspace rules (M4)

- SwiftUI. No React Native (a v1 leftover).
- The heart of the product is the Share Extension: one tap in the share sheet to save complete in under 2 seconds is the DoD.
- Self-contained mode stores **no** API key: the app mints a fresh random one per launch for its in-process server, and the Share Extension bypasses HTTP entirely by writing to the shared SQLite. The Keychain (shared via app group) is where a **home-server** key goes once that mode exists. Home-server access goes through Tailscale — use the IP form (`http://100.x.y.z:8420`), since a hostname over plaintext HTTP is blocked by ATS. Note `NSAllowsLocalNetworking` does **not** cover Tailscale's 100.64/10 — that exemption is untested here.
- The API client uses code generated from `api/openapi.yaml` by **swift-openapi-generator** (Apple official, URLSession transport) — never hand-write API request/response types (`docs/v2/06-API-SPECIFICATION.md` is commentary).
- The App Group local queue (review item ⑥) was **not built** — the extension writes to the shared SQLite directly, so there is no POST to lose and nothing to drain. `docs/v2/04-DATA-FLOW.md` §7.2 keeps the original design as the revert path if `0xdead10cc` shows up on a real device; §7.4 is what actually runs. Item ⑦ (ATS / low power mode / On-Demand VPN / developer account) is still open.
- The list uses cursor-based infinite scroll (`next_cursor`) — never assume page numbers.
- Display fallback: when og and title are missing, the server returns `title` as an empty string as-is (it does not hide the fact). When `title` is empty, the app shows `domain` instead (then `url`) — preventing empty cells is the client's responsibility.

## Build inputs that go stale silently

- **`ios/Frameworks/` is a gitignored local build product, and nothing about a `git pull` refreshes it.** Run `just ios-bind` after any change under `backend/` — `just ios-build` now depends on `just ios-bind-check`, which compares a content hash of the binding's inputs (non-test Go sources, `migrations/*.sql`, `go.mod`/`go.sum`, `extension/src/extract.js`) against the stamp `ios-bind` writes. It is a **local** gate: CI has neither macOS nor gomobile, exactly like `ios-api-gen-check`.
- Why it exists: the binding was two days stale on 2026-07-28 and the app carried **30 of 42 tags** because migrations 0008–0011 were not in it. The symptom on screen was "why does this one link have no tags", and nothing pointed at the binding. Test files are excluded from the hash on purpose — a 15-minute rebind after editing a test is how a gate gets bypassed.

## Custom fonts

- The app ships **Wanted Sans / Geist Mono** (OFL) from `ios/PushPoint/Resources/Fonts/*.ttf`, registered via `UIAppFonts` in `project.yml`. Sources are the same files the web serves as woff2 (§10 2.2.1); iOS cannot read woff2, so the ttf is a conversion of the same original.
- **Call `Font.custom` with the named-instance PostScript name, never the filename** (`WantedSansVariable-SemiBold`, not `WantedSans-Variable`). A wrong name does not error — it falls back to the system font, so the build passes and only the screen is wrong.
- **`.weight()` does not move a variable font's axis in SwiftUI.** Ask for the weight by naming its instance. `PushPointTests/FontNameTests` pins both of these; it was watched failing (everything resolved to Helvetica) before it passed.
- XcodeGen has **no target-level `resources:` key** — it is silently ignored. Resources go in `sources` with `buildPhase: resources`. The app target needs nothing because its files already sit under a scanned path.

## The list must show work happening

- **A non-terminal link has to visibly become a finished one without user action.** `ContentView.pollWhileWorking` runs while any visible link is non-terminal and stops on its own, so an idle archive issues zero requests; §1.4 S2 calls this the product's signature and specifies it as poller-driven.
- **Polling must not call `load()`** — that replaces `links` with page one and discards everything the user scrolled to. Use `pollRefresh()`, which writes matching links in place and leaves the cursor alone. The pagination UI test catches this; it already caught it once.

## Reading the simulator's database

- The app group DB lives under `.../Containers/Shared/AppGroup/<id>/data/pushpoint.db`. **Copy `.db`, `-wal` and `-shm` together** — SQLite is in WAL mode, so copying the `.db` alone reads a stale or empty database. This was misread three times in one session, twice producing a wrong conclusion and once producing an unusable backup.
