---
paths:
  - "frontend/**"
---

# Frontend 워크스페이스 규칙 (웹 앱)

웹은 iOS와 대등한 full-feature 정식 클라이언트다 (2026-07-21 편입). 같은 `api/openapi.yaml`을 소비하므로 저장/목록/검색/태그 필터/상세/태그 편집/삭제/재시도/통계 전부 웹에서 완결로 구현한다. **유일한 차이**: 저장의 "iOS 공유 시트 2초 진입"은 OS 기능이라 웹엔 없다 — 웹은 URL 입력창(+선택적 북마클릿)으로 저장한다.

## 스택 (고정 — §16)

- **Vite + React 19 + TypeScript, 순수 SPA** (SSR 없음 — 단일 사용자 로컬 앱).
- **Tailwind CSS v4**(CSS-first `@theme`) + **shadcn/ui** — Radix primitives **명시적 핀**(2026-07 shadcn 기본이 Base UI로 바뀌었으나 a11y 성숙도로 Radix 유지) + lucide-react.
- **TanStack Router**(typed search params — `?q ?tag ?status cursor`가 URL 상태) + **TanStack Query v5**(useInfiniteQuery).
- 계약 타입 생성: **openapi-typescript**(핀) + **openapi-fetch**(Bearer를 onRequest 미들웨어로 주입 — iOS ClientMiddleware와 대칭). **openapi-react-query 채택 금지**(maintenance mode + useInfiniteQuery 타입 결함이 목록·검색 화면과 겹침) — TanStack `useInfiniteQuery`를 openapi-fetch 위에 직접 감싼다(훅 2개, API 타입 수작성 0).
- 전역 상태 라이브러리 없음(서버 상태=Query, URL 상태=Router, 로컬 UI=useState). 폼 라이브러리 없음(useState+Zod). 다크모드는 prefers-color-scheme + localStorage 토글 + `dark:`.

## 계약 정렬 (contract-first — openapi.yaml의 3번째 소비자)

- `frontend/src/lib/api/schema.d.ts`는 `api/openapi.yaml`에서 **생성·커밋**하는 계약 아티팩트다 — **수작성 금지**, 직접 편집 금지. API 타입 불일치도 openapi.yaml을 고친 뒤 `just web-gen` 재생성으로만 해결한다.
- API를 바꾸면 `just web-gen`(`openapi-typescript`, 버전 핀 — `@latest` 금지)을 재실행하고 `schema.d.ts`를 함께 커밋한다. `just web-gen-check`(web-gen 후 `git diff --exit-code`)가 CI에서 드리프트를 차단한다 — 백엔드 `gen-check`와 대칭.

## 경로·인증·배포

- **오리진 상대 경로만 쓴다** — 절대 URL·호스트 하드코딩 금지(= `http://localhost:8080/...` 류 금지)이지, 문서 상대(`./`) 강제가 아니다. 경로는 `/`로 시작하는 오리진 기준: `/api/v1/...`·`/thumbs/...`·`/healthz`만 호출하고, Vite `base`도 `'/'`로 둔다(`'./'`는 `/links/123` 딥링크에서 자산이 `/links/assets/...`로 풀려 SPA 폴백 index.html이 반환되고 MIME 거부로 부팅이 깨진다). dev(Vite 프록시 :5173 → Go :8080)와 prod(embed same-origin)에서 같은 코드가 동작하는 근거다.
- **인증**: API 키는 설정 화면에서 입력받아 localStorage에 저장하고 `Authorization: Bearer`로 붙인다(iOS 패리티). 새 인증 면제를 추가하지 말고, 서버측 loopback 우회·인증 완화도 요구하지 않는다(api.md 규칙 — 면제는 healthz·thumbs 2개뿐).
- **`dist/`는 미커밋**(빌드 아티팩트 — CI가 `just web-build`로 생성). 프로덕션은 `dist/`를 `//go:embed all:dist` + `http.FileServerFS`로 서빙하되, embed 서빙 코드는 빌드 태그 `embed_frontend` 뒤에 둔다 — dist/ 없으면 컴파일 실패하므로 백엔드 전용 `just build`·CI는 태그 없이 그린 유지, 릴리스만 `web-build && go build -tags embed_frontend`.
- 표시 폴백은 iOS와 동일 규율: 서버가 `title`을 빈 문자열로 주면(og·title 부재) `domain`(그다음 `url`)을 대신 표시한다 — 빈 셀 방지는 클라이언트 책임.
