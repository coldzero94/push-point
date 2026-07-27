---
paths:
  - "nlu/**"
---

# NLU workspace rules

- `nlu/` is for **offline assets only**. Runtime inference code (rule tagger, ONNX inference) lives in `backend/internal/tagger` (Go), not here.
- Across the whole repo, Python is allowed only under `nlu/models/` (model conversion and quantization scripts).
- `nlu/dictionary/` — a controlled tag dictionary (30–50 entries initially, name + aliases). The principle is "classification against this dictionary", not free-form tag "generation". Dictionary changes affect evaluation numbers, so re-run `just eval` and record the result whenever it changes.
- `nlu/golden/` — the golden set for evaluation (JSONL, committed). The golden set is the yardstick: fixing the tagger to match the golden set is allowed, **fixing the golden set to match the tagger is forbidden** (correcting mislabeled entries is the only exception).
- Three sets, reported separately and never merged: `dev` (tuning), `test` (frozen, the only gate), `wild` (the open web outside dev blogs — social, commerce, app stores, communities). Measured 2026-07-27 (after re-capture, the test expansion to 84, and the 0011 alias additions): dev 0.984 / test **0.893** / **wild 0.821**. A change that moves dev and test but not wild has not been shown to help what the user actually saves — see `nlu/golden/README.md` for the five defect classes only wild exposes (A, C, E, F fixed and B partly fixed 2026-07-27).
- **A recall number alone conflates tagger quality with capture quality.** Four of wild's 28 snapshots hold under 200 characters of signal because the scraper fetched a bot wall, not a page; no tagger change can ever score them. `just eval` therefore prints both — wild is 0.821 overall and **0.833 across the 24 usable entries**, and only the second bounds what fixing the tagger can buy. Attribute a miss to the tagger only after checking which side of that line it falls on.
- Quality gates are relative: entering M5 = Phase A is +15pp over the "domain heuristics only" baseline; exiting M5 = the ensemble is +10pp over Phase A (the absolute 60%/80% figures are reference points only). Verdicts use only the frozen test items (84 as of 2026-07-27, expanded from 61 — figures recorded against 61 are not directly comparable). Do not claim quality without measuring.
- Pipeline details are sourced from the NLU section of `docs/v2/02-TECH-SPEC.md`; the evaluation protocol (offline snapshot, dev/test split) is in `nlu/golden/README.md`. Background and rationale are in `docs/v2/09-PLAN-REVIEW.md` (already applied).
