---
paths:
  - "docs/**"
---

# Documentation rules

- **`docs/v2/` is bilingual: `ko/` and `en/`, one file each.** The Korean side is the
  source of truth — write there first, then bring the English twin along in the same
  change. `just docs-parity` fails when the two drift in heading structure, table shape,
  code blocks or numbers; it deliberately cannot check whether the prose means the same
  thing, so that part is on the person editing.
- `docs/v2/` is the single source of truth. `docs/v1/` is an archive — **never modify it for any reason** (broken links included; preserve it as it was).
- Language policy: see CLAUDE.md (`docs/` Korean, README and agent-facing files English). Keep the tone plain, no emoji in section headings, and a single line at the top: `> Push-Point v2 — 마지막 업데이트: YYYY-MM-DD`.
- Source/derivative relationships: schema DDL lives in 05, the API in `api/openapi.yaml` (machine source of truth — 06 is human-facing commentary; if they disagree, openapi.yaml wins), and milestones and DoD in 08. When another document (00/03/04, etc.) carries the same content, match the source character for character. The 5-metric performance table must be identical everywhere.
- Source/derivative pairs (05 ↔ quoted DDL, `api/openapi.yaml` ↔ 06, 08 ↔ DoD and performance tables in other documents) are edited by a single worker (agent) who updates source and derivative in the same piece of work — never split a pair across parallel assignees. After editing, verify the derivative side with grep.
- Mention the v1 stack (PostgreSQL/Redis/RabbitMQ/MinIO/OpenAI/JWT/k8s/HPA/Gin/Ent/React Native) only in "v1 vs v2" context. It must not appear in descriptions of the current architecture.
- Cross-links inside docs/v2 use the filename only (`05-DATA-SCHEMA.md`); references from the repo root use the `docs/v2/...` path.
- When adding or renaming a document, update the comparison index in `docs/README.md` and the table of contents in `docs/v2/ko/00-README.md` together.
- When editing the plan (08), check that it does not conflict with the applied recommendations in `09-PLAN-REVIEW.md` (the settled v2.1 decisions). If you run a new review, record the outcome and the date it was applied in 09.
