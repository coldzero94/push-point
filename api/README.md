> Push-Point v2.1 — 마지막 업데이트: 2026-07-20

# api/ — API 계약 (contract-first)

`api/openapi.yaml`(OpenAPI 3.1)이 **Push-Point API의 기계 원본**이다.
백엔드 서버 인터페이스와 클라이언트 코드는 전부 이 파일에서 생성한다.
`docs/v2/06-API-SPECIFICATION.md`는 사람용 해설·예시 문서로 유지하며,
둘이 다르면 **openapi.yaml이 우선**한다 — 발견 즉시 06을 스펙에 맞춰 고친다.

## 소비자 (코드 생성 대상 3곳)

| 워크스페이스 | 생성기 | 산출물 |
|---|---|---|
| backend (M1+) | **oapi-codegen** | chi 서버 인터페이스 + 요청/응답 타입 → `backend/internal/api/gen/` (**생성물 커밋**) |
| ios (M4) | **swift-openapi-generator** (Apple 공식) | URLSession 트랜스포트 기반 클라이언트 |
| frontend (미래) | **openapi-typescript** (예정) | 타입 정의 — frontend 착수 시 활성화 |

## 워크플로

1. API를 바꾸려면 `api/openapi.yaml`을 먼저 수정한다 (핸들러 코드부터 고치지 않는다).
2. `just gen` — 생성물 재생성. `backend/internal/api/gen/`이 갱신되며 커밋 대상이다.
   oapi-codegen은 **v2.8.0 핀**이다 (`@latest` 금지 — OpenAPI 3.1 생성 실측 통과 버전, 2026-07-20).
3. 서버/클라이언트 구현이 새 인터페이스와 어긋나면 **컴파일 에러**로 드리프트가 드러난다.
   에러를 따라 구현을 맞춘 뒤 스펙·생성물·구현을 함께 커밋한다.
4. `just gen-check` — `just gen` 후 `git diff`가 남으면 실패. **CI 게이트**로,
   스펙과 커밋된 생성물의 불일치를 머지 전에 차단한다.

## 스펙 작성 규칙 요약

- 필드 타입·상태 enum은 `docs/v2/05-DATA-SCHEMA.md` 스키마와 일치.
  시각 필드는 전부 **integer unix epoch 초** (date-time 문자열 금지).
- 인증: bearer (`PUSHPOINT_API_KEY`). 면제는 `GET /healthz`·`GET /thumbs/{path}` 2개뿐 (`security: []`).
- `POST /api/v1/links`는 url_hash 멱등 — 중복 저장 시 `200 {id, duplicate:true}` (Share Extension 재시도의 근거).
- 검색 `q`가 3자 미만이면 LIKE 폴백 (`"mode":"like"`), 3자 이상은 FTS5 (`"mode":"fts"`).
- operationId는 camelCase (listLinks, createLink, getLink, ...).

## 스펙 변경 규칙

- 하위호환을 깨는 변경(필드 삭제·타입 변경·경로 변경)은 06과 관련 docs 갱신을 **같은 커밋**에 포함한다.
- 단일 사용자 앱이라 파괴적 변경 자체는 허용된다. 단, 배포된 iOS 앱 버전과의 정합은 변경자가 책임진다.
