# ios — SwiftUI 앱 + Share Extension (M4 예정)

> Push-Point v2 — 마지막 업데이트: 2026-07-20

Push-Point의 iOS 클라이언트 워크스페이스다. SwiftUI 앱과 Share Extension으로
구성되며, M4(5주)에서 신규 작성한다.

## 현재 상태

아직 코드가 없다. M4 시작 시 Xcode 프로젝트를 이 디렉터리에 생성한다.

## 핵심 요구

- **Share Extension 최우선.** 공유 시트에서 한 탭 → App Group 로컬 큐에 우선
  기록 → 2~3s 타임아웃으로 `POST /api/v1/links` → 시트 닫힘까지 **2초 미만**.
  서버가 꺼져 있어도 저장은 성공(큐 적재)하고, 복구 후 자동 업로드된다
  (url_hash 멱등이라 재시도 안전). "POST 후 즉시 닫기"는 요청 유실 경로라 금지.
  저장 마찰이 2초를 넘으면 매일 쓰는 앱이 못 된다.
- API 키는 **Keychain에 앱 그룹 공유로 저장** — 앱과 Share Extension이 같은
  키를 사용한다.
- 앱 화면: 목록(커서 페이지네이션 무한 스크롤), 태그 필터, 검색,
  상세(태그 수정 = `PATCH /api/v1/links/{id}`).

## 서버 연결

- 백엔드는 단일 Go 바이너리로 홈 서버에서 동작하며, 기기에서는 Tailscale
  사설망으로 접근한다.
- 인증은 `Authorization: Bearer {API 키}` 헤더 하나다 (단일 사용자, 로그인 없음).
- 연결 구성 상세: [../docs/v2/07-DEPLOYMENT.md](../docs/v2/07-DEPLOYMENT.md)

## 계획

- M4 상세 계획: [../docs/v2/08-DEVELOPMENT-PLAN.md](../docs/v2/08-DEVELOPMENT-PLAN.md)의 M4 절
- DoD: 본인이 매일 실사용을 시작하고, 공유 시트 → 저장 완료가 2초 미만일 것.
- M4가 끝나기 전까지는 아무것도 "완성"이 아니다 — 실사용 시작이 M5~M6의 전제다.
