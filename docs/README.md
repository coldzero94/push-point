# Push-Point 문서 인덱스 (v1 ↔ v2)

> Push-Point v2 — 마지막 업데이트: 2026-07-22

## 안내

- **[docs/v2/](v2/)가 현행 문서이자 단일 진실 원천이고, `ko/`·`en/` 두 벌로 있다.** 한국어가 원본이고 영어가 쌍둥이다 — `just docs-parity`가 구조·표·코드·수치가 갈라지면 실패시킨다(본문의 뜻은 못 본다). 구현·리뷰·의사결정은 전부 v2 문서를 기준으로 한다.
- **[docs/v1/](v1/)은 2025-10 시점 v1 기획서 아카이브다.** 참고용으로만 보존하며 **수정하지 않는다.** v1 문서 내부 링크는 당시 상태 그대로라 일부 깨져 있다 (예: v1의 00-README.md는 `01-프로젝트-개요.md` 같은 옛 한글 파일명으로 링크하지만 실제 파일은 `01-PROJECT-OVERVIEW.md`다).

## 문서 비교 (v1 ↔ v2)

| 문서 | v1 | v2 (ko) | v2 (en) | 핵심 변경 한 줄 |
|---|---|---|---|---|
| 00 README | [v1](v1/00-README.md) | [ko](v2/ko/00-README.md) | [en](v2/en/00-README.md) | 8-10주 링크 아카이브 서비스 기획 인덱스 → 단일 바이너리 개인 앱 소개 + `just dev` 빠른 시작 |
| 01 프로젝트 개요 | [v1](v1/01-PROJECT-OVERVIEW.md) | [ko](v2/ko/01-PROJECT-OVERVIEW.md) | [en](v2/en/01-PROJECT-OVERVIEW.md) | 멀티유저 서비스 지향 → 단일 사용자 "내가 매일 쓰는 앱", 인프라보다 제품 우선 |
| 02 기술 스택 | [v1](v1/02-TECH-SPEC.md) | [ko](v2/ko/02-TECH-SPEC.md) | [en](v2/en/02-TECH-SPEC.md) | Gin/Ent + PostgreSQL·Redis·RabbitMQ·MinIO·OpenAI·React Native → 표준 라이브러리 + chi, SQLite, LLM 없는 경량 NLU, SwiftUI |
| 03 시스템 아키텍처 | [v1](v1/03-SYSTEM-ARCHITECTURE.md) | [ko](v2/ko/03-SYSTEM-ARCHITECTURE.md) | [en](v2/en/03-SYSTEM-ARCHITECTURE.md) | k8s 멀티 컴포넌트 구성도 → API + 워커가 한 프로세스인 단일 바이너리, SQLite WAL 설정이 성능의 근거 |
| 04 데이터 플로우 | [v1](v1/04-DATA-FLOW.md) | [ko](v2/ko/04-DATA-FLOW.md) | [en](v2/en/04-DATA-FLOW.md) | Redis Streams 큐 + 클라이언트 동기화 → SQLite jobs 테이블 기반 인프로세스 워커 풀, 재시도·`kill -9` 크래시 복구 |
| 05 데이터 스키마 | [v1](v1/05-DATA-SCHEMA.md) | [ko](v2/ko/05-DATA-SCHEMA.md) | [en](v2/en/05-DATA-SCHEMA.md) | PostgreSQL 9테이블 + Redis + MinIO → SQLite 8테이블 + FTS5(trigram) 전문 검색, 백업 = 파일 복사 |
| 06 API 명세 | [v1](v1/06-API-SPECIFICATION.md) | [ko](v2/ko/06-API-SPECIFICATION.md) | [en](v2/en/06-API-SPECIFICATION.md) | JWT·회원가입·sync·Rate Limiting → 정적 API 키 1개, keyset 커서 페이지네이션, FTS5 검색 |
| 07 배포 | [v1](v1/07-K8S-SETTINGS.md) | [ko](v2/ko/07-DEPLOYMENT.md) | [en](v2/en/07-DEPLOYMENT.md) | Minikube k8s 배포 YAML → 로컬 실행·운영 (Go 1.25 + just가 전부), k8s 매니페스트는 `deploy/k8s-future/`에 보존 |
| 08 개발 계획 | [v1](v1/08-DEVLOPMENT-PLAN.md) | [ko](v2/ko/08-DEVELOPMENT-PLAN.md) | [en](v2/en/08-DEVELOPMENT-PLAN.md) | 8-10주 4 Phase (OpenAI 연동·k8s 배포 포함) → 6개월 M1~M6, golden set 정확도·벤치 등 측정 가능한 DoD |
| 09 계획 점검 | — (v2 전용) | [ko](v2/ko/09-PLAN-REVIEW.md) | [en](v2/en/09-PLAN-REVIEW.md) | 2026-07-20 적대적 점검 결과: 팩트체크 요약 + 수정 권고 8건(v2.1에 반영 완료) |
| 10 디자인 시스템 | — (v2 전용) | [ko](v2/ko/10-DESIGN-SYSTEM.md) | [en](v2/en/10-DESIGN-SYSTEM.md) | v1에는 디자인 명세가 없었음 → 웹·iOS가 공유하는 토큰/컴포넌트/모션/접근성 단일 원본 |
| 11 웹 UX 명세 | — (v2 전용) | [ko](v2/ko/11-WEB-UX-SPEC.md) | [en](v2/en/11-WEB-UX-SPEC.md) | v1에는 클라이언트 UX 명세가 없었음 → 웹 화면 7개의 레이아웃·계약 필드 매핑·단축키·구현 우선순위 |
| 12 백로그 | — (v2 전용) | [ko](v2/ko/12-BACKLOG.md) | [en](v2/en/12-BACKLOG.md) | v1에는 백로그가 없었음 → 08 다음에 볼 후보 4건과 착수·폐기 조건, 그리고 자른 20건의 이유(재논의 방지) |
| 13 클라이언트 대응 | — (v2 전용) | [ko](v2/ko/13-CLIENT-PARITY.md) | [en](v2/en/13-CLIENT-PARITY.md) | v1은 클라이언트가 하나(React Native)라 판정할 것이 없었음 → 새 기능을 iOS·웹 어느 쪽에 넣을지 정하는 세 축과 현재 판정표 |
| 14 통계 재설계 | — (v2 전용) | [ko](v2/ko/14-STATS-REDESIGN.md) | [en](v2/en/14-STATS-REDESIGN.md) | v1에는 통계 화면이 없었음 → 하루 1~3건에서 어떤 주장이 성립하는지 측정하고, 성립하지 않는 것을 빼는 기획 |

