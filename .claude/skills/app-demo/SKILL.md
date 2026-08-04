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

## Typing is not usable yet

`axe type` goes through the active IME. English text with a Korean keyboard comes out as
jamo (`포ㅏ ㅣ ㄴㅁ으ㅐ`), and Korean text with an English keyboard types nothing at all.
`defaults write .GlobalPreferences AppleKeyboards` does not take effect without a
simulator reboot. Until that is solved, **end the recording where the field opens** rather
than shipping garbled input to demonstrate text entry.

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

## After recording

- Cut dead time with `trim` + `concat`, and `setpts=PTS/1.25`–`1.5` for pace.
- h264 at crf 26, 440 px wide: ~0.8 MB for 18 s. A GIF of the same clip needs 9 fps to fit
  2 MB and looks slow because of it.
- GitHub README keeps `<video src>`, `width` and `muted` but **strips `autoplay` and
  `loop`** (verified with `gh api -X POST /markdown`), so it is click-to-play there.
- Look at the last few seconds before publishing. Both shipped defects were visible in a
  single frame.
