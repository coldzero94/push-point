# Design System

> Push-Point v2 — last updated: 2026-07-25

**2026-07-25 revision — from rows to cards.** The basic unit of the list stops being a 76px row and becomes a **card**, `description` comes on screen, and a link with no thumbnail gets a **generated cover** (R4, §4.5). A time spine cuts the list apart, and serif is allowed in those headings and nowhere else (§2.2.5). The neutral ramp moved to hue-170 — L was kept and only C was cut to 0.62x, and how much CVD headroom that cost is written into §2.1.1 as a measurement. **The six facet colors, the accent, the hue lock and the chroma ceiling did not change**: what this revision changed is structure, not color. The rewritten exclusion list is §1.3.

This document is Push-Point's **single source of truth for the visual and interaction layer**. So that the web (`frontend/`) and iOS (`ios/`, M4) come out of one original, it pins token values, component specs, motion numbers, accessibility standards and the platform mapping table down to a level you can implement from.

- This document is the source; the `@theme` block in `frontend/tailwind.css` is the derivative. To change a value, change this document first.
- The token original is promoted to `design/tokens/` plus a generation pipeline **when M4 (iOS) starts**. Building the generator now, while the web is the only consumer, is premature optimization. Instead the semantic token names are **platform-neutral** (`surface`/`elevated`/`fg-1..3`/`accent`/`danger`) so that promotion never renames anything.
- Related documents: the screen and interaction spec is [11-WEB-UX-SPEC.md](11-WEB-UX-SPEC.md), the product identity is [01-PROJECT-OVERVIEW.md](01-PROJECT-OVERVIEW.md), the contract for the data those screens handle is [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md) plus `api/openapi.yaml`, and the frontend stack rules are `.claude/rules/frontend.md`.

**The ownership boundary between 10-DESIGN-SYSTEM.md and 11-WEB-UX-SPEC.md (the no-duplication rule).** The same value is never written into both documents. Reference the other side's territory by section number instead of copying the value.

| What this document (10) owns | What 11-WEB-UX-SPEC.md owns |
|---|---|
| Color, type, spacing, radius, the z ladder, motion duration/easing tokens | Per-screen layout, field mapping, states, responsive behavior |
| Component dimensions and variants, row dimensions, breakpoints | **The keyboard shortcut table** (§1.2) |
| **The toast visual spec** (position, count, duration, color, §4.10) | **When a toast is raised** + the error code → UI mapping (§1.4) |
| Accessibility standards (contrast, focus, motion, hit area) | Implementation priority (P0~P3) and the contract-gap list (§10) |
| The iOS mapping table (§8) | The flow and copy placement of the web screens |

## 1. Design principles

### 1.1 Concept

> **"An instrument panel where a person skims what a machine classified. Hue says what it is about; fill says who touched it."**

Push-Point's identity is not "a pretty place to collect links" but **a classifier that runs without an LLM** (classification against a controlled dictionary of 30~50 entries, finished within three seconds of the save, and no data leaves the machine). So the protagonist of the screen is neither the thumbnail nor the content but **the contrast between the machine's output (title/domain/tag/status) and the person's intervention (manual tag/note/filter)**. Encoding that contrast into hue, fill and typeface is what this app's design is.

Design dials: expressive variance 4 / motion intensity 3 / visual density 7. This is a tool, not a landing page, so asymmetric layouts and scroll choreography never get to shave the speed of scanning a list. The entire remaining expression budget goes to **type precision + 3 color rules + state expression**.

### 1.2 The 3 governing rules (R1/R2/R3)

| Rule | What it says | What it means in implementation |
|---|---|---|
| **R1. Hue is identity, fill is intervention** | **hue** encodes only "what is this about" — the 3 tag facets (craft 168 / media 112 / life 318) and brand hue-168, nothing else. **fill level** encodes "who touched it, and is it on right now" (0 = machine output, transparent / 1 = attached by a person, control tint / 2 = selected by the user, solid). State (in progress, failed, warning) never uses a hue from the tag palette; it is expressed with reserved hues + shape + a sentence, and nothing else | A color outside the hue reservation table (§2.1.4) arriving as a token is rejected in review. No per-facet `on-color` is made (§2.1.3, the chip-interior contrast table) |
| **R2. Machine data is mono** | Domain, URL, save time, counts, confidence and search rank are fixed-width. What a person wrote (the displayed title, the note) is sans | What `--font-mono` applies to is pinned to fields that actually exist in the contract (§2.2.4) |
| **R3. State is a stroke, not a badge** | One 2px rail on the card's leading edge expresses in-progress / failed / selected. The done state shows nothing at all | The 5-color mapping in `StatusBadge` is retired and replaced by `StatusRail` (§4.7). **Chips became chromatic and state is still one achromatic rail — R3 is the precondition for the R1 revision** |
| **R4. Never leave a blank — generate one instead** | A link with no thumbnail gets neither a gray box nor a domain initial but a **generated cover**. The ground is the dominant tag's facet tint, the pattern is one of four picked by hashing the domain, and the domain wordmark sits on top | `thumb: failed` + `status: done` is a normal combination, so the cover is not "a stand-in for a missing image" but **the final cover of a large share of links**. **What the hash decides is geometry only; color comes from the facet alone** — that boundary is what keeps the §5.4-ban on hashing tag colors alive exactly as written. Spec in §4.5 |

**How R1 came to be revised (2026-07-22).** Before the revision R1 read "color is the trace of a person", and hue encoded whether there had been intervention. The revision **keeps the purpose and changes only the channel** — the human trace is carried by fill now. What gets stronger than before: the manual marker appears in **the same shape across every facet**, so it survives color-blind conditions (hue did not). Background and settled values are in §2.1.4 · §5.

### 1.3 Exclusion list (what we will not do)

> **Rewritten 2026-07-25.** Three items below have been released — the card grid, a choreographed-motion budget, and serif in screen headings.
> The grounds for each release are written at the item. Everything else stays banned.

- **Marketing hero, scrollytelling, parallax, marquee, scroll cues** — landing-page grammar does not go on top of a tool.
- ~~**Card grid**~~ **→ released (2026-07-25).** The ban rested on "a field of gray boxes once thumbnails are empty", and R4 removes that premise — a cover always exists, and where it does not it is generated. **Masonry and mood boards stay banned, though.** Only a 16:9-locked cover ratio can hold CLS at zero, and a grid of ragged heights shaves scanning speed.
- **Reader view** — the primary action is forever "open the original". (Showing `description` on the card in two lines is not a reader view — the contract already hands over a 200-character `description` in the list response, and not using it was the waste.) **`summary` (the M5 extractive summary) is not a reader view either** — it is not on the card and not in the list response (the contract gives it on `LinkDetail` only), and it is a 2~3-sentence extract from the original, not the whole body. It is an aid for judging "open it or not" in the inspector, never a substitute for the original.
- ~~**Density toggle**~~ **→ released for iOS only (2026-07-29).** The ban rested on "density is decided by the viewport", but
  **iOS has exactly one viewport**, so on a phone that sentence does not mean "no choice" — it means "one way forever".
  Unlike the web, where widening the window adds columns, a phone has no way at all to change the density it is scanned
  at, and that happens to be the platform where scanning matters most. It is the same shape as the card grid losing its
  premise to R4 and being released. **The web stays banned** — window width already does that job (13 §1 ② axis:
  platform features go wherever each is best). What is allowed is **two list densities (card / compact) and nothing
  else**, and what compact contains is set by §4.4.1. Column count, font size and theme presets are not user choices.
- **View switcher (layout toggle), theme preset gallery** — for a single user, a choice is not a feature but a tax. The number of card columns is not chosen by the user; **the board container width** decides it (§2.3).
- **Gradients, glows, neumorphism, glass cards** — glass (backdrop blur) exists in exactly two places: the top bar and the command palette.
- **Emoji** — nowhere: not in a label, not in an empty state, not on a button. Icons are lucide (web) / SF Symbols (iOS) only, strokeWidth 1.5, in two sizes: 16·20px.
- ~~**Choreographed motion**~~ **→ conditionally released (2026-07-25).** Only S2 (the card that fills in) gets a choreography budget. Becoming a card raised the number of slots waiting to be filled to 4 (title, body, tags, cover), and their lighting up in order is itself the proof that "classification finished inside three seconds". **This is still the only choreography in the whole app** — no choreography is added to other screen transitions, to list entry, or to hover.
- ~~**Serif in body text**~~ **→ released in a limited way (2026-07-25).** Serif is allowed in the time-spine headings (`오늘` / `이번 주` / `7월`) and nowhere else (§2.2.5). Body, labels, controls, chips and buttons all stay sans. The grounds: on that line the human language (the spine label) and the machine language (the count, in mono) stand side by side, and the typeface contrast says R2 one more time.
- **Onboarding tour, empty-state mascot, number count-ups, 3-column feature cards.**
- **Tag color hashing + a user-chosen color per tag** — the §5.4-rationale spells out the grounds for banning both: a hash (change the dictionary and the whole set is reshuffled; color has nothing to do with meaning) and individual assignment (the GitHub label and Notion select failures).
- **Adopting shadcn/ui** — only the primitives that need accessibility behavior (`@radix-ui/react-*`, 4~5 of them) go in as direct dependencies, and we own the components. This token system overwrites 90% of shadcn's defaults (slate neutrals + a single 8px radius + a desaturated primary), so all that would be left is the file layout.

### 1.4 The 2 signature elements

**S1. The status rail** — one 2px vertical stroke on each card's leading edge replaces the status badge, the status dot, the selection checkbox and the focus marker, all of them. What makes it memorable is not decoration but **removal**. The normal state carries no marker at all, so every stroke left on screen means "something is happening right now, or something went wrong". Rows became cards and **the thickness is still 2px** — the chroma budget table in §2.1.4 pinned the size ceiling of a chromatic rail at 2px (chroma on the order of danger 0.180 / accent is allowed at that size and no other), and the §4.7-rule that thickness never makes hierarchy holds as well.

**S2. The card that fills in (the fill)** — the moment a save is submitted, a card appears at the top of the board. At that point the only real value is the domain, and the title/body/tag/cover slots sit **empty at their final dimensions**. As each worker finishes, title → body → tags → cover fill in place (that card alone is replaced; re-rendering the whole board is banned). It is the only choreographed motion in the app, and it proves the product's core claim (automatic classification inside three seconds) by behavior rather than by sentence.

- The implementation is **not a timer.** Each slot carries `.fill-step` + `data-in`, and `data-in` looks at one thing only: "has that value arrived" (title = `title` is not empty, cover = `status` is terminal). When the poller refreshes the link the slots light up by themselves, so the order of the choreography *is* the order the workers actually finished in.
- A card that is already complete at first render enters the DOM with `data-in="true"`, so it is drawn as-is with no transition. The accident where every card fades in each time you open the list is structurally impossible.
- **A slot that has not been filled yet is `inert`.** `opacity: 0` removes it from sight only; it stays in the focus order and in the accessibility tree — leave it that way and a keyboard user tabs into invisible links while a screen reader reads a card that is visibly empty. `inert` keeps the element mounted and takes it out of exactly those two trees, so it does not lose the property that the fade actually runs when the value arrives (§7.2).
- Under `prefers-reduced-motion` it is **removal**, not reduction (straight to the final state).

**The restraint rule:** no "memorable detail" is added beyond S1 and S2. The moment a third signature goes in, both die. **The 3-level chip fill (§4.3) is not a third signature competing with S1 and S2** — it is not a new shape but the state axis of a chip that already exists. **The generated cover (R4 / §4.5) is not a third signature either** — it is not new choreography but a rule that removes blanks, and the moment it is noticed gets absorbed into the last step of S2.

### 1.5 The identity mark (added 2026-07-28)

**The same shape three times over, stacked as a 0 → 1 → 2-fill ladder.** The top is stroke only (machine output), the middle is tint (attached by a person),
the bottom is solid (selected by the user). The ground is `accent-tint` dark (`#0F2C22`).