v1의 07·08은 옛 파일명(`07-K8S-SETTINGS.md`, `08-DEVLOPMENT-PLAN.md` — 오타 포함) 그대로다.

## v1 → v2 전환 요약

| 영역 | v1 | v2 | 이유 |
|---|---|---|---|
| 배포 | Minikube + k8s + HPA | 단일 Go 바이너리 (`just dev` 한 번) | 유저 0명에 오토스케일링은 역설계. 로컬 테스트 마찰 제거 |
| DB | PostgreSQL (k8s pod) | SQLite (WAL 모드) + FTS5 | 개인 앱 규모에서 충분히 고성능. 백업 = 파일 복사 |
| 메시지 큐 | Redis Streams | 인프로세스 워커 풀 (goroutine + SQLite jobs 테이블) | 프로세스 하나면 네트워크 큐 불필요. 재시작 내구성은 jobs 테이블이 보장 |
| 오브젝트 스토리지 | MinIO | 로컬 디스크 (`data/thumbs/`) | 썸네일 몇 GB에 S3 API는 과함 |
| AI 태깅 | OpenAI API | 경량 NLU (규칙 기반 → ONNX 임베딩 2단계) | 비용 0, 수백 ms 응답, 프라이버시. 이 프로젝트의 기술적 차별점 |
| 클라이언트 | React Native (미정) | iOS Share Extension 최우선 (SwiftUI) | 저장 마찰이 2초를 넘으면 매일 쓰는 앱이 못 됨 |
| 인증 | JWT + 회원가입 | 단일 사용자, 정적 API 키 1개 | 멀티유저는 명시적 비목표 |

## 읽는 순서 추천

- 처음이면: [v2/00-README.md](v2/ko/00-README.md) → [v2/08-DEVELOPMENT-PLAN.md](v2/ko/08-DEVELOPMENT-PLAN.md) → [v2/03-SYSTEM-ARCHITECTURE.md](v2/ko/03-SYSTEM-ARCHITECTURE.md)
- 구현하려면: [v2/02-TECH-SPEC.md](v2/ko/02-TECH-SPEC.md) → [v2/05-DATA-SCHEMA.md](v2/ko/05-DATA-SCHEMA.md) → [v2/06-API-SPECIFICATION.md](v2/ko/06-API-SPECIFICATION.md) → [v2/04-DATA-FLOW.md](v2/ko/04-DATA-FLOW.md)
- v1에서 무엇이 왜 바뀌었는지 궁금하면: 위의 전환 요약 표 → [v2/01-PROJECT-OVERVIEW.md](v2/ko/01-PROJECT-OVERVIEW.md) → [v2/07-DEPLOYMENT.md](v2/ko/07-DEPLOYMENT.md)
- 클라이언트(웹·iOS)를 만들려면: [v2/10-DESIGN-SYSTEM.md](v2/ko/10-DESIGN-SYSTEM.md) → [v2/11-WEB-UX-SPEC.md](v2/ko/11-WEB-UX-SPEC.md) → [v2/06-API-SPECIFICATION.md](v2/ko/06-API-SPECIFICATION.md)
