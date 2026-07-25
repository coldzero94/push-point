# ios/design — M4 화면 기준

> 마지막 업데이트: 2026-07-25

M4에서 SwiftUI로 옮길 화면의 시각 기준이다. Xcode 프로젝트를 만들기 전에
"무엇을 만들 것인가"를 눈으로 합의해 두려고 먼저 그렸다.

## 파일

- [`prototype.html`](prototype.html) — 화면 4개(공유 시트 · 목록 · 상세 · 태그 편집)
  + SwiftUI 변환 대응표. 외부 의존성이 없으니 브라우저로 그냥 열면 된다.
  라이트/다크는 OS 설정을 따라간다.

## 읽는 법

- 색·타입·간격 값의 **출처는 [`../../docs/v2/10-DESIGN-SYSTEM.md`](../../docs/v2/10-DESIGN-SYSTEM.md)**
  이고, 이 파일은 사본이다. 값이 어긋나면 10번 문서가 옳다.
- 웹(`frontend/`)과 공유하는 것: 카드, 생성 커버(R4), 시간 척추, 칩 채움 3단,
  상태 획(S1). iOS에만 있는 것은 **공유 시트** 하나다.
- 생성 커버의 규칙은 웹 [`frontend/src/lib/covers.ts`](../../frontend/src/lib/covers.ts)와
  같다 — 무늬 4종은 도메인 해시로 고르고, **색은 facet에서만** 온다. Swift로 옮길 때
  같은 FNV-1a 해시를 써야 같은 도메인이 두 클라이언트에서 같은 무늬로 나온다.

## 구현 전에 다시 읽을 것

프로토타입은 화면만 말한다. 동작의 제약은 아래에 있다.

- [`../README.md`](../README.md) — 2초 저장, App Group 큐, Keychain 공유
- [`../../docs/v2/09-PLAN-REVIEW.md`](../../docs/v2/09-PLAN-REVIEW.md) ⑥·⑦ —
  큐 설계와 ATS·저전력 모드·On-Demand VPN·개발자 계정
- [`../../.claude/rules/ios.md`](../../.claude/rules/ios.md) — 워크스페이스 규칙
