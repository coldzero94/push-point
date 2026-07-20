---
paths:
  - "api/**"
---

# API 계약 규칙 (contract-first)

- `api/openapi.yaml`이 **API의 기계 원본**이다. `docs/v2/06-API-SPECIFICATION.md`는 사람용 해설 — 스펙을 바꾸면 06도 **같은 커밋**에서 갱신한다. 둘이 다르면 openapi.yaml이 우선.
- 필드 타입·상태 enum(`content_type`, `status`, `source` 등)은 `docs/v2/05-DATA-SCHEMA.md` 스키마와 일치를 유지한다.
- 시각 필드는 전부 **integer unix epoch 초** (`created_at`, `published_at` 등). `format: date-time` 문자열 금지.
- 엔드포인트를 추가/변경하면 `just gen`을 재실행하고 생성물(`backend/internal/api/gen/`)을 함께 커밋한다. `just gen-check`가 CI에서 드리프트를 차단한다.
- 하위호환: 단일 사용자 앱이라 파괴적 변경(필드 삭제·타입 변경·경로 변경)은 허용된다. 단, 배포된 iOS 앱 버전과의 정합은 변경한 본인이 책임진다 — 앱을 먼저(또는 같은 작업에서) 맞출 것.
- 인증 면제는 `GET /healthz`·`GET /thumbs/{path}` **2개뿐** (`security: []`). 그 외 전 엔드포인트는 bearer 필수 — 새 면제를 추가하지 말 것.

## 코드 생성 스택 확정 (2026-07-20 심사)

- oapi-codegen은 **v2.8.0 핀** — `@latest` 금지. OpenAPI 3.1 생성을 실측으로 통과한 버전이며, 버전 변경은 생성물 diff 검토를 거쳐 의도적으로만 한다.
- generate 세트는 `types,chi-server,strict-server,spec` 고정 (justfile `gen` 레시피와 동일).
- swift-openapi-generator(M4): **SPM 빌드 플러그인이 아니라 CLI 실행 + 생성물 커밋** 방식을 쓴다 (재현성·드리프트 검사 일관성).
- Swift `allOf` 조합 시 value1/value2 래핑 이슈는 알려진 사항 — 스펙을 미리 비틀지 말고 **M4 실측 후 결정**한다.
- swift-openapi-generator는 securitySchemes 클라이언트 코드를 생성하지 않는다 — **Bearer 주입은 ClientMiddleware 수작업 구현**으로 한다.
- TypeSpec 재평가 트리거: 오퍼레이션 40개 이상이 되거나, 웹 프론트 착수로 Node 툴체인이 어차피 도입될 때. 그 전에는 openapi.yaml 수기 유지가 정본.
