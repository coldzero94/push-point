# Push-Point

개인용 링크 저장·자동 태깅 앱 — 단일 사용자, LLM 없는 경량 NLU가 기술적 차별점.
현재 상태: 문서·계획 단계 (M1 전, Go 코드 미작성. `backend/go.mod`는 의존성 0으로 초기화됨).

## 워크스페이스 (모노레포)

- `api/` — API 기계 원본 `openapi.yaml` (OpenAPI 3.1) — 백엔드·클라이언트 코드 생성원 (`just gen`).
- `backend/` — Go 단일 바이너리 (API + worker + NLU 런타임 추론). 모든 실행 코드는 여기.
- `nlu/` — NLU 오프라인 **자산 전용**: dictionary/(태그 사전), golden/(평가셋), models/(ONNX 변환). 런타임 추론은 `backend/internal/tagger`(Go)이며, Python은 `nlu/models/`에서만 허용.
- `ios/` — M4: SwiftUI 앱 + Share Extension. 아직 코드 없음.
- `frontend/` — 웹 앱 (Vite+React+TS, `api/openapi.yaml` 계약 소비, iOS와 대등). 저장의 "공유 시트 2초 진입"만 iOS 고유, 나머지 기능은 동일.
- `docs/v2/` = 단일 진실 원천, `docs/v1/` = v1 아카이브 (수정 금지), 비교는 `docs/README.md`.
- `deploy/k8s-future/` — v1 k8s 매니페스트 보존 (미사용, 수정 금지).

## 명령 (루트 justfile, Go 레시피는 backend/ 대상)

- `just dev` — 로컬 실행 (PUSHPOINT_API_KEY=dev-key)
- `just test` — 전체 테스트 (`cd backend && go test ./...`)
- `just bench` — 마이크로벤치 (p99 판정 수단 아님 — bench-http가 담당)
- `just bench-http` — 저장 API HTTP 경로 p99 게이트, p99 < 50ms 초과 시 exit 1 (M1+)
- `just eval` — nlu/golden/ 태깅 top-3 정확도 측정 (M3+)
- `just gen` — api/openapi.yaml → backend/internal/api/gen/ 생성 (oapi-codegen v2.8.0 핀, 생성물 커밋)
- `just web-dev` — Vite dev 서버 :8421 (프록시로 /api·/thumbs·/healthz → :8420, 상대 경로 코드가 prod embed와 동일)
- `just web-build` — frontend/dist/ 빌드 (dist/는 미커밋)
- `just release` — 웹 빌드 + SPA를 embed한 단일 바이너리 (`backend/bin/pushpoint`, `-tags embed_frontend`)
- `just web-gen` — api/openapi.yaml → frontend/src/lib/api/schema.d.ts 생성 (openapi-typescript 핀, 생성물 커밋)
- 그 외 레시피(build/gen-check/web-gen-check/test-crash/seed/lint/fmt)는 `just` 실행으로 목록 확인

## 핵심 규칙

- `docs/`는 한국어로 쓴다 (코드·식별자·기술 용어는 영어). 루트 `README.md`는 영어 — 퍼블릭 GitHub 첫 화면이다.
- 커밋 메시지는 Conventional Commits(`feat:`/`fix:`/`docs:`/`chore:` 등), 제목은 영어 한 줄.
- 태스크 러너는 just (2026-07-20 평가 후 채택 — 재평가 트리거: frontend 착수·협업자 합류). API 계약 스택은 수작성 OpenAPI 3.1 + oapi-codegen v2.8.0 핀 + swift-openapi-generator (2026-07-20 심사 확정, 배경은 docs/v2/09-PLAN-REVIEW.md와 .claude/rules/api.md).
- 설계 원본: 스키마 = `docs/v2/05-DATA-SCHEMA.md`, API = `api/openapi.yaml` (`docs/v2/06-API-SPECIFICATION.md`는 해설), 계획 = `docs/v2/08-DEVELOPMENT-PLAN.md`. 설계를 바꿀 땐 원본을 먼저 고치고 나머지를 따라가게 한다 (API는 `just gen`으로 생성물 재생성).
- 측정 없는 "잘 되는 것 같다" 금지 — 성능·품질 주장은 `just bench-http`(p99 게이트) / `just bench` / `just eval` 수치로 뒷받침한다.
- **완료 정의**: 구현 작업은 `just fmt`·`just lint`·`just test`·`just gen-check`(프론트 변경이면 `just web-gen-check`·`just web-build`도)를 전부 통과시킨 뒤에만 완료를 선언하고, 실행한 명령과 출력을 증거로 제시한다 (출력 없는 성공 주장 금지).
- **스윕 규칙**: 여러 파일에 걸친 일괄 수정은 기억으로 담당을 배정하지 말고, `grep -l`/glob으로 대상 목록을 먼저 생성해 파일로 저장한 뒤 체크리스트로 소거한다. 완료 시 같은 검색을 재실행해 잔여 0을 확인한다.
- v1 스택(PostgreSQL/Redis/MinIO/OpenAI/k8s/Gin/Ent)은 "v1→v2 대비" 맥락에서만 언급한다. 현재 아키텍처 설명에 등장 금지.
- 계획 점검(2026-07-20) 권고 8건은 반영 완료 — 배경·근거는 `docs/v2/09-PLAN-REVIEW.md` 참조.
- 웹 프론트엔드는 정식 편입(2026-07-21) — 비목표 폐기, iOS와 대등한 full-feature 클라이언트로 승격. 배경·근거는 `docs/v2/09-PLAN-REVIEW.md`, 세부 규칙은 `.claude/rules/frontend.md`.
- **코드리뷰 게이트**: 구현 작업(마일스톤·기능 단위)이 끝나면 커밋 전에 `/pr-review-toolkit:review-pr`로 코드리뷰를 돌린다. high/medium 발견 사항을 수습한 뒤에 커밋·푸시하고, 의도적으로 넘기는 항목은 사유를 남긴다.
- **머지 규칙**: main 직접 push 금지 (GitHub 룰셋 `main-protection`이 강제 — PR 필수, `ci` 체크 통과 필수, force-push·삭제 차단). 흐름: 브랜치 → 커밋·푸시 → PR → CI 녹색 + 코드리뷰 게이트 → 머지. CI가 main에서 깨지면 다른 작업보다 먼저 수습한다.
- 영역별 세부 규칙은 `.claude/rules/`에 경로 스코프로 분리돼 있다 (backend·nlu·ios·frontend·docs·api).
