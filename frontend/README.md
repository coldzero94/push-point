# frontend — 웹 앱 (iOS와 대등한 full-feature 클라이언트)

> Push-Point v2.1 — 마지막 업데이트: 2026-07-21

Push-Point 웹 프론트엔드다. **2026-07-21, 웹은 명시적 비목표에서 iOS와 기능적으로
대등한(feature parity) 정식 클라이언트로 승격됐다.** 두 클라이언트는 같은
[`../api/openapi.yaml`](../api/openapi.yaml) 계약을 소비하므로 서버 API에 대해 할 수 있는
일이 동일하다 — 저장·목록·검색·태그 필터·상세·태그 편집·삭제·재시도·통계 전부 양쪽에서
완결로 구현한다.

**유일한 차이는 저장의 진입 방식**이다. iOS의 "공유 시트 2초 진입"은 OS 기능(Share
Extension)이라 웹엔 없다 — 웹은 URL 입력창(+선택적으로 북마클릿/확장)으로 저장한다.
"역할 분담"이 아니라 "동일 기능, 저장 진입 방식만 다름"이다.

## 스택

- **Vite + React 19 + TypeScript, 순수 SPA** — SSR 없음(단일 사용자 로컬 앱). Vite는
  Node `^20.19 || >=22.12`를 요구한다(로컬 v22.14 충족).
- **Tailwind CSS v4** (CSS-first `@theme`) + **shadcn/ui** + **lucide-react**.
  shadcn primitives는 **Radix로 명시적 핀** — 2026-07 shadcn 기본이 Base UI로 바뀌었으나
  a11y 성숙도를 이유로 Radix를 선택한다.
