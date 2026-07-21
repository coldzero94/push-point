---
paths:
  - "ios/**"
---

# iOS workspace rules (M4)

- SwiftUI. No React Native (a v1 leftover).
- The heart of the product is the Share Extension: one tap in the share sheet to save complete in under 2 seconds is the DoD.
- Store the API key in the Keychain (shared via app group). Server access goes through Tailscale — use the IP form for the address (`http://100.x.y.z:8420`) (a hostname over plaintext HTTP is blocked by ATS).
- The API client uses code generated from `api/openapi.yaml` by **swift-openapi-generator** (Apple official, URLSession transport) — never hand-write API request/response types (`docs/v2/06-API-SPECIFICATION.md` is commentary).
- **Before starting implementation, review items ⑥ (App Group local queue — prevents save loss when the server is unreachable) and ⑦ (ATS / low power mode / On-Demand VPN / developer account) in `docs/v2/09-PLAN-REVIEW.md`.** The "POST then close immediately" pattern loses requests and is forbidden.
- The list uses cursor-based infinite scroll (`next_cursor`) — never assume page numbers.
- Display fallback: when og and title are missing, the server returns `title` as an empty string as-is (it does not hide the fact). When `title` is empty, the app shows `domain` instead (then `url`) — preventing empty cells is the client's responsibility.
