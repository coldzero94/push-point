# Push-Point Documentation Index (v1 ↔ v2)

> Push-Point v2 — last updated: 2026-07-22

## Notes

- **[docs/v2/](v2/) is the current documentation and the single source of truth, and it exists as two copies, `ko/` and `en/`.** Korean is the original and English is its twin — `just docs-parity` fails the build when structure, tables, code or figures drift apart (it cannot see what the prose means). Implementation, review and decisions all take the v2 documents as their reference.
- **[docs/v1/](v1/) is the archive of the v1 planning documents as of 2025-10.** It is preserved for reference only and **is not modified.** The links inside the v1 documents are exactly as they were then, so some of them are broken (for example, v1's 00-README.md links to old Korean filenames like `01-프로젝트-개요.md`, while the actual file is `01-PROJECT-OVERVIEW.md`).

## Document comparison (v1 ↔ v2)

| Document | v1 | v2 (ko) | v2 (en) | The change in one line |
|---|---|---|---|---|
| 00 README | [v1](v1/00-README.md) | [ko](v2/ko/00-README.md) | [en](v2/en/00-README.md) | An index to the plan for an 8-10 week link-archive service → an introduction to a single-binary personal app plus a `just dev` quick start |
| 01 Project overview | [v1](v1/01-PROJECT-OVERVIEW.md) | [ko](v2/ko/01-PROJECT-OVERVIEW.md) | [en](v2/en/01-PROJECT-OVERVIEW.md) | Aimed at a multi-user service → a single user, "the app I use every day", product before infrastructure |
| 02 Tech stack | [v1](v1/02-TECH-SPEC.md) | [ko](v2/ko/02-TECH-SPEC.md) | [en](v2/en/02-TECH-SPEC.md) | Gin/Ent + PostgreSQL·Redis·RabbitMQ·MinIO·OpenAI·React Native → the standard library + chi, SQLite, an LLM-free lightweight NLU, SwiftUI |
| 03 System architecture | [v1](v1/03-SYSTEM-ARCHITECTURE.md) | [ko](v2/ko/03-SYSTEM-ARCHITECTURE.md) | [en](v2/en/03-SYSTEM-ARCHITECTURE.md) | A multi-component k8s diagram → a single binary in which API and worker are one process, with the SQLite WAL settings as the basis of the performance |
| 04 Data flow | [v1](v1/04-DATA-FLOW.md) | [ko](v2/ko/04-DATA-FLOW.md) | [en](v2/en/04-DATA-FLOW.md) | A Redis Streams queue + client sync → an in-process worker pool on the SQLite jobs table, retries and `kill -9` crash recovery |
| 05 Data schema | [v1](v1/05-DATA-SCHEMA.md) | [ko](v2/ko/05-DATA-SCHEMA.md) | [en](v2/en/05-DATA-SCHEMA.md) | 9 PostgreSQL tables + Redis + MinIO → 8 SQLite tables + FTS5 (trigram) full-text search, backup = copying a file |
| 06 API specification | [v1](v1/06-API-SPECIFICATION.md) | [ko](v2/ko/06-API-SPECIFICATION.md) | [en](v2/en/06-API-SPECIFICATION.md) | JWT, sign-up, sync, rate limiting → one static API key, keyset cursor pagination, FTS5 search |
| 07 Deployment | [v1](v1/07-K8S-SETTINGS.md) | [ko](v2/ko/07-DEPLOYMENT.md) | [en](v2/en/07-DEPLOYMENT.md) | Minikube k8s deployment YAML → running and operating it locally (Go 1.25 + just is the whole of it), the k8s manifests preserved in `deploy/k8s-future/` |
| 08 Development plan | [v1](v1/08-DEVLOPMENT-PLAN.md) | [ko](v2/ko/08-DEVELOPMENT-PLAN.md) | [en](v2/en/08-DEVELOPMENT-PLAN.md) | Four phases over 8-10 weeks (OpenAI integration and k8s deployment included) → M1~M6 over six months, with a measurable DoD — golden-set accuracy, benchmarks |
| 09 Plan review | — (v2 only) | [ko](v2/ko/09-PLAN-REVIEW.md) | [en](v2/en/09-PLAN-REVIEW.md) | The outcome of the adversarial review of 2026-07-20: a fact-check summary plus 8 correction recommendations (all applied in v2.1) |
| 10 Design system | — (v2 only) | [ko](v2/ko/10-DESIGN-SYSTEM.md) | [en](v2/en/10-DESIGN-SYSTEM.md) | v1 had no design spec → the single source for the tokens/components/motion/accessibility that web and iOS share |
| 11 Web UX spec | — (v2 only) | [ko](v2/ko/11-WEB-UX-SPEC.md) | [en](v2/en/11-WEB-UX-SPEC.md) | v1 had no client UX spec → layout, contract field mapping, keyboard shortcuts and implementation order for seven web screens |
| 12 Backlog | — (v2 only) | [ko](v2/ko/12-BACKLOG.md) | [en](v2/en/12-BACKLOG.md) | v1 had no backlog → 4 candidates to look at after 08 with their entry and retirement conditions, and the reasons the 20 that were cut were cut (so they are not re-argued) |
| 13 Client parity | — (v2 only) | [ko](v2/ko/13-CLIENT-PARITY.md) | [en](v2/en/13-CLIENT-PARITY.md) | v1 had one client (React Native), so there was nothing to decide → the three axes that decide whether a new feature goes to iOS or the web, and the current decision table |
| 14 Stats redesign | — (v2 only) | [ko](v2/ko/14-STATS-REDESIGN.md) | [en](v2/en/14-STATS-REDESIGN.md) | v1 had no stats screen → a plan that measures which claims hold at 1~3 saves a day, and removes the ones that do not |

v1's 07 and 08 keep their old filenames (`07-K8S-SETTINGS.md`, `08-DEVLOPMENT-PLAN.md` — typo included).

## v1 → v2 transition summary

| Area | v1 | v2 | Why |
|---|---|---|---|
| Deployment | Minikube + k8s + HPA | A single Go binary (one `just dev`) | Autoscaling for zero users is design in reverse. It removes the friction of testing locally |
| DB | PostgreSQL (k8s pod) | SQLite (WAL mode) + FTS5 | Fast enough at personal-app scale. Backup = copying a file |
| Message queue | Redis Streams | An in-process worker pool (goroutines + the SQLite jobs table) | With a single process, a network queue is unnecessary. Durability across restarts is guaranteed by the jobs table |
| Object storage | MinIO | Local disk (`data/thumbs/`) | The S3 API is overkill for a few GB of thumbnails |
| AI tagging | OpenAI API | A lightweight NLU (rule-based → ONNX embedding, two stages) | Zero cost, an answer in the hundreds of ms, privacy. The technical differentiator of this project |
| Client | React Native (undecided) | The iOS Share Extension first (SwiftUI) | If the friction of saving goes past 2 seconds, it cannot be an app you use every day |
| Auth | JWT + sign-up | A single user, one static API key | Multi-user is an explicit non-goal |

## Suggested reading order

- If this is your first time: [v2/00-README.md](v2/en/00-README.md) → [v2/08-DEVELOPMENT-PLAN.md](v2/en/08-DEVELOPMENT-PLAN.md) → [v2/03-SYSTEM-ARCHITECTURE.md](v2/en/03-SYSTEM-ARCHITECTURE.md)
- If you are implementing: [v2/02-TECH-SPEC.md](v2/en/02-TECH-SPEC.md) → [v2/05-DATA-SCHEMA.md](v2/en/05-DATA-SCHEMA.md) → [v2/06-API-SPECIFICATION.md](v2/en/06-API-SPECIFICATION.md) → [v2/04-DATA-FLOW.md](v2/en/04-DATA-FLOW.md)
- If you want to know what changed from v1 and why: the transition summary table above → [v2/01-PROJECT-OVERVIEW.md](v2/en/01-PROJECT-OVERVIEW.md) → [v2/07-DEPLOYMENT.md](v2/en/07-DEPLOYMENT.md)
- If you are building a client (web or iOS): [v2/10-DESIGN-SYSTEM.md](v2/en/10-DESIGN-SYSTEM.md) → [v2/11-WEB-UX-SPEC.md](v2/en/11-WEB-UX-SPEC.md) → [v2/06-API-SPECIFICATION.md](v2/en/06-API-SPECIFICATION.md)
