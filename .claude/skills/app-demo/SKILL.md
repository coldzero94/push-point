---
name: app-demo
description: Record a polished demo video of the iOS app on the simulator — drive the flow, composite a finger cursor, and verify frame-by-frame that the cursor is where it actually tapped. Use when asked for a demo video, a screen recording, a GIF of the app, or promo material showing the app in use.
---

# Recording a demo of the app

The output is a screen recording with a finger cursor composited on top, at the quality
bar of Screen Studio / Recordly. The advantage over those tools: they infer where to
point from cursor movement, this pipeline **knows** every tap coordinate and timestamp.

That advantage is also the trap. Everything below is a mistake that shipped.

## The pipeline

```
just demo-record scripts/demo-flows/share-ko.json /tmp/demo.mp4   # drive + record + composite
just demo-check /tmp/demo.mp4                                     # ← never skip this
ffmpeg ... trim/speed/scale                                       # cut dead time
```

`scripts/demo_record.py` drives the simulator with `axe`, records with
`xcrun simctl io recordVideo`, stamps the **actual** elapsed time of every action, and
composites the cursor from those stamps. `scripts/demo_check.py` pulls the frame at each
tap timestamp, finds the cursor's magenta fiducial, and fails if it is more than 12 pt
from where the event says the finger was.

## Never publish a recording that has not passed `demo-check`

A demo whose cursor points at the wrong thing is worse than no demo — the cursor is the
only reason to composite anything. This is not hypothetical: a video shipped to the
landing page with the finger on Safari's address bar while the share sheet was open, and
it stayed up until a human watched it. A build cannot catch this and nobody will watch
every re-render.

## The clock is not the clock

`simctl io recordVideo`'s timeline is **not** the host wall clock, and how far off it is
**changes between runs on the same machine** — measured at 0.2 s in one recording and
7.0 s in another an hour apart. Compositing `frame_index / fps` as if it were the event
clock is what produced the shipped defect.

`measure_lag()` handles it: the flow's `open` step stamps a `sync` event, and the first
large frame-to-frame difference in the video is that same `openurl` repainting the screen.
The difference between the two is the offset. It prints the value every run — **read it**.
A number far from zero is not a failure, but a number that changes wildly between
otherwise identical runs means something else is wrong.

## Coordinates are points, and they move

- **Points (402×874), never screenshot pixels (1206×2622).** Divide by 3. `axe` reports
  "Tap completed successfully" for an off-screen coordinate, so a wrong-space tap looks
  exactly like a working one.
- **The share sheet's app icons move between installs.** Push-Point sat at x=261 one hour
  and x=245 the next; the fixed coordinate meant no save happened at all, which is
  visually identical to the extension crashing (which it also did that day). Use the
  `find_tap` step — it locates the icon by colour each run.
- Safari's toolbar **collapses to a bare address pill once the page scrolls**. Tapping the
  pill's coordinate on an unscrolled page hits the address field and opens the URL editor
  instead. Scroll first, then tap the pill to expand, then `···`, then Share.
- Let the page settle before the first swipe — 13 s after `openurl`. A swipe on a page
  that is still laying out does nothing, and then every later coordinate is wrong.

## Verify the save happened, not just that taps were sent

Count the lines in `save-timing.jsonl` before and after. A missed tap and a crashed
extension both produce "no new row", and neither produces an error. Reading the last line
without comparing counts will show you the *previous* run's success and you will believe it.

## Typing: `axe type` cannot, the pasteboard can

**Use `scripts/sim_type.sh`.** Text entry on camera works; three earlier attempts failed
because they all went through `axe type`, and `axe type` is the one route that cannot work.

`axe type --help` says it outright: *"Only US keyboard characters are supported via HID
keycodes."* It presses physical keys. So the simulator's hardware input mode decides what
lands, and that is not something the text is consulted about:

- English text with the Korean IME active → `axe type "Hello demo"` put `ㅗㄷ| |ㅐ ㅇㄷㅔ`
  in the field. Verified again on 2026-08-04, so the old note was right about the symptom.
- Korean text → nothing, ever. There is no HID keycode for `팀`. Rewriting
  `AppleKeyboards` and rebooting cannot conjure one; the setting was never the problem.
- `Ctrl+Space` (`axe key-combo --modifiers 224 --key 44`) does flip the input mode — once.
  It is a cycle with no readable state, and the second attempt an hour later typed jamo
  again. Do not build a flow on it.

The way in is the **device pasteboard**, which does not touch the IME:
`xcrun simctl pbcopy booted` then a paste. Korean, English, em-dash and emoji all arrive
intact. The field must already be first responder — tap it first (**points**, 402×874).

```
scripts/sim_type.sh --tap 201,608 '나중에 팀에 공유할 것'          # paste   — instant, invisible
scripts/sim_type.sh --mode menu --tap 201,608 '주말에 정리해서…'   # menu    — long-press → 붙여넣기
scripts/sim_type.sh --mode stream '주말에 정리해서 팀 위키로'       # stream  — types out, 0.32 s/char
scripts/sim_type.sh --mode stream-exact 'Read this later — 팀 공유'
```

**For the camera, `--mode menu`.** It long-presses the field, finds `붙여넣기` in the
Maestro hierarchy (the menu follows the caret, so a fixed coordinate misses) and taps it —
a real finger action the cursor compositor can point at. Everything else changes the field
with no on-screen cause, and note that the software keyboard never appears at all while a
hardware keyboard is connected, so there is nothing keyboard-shaped to film.

