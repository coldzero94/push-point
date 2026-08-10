# UI verification (how to check a screen without asking a human)

**The problem this solves.** Every UI failure this project has shipped was of one kind: types correct, screen wrong. Thumbnails blank because `thumb_url` is relative and produced a host-less URL. The card's 3:1 ratio applied to the image, so it was ignored. `.tags` on a `listTags` response that is an array. Seeding that ran after the state flipped to `.running`, so the list read an empty database. None of these were caught by the compiler, by `go test`, or by reading the code — and until 2026-07-26 the only way to catch them was to ask the user to look at their phone.

Three tools now cover this. They are **not alternatives**; they do different jobs. The real cost is not the number of tools, it is the number of committed test assets — so the tool with no committed assets is free to keep.

| | Job | Committed assets | Command |
|---|---|---|---|
| **Maestro** | Read the screen, drive it, keep portable flows — **iOS and web** | `maestro/*.yaml` | `just flow [file]` |
| **AXe** | Screenshot and coordinate input, no test file needed (iOS) | none | `axe` (see below) |
| **XCUITest** | Deterministic CI gate with seeded fixtures (iOS) | `ios/PushPointUITests/` | `just ios-uitest` |
| **Chrome headless** | Rasterize an SVG/HTML, screenshot a URL | none | see "Web" below |

**Web screens can be inspected.** `list_devices` returns a `chromium` device; point a flow
at a URL instead of an `appId`. Do not repeat the claim that they cannot — it sat in
`docs/v2/ko/13-CLIENT-PARITY.md` §4 for a day without anyone calling `list_devices`, and it
was used to justify shipping web UI on "it typechecks and builds".

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

## Driving the share sheet (the M4 timing verdict)

`just save-timing` reads `save-timing.jsonl`, which **only the Share Extension
writes** — `SaveTiming.begin()` sits in `ShareViewController.viewDidLoad`, so the
measured span is extension-launch to save-complete and no unit test can produce
it. The verdict needs a real share, and the simulator can do it. Working recipe,
all coordinates in **points**:

```
xcrun simctl openurl booted "https://example.com/article"
axe tap -x 200 -y 842   # the collapsed address pill → expands the toolbar
axe tap -x 342 -y 816   # ··· (more)
axe tap -x 203 -y 541   # Share
axe tap -x 244 -y 653   # Push-Point in the share sheet
just save-timing
```

Two things that cost time on 2026-07-30:

- **The coordinate-space warning above is the one that bites here.** A tap at the
  pill's *screenshot pixel* y (2527) landed off-screen in the 874-point space and
  Safari went to the home screen — AXe reported success. Divide screenshot pixels
  by 3.
- **Maestro cannot see Safari's collapsed toolbar.** `id: ShareButton` is not in
  the tree and the hierarchy returns only page content, so semantic targeting is
  not an option until the toolbar is already expanded. AXe coordinates are the
  way in. I concluded "this cannot be driven" before checking my own arithmetic —
  the failure was the point conversion, not the tooling.

The first successful run recorded 91.8 ms against the 2000 ms budget, with all
four tags attached, so the whole scrape-and-tag pipeline finished inside it.

## XCUITest

`just ios-uitest`. The app launches with `-uitest`, which points it at a temp directory instead of the App Group and seeds fixtures through `POST /api/v1/links` (`ios/PushPoint/UITestMode.swift`). Three consequences worth keeping:

- Results do not depend on what happens to be in the simulator.
- The real archive cannot be touched by a test.
- A broken save contract fails during setup rather than passing silently.

On failure the suite attaches a screenshot and the accessibility tree — the only way to tell "not drawn" from "drawn differently" from "still loading". Extract them with `xcrun xcresulttool export attachments --path <bundle>.xcresult --output-path <dir>`.

Target rows in long lists by `accessibilityIdentifier`, not by display text: the dictionary has 40+ tags, and matching on visible copy breaks the test the moment the wording is edited.

## Recording a demo video

`.claude/skills/app-demo/SKILL.md` — driving the simulator while recording, compositing a
finger cursor, and **verifying frame-by-frame that the cursor is where it actually tapped**.
The last one is the point: a demo whose cursor points at the wrong thing shipped to the
landing page and stayed up until a human watched it, because a build cannot see it and
nobody watches every re-render. `just demo-check` is that gate.

## Exercising the app like a person (the periodic pass)

Everything below was found this way on 2026-08-10, and none of it by reading code: `devops` had
no word for deployment, a Settings edit that silently changed nothing, and an Export button that
wrote a 164 KB file while showing the user nothing. A build cannot see any of them.

The pass is manual on purpose — the value is in *looking and judging*, which is exactly the part
no assertion captures. Roughly twenty minutes.

