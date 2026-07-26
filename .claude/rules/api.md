---
paths:
  - "api/**"
---

# API contract rules (contract-first)

- `api/openapi.yaml` is the **machine source of truth for the API**. `docs/v2/06-API-SPECIFICATION.md` is human-facing commentary — when the spec changes, update 06 in the **same commit**. If the two disagree, openapi.yaml wins.
- Field types and status enums (`content_type`, `status`, `source`, etc.) must stay consistent with the schema in `docs/v2/05-DATA-SCHEMA.md`.
- All time fields are **integer unix epoch seconds** (`created_at`, `published_at`, etc.). No `format: date-time` strings.
- After adding or changing an endpoint, re-run `just gen` and commit the generated output (`backend/internal/api/gen/`) alongside. `just gen-check` blocks drift in CI.
- This contract has **three consumers**: backend (oapi-codegen v2.8.0), iOS (swift-openapi-generator), and web (openapi-typescript, `just web-gen`). A spec change is done only when all three generated outputs are regenerated and committed — web specifics in `.claude/rules/frontend.md`.
- CI blocks drift for all three, but not identically. Backend and web regenerate and diff (`gen-check`, `web-gen-check`). iOS cannot — regenerating needs macOS and Swift — so `ios-stamp-check` compares a committed hash of `api/openapi.yaml` against `ios/PushPoint/Generated/.openapi.sha256`. That catches the drift that actually happens (spec edited, iOS regeneration forgotten); it does not catch hand-edits to the generated Swift. Run `just ios-api-gen-check` locally for the complete check.
- Backward compatibility: this is a single-user app, so breaking changes (removing fields, changing types, changing paths) are allowed. But whoever makes the change owns consistency with the deployed iOS app version — update the app first, or in the same piece of work.
- The contract exempts exactly **two** endpoints from auth: `GET /healthz` and `GET /thumbs/{dir}/{file}` (`security: []`). Every other endpoint requires bearer auth — do not add new exemptions. The out-of-contract diagnostic route `/debug/pprof` is an intentional exception that uses **loopback-only blocking** (a stronger boundary) instead of bearer auth.

## Code generation stack, settled in the 2026-07-20 review

- oapi-codegen is **pinned to v2.8.0** — no `@latest`. It is the version verified to handle OpenAPI 3.1 generation; change it only deliberately, after reviewing the generated diff.
- The generate set is fixed at `types,chi-server,strict-server,spec` (identical to the justfile `gen` recipe).
- swift-openapi-generator (M4): use **CLI invocation with committed output, not the SPM build plugin** (for reproducibility and consistent drift checking).
- The Swift `allOf` value1/value2 wrapping issue is known — do not pre-contort the spec; **decide after measuring in M4**.
- swift-openapi-generator does not generate client code for securitySchemes — **inject the bearer token via a hand-written ClientMiddleware**.
- TypeSpec re-evaluation triggers: reaching 40+ operations, or the Node toolchain arriving anyway when web frontend work starts. Until then, hand-maintained openapi.yaml is canonical.
