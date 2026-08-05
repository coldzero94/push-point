# extension — Push-Point 저장 확장 (Chrome/Firefox, MV3)

보고 있는 페이지를 **본문까지** Push-Point에 저장한다. 서버가 직접 가져올 수 없는 페이지
(자바스크립트로 그리는 SPA, 봇 차단, 로그인·구독이 필요한 글)를 위한 경로다 —
사용자는 이미 그 페이지를 로그인한 채 보고 있으므로, 서버가 더 열심히 긁는 대신
**브라우저가 이미 렌더한 콘텐츠를 함께 보낸다.**

## 구조 — 저장 규칙은 한 곳에만 있다

```
src/extract.js         캡처 규칙. 플랫폼 API를 전혀 참조하지 않는다.
                       "이 페이지에서 무엇을 본문으로 볼 것인가" — 이 파일이 전부다.
src/service-worker.js  Chrome 전송 계층. 언제 실행하고 어디로 보내는지만 안다.
src/options.html/.js   서버 주소·API 키 입력, 해당 origin 권한 요청.
```

`extract.js`는 문서를 받아 저장 계약(`{url, title, description, body_text}`)을 만들고
**마지막 표현식으로 그 값을 돌려준다.** Chrome의 `chrome.scripting.executeScript`와
iOS `WKWebView.evaluateJavaScript`가 값을 받는 방식이 정확히 같으므로, **같은 파일을 그대로**
다른 플랫폼에서 실행할 수 있다. 플랫폼을 더할 때 새로 쓰는 것은 전송 코드뿐이고, 빼려면
그 폴더만 지우면 된다. 서버는 어느 플랫폼이 보냈는지 알지 못한다 — `body_text`가 왔다는
사실만 본다(`docs/v2/ko/04-DATA-FLOW.md` §7.3).

## 설치 (Chrome)

1. `chrome://extensions` → 우측 상단 **개발자 모드** 켜기
2. **압축해제된 확장 프로그램을 로드** → 이 `extension/` 폴더 선택
3. 확장 **옵션**에서 서버 주소와 API 키 입력 → 저장
   (웹 앱 설정 화면의 “저장 도구”에서 두 값을 복사할 수 있다)

Firefox는 `about:debugging` → 임시 부가 기능 로드, 또는 `web-ext sign`으로 서명한 `.xpi`.

## 사용

툴바 아이콘 클릭 또는 `Cmd/Ctrl+Shift+S`. 결과는 배지로 3초간 표시된다.

| 배지 | 뜻 |
|---|---|
| `✓` | 새로 저장됨 (201) |
| `=` | 이미 있는 링크 (200) — 저장된 본문이 서버 출처였다면 이번 본문으로 보충된다 |
| `!` | 실패 — 자세한 사유는 확장의 service worker 콘솔 |

## 보안

- **API 키는 `chrome.storage.local`에만 있다.** 페이지에 주입되는 것은 `extract.js`뿐이고
  그 코드는 키를 보지 못하며, 네트워크 요청은 service worker에서만 나간다.
  방문한 페이지의 자바스크립트가 키를 가져갈 표면이 없다.
- `chrome.storage.sync`는 쓰지 않는다 — 벤더 서버로 동기화되는 경로다.
- `manifest.json`에 `<all_urls>`를 정적으로 선언하지 않는다. 옵션 화면에서 **사용자 서버
  origin 하나**만 권한을 받고, 페이지 접근은 `activeTab`(사용자가 버튼을 누른 탭)으로 제한된다.

## 한계

- CAPTCHA·로그인 자체를 대신하지 않는다. **사용자가 이미 보고 있는 페이지**를 캡처할 뿐이다.
- 추출은 규칙 기반(`article`/`main` 우선, 내비·푸터 제거)이라 특이한 레이아웃에서는 부정확할
  수 있다. 그때도 서버가 스크랩한 메타데이터는 그대로 남는다.
