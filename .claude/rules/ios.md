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