`--mode stream` is the typing animation, and it is **Korean-only**: iOS smart-insert treats
each pasted fragment as a word, adds a space before it and strips real space characters, so
`Read this later` streams in as `R e a d t h i s l a t e r`. `--mode stream-exact` re-pastes
the whole prefix each step and is correct for any script, but Cmd+A leaves the selection
highlight and its green grab handles in the frame — 2 of 80 frames in a test recording.
Latin text on camera: use `menu` or `paste`.

Confirm it landed in the app, not just on the screen: after tapping `메모 저장`,
`sqlite3 <AppGroup>/data/pushpoint.db "select note from links where id=21"` returned
`Read this later — 팀 공유`.

## Language

The app follows the system language, and there is now a globe menu in the list toolbar —
but it lives on the **list** screen, so switching from the detail view does nothing.
`xcrun simctl launch booted com.pushpoint.app -AppleLanguages '(ko)'` works for a launch
you control, but **not** for the app opened by tapping a notification: iOS launches that
one. A recording that ends inside the app therefore shows whatever language the app was
last set to — check it, and say which language the video is in rather than letting the
page imply otherwise.

## Preparing the data

The link being saved must not already exist, or the save takes the duplicate path and the
notification says so. Delete it from the App Group database first — and delete `-wal`/`-shm`
alongside `.db` when restoring a database, or a stale WAL replays over the copy and the app
reads a file that is not the one you put there.

## Auto-zoom

`camera()` returns the centre and factor per frame: ease in 0.45 s before a tap, hold
0.5 s after, ease out 0.5 s, and taps closer than ~1.2 s stay in one continuous move
rather than pumping in and out. The target sits slightly above centre so the surrounding
screen is still readable — a finger pinned dead centre shows what was pressed but loses
where it was.

**The crop is done in PIL, not by an ffmpeg filter.** `render_camera()` pipes the
cursor-composited frames out of ffmpeg as `rawvideo`, applies the per-frame rectangle with
`Image.transform(AFFINE, BICUBIC)`, and pipes them straight into a second ffmpeg encoding
yuv420p H.264 at the source size. 47 s of 1206×2622 takes ~43 s.

Two things this buys, both of which the filter routes gave up:

- **Sub-pixel crop.** `crop()` takes integers, so the camera would step 1 px at a time going
  in and out. The affine takes the float rectangle directly.
- **No generation loss.** It crops the *raw* recording, so nothing is encoded twice. The
  zoom blows an 830 px-wide region up to 1206 px and a first-pass crf-18 artifact would be
  magnified along with it.

Both ffmpeg-side routes were measured and neither works (2026-08-04):

- `sendcmd` + `crop` — `crop` will not accept `w`/`h` as runtime commands (**exit 234**).
  Pinning the size and moving only `x`/`y` is accepted, but then the magnification is
  constant for the whole segment, so it is a cut, not a zoom.
- `zoompan` — zooms correctly for a simple expression, but the only channel for a per-frame
  trajectory is the expression itself. 1413 frames written as `between(on,i,i)*v` terms is a
  34 KB expression and filter init dies with **exit 244 / ENOMEM**.

**`demo_record.window()` and `demo_check.to_source()` are the same formula, one forward and
one back.** The checker re-derives the crop rectangle from `<out>.camera.json` +
`<out>.zoom.json` to undo the magnification; change how the frame is cut without changing
both, and the checker either clears a wrong cursor or flags a right one — and a checker that
cries wolf gets switched off. It un-projects with the segment's max zoom, which is exact at
tap frames because `camera()` is at full magnification there by construction.

Measured on `/tmp/pp-demo-raw.mp4` (6 taps, 4 zoom segments, 302 of 1414 frames zoomed): all
six taps land within **0.3 pt**, and sampling every frame of a zoom-out ramp un-projects to
the same source point to within 0.3 pt — so the trajectory is truthful frame by frame, not
just where the checker looks.

The fallback stayed: if the camera stage raises, the flat video is emitted with a warning —
the camera is decoration and the cursor is content, and decoration must not cost you the
artifact. It also **blanks `camera.json` and resets `zoom.json` to 1.0**, or the checker
would undo a crop that was never applied and fail a perfectly good cursor.

## After recording

- Cut dead time with `trim` + `concat`, and `setpts=PTS/1.25`–`1.5` for pace.
- h264 at crf 26, 440 px wide: ~0.8 MB for 18 s. A GIF of the same clip needs 9 fps to fit
  2 MB and looks slow because of it.
- **A README video has to be hosted by GitHub itself.** `<video src>` pointing at
  `raw.githubusercontent.com` is deleted by the README renderer — tag and all, leaving an
  empty paragraph. Pointing at `https://github.com/user-attachments/assets/<uuid>`
  survives and renders as a collapsible player. The file content is irrelevant; only the
  host is.

  That URL is only produced by dragging the file into an issue or PR comment box in the
  browser — there is no CLI endpoint for it. Ask for it rather than trying to link the
  copy in the repo.

  **`POST /markdown` cannot answer this question.** It keeps every variant, including the
  ones the README renderer strips, so it has no discriminating power at all. Testing there
  produced both wrong conclusions in a row: first "video works", which shipped a blank
  hero, then "GitHub strips video", which was the opposite and equally wrong. Test against
  the renderer that actually runs, on a branch:
  `curl -H "Accept: application/vnd.github.html" ".../repos/OWNER/REPO/readme?ref=BRANCH"`
- Look at the last few seconds before publishing. Both shipped defects were visible in a
  single frame.
