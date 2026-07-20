---
paths:
  - "ios/**"
---

# iOS 워크스페이스 규칙 (M4)

- SwiftUI. React Native 금지 (v1 잔재).
- 제품의 심장은 Share Extension: 공유 시트에서 한 탭 → 저장 완료까지 2초 미만이 DoD다.
- API 키는 Keychain(앱 그룹 공유)에 저장. 서버 접근은 Tailscale 경유 — 주소는 IP 형식(`http://100.x.y.z:8080`)을 쓴다 (호스트네임 + 평문 HTTP는 ATS 차단).
- API 클라이언트는 `api/openapi.yaml`에서 **swift-openapi-generator**(Apple 공식, URLSession 트랜스포트)로 생성한 코드를 사용한다 — API 요청/응답 타입 수작성 금지 (`docs/v2/06-API-SPECIFICATION.md`는 해설).
- **구현 착수 전 `docs/v2/09-PLAN-REVIEW.md`의 ⑥(App Group 로컬 큐 — 서버 불달 시 저장 유실 방지)·⑦(ATS/절전/On-Demand VPN/개발자 계정) 확인.** "POST 후 즉시 닫힘" 패턴은 요청 유실 경로이므로 금지.
- 목록은 커서 페이지네이션(`next_cursor`) 기반 무한 스크롤 — 페이지 번호 가정 금지.