- **TanStack Router** — typed search params. `?q` `?tag` `?status` `cursor`가 URL 상태다.
- **TanStack Query v5** — 서버 상태 캐시. 목록·검색은 `useInfiniteQuery`.
- **계약 타입 생성**: `openapi-typescript`(버전 핀) + `openapi-fetch`.
  Bearer 토큰은 `openapi-fetch`의 `onRequest` 미들웨어로 주입한다(iOS `ClientMiddleware`와 대칭).
  - **`openapi-react-query`는 채택하지 않는다** — 2025-12 maintenance mode + `useInfiniteQuery`
    타입 결함(#2458/#2355)이 하필 핵심 화면 2개(목록·검색)와 겹친다. 대신 TanStack
    `useInfiniteQuery`를 `openapi-fetch` 위에 직접 감싼다(훅 2개, 타입 수작성 0, contract-first
    유지). `openapi-typescript` 자체는 maintenance 아님.
- 전역 상태 라이브러리 없음 — 서버 상태=Query, URL 상태=Router, 로컬 UI=`useState`.
- 폼 라이브러리 없음 — `useState` + Zod. 필요 화면이 생기면 그때 추가.
- 다크모드: `prefers-color-scheme` + localStorage 토글 + Tailwind `dark:`.

## 계약 정렬 (openapi.yaml의 3번째 소비자)

[`../api/openapi.yaml`](../api/openapi.yaml)이 backend(oapi-codegen v2.8.0)·iOS(swift-openapi-generator)·web(openapi-typescript)
세 클라이언트의 **단일 타입 원본**이다. 웹도 backend `gen`/`gen-check`와 동일한 규율을 따른다.

- **`just web-gen`** — `openapi-typescript`로 계약을 TypeScript 타입으로 생성한다
  (`api/openapi.yaml` → `src/lib/api/schema.d.ts`). 버전 핀, `@latest` 금지.
- **`just web-gen-check`** — `web-gen` 후 `git diff --exit-code`. 스펙과 커밋된 타입이
  어긋나면 실패하는 드리프트 게이트다.
- 생성물 **`src/lib/api/schema.d.ts`는 커밋한다**(계약 아티팩트 — 백엔드 `gen` 산출물과 패리티).
  빌드 산출물 **`dist/`는 커밋하지 않는다**(CI가 빌드).
- API를 바꿀 땐 항상 `api/openapi.yaml`을 먼저 고치고 `web-gen`으로 재생성한다. 핸들러/화면
  코드를 먼저 고치지 않는다.
- `CLAUDE.md`의 완료 정의에 `web-gen-check`가 포함된다. `.github/workflows/ci.yml`의 `web` job이
  `setup-node`(22) → `web-gen-check` → `web-build` 순으로 이를 강제한다.

## 명령

Go/backend와 동일하게 루트 `justfile`에서 실행한다. 웹 레시피는 `web-` 접두어를 쓴다.

| 명령 | 하는 일 |
|---|---|
| `just web-dev` | Vite dev 서버(:8421) — `/api`·`/thumbs`·`/healthz`는 프록시로 Go(:8420)에 전달 |
| `just web-gen` | `api/openapi.yaml` → `src/lib/api/schema.d.ts` 타입 생성 (핀 버전) |
| `just web-gen-check` | `web-gen` 후 `git diff --exit-code` (드리프트 게이트, CI) |
| `just web-build` | 프로덕션 번들 → `frontend/dist/` (embed 대상) |

## 배포 — 단일 Go 바이너리 유지

웹을 별도 서버로 띄우지 않는다. Vite 번들을 Go 바이너리에 embed 해 같은 프로세스가 서빙한다.

- **프로덕션**: `frontend/dist/`를 `//go:embed all:dist` + `http.FileServerFS`(Go 1.22+ stdlib)로
  서빙한다. chi catch-all `r.Handle("/*", ...)`를 기존 `/api/v1`·`/thumbs`·`/healthz` **뒤에**
  마운트하고, 미매칭 경로는 `index.html`로 SPA 폴백한다. 캐시: `/assets/*`는 immutable,
  `index.html`은 no-cache.
- **빌드 태그 `embed_frontend`**: `//go:embed dist`는 `dist/`가 없으면 **컴파일 실패**하므로
  embed 서빙 코드를 `//go:build embed_frontend` 뒤에 둔다. 백엔드 전용 `just build`·CI는
  태그 없이 그린을 유지하고, 릴리스만 `web-build && go build -tags embed_frontend`로 묶는다.
- **개발**: Vite dev 서버(:8421) + `server.proxy`로 `/api`·`/thumbs`·`/healthz`를 Go(:8420)에
  넘긴다. 클라이언트는 **상대 경로만** 쓰므로 dev(프록시)·prod(embed same-origin) 코드가 동일하다.
- **인증**: API 키를 설정 화면에서 입력받아 localStorage에 저장하고 `Authorization: Bearer`로
  붙인다(iOS 패리티 — 새 인증 면제를 추가하지 않는다). 서버측 loopback 우회 인증 완화는 **금지**
  (`api/` 규칙).

## 디렉터리·스캐폴드 범위

```
frontend/
├── index.html
├── package.json
├── vite.config.ts          # server.proxy, embed용 dist 출력
├── tsconfig.json
├── tailwind.css            # Tailwind v4 @theme
└── src/
    ├── lib/api/            # schema.d.ts(생성·커밋) + openapi-fetch 클라이언트 + 미들웨어
    ├── routes/             # TanStack Router — 화면별 라우트, typed search params
    ├── components/         # shadcn/ui 기반 UI
    └── hooks/              # useInfiniteQuery 래퍼(목록·검색) 등
```

화면 6개(저장·목록·검색·태그 필터·상세·태그 편집) + retry/delete 뮤테이션 + 선택적 통계 대시보드.
**첫 스캐폴드는 셋업 + 계약 파이프라인 + 저장·목록 화면까지 동작하면 충분**하다(나머지는 후속).

## 관련 문서

- API 계약(기계 원본): [`../api/openapi.yaml`](../api/openapi.yaml), 해설: [`../api/README.md`](../api/README.md)
- API 사람용 명세: [`../docs/v2/06-API-SPECIFICATION.md`](../docs/v2/06-API-SPECIFICATION.md)
- 프로젝트 개요·클라이언트 방향: [`../docs/v2/01-PROJECT-OVERVIEW.md`](../docs/v2/01-PROJECT-OVERVIEW.md)
- 마일스톤상 위치: [`../docs/v2/08-DEVELOPMENT-PLAN.md`](../docs/v2/08-DEVELOPMENT-PLAN.md)
