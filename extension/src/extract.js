// 캡처 규칙 — **이 파일이 플랫폼 간 공유되는 유일한 로직이다.**
//
// 플랫폼 API(chrome.*, browser.*, webkit.*)를 절대 참조하지 않는다. 문서 하나를 받아
// 저장 계약({url, title, description, body_text})을 만들 뿐이라, 이 파일을 실행할 수 있는
// 환경이면 어디서든 같은 규칙으로 캡처된다:
//
//   Chrome/Firefox 확장 → chrome.scripting.executeScript로 페이지에 주입
//   iOS Share Extension → WKWebView 또는 Safari의 JS preprocessing file에서 평가
//   새 플랫폼          → 전송(HTTP POST)만 구현하면 된다
//
// 그래서 플랫폼을 더하거나 빼도 "무엇을 본문으로 보는가"는 한 곳에서만 바뀐다.
// 서버는 어느 플랫폼이 보냈는지 알 필요가 없다 — body_text가 왔다는 사실만 본다.

/** 본문 후보에서 걷어낼 요소 — 내비게이션·광고·폼은 본문이 아니다. */
const DROP_SELECTOR = 'script, style, noscript, iframe, nav, header, footer, aside, form, svg, template';

/** 본문 컨테이너 후보를 우선순위대로 — 없으면 body 전체. */
const ROOT_SELECTORS = ['article', '[role="main"]', 'main', '#content', '.post-content'];

// 서버와 같은 상한(backend/internal/textutil). 서버가 바이트로 다시 자르므로 여기서는
// 넉넉한 문자 수로만 막아 전송량을 줄인다 — 경계를 정확히 맞추려 애쓰지 않는다.
const MAX_BODY_CHARS = 32000;
const MAX_TITLE_CHARS = 300;
const MAX_DESC_CHARS = 1000;

function metaContent(doc, ...keys) {
  for (const k of keys) {
    for (const attr of ['property', 'name']) {
      const el = doc.querySelector(`meta[${attr}="${k}"]`);
      const v = el && el.getAttribute('content');
      if (v && v.trim()) return v.trim();
    }
  }
  return '';
}

/** 본문 컨테이너를 고른다 — 가장 앞선 후보 중 텍스트가 충분한 것. */
function pickRoot(doc) {
  for (const sel of ROOT_SELECTORS) {
    const el = doc.querySelector(sel);
    // 후보가 껍데기뿐인 사이트가 있어 최소 길이로 거른다.
    if (el && (el.innerText || '').trim().length > 200) return el;
  }
  return doc.body;
}

/**
 * capture는 문서에서 저장 계약 페이로드를 만든다.
 * @param {Document} doc
 * @param {string} url  캡처 대상 URL (문서의 최종 URL)
 * @returns {{url: string, title: string, description: string, body_text: string}}
 */
function capture(doc, url) {
  const root = pickRoot(doc);
  // 원본 DOM을 건드리지 않으려고 복제한 뒤 잡음을 제거한다 — 사용자가 보고 있는
  // 페이지를 캡처가 바꾸면 안 된다.
  let text = '';
  if (root) {
    const clone = root.cloneNode(true);
    clone.querySelectorAll(DROP_SELECTOR).forEach((el) => el.remove());
    // innerText는 렌더된 텍스트(줄바꿈·숨김 요소 반영)라 textContent보다 사람이 읽는 것에 가깝다.
    text = (clone.innerText || clone.textContent || '').replace(/[ \t ]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim();
  }
  return {
    url,
    title: (metaContent(doc, 'og:title') || doc.title || '').trim().slice(0, MAX_TITLE_CHARS),
    description: metaContent(doc, 'og:description', 'description').slice(0, MAX_DESC_CHARS),
    body_text: text.slice(0, MAX_BODY_CHARS),
  };
}

/** captureOnce는 한 번만 캡처하고 결과를 재사용한다 — 아래 두 규약이 같은 결과를 쓴다. */
let captured = null;
function captureOnce() {
  if (captured === null) captured = capture(document, location.href);
  return captured;
}

// ── 플랫폼 어댑터 ────────────────────────────────────────────────────────────
// 위의 캡처 규칙은 플랫폼을 모른다. 갈라지는 것은 "결과를 어떻게 돌려주는가"뿐이라
// 그 규약만 여기 둔다 — 플랫폼을 더하려면 이 아래에 줄을 더하면 되고, 규칙 자체는
// 건드리지 않는다.
//
//   iOS Safari : 공유 확장이 전역 ExtensionPreprocessingJS를 찾아 run()을 호출하고,
//                결과는 반환값이 아니라 completionFunction으로 넘겨야 한다.
//   Chrome MV3 : executeScript는 **마지막 표현식의 값**을 결과로 돌려준다.
//
// 모듈 시스템(export/import)에 의존하지 않는 이유도 같다 — 두 실행 환경 모두
// 이 파일을 평문으로 평가한다.
var ExtensionPreprocessingJS = { // eslint-disable-line no-var, no-unused-vars
  run(args) {
    args.completionFunction(captureOnce());
  },
  finalize() {},
};

captureOnce();
