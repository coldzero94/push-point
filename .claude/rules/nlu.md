---
paths:
  - "nlu/**"
---

# NLU workspace rules

- `nlu/` is for **offline assets only**. Runtime inference code (rule tagger, ONNX inference) lives in `backend/internal/tagger` (Go), not here.
- Across the whole repo, Python is allowed only under `nlu/models/` (model conversion and quantization scripts).
- `nlu/dictionary/` — a controlled tag dictionary (30–50 entries initially, name + aliases). The principle is "classification against this dictionary", not free-form tag "generation". Dictionary changes affect evaluation numbers, so re-run `just eval` and record the result whenever it changes.
- `nlu/golden/` — the golden set for evaluation (JSONL, committed). The golden set is the yardstick: fixing the tagger to match the golden set is allowed, **fixing the golden set to match the tagger is forbidden** (correcting mislabeled entries is the only exception).
- Quality gates are relative: entering M5 = Phase A is +15pp over the "domain heuristics only" baseline; exiting M5 = the ensemble is +10pp over Phase A (the absolute 60%/80% figures are reference points only). Verdicts use only the 50 frozen test items. Do not claim quality without measuring.
- Pipeline details are sourced from the NLU section of `docs/v2/02-TECH-SPEC.md`; the evaluation protocol (offline snapshot, dev/test split) is in `nlu/golden/README.md`. Background and rationale are in `docs/v2/09-PLAN-REVIEW.md` (already applied).