1. **Seed a believable archive.** Write straight into the App Group DB (`ios.md` has the container
   recipe) with `created_at` spread across weeks. Backdating is the only way to get a resurface
   candidate — the rule is 7 days, and nothing you save today qualifies.
2. **Save two or three real URLs through the app**, not through SQL. Only this path runs the
   scraper and the tagger, and the tags it produces are the material for judging the dictionary.
   **Read the tags and ask whether they are what the piece is about.** That question found the
   0012 gap: a microservices article tagged `data`/`database` because the dominant concepts had no
   surface in the dictionary.
3. **Break one thing on purpose.** `https://this-host-does-not-exist.invalid/page` exercises the
   whole failure route. Give it a minute — retries back off, and it is `pending` until they are
   spent, which is *not* the same screen as failed.
4. **Walk the surfaces that are not the list**: settings, the widget gallery, the share sheet.
   These are the ones no test opens.
5. **Look at pixels, not just the hierarchy.** The status rail is a 2pt stroke; in a downscaled
   screenshot it is invisible, and reading one that way produced a wrong "the app shows no failure
   indicator" conclusion on 2026-08-10. Crop before concluding:
   `sips -c <h> <w> --cropOffset <y> <x> shot.png --out crop.png && sips -Z 900 crop.png`

Two traps worth repeating because both cost time today:

- **The app restores its last screen on relaunch.** Coordinates aimed at the toolbar can land on
  a link detail — one tap opened a delete confirmation. Screenshot before tapping anything.
- **`kill %1` is not reliable across Bash tool calls.** A server that did not die kept serving the
  old database and made a working restore look broken. Capture `$!` and kill that.

## The duplication to watch

Maestro flows and XCUITest cases both describe the same screens. That is two sets of assets to update per UI change, which is exactly what the sweep rule in CLAUDE.md exists to prevent. It is tolerable now because the two cover different ground (structure-on-real-data vs behaviour-on-fixtures). **If they start asserting the same things, converge on one** — and the one to keep is whichever runs in CI.

## Web

Serve the real thing, not a mock: `just release` then run the binary with
`PUSHPOINT_DATA_DIR` and `PUSHPOINT_API_KEY` pointed at a scratch directory, so the
embedded SPA and the API share an origin exactly as they do in production.

```
maestro: device_id "chromium", flow starts `url:` + `openLink:` (not appId/launchApp)
key entry: tapOn {id: "apikey", index: 1} → inputText → tapOn {text: "저장", below: "API 키"}
```

**A failed Maestro text assertion on `chromium` is not proof of absence.**
`extendedWaitUntil: { visible: "no such host" }` failed on 2026-07-30 against a card
that a screenshot from the same flow showed rendering that exact sentence. Short
strings like `오늘` matched fine. Whatever the cause, the screenshot is the
adjudicator — take one before concluding the feature is broken, or a working
change gets reverted on the tool's word.

Chrome headless covers what Maestro does not — rasterizing an SVG at a fixed size, or
screenshotting a URL without a driver session:

```
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless --disable-gpu \
  --screenshot=out.png --window-size=1024,1024 --hide-scrollbars "file://$PWD/page.html"
```

Two traps, both hit for real:

- **Chrome has a minimum window size.** `--window-size=16,16` does not scale the page down;
  it screenshots the top-left 16px of a full-size canvas, which for an icon is empty
  background. Render once at a large size and resample with `sips -Z`.
- **A page served by `python3 -m http.server` has no charset**, so Korean renders as mojibake.
  Put `<meta charset="utf-8">` in any scratch HTML you intend to look at.

### The Tailwind scale is explicit — off-scale utilities are silently dropped

`frontend/tailwind.css` resets `--spacing-*` and `--radius-*` to `initial` and defines only
**2 4 6 8 12 16 20 24 32 40 56 80** (the number *is* the pixel value). So `h-64`, `gap-1` and
`rounded-xs` produce **a class with no CSS** — no lint error, no type error, nothing on screen.
A loading skeleton written as `h-64` reserved no height at all. Dimensions outside the 12 steps
belong in `--size-*` and are used as `h-(--size-name)`.

## What driving finds, and what it does not

Every defect in this file's opening list was found by *looking*. Reading the diff found none
of them. But the reverse is also true and worth stating: driving the app has never found an
error path, a cancellation bug, or a race. **Amended 2026-08-10**: driving can *reach* an error
path — saving `https://…invalid/page` walks the whole scrape-failure route to a red rail, the
`lookup … invalid` message and a Retry button, and the app was correct at every step. What driving
still has not done is *find a defect* on one. Reaching is cheap and worth doing; the claim to keep
is about yield, not access. The one such defect this project shipped — a poller
that replaced the list with page one and discarded pagination — was caught by **XCUITest**, not
by any amount of tapping. Use both; they do not overlap.