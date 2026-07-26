# UI verification (how to check a screen without asking a human)

**The problem this solves.** Every UI failure this project has shipped was of one kind: types correct, screen wrong. Thumbnails blank because `thumb_url` is relative and produced a host-less URL. The card's 3:1 ratio applied to the image, so it was ignored. `.tags` on a `listTags` response that is an array. Seeding that ran after the state flipped to `.running`, so the list read an empty database. None of these were caught by the compiler, by `go test`, or by reading the code — and until 2026-07-26 the only way to catch them was to ask the user to look at their phone.

Three tools now cover this. They are **not alternatives**; they do different jobs. The real cost is not the number of tools, it is the number of committed test assets — so the tool with no committed assets is free to keep.

| | Job | Committed assets | Command |
|---|---|---|---|
| **Maestro** | Read the screen, drive it, keep portable flows | `maestro/*.yaml` | `just flow [file]` |
| **AXe** | Screenshot and coordinate input, no test file needed | none | `axe` (see below) |
| **XCUITest** | Deterministic CI gate with seeded fixtures | `ios/PushPointUITests/` | `just ios-uitest` |

## Reach for these in this order

1. **Looking at what is on screen right now** → `maestro hierarchy` (semantic text) or `axe screenshot` (pixels). No test file, no build.
2. **Checking a flow still works after a change** → `just flow`.
3. **Locking behaviour so it cannot regress** → add to `just ios-uitest`.

Never report a UI change as done on the strength of a successful build. Build success means it compiles, which is precisely the bar every failure above cleared.

## Maestro

Installed via `brew install mobile-dev-inc/tap/maestro`. Note the tap: Homebrew's bare `maestro` cask is an unrelated product.

```
maestro hierarchy                 # the whole view tree as JSON — this is how to see the screen
just flow                         # run maestro/smoke.yaml
just flow maestro/other.yaml
```

An MCP server is registered project-scoped in `.mcp.json` (`maestro mcp --no-viewer`), so its tools — `inspect_screen`, `take_screenshot`, `run`, `list_devices` — are callable directly rather than through the shell.

Flows in `maestro/` run against **whatever is actually in the simulator**, so they must not assert data content. They assert structure: the search field exists, the tabs switch, the stats sections render. Data-dependent assertions belong in XCUITest, which seeds its own fixtures.

## AXe

Installed via `brew install cameroncooke/axe/axe`. Wraps IDB; `describe-ui`, `tap`, `type`, `screenshot`, gestures.

```
axe screenshot --output /tmp/s.png --udid $UDID
axe tap -x 243 -y 823 --udid $UDID
axe type '검색어' --udid $UDID
```

Two things to know before using it:

- **Coordinates are points, not screenshot pixels.** The screenshot is 1206×2622; the coordinate space is 402×874 (3× scale). AXe reports "Tap completed successfully" for an off-screen coordinate, so a wrong-space tap looks like a working tap that changed nothing.
- **`axe describe-ui` returns an empty tree** for this app on Xcode 26.5 / iOS 26.5 (root frame `{{0,0},{0,0}}`, no children), which also disables `--label` and `--id` targeting since both resolve through it. Verified 2026-07-26 against a running app with a matching pid, after a clean terminate/relaunch and with Simulator.app frontmost. Use `maestro hierarchy` for semantic inspection instead; AXe's screenshot and coordinate input both work.

## XCUITest

`just ios-uitest`. The app launches with `-uitest`, which points it at a temp directory instead of the App Group and seeds fixtures through `POST /api/v1/links` (`ios/PushPoint/UITestMode.swift`). Three consequences worth keeping:

- Results do not depend on what happens to be in the simulator.
- The real archive cannot be touched by a test.
- A broken save contract fails during setup rather than passing silently.

On failure the suite attaches a screenshot and the accessibility tree — the only way to tell "not drawn" from "drawn differently" from "still loading". Extract them with `xcrun xcresulttool export attachments --path <bundle>.xcresult --output-path <dir>`.

Target rows in long lists by `accessibilityIdentifier`, not by display text: the dictionary has 40+ tags, and matching on visible copy breaks the test the moment the wording is edited.

## The duplication to watch

Maestro flows and XCUITest cases both describe the same screens. That is two sets of assets to update per UI change, which is exactly what the sweep rule in CLAUDE.md exists to prevent. It is tolerable now because the two cover different ground (structure-on-real-data vs behaviour-on-fixtures). **If they start asserting the same things, converge on one** — and the one to keep is whichever runs in CI.