**The hue of the three cells is one hue.** They are all `accent` (#2EB88F, hue 168) and the only thing that differs is the fill —
the mark is R1 said out loud. The first draft used a different color in each cell, and since that broke R1
head-on it was thrown out.

**It is not a third signature.** The §1.4-restraint rule applies unchanged. This mark is neither new choreography nor a
new shape; it is **a quotation of one rule that already exists**. It is not used inside the app's screens —
the header wordmark stays text as it is today (bring the mark on screen and it competes with S1 and S2).

#### Why this mark

| Candidate | Grounds | Why not |
|---|---|---|
| **The fill ladder** | R1 · §1.1 | — **adopted** |
| The rail | R3 | Restrained, but **it says nothing else** |
| The generated cover (hatch) | R4 | Self-reference is attractive, but the pattern aliases at 16px |
| The instrument panel (bars) | The first words of §1.1 | The silhouette is the clearest of them, but **it reads as a chart app** — this product is not a statistics tool |

#### Source and generated files

The source is `design/icon/mark.svg` and **nothing else**; `just icons` (`scripts/gen_icons.sh`) generates the
8 files of four surfaces. Generated files are never edited directly.

| Surface | File |
|---|---|
| iOS | `ios/PushPoint/Assets.xcassets/AppIcon.appiconset/icon-1024.png` (no alpha) |
| Web | `frontend/public/favicon.svg` · `favicon-32.png` · `favicon-16.png` · `apple-touch-icon.png` |
| Extension | `extension/icons/icon-{16,48,128}.png` |

**16·32px come out of a separate optical size called `mark-small.svg`.** Shrink the master to 16px and
**the fill-0 stroke dies first**, so a three-step ladder looks like two (a 24/1024 stroke = 0.4px at 16px).
So at small sizes the stroke thickens and the gaps widen — **it is not simplified.** The fact that there are three steps
*is* the meaning of this mark, so shaving there leaves nothing behind.

**There is no drift gate.** CI has neither macOS nor Chrome, so it cannot check by regenerating
(this is where it differs from `gen-check` on contract-generated code). If you changed the mark, run `just icons` yourself and
put the result **in the same commit**.

#### What this was before this section existed (for the record)

- **The web favicon was the default Vite logo** (a purple lightning bolt). It arrived with the 2026-07-21 scaffold and was never
  touched again, and **the design system landed one day later, on 07-22.** One day was the gap that left a file
  which never once met the system.
- **The iOS icon was the Xcode scaffold placeholder** and had a blue dot in it. That is a color outside R1's hue
  reservation table, but the rejection rule reads "when it arrives as a token", so **the icon slipped through that net.**
- The extension had no `icons` entry at all.

## 2. Design tokens

### 2.1 Color

#### 2.1.1 Neutral ramp (a 12-step raw palette)

**A very low chroma at hue-170** (revised 2026-07-25 — previously hue 225~257). Fully achromatic dies cold under Korean body text, and a high-chroma blue neutral reads as "fintech". Between the two. **Pure `#000`/`#fff` are not used.**

**How it was revised, and what it cost.** At every step **L was left alone, only the hue moved, and C was cut to 0.62x.** Because L is preserved, the §7.1-luminance contrast table is effectively unchanged. What it cost: as the neutral ramp moved closer to `craft` (168), CVD headroom shrank — the minimum separation between neutral chip ink and facet ink went **from a first-order deutan approximation of ΔE 9.69 → 8.67 (light) / 10.14 → 9.17 (dark)**. Both still clear the 8-line target of §5.1, so the revision is accepted, but **raising the ramp's C any further from here breaks that target**. To touch chroma, redo the §5.1-measurement first. (The approximation model: distance in oklab with the red-green axis `a` removed — not a precise CVD simulation but **a relative measurement for putting the two ramps against the same ruler**.)

| Token | Value | Fixed role |
|---|---|---|
| `--ink-0` | `#FCFDFD` | Light surface (card/input/panel) |
| `--ink-50` | `#F4F6F5` | Light page floor |
| `--ink-100` | `#EAEEEC` | Light hover |
| `--ink-200` | `#DBE0DE` | Light border (strong) |
| `--ink-300` | `#C2CAC7` | Disabled text / divider (strong) |
| `--ink-400` | `#919B97` | Light tertiary text, **dark progress rail** |
| `--ink-500` | `#6B7672` | Dark tertiary text (approximated as `#6E7B76`) |
| `--ink-600` | `#515C58` | Light secondary text, **light progress rail** |
| `--ink-700` | `#3A4440` | Dark border (strong) |
| `--ink-800` | `#262F2B` | The top step of the dark surface — **not mapped to a semantic token directly** (dark `--bg-selected` is the brand tint `#0F2C22`). Kept for ramp continuity |
| `--ink-900` | `#171D1B` | Light primary text |
| `--ink-950` | `#0D1210` | Dark page floor |

The raw palette is referenced **only where semantic tokens are defined**. Using `--ink-*` directly in component code is a lint failure.

#### 2.1.2 Semantic tokens (the real light/dark values)

| Token | Light | Dark | Use |
|---|---|---|---|
| `--bg-canvas` | `#DEF0E8` | `#07140F` | The page floor. **Not the neutral ramp but a low-chroma brand hue** (§2.1.4) |
| `--bg-surface` | `#FCFDFD` | `#141A18` | Card, input and toolbar surfaces |
| `--bg-hover` | `#EAEEEC` | `#1A221F` | hover, and the ground under a cover before it loads |
| `--bg-elevated` | `#FCFDFD` | `#1C2320` | Inspector, popover, sheet, palette |
| `--bg-selected` | `#E1F9EF` | `#0F2C22` | Selected card background (= `--accent-tint`) |
| `--fg-1` | `#171D1B` | `#E9EDEB` | Primary text (title, body) |
| `--fg-2` | `#515C58` | `#A9B3AF` | Secondary text (description, label) |
| `--fg-3` | `#919B97` | `#6E7B76` | Tertiary text (domain, time) — **secondary metadata only** |
| `--fg-inverse` | `#FCFDFD` | `#06120E` | Text on accent or deep backgrounds |
| `--line-1` | `rgb(13 18 16 / .08)` | `rgb(233 237 235 / .08)` | **Decorative hairlines only** — the spine's bottom line, the card ring |
| `--line-2` | `rgb(13 18 16 / .14)` | `rgb(233 237 235 / .14)` | **Decorative hairlines only** — section dividers, panel outlines |
| `--line-control` | `#77817D` | `#77817D` | **Control borders only** — input, select, **filter bar chip** and secondary button borders (display chips excluded, §4.3) |
| `--rail-progress` | `var(--ink-600)` | `var(--ink-400)` | The status rail while in progress (pending/scraping/tagging) |
| `--accent` | `#197459` | `#2EB88F` | Focus ring, primary button, selected card rail, the fill of a selected `craft` chip |
| `--accent-hover` | `#096149` | `#37CFA1` | hover on accent elements |
| `--accent-tint` | `#E1F9EF` | `#0F2C22` | Selected card background only (= `--bg-selected`). **Not used on manual chips** — manual takes that tag's own facet tint (§5.2) |
| `--on-accent` | `#FCFDFD` | `#06120E` | Text on an accent fill |
| `--danger` | `#B4232B` | `#F2706B` | `status: failed`, 4xx/5xx, delete confirmation |
| `--danger-tint` | `#FCEBEA` | `#2A1518` | Failure banner background |
| `--warn` | `#8E6400` | `#B28738` | Duplicate save, a tag outside the dictionary, API key not set |
| `--warn-tint` | `#FDF3E2` | `#292113` | Warning banner background |

**The tag facet palette (6 new tokens — the strategy is §5).** Color belongs not to an individual tag but to **3 facets + 1 achromatic**.

```
hue lock:  craft 168  /  media 112  /  life 318
           hex 반올림 drift: ink 기준 ≤ 0.6°  /  tint 포함 ≤ 1.6°
           (실측 최대 — ink: craft L 0.55° · tint: media L 1.52°. C 0.028~0.040 tint라 시각 영향 없음)
```

| facet | Meaning | ink (Light) | tint (Light) | ink (Dark) | tint (Dark) |
|---|---|---|---|---|---|
| `craft` | What you make (brand) | **`#004E39`**<br>`oklch(37.63% 0.0767 167.45)` | **`#E1F9EF`**<br>`oklch(96.19% 0.0284 168.81)` | **`#84E4C1`**<br>`oklch(84.97% 0.1047 168.31)` | **`#0F2C22`**<br>`oklch(26.73% 0.0400 168.06)` |
| `media` | Form (read/watch/listen) | **`#54570B`**<br>`oklch(43.98% 0.0921 112.04)` | **`#F2F5E0`**<br>`oklch(96.2% 0.028 112)` | **`#BFC573`**<br>`oklch(80.04% 0.1055 112.17)` | **`#262810`**<br>`oklch(26.8% 0.040 112)` |
| `life` | The world and daily life | **`#4B2656`**<br>`oklch(33.98% 0.0915 318.04)` | **`#FBEDFF`**<br>`oklch(96.2% 0.028 318)` | **`#EEBBFE`**<br>`oklch(85.92% 0.1057 317.98)` | **`#2E2033`**<br>`oklch(26.8% 0.040 318)` |
| `neutral` | No classification | `--fg-2` `#515A67` | `--bg-hover` `#EAEDF1` | `--fg-2` `#A8B1BE` | `--bg-hover` `#1A2029` |

The token names are `--tag-craft-ink` / `--tag-craft-tint` / `--tag-media-ink` / `--tag-media-tint` / `--tag-life-ink` / `--tag-life-tint`, six of them. **`neutral` reuses existing tokens instead of minting new ones** — the token name says outright that achromatic is "the state of having no color".

**L differing per slot is intentional** (light 0.340~0.440, dark 0.800~0.860). The moment they are equalized in lightness, CVD separation collapses — L is the only channel that survives protan and deutan. C, by contrast, is bound to **one ceiling shared by every hue** (light 0.092, dark 0.106 — the same values as the chroma budget table in §2.1.4; the measured maxima are light `media` 0.0921 and dark `life` 0.1057), which stops any one hue from shouting louder than the rest.

#### 2.1.3 Contrast (subject to the build gate)

The numbers below are **actually calculated** with the WCAG 2.x sRGB relative-luminance formula (rounded to two decimals). Change a token value and this table gets refilled by the same calculation — no estimates are written here. (On 2026-07-29, when `--bg-canvas` became a brand hue, the five rows that involve canvas were recalculated. That rule had not been kept at the time, and the table had been sitting on the old values.)

**Text (the bar is 4.5:1)**

| Combination | Light | Dark | Verdict |
|---|---|---|---|
| `fg-1` on `bg-canvas` | 14.44 | 15.93 | AAA |
| `fg-1` on `bg-surface` | 16.78 | 14.93 | AAA |
| `fg-2` on `bg-canvas` | 5.87 | 8.74 | AA / AAA |
| `fg-2` on `bg-surface` | 6.82 | 8.19 | **AA** (light misses 7:1) / AAA |
| `fg-3` on `bg-canvas` (13px meta) | **2.42** | 4.26 | **Under the bar — see the exception rule below** |
| `fg-3` on `bg-surface` (13px meta) | **2.81** | 4.00 | **Under the bar — see the exception rule below** |
| `accent` text on `bg-canvas` | **4.81** | **7.50** | AA |
| `accent` text on `bg-surface` | **5.59** | **7.03** | AA |
| `on-accent` on `accent` (primary button) | **5.60** | **7.60** | AA |
| `on-accent` on `accent-hover` (primary hover) | **7.32** | **9.63** | AAA |
| `danger` on `bg-canvas` | 5.51 | 6.55 | AA |
| `danger` on `danger-tint` | 5.66 | 5.99 | AA |
| `warn` on `bg-canvas` | 4.88 | 5.79 | AA |
| `warn` on `warn-tint` | 4.81 | 4.86 | AA |

**Non-text (the bar is 3:1 — state and control borders only, §7.1)**

| Combination | Light | Dark | Verdict |
|---|---|---|---|
| `line-control` on `bg-surface` | 3.98 | 4.36 | Pass |
| `line-control` on `bg-hover` (input hover) | 3.45 | 4.04 | Pass (lowest over all backgrounds 3.45 / 3.96) |
| `rail-progress` at its pulse floor `opacity .7` on `bg-surface` | 3.38 | 3.68 | Pass (lowest over all backgrounds 3.13 / 3.44) |
| `rail-progress` opaque on `bg-surface` | 6.85 | 6.13 | Pass |
| `danger` rail on `bg-surface` | 6.41 | 6.14 | Pass |
| `accent` rail on `bg-selected` | **5.16** | **5.96** | Pass |
| Focus ring `accent` on `bg-surface` | **5.60** | **7.03** | Pass |
| Focus ring `accent` on `bg-hover` | **4.85** | **6.53** | Pass |
| `line-1` / `line-2` (after alpha compositing) | 1.18 / 1.35 | 1.21 / 1.47 | **Not gated — the decorative hairline exception (§7.1)** |

Against the previous values the brand ramp **improves every contrast**: `on-accent` on `accent` 5.22→**5.60**, `accent` on `bg-canvas` 4.91→**5.26** (AA headroom secured). Dark is effectively identical at the 7.58→7.60 / 7.52→7.55-level.

**The grounds for the `fg-3` exception, and its scope.** In light it sits at 2.66~2.83x, short of even the 3:1-floor for non-text. The reason the value is kept anyway is measurement — clearing the 4.5:1-bar on every light background (canvas/surface/hover) would take something around `#666E7B`, and that is effectively indistinguishable from `--fg-2` (`#515A67`, 6.44~6.85), which collapses a three-step color hierarchy into two (`--ink-500` `#6B7482` misses as well, at canvas 4.36 / hover 4.02x). What it does instead is **pay for it by narrowing where it may be used**:

- Used **only for information duplicated somewhere else**: `domain` (the original is in the inspector's `url`), relative time (the absolute time is in the `title` attribute), placeholder (a label exists separately), disabled text.
- Banned on text that carries meaning on its own, on labels of interactive elements, and on **non-text elements such as borders, icons and rails** (it is under 3:1, so it cannot even be used to justify the non-text bar). Enforced by lint.
- This exception is acknowledged and recorded as a WCAG 1.4.3 violation. If domain or time ever has to be promoted to standalone information, that field moves up to `--fg-2` (moving where it is used, not widening the exception).

**Tag chip contrast — chip ink × every background (gate: 4.5:1)**

| facet | canvas | surface | hover | selected | ‖ canvas-D | surface-D | hover-D | elevated-D | selected-D |
|---|---|---|---|---|---|---|---|---|---|
| craft | 9.02 | 9.60 | 8.32 | 8.84 | ‖ 12.49 | 11.64 | 10.80 | 10.57 | 9.87 |
| media | 7.06 | 7.51 | 6.51 | 6.92 | ‖ 10.33 | 9.62 | 8.93 | 8.74 | 8.16 |
| life | 11.33 | 12.05 | 10.45 | 11.10 | ‖ 11.88 | 11.07 | 10.27 | 10.05 | 9.39 |
| neutral | 6.44 | 6.85 | 5.94 | 6.32 | ‖ 8.75 | 8.15 | 7.56 | 7.40 | 6.91 |

**The lowest is 5.94:1 — every combination passes AA, most reach AAA.** A fill 0 chip is more legible than today's unselected chip text (`--fg-2`, 6.44~6.85).

**Tag chip contrast — chip interior (fill 1 / fill 2)**

| facet | ink/tint L | ink/tint D | tint/surface L | tint/surface D | solid fill L | solid fill D |
|---|---|---|---|---|---|---|
| craft | 8.84 | 9.87 | 1.09 | 1.18 | 9.60 | 11.64 |
| media | 6.89 | 8.22 | 1.09 | 1.17 | 7.51 | 9.62 |
| life | 10.91 | 9.61 | 1.11 | 1.15 | 12.05 | 11.07 |
| neutral | 5.94 | 7.56 | 1.15 | 1.08 | 6.85 | 8.15 |

solid fill = `--bg-surface` text × a facet `ink` background. **No per-facet `on-color` is needed** — every column above clears 4.5x, so `--bg-surface` alone is enough (the R1 revision: selection does not change hue, it raises fill). `tint/surface` sitting around 1.1 is normal — tint carries no information, it says only "a person attached this", and the information is carried by the ink.

**Color-blindness verification (Viénot-Brettel-Mollon 1999, OKLab ΔE×100, all-pairs including neutral)**

LIGHT

| Pair | normal | protan | deutan |
|---|---|---|---|
| craft × media | 10.19 | 7.49 | 9.95 |
| craft × life | 16.68 | 13.38 | 7.90 |
| craft × neutral | 11.94 | 8.20 | 9.47 |
| media × life | 20.50 | 20.78 | 17.73 |
| media × neutral | 11.54 | 11.67 | 11.85 |
| life × neutral | 14.96 | 16.16 | 12.58 |
| **worst** | **10.19** | **7.49** | |

DARK

| Pair | normal | protan | deutan |
|---|---|---|---|
| craft × media | 11.06 | 10.86 | 10.03 |
| craft × life | 20.33 | 13.07 | 8.37 |
| craft × neutral | 14.10 | 13.76 | 9.99 |
| media × life | 21.41 | 18.99 | 18.11 |
| media × neutral | 13.08 | 13.00 | 13.26 |
| life × neutral | 14.06 | 9.71 | 11.48 |
| **worst** | **11.06** | **8.37** | |

**Verdict: in light and dark alike the worst CVD ΔE is ≥ 7.49-level, which clears the acceptance line (≥6) under the premise that "a chip always carries its name" and comes close to the target line (≥8).** Deuteranomaly is the most common type (about 6% of men), so the verdict was taken on the protan/deutan minimum. The §9-listed color-blindness gate checks this automatically.

**1 unresolved risk (recorded explicitly).** Of the 2 that existed before the revision, `media`×`--warn` improved toward resolution when the warn hue was re-picked on 2026-07-24 (see below). The one that remains is untouched, because danger was not touched.

| Combination | normal ΔE | CVD ΔE | Mitigation |
|---|---|---|---|
| `craft` ink × `--danger` (light) | 27.61 | **1.75** | **This is a property the existing system already had** (the previous values `#0B7A5B` × `#B4232B` behaved identically). §2.1.4(b) already blocks it with "facet ink and danger are never used as fill colors side by side in the same component" (mandatory for danger), and so does the §4.7-rule with "failure = rail + text + icon, all three" |

**`media` ink × `--warn` (light) — improved toward resolution (2026-07-24).** Moving the warn hue 66.9°→80° (unified across light and dark) and redistributing the AA contrast budget into L separation made CVD ΔE jump from 1.53→**6.75** in light and 5.25→**14.92** in dark (normal ΔE in light 9.57→11.05). The light worst case of 6.75-level clears the acceptance line (≥6) — it still falls short of the target line (≥8), though. hue was the weak lever (hue alone got light to 2.59); the real fix was **L separation**, pushing warn out to the AA boundary and away from media's lightness (light L 0.44 / dark 0.80). So the "no parallel fill of facet ink and warn" of §2.1.4-b and the §4.6-rule is **relaxed from mandatory to advisory**. The practice of the `notice` badge always coming with a sentence and never carrying a tag name (§4.6) stays as advice. The cost is recorded honestly too — it was bought by cutting warn's AA contrast headroom from ~5.8x to ~4.8x (still AA), and the color impression moved from amber to khaki-gold/tan (`#8E6400`/`#B28738`).

**The measured verdict on the danger constraint.** For `--warn`×`--danger` (state color against state color) a CVD ΔE of 8-or-more is physically impossible — red×amber always merges under deutan (the light-deutan baseline is 1.82, a hidden entry the facet table above never had). Re-picking the warn hue at 80 improved the binding worst case, light, from 1.82→3.33-level (dark drops 7.90→6.45-level but stays higher than light, so the worst case is unaffected). In normal vision it is ΔE 15.18/14.29-level, which satisfies 8-or-more. So the danger constraint is applied in two separate parts — **≥ 8 in normal vision + no regression in the CVD worst case** — and pushing warn in the hue↑ direction serves both goals, danger and media, as measured on the light worst case (danger does not become the ceiling at the optimum).

Everything else is safe: `life` × danger 12.30~21.74, `life` × warn cvd_min 25.99 (light) / 28.02 (dark), `neutral` × danger 9.73~11.86.

#### 2.1.4 Brand hue rules

The brand accent is **a single deep evergreen, hue 168, locked** (`#197459` / `#2EB88F`). **The hue is nailed at hue-168 and only L·C are specified per slot.**

| Token | Light | Light OKLCH | Dark | Dark OKLCH |
|---|---|---|---|---|
| `--accent` | `#197459` | `oklch(50.04% 0.0931 167.97)` | `#2EB88F` | `oklch(70.07% 0.1300 167.99)` |
| `--accent-hover` | `#096149` | `oklch(43.89% 0.0859 168.00)` | `#37CFA1` | `oklch(76.54% 0.1415 167.76)` |
| `--accent-tint` = `--bg-selected` | `#E1F9EF` | `oklch(96.19% 0.0284 168.81)` | `#0F2C22` | `oklch(26.73% 0.0400 168.06)` |
| `--on-accent` | `#FCFDFE` | (achromatic) | `#06120E` | `oklch(16.9% 0.020 171)` |

The hue drift caused by hex rounding is at most **0.81°**.

- Why green: (a) the brand hue must not collide with the state hues, and cutting the state hues down to red and amber leaves green empty (a separation of 143.7° / 88° from danger 24.3 / warn 80). (b) It fits the archival story of "kept / checked". (c) Button contrast survives in both modes at light 5.60 / dark 7.60 (§2.1.3, measured), and being a desaturated evergreen it never reads as neon. (d) **The sRGB gamut ceiling at the L 0.52-level is 0.105 for hue 168 against 0.269~0.289-level for the violet band — 2.6~2.8x narrower. It could not go neon if it wanted to.** Restraint enforced by physics beats restraint held by a rule.
- Rejected **accent candidates**: violet and indigo (the default of generative tools and the fingerprint of one particular product), cobalt blue (the default of dev tools, and it collides with the OS focus ring), magenta and rose (they merge with danger under color blindness), cyan (reads as a terminal in dark). **The blue band (205~275) is excluded from the tag palette too** — the neutral ramp of §2.1.1 sits at hue-257, so a blue family would land on top of the `neutral` chip at CVD ΔE 2.93~5.15-level (measured).
- **The app background (`--bg-canvas`) is a low-chroma version of the brand hue** (added 2026-07-29). The icon is deep
  evergreen, but opening the app gave you achromatic, and the two did not read as the same object. **It cannot become
  green while keeping its lightness** — at the L 0.97-level the sRGB gamut ceiling for hue 168 = 0.105-level, so raising
  chroma sixfold still gives `#EDF9F4`, still white. So L comes down: light `#DEF0E8` (L 0.94 C 0.022) / dark `#07140F`.
  **What sets the floor is the `craft` cover** — the closer the background is to green, the more the cover sinks into it
  (background↔craft cover 1.32 → 1.21 → 1.14:1), and `craft` is the most common facet, nineteen of the 42 tags.
  It stopped at the 1.21:1-point. Every contrast holds (fg-1 14.44 / fg-2 5.87 / accent 4.81 /
  line-control 3.40, all above the bar). This is not a solid, so it does not lengthen the list of four places below.
- **There are exactly four places that use a solid of the brand hue (168):** ① the focus ring ② the primary button ③ the rail of a selected row ④ the fill of a selected `craft` chip. `--accent-tint` is **for the selected row background only**. **⑤ The manual tag chip drops out of this list** — manual now takes that tag's own facet tint (§5.2). Any other use is rejected in review.
- **Managing color-blindness risk:** (a) is mandatory; (b) is mandatory for danger and advisory for warn. (a) State is never expressed by color alone — rail + text label + icon, all three. (b) **The three facet ink colors and danger are never used as fill colors side by side inside one component** (mandatory — the retry button is an achromatic outline). **The ban on parallel fill with warn is relaxed to advice** (re-picking the warn hue at 80 on 2026-07-24 moved `media`×warn CVD ΔE 1.53→6.75, clearing the acceptance line and missing the target line). The grounds are the measurements in §2.1.3 (`craft`×danger CVD ΔE 1.75, `media`×warn 6.75).

**The hue reservation table — a hue outside this table arriving as a token is rejected in review.**

| Band | Status | Grounds |
|---|---|---|
| `[4, 44]` | **Banned** | `--danger` 24.3 ±20° |
| `[62, 98]` | **Banned** | `--warn` 80 ±18° = [62, 98]. One hue unified across light and dark (re-picked 2026-07-24, 66.9°/74.3° → 80°) |
| `[133, 203]` | **Brand-reserved** | Brand 168 ±35°. The brand is also the largest tag family (`craft`), so anything around it turns into "another green" |
| `[205, 275]` | **Banned** | The neutral ramp hue (§2.1.1, `--fg-2` = `oklch(46.5% 0.024 257.5)`). A blue tag competes with "no family" |
| `[275, 300]` | **Banned** | AI violet (an editorial call — the §1.3 exclusion list) |
| `112` / `318` | **Available for tags** | 1 each out of the two remaining bands `[98, 133]` ∪ `[300, 360]` |

**The chroma budget (a mandatory rule at token level).**

| Element | Max chroma (Light / Dark) |
|---|---|
| Body, title and meta text, card backgrounds, borders | **0.024** (the neutral ramp ceiling) |
| Chip tint background | **0.028 / 0.040** |
| Chip ink text, selected chip fill | **0.092 / 0.106** |
| `--accent` solid, `--accent-hover` | **0.094 / 0.142** |
| `--danger` / `--warn` | danger 0.180 / warn 0.111 (light) · 0.109 (dark) — but only at **a size of ≤ 2px for a rail, or an 18px badge** |

What this table actually does: **the only things on screen allowed to use C ≥ 0.11-level are the brand accent (1 primary + 1 focus ring per screen) and dark-mode chip ink.** Lay 40 chips across the screen and the chromatic pixels exist **in the letter strokes only** (fill 0 = no background, fill 1 = a C 0.028 background). The instrument that prevents a rainbow is not the number of hues; it is this table.

#### 2.1.5 Semantic color: two hues and done (success and info have no color)

| Meaning | Color | Where |
|---|---|---|
| Success | **No color — achromatic** (no success-only hue is created) | The success toast (§4.10), the "connection OK" sentence. The accent is never reused as a success marker — the brand solid has only the four places of §2.1.4 |
| Warning | `--warn` | Duplicate save (the existing item is returned), typing a tag outside the dictionary, the API-key-not-set banner |
| Failure | `--danger` | `status: failed`, 4xx/5xx, delete confirmation |

An informational blue (info) is **not created.** Information is carried by neutral color and by a sentence. Adding blue would make the accent two things and **collide, at the same time, with the neutral ramp (hue 257) and the `neutral` chip under CVD** (measured ΔE 2.93~5.15 — the §2.1.4 hue reservation table). **No color is used for in-progress states (pending/scraping/tagging)** (an achromatic rail pulse). **Nothing at all is shown for done.**

#### 2.1.6 Dark mode implementation

- The web theme toggle is **3-state: light / dark / system**. The present situation, where `system` is unreachable from the UI, is treated as a bug.
- **Separate the preference from the resolved result.** The 3-state preference lives in `localStorage` only, and `<html>` **always carries exactly one resolved class (`.light` or `.dark`)**. Leave the class empty for `system` and `:root`'s light values apply as written, which is wrong under an OS in dark.
- **`color-scheme` follows the 3-state value exactly**: `:root { color-scheme: light dark }` (the pre-JS default) / `.light { color-scheme: light }` / `.dark { color-scheme: dark }`. Without the `.light` rule, on the path where the OS is dark and the user chose light, the scrollbar, the form controls and the default `input` background are the only things left dark.
- **Preventing the first-paint flash is the job of an inline script, not of CSS.** A render-blocking script in `index.html`'s `<head>` plants the class before the stylesheet applies (the code at the bottom of §3). The reason there is no `@media (prefers-color-scheme: dark)` block in the CSS: in a 3-state model the class has to beat the media query every time, and keeping both paths would need guards like `:root:not(.light)`, which splits the token definitions into two copies. The `data-loading` seal only blocks transitions; it does not block this flash.
- **Facet tokens also lock the hue and specify L·C per slot.** No global attenuation formula such as "−20% chroma in dark" is used — the measured attenuation ratio for these hues goes as far as 0.092→0.106 (upward), and `--accent` climbs 0.093→0.130-level as well. The reason is that the sRGB gamut ceiling is higher at dark L, and a global formula is wrong for these hues.
- The `light-dark()` CSS function is not used yet (as of 2026-07 it has not reached baseline, and it does not cover a manual toggle).
- **Dark thumbnail correction:** a white-background OG image glows, so `filter: brightness(.92)` + `box-shadow: inset 0 0 0 1px var(--line-1)`.

### 2.2 Typography

#### 2.2.1 Font stack

```css
--font-sans:  "Wanted Sans Variable", -apple-system, BlinkMacSystemFont,
              "Apple SD Gothic Neo", system-ui, sans-serif;
--font-mono:  "Geist Mono Variable", ui-monospace, "SF Mono", SFMono-Regular, Menlo, monospace;
--font-serif: Charter, "Iowan Old Style", Georgia, AppleMyungjo, "Nanum Myeongjo",
              "Apple SD Gothic Neo", serif;   /* 시간 척추 머리글 전용 — §2.2.5 */
```

**Wanted Sans (body and UI) + Geist Mono (machine data) are self-hosted.** Both are OFL 1.1-licensed, and
the license text is kept beside them in `design/fonts/`. The files live in `frontend/public/fonts/` (woff2) and
`ios/PushPoint/Resources/Fonts/` (ttf); the web registers them through `@font-face`, iOS through `UIAppFonts`.

serif stays the system stack — the one place it is used is the time-spine heading (§2.2.5),
so it is not worth shipping one more file.

#### 2026-07-28, a decision reversed — the original call was to use no web fonts

This section originally read: *"No web fonts are loaded."* There were three grounds, and when they were
actually checked, **only one held.**

| The original ground | What checking found |
|---|---|
| The web and iOS impressions match automatically | **True.** It was the one valid argument — except that **putting the same font on both sides** makes it hold the same way. What used to be automatic simply becomes explicit |
| A custom font makes iOS **lose Dynamic Type** | **Not true.** From iOS 14-onward `Font.custom(_:size:relativeTo:)` scales along with Dynamic Type. The §8-mapping changes to this shape |
| FOUT 0, requests 0, **binary growth 0** | **Re-measured, the ground is weak.** Measured (2026-07-28): the binary is 22 MB and dist 612 KB, while all of Wanted+Geist woff2 comes to **1,316 KB (+6%)**. It is a same-origin embed, so the request is local, and with `font-display: block` plus a local file there is effectively no FOUT either |

**The real reason left is tone.** The system stack means using the same letters as every iOS app and every
macOS website, and if you want the thing to look made, the typeface is the single highest-leverage move.

#### Why not Pretendard

It is the de facto standard among free Korean fonts, it is OFL, and its coverage is complete. **But its Latin is
Inter-based.** This product's screens permanently mix domains, URLs, tags and English titles, so the Latin
impression *is* the product impression — and Inter is the most common tech-product Latin there is right now. **It is a trade that swaps
"generic Apple" for "generic startup"**, which betrays the very purpose of building a tone.

Wanted Sans designed Hangul and Latin together (geometric + humanist), and its files are smaller as well.

#### Measured (2026-07-28, downloaded and weighed by hand)

| | Full woff2 | 2350-syllable common Hangul subset | Hangul coverage | Variable axis |
|---|---|---|---|---|
| **Wanted Sans Variable** | **1,259 KB** | 321 KB | 11,172 / 11,172 | `wght 400–1000` |
| Pretendard Variable | 2,010 KB | 456 KB | 11,172 / 11,172 | `wght 45–930` |
| **Geist Mono Variable** | **57 KB** | — | 0 (Latin only) | `wght 100–900` |

**The whole font ships, unsubsetted.** It is self-hosted, so the download is local or LAN, and a subset
opens fallback holes on rare syllables — the link titles a user saves are arbitrary Hangul.
Buying "in some titles the letters look different" to save 1.3 MB is a bad trade.

That the Wanted Sans weight axis **starts from wght-400** lines up exactly with the "nothing below 300" of §2.2.3.
Geist Mono having no Hangul is not a problem either — R2's mono targets are domain, URL, numbers and rank,
and every one of them is Latin (§2.2.4).

#### 2.2.2 Type scale (8-step, fixed in rem/px — `clamp()` banned)

`clamp()` and `vw` are not used on font size. **Type is a discrete staircase; only whitespace is fluid.**

The 2026-07-25 revision **only added two steps, `card` and `spine`; not one value of the existing six changed.** Tracking was tied to SF's optical curve (below), so shaking a size means recalculating the curve.

| Token | size / line-height | weight | letter-spacing | Use |
|---|---|---|---|---|
| `label` | 12px / 16px | 500 | `0` | Tag chips, status text, counts, small buttons |
| `meta` | 13px / 18px | 400 | `0` | Domain, save time, secondary description (a mono variant exists) |
| `card` | 13px / 20px | 400 | `0` | **The card's `description`, two lines**. `body` makes the card 8px taller and `meta` glues two Hangul lines together — between the two |
| `body` | 15px / 24px | 400 | `0` | Description, note, input field |
| `title` | 15px / 20px | 600 | `0` | Link title (clamped to two lines on the card) |
| `head` | 20px / 26px | 600 | `0` | Screen title, inspector title |
| `spine` | 21px / 28px | 600 | `0` | **Time-spine heading** (serif) |
| `display` | 32px / 36px | 600 | `-0.012em` | Detail screen title (iOS) |

#### 2026-07-29 — "the stats number" was taken out of `display`'s uses

This token is **the only one in the scale whose tracking is not zero** (−0.012em), and the ground for that was "the 32px stats number".
But there are no big numbers on the stats screen — 11 §8(3-1) rejected the layout that puts two big numbers at the top,
and the 14-redesign made the sentence the protagonist without rebuilding that slot. Half of the stated use was dead while
only the description stayed on.

**The token stays.** It has a live consumer — the title in the iOS `LinkDetailView`. And this table
**deliberately splits the inspector title (`head`) from the detail screen title (`display`)**: the web's inspector is
a drawer overlapping the board while the iOS detail is a whole screen pushed in, so they are not the same surface. The two
clients are each using what fits them, so there is no parity problem here — on 2026-07-29 that was mistaken
for a divergence, and the attempted fix stopped after the table got read again.

The ground for the −0.012em tracking also moves from "the 32px stats number" to "a 32px title". The value does not change.

#### 2026-07-28 — the SF optical curve was scrapped

This table originally carried a different negative tracking at every size (`label -0.006` … `title -0.016`,
and for `display` a **positive** `+0.004`), and the ground was **Apple SF's optical curve**. SF Pro is
drawn as two separate optical sizes, Text (≤19px) and Display (≥20px), each with its own recommended tracking,
and those values were carried straight over. The document even went as far as to pin down that "the 32px positive
tracking is not a typo, it is this curve".

**Change the font and the ground for that curve disappears wholesale.** Wanted Sans is not an optical-size family
but **a single master**, and its own spacing is already tuned to the screen size band.
Laying another font's optical correction on top of it means correcting twice.

So **the default goes back to zero, and an exception is made only where there are grounds.** There is one exception right now:

- **`display` −0.012em.** On the 32px stats number only. A single-master font looks loose in tracking at large
  sizes, and this slot is `tabular-nums` fixed-width digits, which makes that looseness especially visible.
  **The value was decided by putting it on screen and looking** — it was not derived from a table.

The Hangul exception is no longer needed. The original exception was "applying the curve's −0.022em to Hangul
makes the jamo collide", and with the curve gone there is nothing left to cap.

#### 2.2.3 Weight

**400 / 500 / 600 and nothing else. Nothing at 700 or above, nothing at 300 or below. No fractional variable-font weights (510/590)** (system fonts snap per platform, which makes them unpredictable).

| weight | Use |
|---|---|
| 400 | Body, description, meta |
| 500 | Labels, chips, active navigation |
| 600 | Link titles, screen titles, time-spine headings, primary buttons |

Hierarchy is made not from weight but from **the four color steps (fg-1/2/3/accent) and from size**. Start shouting with weight and everything in a density-7 list goes bold.

#### 2.2.4 Numbers, and what mono applies to (R2)

- Lists, stats, counts, times: `font-variant-numeric: tabular-nums`. Digit widths do not wobble while scrolling.
- The fields `--font-mono` applies to (only fields that exist in the contract — per `api/openapi.yaml`): `domain`, `url`, the display of `created_at` and `published_at`, `confidence`, `rank` (search bm25), `Tag.link_count`, and the numbers in `Stats`.
- Stays `--font-sans`: `title`, `description`, `note`, `author`, tag names, every UI label.
- A value that is not in the contract is never written down as a mono target. `updated_at`, request IDs and error IDs are **not in the contract** (`Error` has `code`/`message` and nothing else). When the schema grows later, add to this list then.
- Stylistic sets whose existence is uncertain (`ss01`, `cv01` and the like) are not declared. Their behavior cannot be guaranteed on SF.
- `-webkit-font-smoothing: antialiased` is not used (it thins text and causes low contrast).

#### 2.2.5 The one place serif is used (added 2026-07-25)

`--font-serif` is used **in exactly one place, the time-spine heading** — `오늘` / `어제` / `이번 주` / `7월`. It is not used on body, labels, controls, chips, buttons or empty states, and the moment that rule is broken the §1.3-ban on "no serif in body text" comes back to life.

- **Why only there.** On the spine line the human language (the label) and the machine language (the count, in mono) stand side by side. It is the one place where a typeface contrast says R2 once more.
- **The intent behind the order of the font stack.** Latin falls to Charter/Iowan (shipped with macOS), Hangul to AppleMyungjo. The browser falls back per glyph, so a Latin serif and a Hangul myeongjo mix naturally inside one line.
- **The principle of loading no web font stands** (§2.2.1). On non-Apple environments it falls to Georgia → generic serif, and even then the intent is met as long as it reads as distinct from sans.
- The iOS counterpart is `.font(.system(.title3, design: .serif))` (§8).

### 2.3 Spacing

**A 12-step scale. A padding, margin or gap value outside these 12 is a lint failure** (the layout constant tokens below are a separate list, and those too are referenced by name only).

| Token | Value | Main use |
|---|---|---|
| `space-2` | 2px | Micro-correction between icon and text (rail thickness is `--size-rail` 2px) |
| `space-4` | 4px | Vertical padding inside a chip, icon gap |
| `space-6` | 6px | Gap between chips, inline meta separation |
| `space-8` | 8px | Horizontal chip padding, vertical padding inside a button |
| `space-12` | 12px | Gap between elements inside a card, horizontal input padding |
| `space-16` | 16px | Right padding of a row, mobile gutter |
| `space-20` | 20px | Padding inside the inspector |
| `space-24` | 24px | List↔inspector gap, spacing between toolbar blocks |
| `space-32` | 32px | Desktop gutter, section spacing |
| `space-40` | 40px | Top margin of a screen |
| `space-56` | 56px | Vertical margin around an empty-state block |
| `space-80` | 80px | Bottom safety margin of a screen |

The small end is dense (2~8) and the large end opens up sharply (32~80). It is one scale that keeps the UI-density range (2~24) separate from the section-rhythm range (32~80).

Layout constants are values outside the 12-step spacing scale, so **every one of them is promoted to a named token**. Anonymous arbitrary values (`h-[76px]`, `w-[380px]`) are a lint failure — reference the token, as in `min-h-(--size-card-title)` or `w-(--w-inspector)` (§3, §9). The cover is the one exception and uses `aspect-[16/9]` — an aspect ratio is a proportion, not a dimension, and making it a token would only make it harder to read.

| Token | Value | Use |
|---|---|---|
| `--w-page` | 1200px | Max page width |
| `--w-content` | 768px | List content width (inspector closed). **A width constant, not a breakpoint** |
| `--w-inspector` | 400px | Inspector width (≥1024) |
| `--w-list-min` | 480px | Minimum list width (inspector open) |
| `--w-search-input` | 480px | Max width of the search input (≥1024, 11 §4(6)) |
| `--w-form` | 560px | Max width of a single-column form (settings screen, 11 §8(6)) |
| `--size-card-title` | 40px | The card title slot (= `text-title` 20px × two lines). The slot is held open even with no value |
| `--size-card-desc` | 40px | The card body slot (= `text-card` 20px × two lines) |
| `--size-thumb` | 56px | Thumbnail (≥560) |
| `--size-thumb-sm` | 44px | Thumbnail (`<560`) |
| `--size-header` | 56px | Header height (sticky, glass) |
| `--size-toolbar` | 44px | Toolbar height (sticky, opaque) |
| `--size-rail` | 2px | Status rail thickness (= `space-2`). **Shared by card, inspector and toast markers — this is the only rail thickness token there is** (§4.7) |
| `--size-spark-min` | 3px | Minimum width of one cell in the stats strip for 30 days (11 §8(6)). **An explicit exception to the 12-step spacing scale** — it is a dimension, not a spacing |
- `--size-cover-row` **44px** — one side of the cover in a compact row (§4.4.1). It is a **dimension**, not a spacing,
  so it sits outside the 12-step scale and still gets a name. 40px is on the scale, but the value has to equal the touch floor (§7.5)
  so that a row can never drop below it, and the name is kept apart from the existing `--size-thumb` (56) and
  `--size-thumb-sm` (44) because the use is different — this is not the inspector's thumbnail but
  the anchor of a list row.
| `--gutter` | `max(env(safe-area-inset-left), clamp(16px, 2.5vw, 32px))` | Left/right gutter |

**4 breakpoints: `<560` / `560~1023` / `≥1024` / `≥1280`.** A value outside this system (Tailwind's default `md` = 768px above all) never goes into a media query — the §3-reset erases the default breakpoint scale and keeps only `sm:560 / lg:1024 / xl:1280`, three of them, so that **`md:` does not exist at compile time**. The number of card columns is decided not by these breakpoints but by **the board container width** (`@board-sm` 460px / `@board-md` 760px) — when the inspector opens, the viewport stays where it is and only the board narrows.

The number of tag chips inside a row is controlled with **a container query (`@container`)** — the width available to a row changes as the inspector opens and closes, so judging by viewport width is wrong. Use `100dvh`; `100vh` is banned.

### 2.4 Radius

**A single `--radius` is banned. It is pinned per element under a semantic name.**

| Token | Value | Applies to |
|---|---|---|
| `radius-chip` | 999px | Tag chips, filter chips, status pills |
| `radius-control` | 10px | Buttons, inputs, selects, icon buttons |
| `radius-thumb` | 8px | Small image slots in the inspector and the shortcut overlay |
| `radius-card` | 16px | **LinkCard** (2026-07-25 — replaces the old `radius-row` 8px) |
| `radius-panel` | 16px | Inspector, popover, command palette, toast |
| `radius-sheet` | 20px | bottom sheet (top corners only) |

On 2026-07-25 every step grew. Once the card is the basic unit of the screen, an 8px corner reads not as a card but as "an angular block", and enlarging the card alone while leaving buttons and inputs behind splits the corner language of a single screen in two. **Holding `radius-panel` and `radius-card` at the same 16px is deliberate too** — the inspector is a card enlarged, not a different kind of surface.

### 2.5 Shadows and layers

```css
--ring:     0 0 0 1px var(--line-1);
--sh-card:  0 1px 2px rgb(13 18 16 / .05), 0 8px 24px -12px rgb(13 18 16 / .16);
--sh-lift:  0 2px 4px rgb(13 18 16 / .06), 0 18px 40px -16px rgb(13 18 16 / .22);
--sh-panel: var(--ring), 0 8px 24px -8px rgb(13 18 16 / .18), 0 2px 6px -2px rgb(13 18 16 / .10);
--sh-sheet: var(--ring), 0 -8px 32px -12px rgb(13 18 16 / .22);
/* dark: 그림자 알파를 증폭 (.40/.60 · .45/.70 · .55/.35 · .55) */
```

- Shallow multi-layer + a 1px hairline ring.
- **A card shadow has two steps and no more**: rest `--sh-card` → hover `--sh-lift`. There is no step in between and no pressed step. The old spec's "rows carry no shadow" was a rule premised on hundreds of 76px rows, and it was scrapped in the move to cards — but **the paint cost that grounded it is still valid**, so shadows are bound to two `box-shadow` layers, and once rendered cards go past two hundred it switches to virtual scrolling (§4.4).
- What moves on hover is **the shadow and a translate of no more than −2px**, and nothing else. Scale transforms are not used (in a grid they overlap the neighboring card).
- Glass (`backdrop-filter: blur(12px) saturate(1.6)`) exists in **exactly two places, the top bar and the command palette**, and under `@supports not (backdrop-filter: blur(1px))` it is made **more opaque** (progressive enhancement in reverse).

The z-index ladder — **any z-index outside these 7 is banned:**

| Name | Value |
|---|---|
| `z-header` | 100 |
| `z-popover` | 600 |
| `z-palette` | 650 |
| `z-overlay` | 699 |
| `z-sheet` | 700 |
| `z-toast` | 800 |
| `z-tooltip` | 1100 |

### 2.6 Motion tokens

```css
--dur-out:   120ms;   /* hover 이탈, 퇴장(팔레트·토스트·행 제거) */
--dur-1:     160ms;   /* 팔레트·컴포저 진입 */
--dur-2:     180ms;   /* 값 교체, 칩 추가/제거, 상태 전환 */
--dur-close: 200ms;   /* 인스펙터·시트 닫기 (열기보다 빠르다) */
--dur-flip:  220ms;   /* 목록 FLIP transform 보정 */
--dur-3:     260ms;   /* 인스펙터·시트·팔레트 열림 */
--dur-pulse: 2400ms;  /* 진행 레일 — 앱 내 유일한 무한 루프 */
--ease-ui:    cubic-bezier(0.4, 0, 0.6, 1);         /* UI 표준 곡선 */
--ease-enter: cubic-bezier(0.25, 0.46, 0.45, 0.94); /* 진입·퇴장 */
```

- **hover enters at 0ms (immediately) and leaves over `--dur-out`.** It is asymmetric. In a tool, hover that lights up slowly feels sluggish, and hover that goes out instantly flickers as the cursor passes over.
- **Springs and overshoot are banned outright.** Bounce on an instrument panel shaves precision. This ban applies to iOS unchanged (§8.2 — where `.spring` is used, `dampingFraction: 1.0` critical damping is fixed).
- **The allowed duration set is those 7 tokens above and nothing else** (120 / 160 / 180 / 200 / 220 / 260 / 2400ms). Writing a duration literal directly into CSS is rejected; reference the token. **The only exception is the 2 toast durations that run on JS timers** (4000ms / 8000ms, §4.10) — they are not CSS transitions, so they are not tokenized and the §4.10 table stays their source.
- What this rule governs is **transition and animation duration**. **A delay threshold is not a duration** — skeleton suppression 200ms (§4.9), tooltip delay 400ms (§4.11), search debounce 120ms (§4.2) and toast duration (§4.10) each have their own defining section as the source, and none of them join this set.
- lint does not hardcode this list; it reads the `--dur-*` tokens declared in `@theme` and generates it (§9).

## 3. Tailwind v4 `@theme` mapping

**The values live in `frontend/tailwind.css` and are not written again here.**

This section originally held **a copy of that file in full** (a 377-line block). So it went stale, inevitably —
as of 2026-07-29 the canvas value, the font stack, the eight tracking steps, the `@font-face` block and the `--cover-*`
tokens all differed from the actual file. `.claude/rules/docs.md` demands that "source and derivative are fixed in the
same piece of work", and **a 377-line derivative structurally cannot keep that demand.**

So the value copy is dropped and only **what the file cannot explain about itself** is kept. The two rules below
are exactly that — not values but **block order**, and break them and the build succeeds while the screen quietly breaks.


1. **The reset block (`--namespace-*: initial`) comes before every definition block.** v4 merges `@theme` in source order, and `initial` erases every key registered up to that point. Put the reset after `@theme inline` and **not one** semantic color utility gets generated (the §9 smoke check inspects this order). **Check this order first when adding a token, too — a new block always goes after the reset block.**
2. **Semantic colors must be `@theme inline`.** A non-inline `@theme` substitutes the value once at `:root`, so the `.dark` override never reaches the utilities. **This applies to the facet tokens (`--color-tag-*`) unchanged.**

## 4. Component inventory

Common rules:

- **The hit area has two levels, depending on the input method** (§7.5). Under `@media (hover: hover) and (pointer: fine)` (mouse and trackpad) it is **at least 24×24px** (WCAG 2.5.8 Target Size Minimum); under `@media (pointer: coarse)` (touch) it is **at least 44×44px**. The expansion uses `::before`, independent of the visual size. The 24px chip and the `sm` button are pointer-environment dimensions, and any component whose 44px expansion would overlap a neighboring target in a touch environment **is not displayed** in that range (for example, tag chips inside a row at `<560` → only the count is shown).
- Focus is the single `:focus-visible` ring defined in §2.6 and the §7-rules.
- disabled is `opacity: .45` + `pointer-events: none` + `aria-disabled`.

### 4.1 Button

| Variant | Background | Text | Border | Where |
|---|---|---|---|---|
| `primary` | `--accent` | `--on-accent` | none | Save, confirm. **At most 1 per screen** |
| `secondary` | `--bg-surface` | `--fg-1` | `1px --line-control` | Cancel, secondary actions |
| `ghost` | transparent | `--fg-2` | none | Row hover actions, toolbar icons |
| `danger` | transparent | `--danger` | `1px --danger` | The confirm button of the delete confirmation dialog (11 §5(4)) |

- **`danger` is never used as a fill color** (§2.1.4-b). The retry button is `secondary`. **There is no `danger-solid` variant** — the delete button in the confirmation dialog is that one outline variant too. 11-WEB-UX-SPEC.md refers to this variant by name only.
- Sizes: `sm` is 24px tall / `text-label` / 8px padding, `md` is 32px tall / `text-label` / 12px padding. Two of them, no more. `sm` is **pointer-environment only** — in a touch environment (`pointer: coarse`) it is promoted to `md` or expanded to 44×44px with `::before` (§4 common rules).
- States: `default` → `hover` (background `--bg-hover`, `--accent-hover` for primary, entering at 0ms / leaving over `--dur-out`) → `active` (no extra transform; transform is banned) → `focus-visible` (ring) → `disabled` → `loading` (the label stays, a 16px spinner is inserted on the left, the width is fixed, `aria-busy`).
- An icon-only button requires `aria-label` plus a connected Tooltip.

### 4.2 Input / Textarea

| Variant | Spec |
|---|---|
| `text` | 32px tall, `--radius-control`, background `--bg-surface`, border **`1px --line-control`**, `text-body`, 12px padding |
| `search` | `text` + a 16px search icon on the left + a clear button on the right (only when there is a value). Toolbar only, 32px tall |
| `url` | `text` + `--font-mono` + `inputmode="url"` + `spellcheck=false`. The save input only |
| `textarea` | `text` + `min-height 72px` + `field-sizing: content`. Notes only |

- States: `default` / `hover` (the border stays `--line-control`, background `--bg-hover` — even this combination lands at 3.45:1 / 4.04:1-level, over the 3:1-bar) / `focus-visible` (**an `outline` ring. `outline-none` plus a border color change is banned** — the current handling on the three inputs is an accessibility defect, so all of them are replaced) / `invalid` (border `--danger` + a 12px `--danger` message beneath + `aria-invalid`) / `disabled` / `readonly` (background `--bg-hover`, cursor default).
- The placeholder is `--fg-3`. **A placeholder is never used in place of a label** — where there is no visible label, attach a `VisuallyHidden` one.
- Search needs no Enter. A 120ms debounce + `?q` `replaceState` sync.
- `--line-1`/`--line-2` (alpha hairlines, 1.2~1.5:1) are not used on an input border. The input's background is the same `--bg-surface` as the page, so **the border is the only visual signal of the control boundary**, and that is a target from which WCAG 1.4.11 demands the 3:1-bar (§7.1).

### 4.3 Chip (tag / filter)

24px tall, `--radius-chip`, `text-label`, padding `4px 8px`, gap 6px.

**The variant axis is not `four variants × facet` but `three levels × four facets` — there is no combinatorial explosion.** The hue is decided by that tag's facet (§2.1.2), and the fill level alone encodes state (R1).

| fill level | When | Background | Text | Border |
|---|---|---|---|---|
| **0** | Attached by the machine (`source: rules`/`embed`), display only | `transparent` | facet `ink` | none |
| **1** | Attached by a person (`source: manual`) **or** an unselected chip in the filter bar | facet `tint` | facet `ink` | none on display chips / **`--line-control` on filter chips only** |
| **2** | Matches the current `?tag` filter | facet `ink` | `--bg-surface` | none |

- **fill-0 has no border.** The display chips of a row or the inspector are `readonly`, so they are not subject to the "control boundary" of WCAG 1.4.11 (§7.1), which is what entitles them to drop the border. Put a border on the chips of a 38~41-chip screen and the list turns into a grid.
- **A filter bar chip is a control, so it keeps its `--line-control` border** — this is the single point that does not change from the previous spec (the same reason as the §4.2-rule).
- **Inside a selected row, only the `fill 1` chip drops to fill-0.** `--bg-selected` and the `craft` tint are the same value, so the **fill 1 background** collides with the row background; the reason holds for fill-1 alone, so the rule applies to fill-1 alone. **A chip that matches the current `?tag` filter keeps fill 2 (solid) even inside a selected row** — the branch order comes from the §5.2-algorithm, which evaluates `selected` first (`onSelectedRow` is used in the `manual` branch only). As a side effect, the row you are looking at gets quieter.
- The grounds for the shape choice (measured): a solid fill sits on a cliff where, at a shared L of 0.55→0.60-level against white text, the passing hues collapse 18/18 → 0/18-level, so it was cut; a chromatic dot did not meet the user's request, so it was cut; a color bar on the left collides head-on in shape with S1 (the row's leading-edge rail), so it was cut; and an outline on display chips turns the list into a grid once forty of them are laid out, so it was cut. Only a tint background + chromatic ink decouples contrast from hue (§2.1.3, the chip-interior contrast table).
- States: **a display chip (`readonly`) has no hover.** Only filter chips carry state — `hover` (at fill-1 the background becomes `--bg-hover`, **at fill-2 the fill is kept exactly as it is** — no per-facet hover token is made; selection is already at maximum strength, so there is nothing left to emphasize) / `focus-visible` / `pressed` (applied immediately — optimistic) / `disabled`.
- A filter chip carrying a count is `name` + 6px + `count (mono, --fg-3)`. **When selected (fill 2), the count color is a 70% mix toward `--bg-surface`** — because no per-facet `on-color` is made, and `--bg-surface` alone clears the 4.5:1-bar on all 4 facets (§2.1.3).
- **There is no `excluded` variant.** The contract's tag filter is **1 `tag` parameter** (exact match) and nothing else, so an exclusion syntax like `-name` cannot be sent. If it becomes necessary, the meaning of the `tag` parameter in `api/openapi.yaml` gets extended first (that is 11 §10-territory, the contract-gap list).
- The decision algorithm is §5.2; the operating rules are §5.3.

### 4.4 LinkCard — the basic unit of the board (2026-07-25, promoted from a row)

```
┌─────────────────────────┐
│ ▍ 커버 16:9 (썸네일 또는 생성 커버)  │   ▍ = 2px StatusRail (S1)
├─────────────────────────┤
│ 제목 (title / 2줄 클램프)          │
│ 본문 (card / 2줄 클램프)           │
│ [칩] [칩] [칩]                   │
│ 도메인 · 저장시각 (meta/mono)  [액션] │
└─────────────────────────┘
```

| Item | Value |
|---|---|
| Width | The column count is decided by **the board container width** — 1-column / `@board-sm` (460px) 2-column / `@board-md` (760px) 3-column. Why the container and not the viewport is §2.3 |
| Cover | `aspect-ratio: 16/9`. It has to be **a ratio, not a pixel height**, for the slot to be reserved at every column width |
| Cover fallback | The generated cover (§4.5). Neither a gray box nor a domain initial (R4) |
| Title slot | `--size-card-title` 40px = `text-title` 20px × two lines |
| Body slot | `--size-card-desc` 40px = `text-card` 20px × two lines. **`description` is used here** — the contract already hands over a 200-character allowance in the list response |
| Chip slot | At least 24px (the chip height). At most 3; they wrap when they overflow |
| Background and corners | `--bg-surface` + `--radius-card` 16px |
| Shadow | rest `--sh-card` → hover `--sh-lift`. **Two steps only**, with nothing in between |
| Selection | `ring-2` accent + `--sh-lift` (the background color is not changed — the cover already occupies the background) |
| `note` present | Shown by a single icon next to the title (the content lives in the inspector) |

- **The height of every slot is settled at mount time.** The slot is held open even with no value yet, so the board does not shift while the workers fill it in (CLS 0). That is the precondition on which S2 stands.
- States: `default` / `hover` (the shadow rises and nothing else; the translate stays within −2px) / `focus-visible` (ring) / `selected` (accent ring) / `pending` (empty slots + a pulsing rail) / `failed` (danger rail + a failure sentence in the body slot + a retry button).
- **The trailing action (open the original) is exposed on hover and focus only.** Under `@media (hover: none)` it is always visible.
- **The rule that hid chips below `<560` is retired.** In a row, three 24px chips and a 44×44 hit area could not coexist inside 76px; on a card the chips get a line of their own.
- **Virtual scrolling:** once rendered cards go past two hundred it switches to `@tanstack/react-virtual` (the target is 100k links). Card height is deterministic given the column width, so virtualization stays cheap.
- **The density cost is acknowledged.** In a 900px viewport about ten rows were visible; cards give 6~9-per-screen at three columns. This revision trades a little scanning density for **a body you can read plus a cover that always exists**, and that call was made on 2026-07-25.

### 4.4.1 The compact row (iOS only) — what gets taken out

```
[커버 44pt]  제목 (1~2줄, 아래 예산)
             도메인 · 오후 2:20
```

What stays: **cover · title · domain · time.** What goes: **body (`description`) · tag chips.**
The failure row (§4.7) is conditional but **does not go** — a recovery nobody finds is no recovery.

#### 2026-07-30 — flipped from taking out the cover to taking out the body and chips

The first cut (2026-07-29) of compact was **taking out the cover and keeping title, tags and meta**. This section flips
that decision and leaves the grounds for flipping it — because a flipped decision gets flipped back when there are no grounds.

**① The purpose of this list is re-finding, and under that condition the image is the most expensive thing to remove.**
Teevan et al. (CHI 2009, N=276) measured "finding" and "re-finding" separately. In re-finding,
**a small image plus a title was fastest and text-only was slowest.** Woodruff et al. (2002) likewise found that only the
image+text combination was best, or statistically tied for best, across every task category. Taking out the cover
optimizes a metric (rows per screen) against the purpose (re-finding).

**② Taking out the body text, by contrast, has grounds.** Cutrell & Guan (CHI 2007) split snippet length into
one line / 2~3-line / 6~7-line and eye-tracked it. **On navigational tasks (where you already know what you are looking for — this product's
usage condition) one line and 2~3-lines were indistinguishable in time**, and as lines grew the gaze **left the positional information (the URL)**
and results became harder to tell apart. Their recommendation applies directly — *"emphasize short snippets and
positional information such as the URL."*

**③ Chips are the most expensive pixels.** They read as pressable (the §4.3-fill ladder strengthens that
impression), they break the title's left alignment and get in the way of skimming, and they duplicate a tag filter this app already has.
And **the §4.4-rule of "at most 3" is not what the tagger actually produces** — a measured average of 3.74, with 49% at
five (golden set of 189, runtime `tagger.Classify`). In card mode the chips get a line of their own so there is no
problem, but putting them on the same line in compact resurrects the 76px row collision that the §7.5-note recorded as retired.

**④ "One line" is not the floor.** Even Apple's densest mail row is **two lines** —
`Preview: None` removes the body snippet only and keeps sender, subject and date unconditionally. And measured,
one line **cuts 79% of titles** (domain and time are mono, so they eat a lot of width) and at AX3 the meta alone
overflows the row width, so **189/189-cases break.**

#### The line budget — the title claims first

`--lines-compact: 2`. **The title claims first, and if no line is left, that is that.** It is not "a switch that shows the
body" but **a single integer shared with the title**. Compact has no body, so today it works only as a ceiling on the
title; the reason it is kept in budget form is that a row height has to be **nearly constant** for skimming to
work — NetNewsWire experimented with variable-height cells and rejected them for scannability, and it is the same argument as the
§1.3-grounds for excluding masonry ("a grid of ragged heights shaves scanning speed").

#### Dimensions and accessibility

- The cover is a **`--size-cover-row: 44px`** square and sits **trailing**. 44px is the same value as the touch floor (§7.5),
  so it cannot shrink below that. It is trailing because of NN/g's rule — when the image is secondary
  information, keeping the title column flush-left is better for skimming.
- **44px is below the recognition floor.** Kaasten et al. (2002) measured that a thumbnail has to exceed 96×96px to
  trigger recognition, and that at 64px or below **only color and layout** carry information. So what lives in this slot
  is not "a picture you read" but **a color and layout anchor**. R4's generated cover is built out of exactly those two channels
  (facet color + FNV-1a geometry), so at this size it is in fact better than a photo.
- The row height is **not fixed** (§8.3). It is a `@ScaledMetric` minimum height and grows when the content does.
- **At accessibility sizes a different layout is used** — not a clamp. Body 17pt becomes 53pt at AX5, so a 44pt
  one-line row cannot be built at that size. Split on `.isAccessibilitySize`, drop the meta onto
  its own line and lift the cover above the title (the same intent as the §8.5-rule for `ViewThatFits`).

### 4.5 GeneratedCover (R4)

Draws the cover of a link that has no thumbnail. **It is not "a placeholder for a missing image"** — `thumb: failed` + `status: done` is a normal terminal state, so for a large share of links this *is* the final cover.

| Layer | Value |
|---|---|
| Ground | The dominant tag's **facet cover** (`--cover-*`). With no tag, `neutral` |
| Pattern | The facet **ink** at alpha 0.16 (0.13 for `stack` alone). Four of them: `hatch` (diagonals) / `lattice` (dots) / `contour` (contour lines) / `stack` (steps) |
| Pattern choice | `FNV-1a(domain)` decides the kind, the rotation (−2°~+2°) and the spacing (12~28px) |
| Wordmark | The domain in mono, bottom left. Laid on **generated covers only** — it is unreadable over a photo, and the meta line already says the domain |

- **The dominant tag** = the first tag in the chip sort order (manual first → confidence descending → name). That is, whatever the user attached by hand if there is one, otherwise whatever the tagger was most confident about.
- **What the hash decides is geometry only.** Color comes from the facet alone — that boundary is what keeps the §5.4-ban, "tag color hashing is permanently excluded", alive as written. Two links from the same domain look alike **because they share a pattern, not because a hash invented a color.**
- The implementation is canvas. Not a CSS gradient, because the §1.3-list bans gradients and because the patterns are geometric shapes, which makes canvas the honest choice. Unlike CSS it cannot repaint itself when the theme changes, so **it subscribes to the resolved theme and redraws** (the store in `lib/theme.ts`).
- iOS uses the same FNV-1a and the same four patterns (`ios/design/prototype.html`). The hash has to match for the same domain to come out with the same pattern on both clients.

#### 4.5.2 The cover ground is not the chip tint (separated 2026-07-29)

The cover originally used the chip's `--tag-*-tint` as its ground. **The screen made it obvious that the two requirements differ.**

- The chip tint is a value picked so that **12px ink text reads on top of it**, which makes it very bright.
- The cover is the card's large surface (web 16:9 / iOS 3:1 — §8.5) and the only lettering that lands on it is the domain wordmark.

Using the same value floated the cover off the card at **1.08:1** — effectively a white rectangle. On a real device
it read as "flat", and re-measuring found the cause was not the background color but this
(the canvas/surface separation of 1.07:1-level is the same value macOS, Notion and Linear use, so it was fine).

`--cover-*` is a separate token that **keeps the hue and lowers only L** (light L 0.87 / dark L 0.32).
The hue lock (§2.1.4) is untouched — color still comes from the facet alone and the hash still decides geometry alone (R4).

| facet | Light | Wordmark contrast | Card contrast | Dark | Wordmark contrast | Card contrast |
|---|---|---|---|---|---|---|
| `craft` | `#A7E4CB` | 6.80:1 | 1.41:1 | `#193A2E` | 8.21:1 | 1.42:1 |
| `media` | `#D5D9A4` | 5.21:1 | 1.44:1 | `#333519` | 6.88:1 | 1.40:1 |
| `life` | `#E8C6F3` | 8.07:1 | 1.49:1 | `#3C2B42` | 8.15:1 | 1.36:1 |
| `neutral` | `#CED6D3` | 4.69:1 | 1.45:1 | `#2F3432` | 5.88:1 | 1.39:1 |

**The wordmark contrast is what sets the floor on these values.** All eight clear the 4.5:1-bar and the lowest is neutral
light at 4.69:1-level — lower L any further from here and that line breaks first.

### 4.5.1 The stats and settings surfaces — surfaces that hold no links

**No surface is laid down.** They sit directly on the canvas, and sections are divided by a 1px `--line-2` rule and nothing else.
No cover, no rail.

#### 2026-07-29 — the Panel component was scrapped

This section originally specified a `Panel` of `--bg-surface` + `--radius-panel` + `box-shadow: var(--ring)` + 20px
padding, and even wrote down two variants (`plain`/`interactive`). **Neither the web nor iOS had an implementation** —
both clients built canvas + rules instead, and they arrived at the same answer without ever consulting
each other.

The screen is adopted over the spec. There are three grounds:

- **The original section carried no argument.** It is a list of values with no sentence about why `surface`. Every other
  decision in this document comes with grounds like a contrast number or a CVD ΔE. It reads as a section written before the
  screen and never read again.
- **`--bg-surface` is the surface of a link card.** §1.4 S2 made that card the app's only choreography, and
  R4 fills that surface with a cover. Put settings and stats on the same surface and **chrome and content end up at the same visual
  rank** — which is exactly the distinction this system spends most of its budget on.
- **The `interactive` variant had nowhere to go.** Stats and settings contain no pressable surface. That one of two specified
  variants was never once needed is itself the signal that the section did not come from a screen.

To bring it back: when a surface appears that holds no links and yet **has to be pressed**. At that point there is a real
question for `interactive` to answer, so this verdict gets looked at again.

### 4.6 Badge

Badges are **not used for state** (R3). That leaves exactly two uses.

| Variant | Style | Where |
|---|---|---|
| `count` | Background `--bg-hover` + `--fg-2` + mono + `--radius-chip` + 18px tall | Tag counts, result counts |
| `notice` | Background `--warn-tint` + `--warn` + `--radius-chip` + 18px tall | Duplicate save (the existing item is returned), a tag outside the dictionary |

- **`notice` sits at CVD ΔE 6.75-level against `media` facet ink** (it was 1.53 before the warn hue was re-picked at 80 on 2026-07-24 — §2.1.3). That clears the acceptance line (≥6) but misses the target line (≥8), so while the ban on parallel fill is advisory, the badge keeps the practice of **always coming with a sentence**, never carrying a tag name, and never sitting shoulder to shoulder with a chip.

### 4.7 StatusRail (S1)

A `--size-rail` (2px) vertical stroke on the card's leading edge. It replaces the 5-color mapping (neutral/blue/violet/emerald/red) of `StatusBadge.tsx`.

| State | Rail | Accompanying marker |
|---|---|---|
| `done` | **Transparent** (nothing at all) | None |
| `pending` / `scraping` / `tagging` | `--rail-progress`, pulsing `opacity .7↔1` over `--dur-pulse` (2400ms) | Skeleton slots |
| `failed` | `--danger`, solid | A "실패" label on the right of the row + a retry button + an `alert-triangle` icon |
| `selected` | `--accent`, solid | Background `--bg-selected` |

- **There is one thickness, `--size-rail` (2px).** Card or top of the inspector, it is the same value — R3 pinned it as "a 2px vertical stroke" (§1.2 · §1.4), and the chroma budget table of §2.1.4 nailed the size ceiling of a chromatic rail at **2px**, which makes a 4px `--danger` or `--accent` rail a budget violation. Hierarchy is not made from thickness but from position (top of a panel vs. the leading edge of a card). An anonymous arbitrary value (`w-[4px]`) is a lint failure.
- **Chips became chromatic and the rail's color vocabulary did not grow (R3).** State is still `--rail-progress` / `--danger` / `--accent`, three colors plus one transparent, and a facet hue never appears on a rail.
- **Semantic tokens only.** Setting a rail color straight from the raw palette, such as `--ink-400`, is a lint failure (§2.1.1). The semantic token for the progress rail is `--rail-progress` (light `--ink-600` / dark `--ink-400`) and it is registered in §2.1.2.
- **The pulse floor is `.7` rather than `.35` because of contrast.** R3 retired the badge, so the rail is the only visual signal of an in-progress state, which makes it a target of WCAG 1.4.11 (non-text 3:1). At `.7` it is light 3.38 / dark 3.68 (lowest over all backgrounds 3.13 / 3.44), so it clears the 3:1-bar even at the floor (§2.1.3). The old spec's `--ink-400` + `.35↔.8` was 1.38~2.22-level in light, under the bar.
- Under reduced-motion the pulse becomes **a static `opacity: 1`** (removal, not reduction — §7.4). The static value is 5.94:1 / 5.57:1-level, over the 3:1-bar as well.
- Per the ban on color-alone expression, the rail is always accompanied by `VisuallyHidden` status text (`aria-label="수집 중"`). That text does not, however, stand in for the visual contrast requirement — the 3:1-bar above has to be satisfied separately.
- When `failed` and `selected` coincide, `selected` wins (selection is the user's present intent).

### 4.8 EmptyState

One `text-head` title line + one `text-body`/`--fg-2` description line + 0~1 action buttons. 56px of vertical margin. **No illustration, no mascot, no emoji.**

| Context | Title | Description | Action |
|---|---|---|---|
| 0 links (first run) | `저장된 링크가 없습니다` | `URL을 붙여넣으면 제목과 태그가 자동으로 채워집니다.` | Focus the save input |
| 0 search results | `검색 결과가 없습니다` | `다른 단어를 쓰거나 태그 필터를 해제해 보세요.` | Reset filters |
| 0 results under a tag filter | `조건에 맞는 링크가 없습니다` | `선택한 태그 조합에 해당하는 항목이 없습니다.` | Reset filters |
| Filtered to failures only | `실패한 링크가 없습니다` | `모든 링크가 정상 처리되었습니다.` | None |

**An empty state is never rendered while `isLoading`.**

### 4.9 Skeleton

- A block of **exactly the same dimensions** as the real content. Background `--bg-hover`, `--radius-thumb` (thumbnail) / 4px (text lines).
- **Skeletons do not move.** No shimmer, no fade loop of their own — the whole app has exactly one infinite loop, the progress rail pulse (§1.4-S1, §6.1), and several skeletons blinking out of phase across a list is noise in itself. The signal that something is in progress is already given by the rail in the same row.
- When the real value arrives, skeleton→value crossfades over `--dur-2` (§6.1). That is the only motion a skeleton takes part in.
- **The 200ms rule:** if the response comes back within 200ms, no skeleton is shown at all (this is a **threshold**, not a motion duration, so it is unrelated to the §2.6 token set). A skeleton that blinks and vanishes feels slower than no skeleton.
- There are **two** loading expressions: `Skeleton` (first entry into a list or a row) / `Spinner` (a single action, 16px, **only while a request is in flight** — a finite rotation bound to a request's lifetime, which is what distinguishes it from the "only infinite loop" of the §6.1-rule). No "inline dots" — waiting on a scrape is already expressed by the rail, and there is no reason to make the same information move twice.
- **CLS 0** — thumbnails, skeletons and row heights are all reserved up front.

### 4.10 Toast

**This section is the source of the toast visual spec.** "When a toast is raised" and the error code → UI mapping are owned by the 11 §1.4-original, and the two documents do not duplicate values.

- **Position: bottom center, fixed.** Identical on desktop and mobile; there is no bottom-right variant. On mobile it floats above `env(safe-area-inset-bottom)`.
- **1 at a time.** A new toast **replaces** the previous one (no stack, no queue). There is never a moment when two toasts overlap on screen.
- Container: `--bg-elevated` + `--sh-panel` + `--radius-panel`, max width 360px, padding 12px 16px, `z-(--z-toast)`.
- Composition: **one sentence + at most 1 action**. No icon, no emoji.

| Variant | Color | Duration | Example |
|---|---|---|---|
| `success` | **Achromatic** (`--fg-1` text, no marker) | 4000ms | `저장했습니다` |
| `warn` | A 2px `--warn` marker on the left | 4000ms | `이미 저장된 링크입니다` + `열기` |
| `error` | A 2px `--danger` marker on the left | **No auto-dismiss** (manual close, or the next toast) | `저장에 실패했습니다` + `다시 시도` |
| `undo` | Achromatic | 8000ms | `삭제했습니다` + `실행 취소` |

- **The accent is not used on success.** The brand solid has exactly four places (§2.1.4) and a toast marker is not among them. Put color on a success toast and R1 breaks — a toast is the system's response, not a person's intervention, and hue encodes identity only. Color is used for failure (`danger`) and warning (`warn`) alone.
- States: `entering` (`opacity 0→1` + `translateY(4px→0)`, `--dur-2`) / `visible` / `leaving` (`--dur-out`) / `paused` (the timer stops on hover or focus).
- `role="status"` (success/warn/undo) / `role="alert"` (error). There is 1 `aria-live` region per app.
- No color-alone — even the variants with a marker have a sentence that states the status outright (erase the marker and the meaning survives).
- The 4000ms·8000ms-timers are JS values, not CSS duration tokens (the two values registered as exceptions to the allowed set of the §2.6-tokens).

### 4.11 The remaining primitives (Radix, owned directly)

| Component | Radix | Notes |
|---|---|---|
| Inspector / Sheet | `Dialog` | ≥1024-and-up is a non-modal right-hand panel, below that a modal sheet (the per-range shape is 11 §6(6)) |
| CommandPalette | `Dialog` + our own list | `Cmd+K`. One of the two glass surfaces |
| Tooltip | `Tooltip` | 400ms delay, disabled on touch devices |
| Popover (sort, filter) | `Popover` | |
| ConfirmDialog (**tag dictionary deletion only**) | `AlertDialog` | Default focus is the cancel button. **Not used for deleting a link — the undo toast does that instead (11 §1.2)** |
| VisuallyHidden | `VisuallyHidden` | Status text, icon button labels |

## 5. Tag color strategy

### 5.1 The conclusion: color belongs to four facets, not to tags

**The palette size is 3 chromatic + one achromatic. No individual tag gets a color, and neither hashing nor user-chosen colors are introduced.**

Giving 30~50 tags 30~50-odd colors is mathematically impossible (it collapses from seven colors on, at CVD ΔE 6.2-level). The question is "how many can we get", and under this system's constraints the answer is not six, not five, not four, but **3**. The hue space is already nearly all reserved (§2.1.4, the hue reservation table):

```
danger  ±20°   →  [  4 ,  44]  금지
warn    ±18°   →  [ 62 ,  98]  금지
brand   ±35°   →  [133 , 203]  금지 (브랜드가 곧 최대 패밀리이므로 그 주변은 "또 다른 초록"이 된다)
중성 램프 hue  →  [205 , 275]  금지  ← 이 시스템만의 제약
AI 바이올렛    →  [275 , 300]  금지 (편집 판단)
────────────────────────────────────
가용            [ 98 ,133] ∪ [300 ,360]
```

**"The neutral ramp hue is banned" is the decisive constraint unique to this project.** §2.1.1 defined the twelve neutral steps around hue 225, and measured, `--fg-2` is `oklch(46.5% 0.024 257.5)`. That is, this app's gray is a **blue gray**, and adding a blue tag family puts "the blue tag" in competition with "no family". Fitting four into the two remaining bands gives two purple-ish families, which get confused even in normal vision.

| Chromatic families | worst normal ΔE | worst CVD ΔE | Verdict |
|---|---|---|---|
| 3 | **10.19 / 11.06** | **7.49 / 8.37** | Adopted |
| 4 (no blue) | 9.88 | 6.17 | Not enough margin |
| 5 (with blue) | 6.30~8.15 | 2.93~5.15 | **Fails** |

All three assignment mechanisms are pinned by rule. **No hashing** (§5.4), **no per-tag user-chosen color** (§5.4), and **the tag → facet mapping is owned by the server** (`Tag.facet`; the user picks only a facet, from four options, on the tag management screen — 11 §5).

For the same reason **no color hash is used on the thumbnail fallback either** (the domain's first letter + `--bg-hover`) — a fallback is about the domain, not the tag, and a domain has no facet.

### 5.2 The chip decision algorithm (two axes: hue × fill)

A chip's style is **fully determined** by the pure function below. **Hue is decided by facet, fill by state (R1).** The branch order is the priority order.

```ts
type TagFacet = 'craft' | 'media' | 'life' | 'neutral'   // api/openapi.yaml 의 TagFacet

/** facet -> 토큰 이름. 여기가 색이 등장하는 유일한 지점이고, 값이 아니라 이름만 있다. */
const FACET_TOKENS: Record<TagFacet, { ink: string; tint: string }> = {
  craft:   { ink: 'tag-craft-ink', tint: 'tag-craft-tint' },
  media:   { ink: 'tag-media-ink', tint: 'tag-media-tint' },
  life:    { ink: 'tag-life-ink',  tint: 'tag-life-tint'  },
  neutral: { ink: 'fg-2',          tint: 'hover'          },  // 새 토큰을 만들지 않는다
}

type TagSource = 'rules' | 'embed' | 'manual'      // api/openapi.yaml 의 tag.source
type ChipInput = {
  facet: TagFacet
  selected: boolean                 // 현재 ?tag 필터와 일치 (계약상 tag 파라미터는 1개)
  source: TagSource | null          // null = 필터 바 (링크에 붙은 태그가 아님)
  role: 'control' | 'readonly'      // 필터 바 칩 = control, 행/인스펙터 칩 = readonly
  onSelectedRow: boolean            // 선택된 행 안에서는 tint 가 행 배경과 겹친다
}

/** 반환값은 토큰 이름만. raw hex 를 만들지 않는다. */
function chipStyle(i: ChipInput) {
  const t = FACET_TOKENS[i.facet]
  if (i.selected)           return { bg: t.ink,  fg: 'surface', border: 'transparent' }   // fill 2
  if (i.role === 'control') return { bg: t.tint, fg: t.ink,     border: 'line-control' }  // fill 1 + 컨트롤 경계
  if (i.source === 'manual' && !i.onSelectedRow)
                            return { bg: t.tint, fg: t.ink,     border: 'transparent' }   // fill 1
  return                           { bg: 'transparent', fg: t.ink, border: 'transparent' } // fill 0
}
```

`bg: 'transparent'` and `border: 'transparent'` are Tailwind static utilities (`bg-transparent`/`border-transparent`), so they are unaffected by the §3-palette reset.

| Axis | What it encodes | How it is expressed |
|---|---|---|
| **hue** | Identity (what that tag is about) | `craft` 168 / `media` 112 / `life` 318 / `neutral` achromatic — §2.1.2 |
| **fill** | Intervention (who attached it, and is it on now) | 0 transparent (machine) / 1 tint (person, control) / 2 solid (selected) — §4.3 |

There is no exclusion-filter branch — the contract does not offer one (§4.3).

`manual` taking **that tag's own facet tint** is the most visible expression of the revised R1. Where a tag I touched by hand is attached is the clue for re-finding, and it matches the product story in which that correction becomes M5 re-ranking training data through `tag_feedback`. **`--accent-tint` does not appear here** — the brand tint went back to being for the selected row background alone (§2.1.4).

**2 re-evaluation triggers (measured).** The former "manual ratio 30%" trigger is retired — fill-1 is a facet tint rather than the accent, so more manual tags no longer bleed brand color.

1. **When the `neutral` chip ratio on one screen goes past 40%**, the facet assignment has failed and the dictionary gets cleaned up.
2. **When the `craft` ratio goes past 75%**, color has lost its information, so either `craft` gets split or the 3-color palette itself is reconsidered.

### 5.3 Chip operating rules

The two lines below are judged only on **data the contract actually provides** (`Tag.link_count` = the global running total).

- **Only tags with a global `link_count === 0` are hidden from the chip bar on the list screen.** The tag management screen shows the zero-count ones too (they are what is being managed). "Hide what is 0 within the current filter" is **impossible** — a filter-scoped count is not in the contract (11 §10-5).
- **Sorting is `link_count` desc → name.** A "times attached in the last 30 days" sort is not implemented for lack of data: `Stats.by_day` is a link count, not a per-tag one, and client-side aggregation collides head-on with the 100k-link target (11 §10-2). Introducing it requires first adding a per-tag period count to `api/openapi.yaml`.
- **The filter bar is not sorted by facet.** The sort is the rule above and nothing else — let color govern position too and the two channels become redundant.
- 1 fixed `태그 없음` chip pinned at the front.
- The filter bar is **one line, fixed**. On overflow, a `+N` button → the full list in a popover.
- `confidence` is not shown in the list. It appears only in the inspector, as a mono number, and `manual` leaves the slot empty (per the contract, `confidence: null`).

### 5.4 Grounds for exclusion (permanent)

The three below are not up for discussion again.

1. **`hash(name) % palette.length` — permanently excluded.** The moment `palette.length` changes the whole set is reshuffled, and change a `name` and that tag's color flips. Color has nothing to do with meaning, so there is nothing for the user to learn from it. A server-given facet, by contrast, is the input to a pure function — **the same tag is always the same color, and adding, deleting or renaming a tag never moves another tag's color.**
2. **A user-chosen color per tag — excluded.** GitHub labels (arbitrary hex + a randomize button) and Notion select (automatic random assignment) created exactly the rainbow problem. For Notion there is even a separate guide on "how to turn every select gray".
3. **The blue family (hue 205~275) — excluded.** The neutral ramp sits at hue-257, so it lands on top of the `neutral` chip at CVD ΔE 2.93~5.15-level (measured — the §5.1 table, the §2.1.4 hue reservation table).

**The boundary against R4 (the generated cover) — added 2026-07-25.** The §4.5-cover hashes the domain. The reason that does not break the first item above is **that the hash's output is geometry, not color**. Only the pattern's kind, rotation and spacing come from the hash; the ground and stroke colors come straight from the dominant tag's facet token. So a change in the dictionary reshuffles no colors, and changing the domain moves no facet color on that link. **The moment the hash touches the color channel it is a violation of the first item.**

**For the record.** The old spec carried an unsealing trigger: "if tags go past a hundred, resume assigning six colors to top-level groups". That condition was **not met** — on 2026-07-22 the user asked for "brand color to be visible on tags" and the design changed because of it. That is a change of requirement, not an unsealing, so the trigger sentence is deleted. Three of the 4 conditions at the time (a declared mapping, passing 4.5:1 contrast, color as a secondary channel) do survive and are satisfied by this section and §2.1.3; only the fourth ("exclude the accent hue from the group palette") was deliberately flipped — the brand hue taking the largest family, `craft`, directly is the core of the revised R1.

### 5.5 Facet definitions and assignment

**The judging criteria.** The Korean labels exposed in the UI are fixed at four — `만드는 것` / `형식` / `세상과 일상` / `분류 없음` — and both clients use the same words (§8.1).

- `craft` — references I **use directly** in what I make. The reason to open it again is "to use it". `ai`/`llm`/`data` sit here because this product's NLU is itself that domain.
- `media` — tags where **the form itself is the information**. It tells you the time cost of opening it again (a three-minute article or a forty-minute video). It is the highest-value scanning axis in a link archive, which is why it gets a color of its own.
- `life` — the world, daily life, and myself. `science` (a reading topic) and `productivity`/`career` (how you work is not what you make; it is about you) are the boundary cases, and the sentence above is the criterion.
- `neutral` — no facet (UI label `분류 없음`). **A new tag that was not in the dictionary is born `neutral`**, and it gains a color when the user picks a facet on the tag management screen (11 §5).

**Assignment of the 30 seed tags** — `craft 18 (60%) · media 5 (17%) · life 7 (23%)`.

| # | tag | facet | # | tag | facet |
|---|---|---|---|---|---|
| 1 | `dev` | **craft** | 16 | `llm` | **craft** |
| 2 | `golang` | **craft** | 17 | `data` | **craft** |
| 3 | `kubernetes` | **craft** | 18 | `design` | **craft** |
| 4 | `ios` | **craft** | 19 | `article` | **media** |
| 5 | `swift` | **craft** | 20 | `video` | **media** |
| 6 | `python` | **craft** | 21 | `tutorial` | **media** |
| 7 | `rust` | **craft** | 22 | `news` | **life** |
| 8 | `javascript` | **craft** | 23 | `science` | **life** |
| 9 | `frontend` | **craft** | 24 | `finance` | **life** |
| 10 | `backend` | **craft** | 25 | `career` | **life** |
| 11 | `database` | **craft** | 26 | `productivity` | **life** |
| 12 | `devops` | **craft** | 27 | `book` | **media** |
| 13 | `security` | **craft** | 28 | `podcast` | **media** |
| 14 | `opensource` | **craft** | 29 | `travel` | **life** |
| 15 | `ai` | **craft** | 30 | `life` | **life** |

**`craft` at 60% is design, not a defect.** When twenty-four of 40 chips carry the brand color the screen looks "like this app", and the remaining sixteen split into two colors, so **scanning becomes the job of finding the exception** — one podcast in a heap of dev links, one travel link, and it jumps out immediately. The opposite design (six groups, evenly distributed) makes every row multicolored and nothing jumps out at all. The objection that "a color covering 60% carries low information" is true, and that is exactly why **`craft` was given the brand color and made to carry identity rather than information.**

**4 supporting mechanisms that keep color from carrying meaning alone** (the §7.1-rules reference this list).

1. **The name text is always there.** A chip always renders its own tag name — this is what satisfies WCAG 1.4.1. Facet color is not an information channel but **an accelerator**, and that premise is what legalizes the CVD 6~8 band of §2.1.3.
2. **Fill level is a shape channel.** The manual/selected distinction is not hue but the presence or inversion of a background, so it is preserved under every CVD type.
3. **The `?tag` filter state is also in the URL and in the toolbar sentence** — even if you cannot see which chip is solid, the sentence "태그: golang · 12건" says it. The number there is that tag's **global `Tag.link_count`**, not the filtered result count (there is no total count in the contract — 11 §10-7).
4. **Facet is exposed as text on the tag management screen (11 §5).** A path exists inside the app to check what a color means by means other than color.

## 6. Motion rules

### 6.1 Where motion is used (exactly six places)

**This table is the list of uses for the allowed duration set of §2.6.** Every value is a token and there are no literals (the §9 lint cross-checks this table against the `--dur-*` declarations). Every motion that appears in 11-WEB-UX-SPEC.md has to be inside this table — a screen spec inventing a transition that is not here is rejected.

| Where | Property | duration | easing | Grounds |
|---|---|---|---|---|
| Row insertion and filling right after a save (S2) | The optimistic new row, `opacity 0→1` + `translateY(-4px→0)` | `--dur-2` (180ms) | `--ease-enter` | A new row comes in from above the list. The container height is already at its final value on the first frame, and correcting the rows below is the next FLIP row's job |
| " | skeleton→value crossfade | `--dur-2` (180ms) | `--ease-ui` | The signature. It shows the product's core claim as it happens |
| " | Thumbnail `opacity 0→1` | `--dur-3` (260ms) | `--ease-enter` | Softens the arrival of a late value |
| " | Height change, FLIP `transform` | `--dur-flip` (220ms) | `--ease-ui` | Removes the layout jump (animating `top/height` is banned) |
| Row removal (delete) | `position: absolute` snapshot + `opacity 1→0` + `scaleY(1→.96)` | `--dur-out` (120ms) | `--ease-ui` | It has to leave the document flow immediately so the rows below start their FLIP on the same frame |
| " | FLIP `transform` on the rows below | `--dur-flip` (220ms) | `--ease-ui` | The same correction as on insertion |
| Inline edit expansion (tag dictionary) | `grid-template-rows: 0fr → 1fr` | `--dur-2` (180ms) | `--ease-ui` | Interpolates the grid track instead of animating height directly. The content's `opacity` enters late, over `--dur-out` |
| Inspector open | `transform` + `opacity` | `--dur-3` (260ms) | `--ease-enter` | Signals a context change while the scroll position is kept |
| Inspector close | Same | `--dur-close` (200ms) | `--ease-ui` | Asymmetric (closing is faster) |
| Command palette / composer entry | `opacity 0→1` + `translateY(-4px→0)` | `--dur-1` (160ms) | `--ease-enter` | Feedback for switching to a new layer. **`scale` is not used** — in a tool-shaped UI, zooming is more distracting than a 4px move, and this keeps it on the same property as row insertion |
| Command palette / composer exit | Same | `--dur-out` (120ms) | `--ease-ui` | |
| The in-progress status rail | `opacity .7↔1` | `--dur-pulse` (2400ms, infinite) | `--ease-ui` | The **only** infinite loop in the app. Real system state: the worker is alive |

- **Interpolating `grid-template-rows` is an approved exception to the "no animating `top`/`height`" rule.** The reason for the ban is that it forces layout every frame, and `0fr → 1fr` costs the same — except that inline edit expansion happens **on one row at a time and only right after a user action** (not on hundreds of rows mid-scroll), and the alternative `max-height` trick is worse because it requires guessing the content height. This exception applies to **exactly one** thing, the inline edit row of the tag dictionary (11 §5(7)), and never to list rows.

Secondary transitions (subordinate to the six places above):

| Target | Enter | Leave |
|---|---|---|
| hover background/text color | **0ms (immediate)** | `--dur-out` |
| Value swap (text, number, status sentence) | `--dur-2` | `--dur-2` |
| Chip add/remove | `--dur-2` | `--dur-out` |
| Toast | `--dur-2` | `--dur-out` |
| Sheet (mobile) | `--dur-3` | `--dur-close` |
| `visibility` | — | `step-end` / `step-start` as a companion transition, so it leaves the focus order and hit testing exactly |

- `will-change` is reclaimed on a 1:1-basis (wherever it is set, it is `unset` when the transition ends).
- **Motion driven by JS (FLIP, the Web Animations API) is out of reach of the CSS seal.** `element.animate()` is not a target of `[data-reduce-motion] * { animation: none }`, so right before it runs, check `matchMedia('(prefers-reduced-motion: reduce)').matches` and apply the final state immediately with `duration: 0` (§7.4).

### 6.2 Where motion is not used (explicit bans)

Scroll reveals, staggered list-item entry, card lift or scale on hover, page transition animations, number count-ups, tag count animations, parallax, marquee, custom cursors, springs and overshoot, vertical `scroll-snap`.

### 6.3 What "responsiveness" actually means here

It is a latency budget, not choreography. The UI has to exploit the fact that the server is local and the save API's p99 is under 50ms (the performance targets are in [00-README.md](00-README.md)).

| Rule | Value |
|---|---|
| Skeleton suppression | No skeleton if the response comes back within 200ms |
| Optimistic application | Save, tag toggle, delete and note save are applied to the UI immediately without waiting on the server, with a rollback + an `error` toast on failure |
| Page transitions | 0ms. Routing is immediate |
| Search | No Enter needed. A 120ms debounce + `?q` `replaceState` |
| CLS | 0 (every dimension reserved up front) |

## 7. Accessibility standards

### 7.1 Contrast

- Body, labels and interactive text: **4.5:1 or better**.
- **The scope of the non-text 3:1 rule is "state and control boundaries".** The targets are exactly three: (a) state markers (progress/failure/selection rails, status icons), (b) control boundaries (the borders of inputs, selects, **filter bar chips** and secondary buttons = `--line-control`), and (c) the focus ring. **The display chips of rows and the inspector are excluded from (b)** — they are `readonly` and therefore not controls, which is why they render without a border at every fill level (§4.3). **Decorative hairlines are an exception too** — row dividers and card rings (`--line-1`/`--line-2`) carry no information themselves, and removing them leaves the structure standing on row height, spacing and background contrast, so they are not targets of WCAG 1.4.11. In exchange, using those two tokens on a control boundary is banned (§4.2 · §4.3).
- `--fg-3` (light 2.66~2.83 / dark 3.96~4.25) is **for duplicated information only** — domain, relative time, placeholder, disabled text. The grounds and scope are §2.1.3; any other use is banned.
- The build gate (§9): text combinations at 4.5:1, `--line-control` and the state colors at 3:1. Alpha colors are calculated **after actually being composited (flattened)** onto the background.
- **No expression by color alone.** State is always color + text + icon/shape, all three. That said, having supporting text does not exempt anything from the visual contrast requirement (§4.7). It is checked under dark + Increase Contrast + Reduce Transparency as well.
- **A tag chip has four supporting mechanisms** (the name text always present / fill level as a shape channel / the filter state in the URL and the toolbar sentence / the facet as text on the tag management screen). The list is owned by the §5.5-original, and it is because those four exist that facet color is treated as an accelerator rather than an information channel.

### 7.2 Focus

```css
:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
:focus:not(:focus-visible) { outline: none; }
```

- The gap is made with `outline-offset` (rather than a double ring built from box-shadow). It does not get clipped inside an `overflow: hidden` container and it does not collide with the shadow tokens.
- **`outline: none` plus a border color change is banned.**
- Modals (Dialog/AlertDialog/Sheet/Palette) trap focus and return it to the trigger on close. Radix provides this by default, so it is not implemented by hand.
- A "skip to content" link at the very top of the page (visible only on focus).

### 7.3 Keyboard navigation

**The shortcut table is owned solely by the 11 §1.2-original.** Values are not duplicated here — the moment there are two copies, the `Cmd+Enter` double binding and the `Esc` handling-order conflict come back. This section defines not a key list but **the accessibility requirements of list navigation**.

- The list has **1 roving tabindex** — thousands of rows must not enter the tab order.
- **An action that is visible only on hover has to be visible on focus as well.** No action is created that a keyboard user cannot reach.
- `aria-label` on every icon button. The list is `role="list"`, rows are `role="listitem"` + `aria-selected`.
- The results of optimistic application and asynchronous state changes are announced through one `aria-live="polite"` region (`3건 저장 완료`).
- An action that has a shortcut **must always also have a pointer-reachable path** (no shortcut-only features). The `?` overlay renders the 11 §1.2 table as-is.
- The multi-select shortcuts (`X`, `Shift+↑↓`) and theme cycling (`T`) **do not exist** (11 §1.2 · §8.5).

### 7.4 Motion accessibility

- `prefers-reduced-motion: reduce` → `[data-reduce-motion]` sets `transition-duration: 1ms` + `transition-delay: 0` + `animation: none` + `animation-delay: 0`. **The delay has to go to zero along with it** or choreography like "enters after a 120ms delay" survives.
- **The infinite loop (the rail pulse) is replaced by a static `opacity: 1`** — removal, not reduction (the static value clears the 3:1-bar as well, §4.7).
- **JS motion is outside the CSS seal.** FLIP and the Web Animations API (`element.animate()`) are not targets of `animation: none`, so before running, check `matchMedia('(prefers-reduced-motion: reduce)').matches` and apply the final state immediately at a duration of zero.
- Keep the MQL object and **react to a settings change in real time** through its `change` event (do not stop at reading it once at startup).
- The iOS counterpart is `@Environment(\.accessibilityReduceMotion)` (§8.2).
- No autoplay, no auto-scroll, no auto-carousel (banned by the §6.2-list to begin with).

### 7.5 Everything else

- **Two hit-area levels** (§4 common rules): pointer environments (`hover: hover` + `pointer: fine`) at least **24×24px** (WCAG 2.5.8 Target Size Minimum, AA), touch environments (`pointer: coarse`) at least **44×44px**. In both environments the minimum spacing between adjacent targets is 4px.
- A dense component whose 44px expansion would overlap a neighboring target is not displayed in that range — put 3 chips of 24px with a 6px gap inside a 76px row and expand each to 44px and they physically overlap, so below `<560` the in-row chips are replaced by a count (§4.4).
- No horizontal scrolling may appear at 200% text zoom (inside a row it is `flex-wrap`).
- Images: a thumbnail is decorative, so `alt=""` + `role="presentation"` (the title is right next to it). The same goes for the fallback initial.
- Language: `<html lang="ko">`. Latin proper nouns get no separate `lang` marking.

## 8. The iOS mapping table (M4)

This section alone has to be enough to build the same design in SwiftUI. **The minimum deployment target is iOS 17** — the mapping below presumes iOS 17 APIs such as `ContentUnavailableView`, `.sensoryFeedback` and `.scrollTargetBehavior`.

### 8.1 What is shared (it comes out of one original)

- **The whole set of semantic color tokens** (**every row** of the §2.1.2 semantic token table: canvas/surface/hover/elevated/selected, fg-1..3, fg-inverse, line-1/2, line-control, rail-progress, accent/accent-hover/accent-tint/on-accent, danger/danger-tint, warn/warn-tint — light and dark as a pair) **+ the 6 facet palette tokens** (the ink and tint of tag-craft/media/life). If state colors and tag colors differ between the two clients, the product is broken.
- **The `Tag.facet` → token mapping rule** (the 4-line `FACET_TOKENS` table of the §5.2-algorithm). **iOS does not copy hex; it picks its own asset from the contract's `Tag.facet`** — that is what "comes out of one original" actually means in practice.
- **The 12-step spacing scale**, and the **semantic names** of the radii (chip/control/thumb/row/panel/sheet).
- **The 3 concept rules (R1/R2/R3)**.
- **The Korean label dictionary**: 5 states (`대기` / `수집 중` / `태깅 중` / `완료` / `실패`), **4 facets** (`만드는 것` = craft / `형식` = media / `세상과 일상` = life / `분류 없음` = neutral), 5 navigation labels, the search mode labels, the error code copy. The two clients must not use different words.
- **The fallback rules**: `title` empty string → `domain` → `url` / the `thumb_url: null` fallback (the domain initial) / an empty `author`, `lang` or `error` **hides the row itself** / `confidence: null` (manual) leaves the slot empty / for `jobs.tag` and `jobs.thumb`, **a missing field and failed are different states** / `thumb: failed` + `status: done` is normal.
- **Information hierarchy**: the field order of a list row and the field order of the inspector are each kept identical **between web and iOS**. What is matched on a 1:1-basis is **client against client**, not row against inspector — the row (thumbnail→title→domain→chips) and the inspector (title→domain→thumbnail→actions→tags→note→description→meta→jobs) are different structures to begin with and ought to differ (11 §3(2) · §6(2)). The same contract plus the same information hierarchy is the substance of the claim that "the two clients are peers".

### 8.2 Token mapping

| Web token | SwiftUI counterpart | Notes |
|---|---|---|
| `--bg-canvas` | Asset Catalog `Color("canvas")` (Any/Dark, two values) | An explicit value rather than a system color. Matching the state colors comes first |
| `--bg-surface` | `Color("surface")` | List row background |
| `--bg-hover` | `Color("hover")` | iOS has no hover → **for the pressed state and fallback backgrounds only** |
| `--bg-elevated` | `Color("elevated")` | sheet background |
| `--bg-selected` | `Color("selected")` | The selected row in edit mode |
| `--fg-1/2/3` | `Color("fg1"/"fg2"/"fg3")` | `.primary`/`.secondary` are not used (the values diverge) |
| `--fg-inverse` | `Color("fgInverse")` | Text on deep backgrounds |
| `--line-1/2` | `Color("line1"/"line2")` | Decorative hairlines only. `Divider().overlay(Color("line1"))` |
| `--line-control` | `Color("lineControl")` | The border of inputs and **filter bar chips**. **It is a 3:1 target, so it is never swapped for the system default border** |
| `--rail-progress` | `Color("railProgress")` | The progress rail. Light `#515A67` / dark `#9099A6` |
| `--accent` / `--accent-hover` | `Color("accent")` + `.tint(Color("accent"))` / `Color("accentHover")` | Also registered as the AccentColor asset (system control tint). The hover counterpart is pressed |
| `--accent-tint` / `--on-accent` | `Color("accentTint")` / `Color("onAccent")` | Selected row background / primary button text. **Not used on manual chips** (the facet tint takes that job) |
| `--tag-craft-ink` / `--tag-craft-tint` | `Color("tagCraftInk")` / `Color("tagCraftTint")` | The craft chip. **hex is never carried over by hand; the asset is picked from `Tag.facet`** (§8.1) |
| `--tag-media-ink` / `--tag-media-tint` | `Color("tagMediaInk")` / `Color("tagMediaTint")` | The media chip |
| `--tag-life-ink` / `--tag-life-tint` | `Color("tagLifeInk")` / `Color("tagLifeTint")` | The life chip |
| `neutral` facet | No asset — reuses `Color("fg2")` / `Color("hover")` | No new token is created (§5.2 `FACET_TOKENS`) |
| `--danger` / `--warn` | `Color("danger")` / `Color("warn")` | |
| `--danger-tint` / `--warn-tint` | `Color("dangerTint")` / `Color("warnTint")` | Failure and warning banner backgrounds |
| `--font-sans` | **Wanted Sans** (`UIAppFonts`, ttf). `Font.custom(_:size:relativeTo:)` keeps Dynamic Type | Uses **the same font files** as the web (§2.2.1). It has to be loaded by the PostScript instance name, and `.weight()` does not move the variable axis |
| `--font-mono` | `.monospaced()` / `.font(.system(.footnote, design: .monospaced))` | The R2 target fields are identical |
| `tabular-nums` | `.monospacedDigit()` | |
| type scale | **Mapped by role, not by size** (§8.3) | Dynamic Type is mandatory |
| spacing, 12-step | `enum Space { static let s2: CGFloat = 2 ... }` + `@ScaledMetric` | At large type the spacing grows along with it |
| `radius-chip` | `Capsule()` | |
| `radius-control` | `10` (`.continuous`) | **`.continuous` is mandatory** |
| `radius-thumb` | `8` (`.continuous`) | |
| `radius-card` | `16` (`.continuous`) | Rows became cards, and this replaced `row` (§4.4) |
| `radius-panel` | `16` (`.continuous`) | The inspector is a card enlarged, so it is the same value (§2.4) |
| `radius-sheet` | The system sheet handles it | We do not draw it ourselves |
| `--ring` / `--sh-panel` | `@Environment(\.displayScale) private var displayScale` → `.overlay(shape.strokeBorder(Color("line1"), lineWidth: 1 / displayScale))` | Shadows are delegated to the system sheet. **`UIScreen.main` has been deprecated from iOS 16-onward** (it gives the wrong value under multi-scene and Catalyst) |
| The z-index ladder | Solved with presentation layers (sheet/alert/overlay) instead of `.zIndex` | No values to port |
| `--dur-*` | **Durations are ported as they are.** The default is `.easeInOut(duration:)` — open `.26`, close `.20`, value swap `.18`, exit `.12`, entry `.16`, FLIP substitute (`.animation`) `.22` | The §2.6-ban on overshoot applies to iOS as well |
| Spring | Where `.spring` is used, **`dampingFraction: 1.0` (critical damping) is fixed** — e.g. `.spring(response: 0.26, dampingFraction: 1.0)` | Below 1.0 overshoot appears, which violates §2.6 |
| `--dur-pulse` | `.easeInOut(duration: 1.2).repeatForever(autoreverses: true)` for `opacity .7↔1` | One-way 1.2s so that a round trip lands at 2.4s-total. It cannot be expressed with `.spring` |
| `--ease-*` | `.easeInOut` (= `--ease-ui`) / `.easeOut` (= `--ease-enter`) | The cubic-bezier coefficients are not ported (SwiftUI has no identical curve) |
| reduced-motion | `@Environment(\.accessibilityReduceMotion) private var reduceMotion` → when `true`, leave the rail pulse at a static `opacity 1` and set transitions to `nil` | The iOS counterpart of the §7.4-rule. Removal, not reduction |

The Swift token skeleton (a hand-written original in the shape `design/tokens/` will generate when M4 starts). **`DS.Palette` has to carry every row of the §2.1.2 semantic token table plus the six facet tokens** — the moment it becomes a subset, iOS cannot produce the color of a tag chip or a progress rail. The Asset Catalog names map on a 1:1-basis to the strings in the §8.2 table above.

```swift
enum DS {
    enum Space {                       // 12스텝. 이 외 값 금지
        static let s2: CGFloat = 2,  s4 = 4,  s6 = 6,  s8 = 8
        static let s12: CGFloat = 12, s16 = 16, s20 = 20, s24 = 24
        static let s32: CGFloat = 32, s40 = 40, s56 = 56, s80 = 80
    }
    enum Radius {                      // chip 은 Capsule() 로 대체 — 값이 없다
        static let control: CGFloat = 6, thumb: CGFloat = 6
        static let row: CGFloat = 8, panel: CGFloat = 12
    }
    enum Palette {                     // Asset Catalog (Any/Dark). §2.1.2 전 행
        static let canvas = Color("canvas"), surface = Color("surface")
        static let hover = Color("hover"), elevated = Color("elevated")
        static let selected = Color("selected")
        static let fg1 = Color("fg1"), fg2 = Color("fg2"), fg3 = Color("fg3")
        static let fgInverse = Color("fgInverse")
        static let line1 = Color("line1"), line2 = Color("line2")
        static let lineControl = Color("lineControl")
        static let railProgress = Color("railProgress")
        static let accent = Color("accent"), accentHover = Color("accentHover")
        static let accentTint = Color("accentTint"), onAccent = Color("onAccent")
        static let danger = Color("danger"), dangerTint = Color("dangerTint")
        static let warn = Color("warn"), warnTint = Color("warnTint")
    }
    enum TagFacetColor {              // Tag.facet(계약) -> asset. hex 복제 금지
        static let craftInk = Color("tagCraftInk"), craftTint = Color("tagCraftTint")
        static let mediaInk = Color("tagMediaInk"), mediaTint = Color("tagMediaTint")
        static let lifeInk  = Color("tagLifeInk"),  lifeTint  = Color("tagLifeTint")
        // neutral 은 새 asset 을 만들지 않는다 — Palette.fg2 / Palette.hover
    }
    enum Motion {                      // §2.6 토큰 이식. 오버슈트 금지
        static let out = 0.12, enter = 0.16, swap = 0.18
        static let close = 0.20, flip = 0.22, open = 0.26
        static let pulseHalf = 1.2     // 왕복 2.4s
    }
}
```

### 8.3 Type scale mapping (by role, not by size)

| Web token | SwiftUI TextStyle | Default size | Notes |
|---|---|---|---|
| `label` | `.caption` (500 → `.weight(.medium)`) | 12/16 | Chips, counts |
| `meta` | `.footnote` | 13/18 | Domain and time. The mono variant is `design: .monospaced` |
| `body` | `.subheadline` | 15/20 | Description, note |
| `title` | `.subheadline` (600) | 15/20 | **The same 15pt as the web.** It used to read "deliberately different at 17pt because it is a touch scale", but putting the same custom font on both sides (§2.2.1) removed the reason to keep that divergence — the two clients looking like the same product matters more |
| `head` | `.title3` | 20/25 | Screen title |
| `display` | `.largeTitle` or `.title` | 34/41 | Detail screen title |

- **Tracking is ported at the same values as the web** (revised 2026-07-29). It used to read "SF applies optical tracking automatically, so no `.tracking()` is added", but the font is not SF now. The default is zero and there is exactly one exception, `display` (§2.2.2), so `PP.Tracking` carries that table over as-is.
- Sizes change with Dynamic Type. **The 76px row height is not ported as a fixed value** — it becomes a `@ScaledMetric`-based minimum height that grows when the content does.

### 8.4 Component mapping

| Web component | SwiftUI | Difference |
|---|---|---|
| Button `primary` | `.buttonStyle(.borderedProminent).tint(DS.Palette.accent)` | |
| Button `secondary` | `.buttonStyle(.bordered)` + `.overlay(Capsule/RoundedRectangle.strokeBorder(DS.Palette.lineControl))` | The border is a control boundary, so the 3:1 token is drawn |
| Button `ghost` | `.buttonStyle(.plain)` + an icon | There is no hover, so it is always visible or replaced by a swipe |
| Button `danger` | `.role(.destructive)` | The system handles the color |
| Input `text` / `url` | `TextField` + `.textFieldStyle(.plain)` + `.textContentType(.URL)` | We draw the custom border ourselves — `lineControl`, 1pt |
| Textarea | `TextField(axis: .vertical)` | |
| Chip | `Text` + a `Capsule` background. **fill 2 = a facet ink fill + `surface` text / fill 1 = facet tint + facet ink / fill 0 = transparent + facet ink.** Only filter chips get a `lineControl` border | The algorithm (§5.2 `chipStyle`) is ported as-is (with no exclusion branch). Hue is decided by `Tag.facet` |
| Row | `List` row + a leading 2pt rail | `.listRowInsets` / `.listRowSeparatorTint(Color("line1"))` |
| Row actions | **`.swipeActions`** (open the original / retry / delete) + `.contextMenu` | There is no such thing as hover exposure |
| StatusRail | `Rectangle().frame(width: 2)` + **`.accessibilityHidden(true)`** | The same four states as the web. A Shape is not an accessibility element by default, so attaching only `.accessibilityLabel` either fails to expose it to VoiceOver or creates a pointless stop inside the row |
| Row accessibility | The whole row gets `.accessibilityElement(children: .combine)` + `.accessibilityValue("수집 중")` (the 5 Korean labels of §8.1) | A `failed` row gets `.accessibilityHint("두 번 탭하면 재시도")`. This is the iOS path for conveying state on a channel other than color |
| Badge `count` | `Text().monospacedDigit()` + Capsule | |
| EmptyState | `ContentUnavailableView` | The copy is exactly the §4.8 table |
| Skeleton | `.redacted(reason: .placeholder)` | The 200ms rule applies identically |
| Toast | A bottom-**center** overlay view (owned directly), 1 at a time | The visual spec is §4.10 as written. No abusing system alerts; `undo` comes with `.sensoryFeedback` |
| Inspector | `.sheet` + `.presentationDetents([.medium, .large])` + a drag indicator | |
| CommandPalette | `.searchable` + `.searchScopes` | Search, not a palette, is the native idiom |
| Tooltip | **None** | Put a label next to the icon, or replace it with `.contextMenu` |
| Segment (3-state) | `Picker` + `.pickerStyle(.segmented)` | Used for the theme only. **Take the system segment as it is and do not paint it** — this is the screen that changes the theme, so a colour of ours that half-follows the new theme reads as a fault |

### 8.5 What follows platform idiom (deliberately different)

**Cover aspect ratio — web 16:9 / iOS 3:1.** The board is 2~3-column on the web, so one card is 1/3rd of the screen,
but an iPhone is one column and the same ratio would give two cards per screen. The cover's job is "recognizing what this was",
and that job does not get half the screen. **The geometry the hash decides and the color the facet decides are the same;
only the ratio differs** — on 2026-07-29 this divergence was "fixed" to match the spec by changing iOS to a 16:9-ratio, then reverted.
The grounds were in a code comment but not in this table, so it read as "a spec violation".

**The type slot of the stats sentence — web `body` 15/24/400 / iOS `head` 20/26/600.** The axis that splits them is **the vessel
that holds it**. On iOS the stats are a whole tab, so the sentence leads the screen; on the web they are one section inside settings, sitting
under a `title` 15/600-weight section heading. Raise the web to 20px and **the sentence outgrows its own heading and the hierarchy
inverts.** Raising only the weight is not the answer either — the §2.2-rule settles that "hierarchy is made not from weight but from
the four color steps and from size". The "the sentence is the protagonist" of 14 §D6 already holds through **order and the placement of evidence**:
the sentence comes first and the bars for 30 days become its evidence underneath.

**The letters of the sentence have to be the same** (13 §3). What differs is only the typesetting, and that is the intent of this table.

| Concern | Web | iOS |
|---|---|---|
| Navigation | 1 sticky top bar | `NavigationStack` + `.navigationTitle` (large). **No custom header** — it loses the large-title transition and the system background effects |
| Multi-select | **P3, deferred on both web and iOS** (there is no batch API — 11 §10-3) | Until a batch endpoint exists in the contract, iOS edit mode is not built either. An implementation where selecting N means N requests does not come back in the name of parity |
| Primary action position | Top right | Bottom / toolbar trailing (one-handed reach) |
| Glass | The top bar and the palette, two places only | System components acquire it automatically. **No custom glass on the content layer** |
| Icons | lucide | SF Symbols. **The assets are not shared, only the semantic names** (licensing + optical alignment) |
| Theme toggle | light / dark / system, 3-state | **Not provided** (HIG: avoid per-app appearance settings). Documented as an explicit exception among peer clients |
| Tap targets | Pointer 24×24px / touch 44×44px (§7.5) | Always 44×44pt (a touch-only platform) |
| Large type handling | `flex-wrap` | `ViewThatFits` switches a horizontal layout to a vertical stack |
| Motion | ms tokens + cubic-bezier | **The same durations, ported as `.easeInOut(duration:)`** (§8.2 `DS.Motion`). `.spring` is allowed only at `dampingFraction: 1.0` — the ban on overshoot is common to both platforms |
| Save entry point | The URL input field (+ a bookmarklet) | The Share Extension (2-second entry) — an OS feature, so the web has none ([13 §1 ② axis](13-CLIENT-PARITY.md)) |

## 9. Implementation verification gates

Work that applies this design system can be declared complete only after passing all of the following.

- **The CSS smoke check (first of all)**: after `just web-build`, grep the generated CSS for `.bg-canvas` / `.text-fg-1` / `.bg-accent` / `.border-line-control` / `.text-tag-craft-ink` / `.bg-tag-life-tint`. Zero hits means the §3-block order is broken (drop the reset `@theme` below `@theme inline` and every semantic color utility disappears — the build succeeds, so without this check you do not find out until you open the screen). In the same output, confirm that `.bg-slate-500` and `md:` (768px) media queries are at **0** as well.
- **The contrast gate**: calculate the combinations below for light and dark separately. Alpha colors are calculated after being flattened onto the background.
  - Text at **4.5:1**: `fg-1`·`fg-2` × (`bg-canvas`, `bg-surface`, `bg-hover`, `bg-elevated`, `bg-selected`), `accent` × (`bg-canvas`, `bg-surface`), `on-accent` × (`accent`, `accent-hover`), `danger` × (`bg-canvas`, `bg-surface`, `danger-tint`), `warn` × (`bg-canvas`, `warn-tint`).
  - Text at **4.5:1 — tag chips**: `tag-{craft,media,life}-ink` and `neutral`'s ink (`fg-2`) × (`bg-canvas`, `bg-surface`, `bg-hover`, `bg-selected`, plus `bg-elevated` in dark, and each one's own `tint`) = **light 4×5 + dark 4×6 = a 44-combination set** (ink×background 36 + ink/own tint 8). The selected chip (fill 2) is calculated as `bg-surface` text × a facet `ink` background. The current values are the two tables of §2.1.3, and the lowest is 5.94:1-level.
  - Non-text at **3:1**: `line-control` × every `bg-*` (the targets extend to **filter bar chip borders**, and display chips are excluded), `rail-progress` (the composited value at the pulse floor `opacity .7`) × every `bg-*`, the `danger` and `accent` rails × backgrounds, the focus ring `accent` × backgrounds.
  - **Excluded**: `fg-3` (the explicit exception of §2.1.3 — it passes only through the duplicated-information whitelist), `fg-inverse` (it is never used on an ordinary background of the same mode — over `accent` it is calculated as `on-accent`), `line-1`/`line-2` (decorative hairlines, §7.1), and the chip `tint` backgrounds themselves (around 1.1 — they carry no information, §2.1.3).
- **The color-blindness gate**: run the all-pairs of the 4 facet inks (`craft`/`media`/`life`/`neutral`) through Viénot-Brettel-Mollon 1999-style protan/deutan simulation, compute OKLab ΔE×100-values, and check **normal ΔE ≥ 10 / CVD ΔE ≥ 6** for light and dark separately. The script lives in `frontend/` and is exposed as `just color-check`.

  | Mode | Pass bar (normal / CVD) | Current | Verdict |
  |---|---|---|---|
  | Light | ≥ 10 / ≥ 6 | 10.19 / 7.49 | Pass |
  | Dark | ≥ 10 / ≥ 6 | 11.06 / 8.37 | Pass |

  Light normal at 10.19-level sits just above the floor — touch the L of a facet token and this gate breaks first.
- **The token lint**: outside generated files, ban raw hex, `rgb(` literals, anonymous spacing values outside the 12-step scale, Tailwind default palette utilities (`bg-neutral-*`, `text-slate-*`), and direct `--ink-*` references in component code. The three items below are **generated from the `@theme` declarations rather than hardcoded**.
  - z-index: nothing outside the 7 `--z-*` tokens.
  - duration: no CSS literal outside the 7 `--dur-*` tokens (the only exceptions are the two JS timer values of the §4.10-table, 4000/8000ms).
  - Layout constants: only `--size-*`/`--w-*` token references are allowed; anonymous arbitrary values (`h-[76px]`, `w-[380px]`, `max-w-[480px]`) are banned. **A width used on one screen is no exception either** — register it in the §2.3-list as a `--w-*` and reference it by name.
- **Contract consistency**: check that the fields the document and the code reference actually exist in `api/openapi.yaml` (`updated_at`, request IDs, a tag exclusion filter and a filter-scoped tag count are **not there**). The tag facet **is in the contract** (the `TagFacet` enum + `Tag.facet` / `TagInput.facet`) — but **`LinkTag` has no facet**, so the chips of a list row are resolved by the client building a `Map<tagId, facet>` from a `GET /api/v1/tags` cache, and **a cache miss renders as `neutral` rather than guessing** (that is a correct fallback, not a bug). Anything the contract does not offer is recorded in the 11 §10-list with its reason.
- **Applying the sweep rule**: `dark:` utilities are currently scattered across 51-odd places in 9 files. Pull the `grep -l` list into a file first, work it off as a checklist, and re-run the same search at the end to confirm zero (the CLAUDE.md sweep rule).
- **Actually checking both modes**: open light and dark for real, and check dark + Increase Contrast + Reduce Transparency too. On the OS-dark + app-light-selected combination, check that the scrollbar and form controls follow into light (§2.1.6), and that there is no white flash on first paint.
- **Commands**: declare completion only after `just fmt`, `just lint`, `just test`, `just web-gen-check` and `just web-build` all pass (no success claims without output).
