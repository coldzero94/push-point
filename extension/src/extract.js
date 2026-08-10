// 캡처 규칙 — **이 파일이 플랫폼 간 공유되는 유일한 로직이다.**
//
// 플랫폼 API(chrome.*, browser.*, webkit.*)를 절대 참조하지 않는다. 문서 하나를 받아
// 저장 계약({url, title, description, body_text, keywords})을 만들 뿐이라, 이 파일을 실행할 수
// 있는 환경이면 어디서든 같은 규칙으로 캡처된다:
//
//   Chrome/Firefox 확장 → chrome.scripting.executeScript로 페이지에 주입
//   iOS Share Extension → WKWebView 또는 Safari의 JS preprocessing file에서 평가
//   새 플랫폼          → 전송(HTTP POST)만 구현하면 된다
//
// 그래서 플랫폼을 더하거나 빼도 "무엇을 본문으로 보는가"는 한 곳에서만 바뀐다.
// 서버는 어느 플랫폼이 보냈는지 알 필요가 없다 — body_text가 왔다는 사실만 본다.

/**
 * 본문 후보에서 걷어낼 요소 — 내비게이션·광고·폼은 본문이 아니다.
 *
 * 태그 이름만으로는 부족하다. 요즘 광고·추천 위젯(Taboola·Outbrain류)은 평범한
 * `div`라서 태그로는 안 걸리는데, 뉴스 사이트에서는 그 덩어리가 본문보다 길 때도 있다 —
 * 실제로 nbcnews.com 기사의 요약이 통째로 협찬 문구로 채워졌다. 그래서 클래스·id·
 * data 속성에 흔히 박히는 표식도 함께 본다.
 */
const DROP_SELECTOR = [
  'script, style, noscript, iframe, nav, header, footer, aside, form, svg, template',
  '[class*="taboola" i], [class*="outbrain" i], [class*="sponsor" i], [class*="advert" i]',
  '[class*="promo" i], [class*="newsletter" i], [class*="related" i], [class*="recirc" i]',
  '[id*="taboola" i], [id*="outbrain" i], [data-testid*="ad" i]',
  'figure figcaption, [role="complementary"], [aria-hidden="true"]',
].join(', ');

/**
 * 블록 경계를 만드는 요소 — 이들 뒤에 개행을 넣어 문장 경계를 살린다.
 *
 * 왜 필요한가: 아래 capture는 원본을 건드리지 않으려고 **복제본**에서 텍스트를 읽는데,
 * 문서에 붙어 있지 않은 노드의 `innerText`는 렌더 정보를 쓸 수 없어 `textContent`처럼
 * 동작한다(명세: 렌더되지 않는 요소는 textContent를 반환). 즉 지금까지 블록 경계가
 * 살아남은 것은 **원본 HTML에 줄바꿈이 있던 사이트에서 우연히** 그랬을 뿐이고,
 * 압축된 HTML(nbcnews.com)에서는 5KB 본문이 두 줄이 되어 요약기가 문장을 나눌 수 없었다.
 *
 * `tr`만으로는 모자란다 — 행 경계만 생기고 **셀은 이어붙는다**(`이름크기`). 그 모양은
 * 요약기의 접착 토큰 휴리스틱에 걸려서 캡처된 표가 통째로 요약에서 버려지고, 화면에는
 * "이 링크는 요약이 없네"로만 보인다. 그래서 `td, th`도 경계다.
 */
const BLOCK_SELECTOR =
  'p, div, section, article, h1, h2, h3, h4, h5, h6, li, blockquote, pre, tr, td, th, figcaption';

/** 본문 컨테이너 후보를 우선순위대로 — 없으면 body 전체. */
const ROOT_SELECTORS = ['article', '[role="main"]', 'main', '#content', '.post-content'];

// 서버와 같은 상한(backend/internal/textutil). 서버가 바이트로 다시 자르므로 여기서는
// 넉넉한 문자 수로만 막아 전송량을 줄인다 — 경계를 정확히 맞추려 애쓰지 않는다.
const MAX_BODY_CHARS = 32000;
const MAX_TITLE_CHARS = 300;
const MAX_DESC_CHARS = 1000;
const MAX_KEYWORDS_CHARS = 512;

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

/**
 * collectKeywords는 **발행자가 스스로 붙인 분류**를 모은다 — 우리가 본문에서 추론한 값이
 * 아니라 사이트가 "이 글은 스포츠다"라고 선언한 값이다. 태거에서 도메인 맵과 같은 급의
 * 신호이면서, 사이트별 등록이 필요 없다는 점에서 그보다 일반적이다.
 *
 * 서버(`backend/internal/scraper` parseKeywords)와 **같은 출처를 본다.** 서버가 못 가져오는
 * 페이지를 위해 클라이언트가 캡처하는 것이므로, 여기서 덜 모으면 바로 그 페이지들에서만
 * 신호가 빠진다.
 *
 * @param {Document} doc
 * @returns {string} 콤마로 이은 분류 문자열 (없으면 '')
 */
function collectKeywords(doc) {
  const parts = [];
  const seen = new Set();
  const add = (v) => {
    if (typeof v !== 'string') return;
    for (const raw of v.split(',')) {
      const f = raw.trim();
      if (!f) continue;
      const key = f.toLowerCase();
      if (!seen.has(key)) {
        seen.add(key);
        parts.push(f);
      }
    }
  };

  // news_keywords는 Google News 규격 — keywords보다 정제된 값을 넣는 사이트가 많다.
  add(metaContent(doc, 'keywords'));
  add(metaContent(doc, 'news_keywords'));
  // article:section(섹션)·article:tag(주제어)는 여러 개일 수 있어 첫 값만 보지 않는다.
  doc
    .querySelectorAll(
      'meta[property="article:section"], meta[property="article:tag"],' +
        'meta[name="article:section"], meta[name="article:tag"]',
    )
    .forEach((el) => add(el.getAttribute('content')));
  // JSON-LD — og를 안 쓰고 schema.org만 쓰는 사이트가 있다. 스키마 모양이 제각각(배열·
  // @graph·중첩)이라 구조를 가정하지 않고 키만 찾아 훑는다.
  doc.querySelectorAll('script[type="application/ld+json"]').forEach((el) => {
    let parsed;
    try {
      parsed = JSON.parse(el.textContent || '');
    } catch {
      return; // 깨진 JSON-LD는 흔하다 — 조용히 넘긴다. 보조 신호다.
    }
    walkJSONLD(parsed, 0, add);
  });

  return parts.join(', ').slice(0, MAX_KEYWORDS_CHARS);
}

/** walkJSONLD는 임의 모양의 값에서 분류 키를 찾아 add에 넘긴다. depth는 깊이 폭주 방지. */
function walkJSONLD(v, depth, add) {
  if (depth > 8 || v === null || typeof v !== 'object') return;
  if (Array.isArray(v)) {
    for (const e of v) walkJSONLD(e, depth + 1, add);
    return;
  }
  for (const [k, val] of Object.entries(v)) {
    if (['articlesection', 'keywords', 'genre'].includes(k.toLowerCase())) {
      if (typeof val === 'string') add(val);
      else if (Array.isArray(val)) val.forEach((e) => add(e));
    } else {
      walkJSONLD(val, depth + 1, add);
    }
  }
}

/**
 * 본문 컨테이너를 고른다.
 *
 * 1순위는 시맨틱 후보(article, main 등)다. 맞으면 가장 정확하다.
 *
 * 2순위는 **문단 밀도**다. 시맨틱 태그를 안 쓰는 사이트에서 곧바로 body로 폴백하면
 * 페이지 크롬이 통째로 본문이 된다 — marketwatch.com에서 실제로 그랬고, 캡처된 본문이
 * 사이트 검색 UI와 시세 티커("DJIA51947.250.46%S&P 500...")로 시작했다. 그 상태로는
 * 광고 셀렉터를 아무리 늘려도 못 막는다. 크롬에는 `<p>`가 거의 없다는 성질을 쓰면
 * 목록·티커·내비게이션을 한 번에 걸러낼 수 있다.
 */
function pickRoot(doc) {
  for (const sel of ROOT_SELECTORS) {
    const el = doc.querySelector(sel);
    // 후보가 껍데기뿐인 사이트가 있어 최소 길이로 거른다.
    if (el && (el.innerText || '').trim().length > 200) return el;
  }
  return pickByParagraphDensity(doc) || doc.body;
}

/**
 * 문단 텍스트가 가장 많은 컨테이너를 고른다.
 *
 * 후보의 **직계 자식** `<p>`만 센다. 조상까지 세면 body가 항상 이기므로 의미가 없다.
 * 동점이면 더 깊은(= 더 좁은) 쪽이 낫다 — 얕은 컨테이너일수록 크롬을 더 안고 있다.
 */
function pickByParagraphDensity(doc) {
  let best = null;
  let bestLen = 0;
  for (const el of doc.querySelectorAll('article, main, section, div')) {
    let len = 0;
    for (const child of el.children) {
      if (child.tagName === 'P') len += (child.textContent || '').trim().length;
    }
    // 문단 몇 줄로는 본문이라 하기 어렵다. 요약 가드(200룬)와 같은 자릿수로 둔다.
    if (len > bestLen && len > 200) {
      best = el;
      bestLen = len;
    }
  }
  return best;
}

/**
 * capture는 문서에서 저장 계약 페이로드를 만든다.
 * @param {Document} doc
 * @param {string} url  캡처 대상 URL (문서의 최종 URL)
 * @returns {{url: string, title: string, description: string, body_text: string, keywords: string}}
 */
function capture(doc, url) {
  const root = pickRoot(doc);
  // 원본 DOM을 건드리지 않으려고 복제한 뒤 잡음을 제거한다 — 사용자가 보고 있는
  // 페이지를 캡처가 바꾸면 안 된다.
  let text = '';
  if (root) {
    const clone = root.cloneNode(true);
    clone.querySelectorAll(DROP_SELECTOR).forEach((el) => el.remove());
    // 블록 경계를 우리가 만든다. 복제본은 문서에 붙어 있지 않아 innerText가 렌더 정보를
    // 쓸 수 없고 textContent와 같아지므로(BLOCK_SELECTOR 주석), 경계를 넣지 않으면
    // 문단들이 한 줄로 이어붙는다 — 그러면 요약기가 문장을 나누지 못한다.
    clone.querySelectorAll('br').forEach((el) => el.replaceWith(doc.createTextNode('\n')));
    clone.querySelectorAll(BLOCK_SELECTOR).forEach((el) => el.append(doc.createTextNode('\n')));
    text = (clone.textContent || '').replace(/[^\S\n]+/g, ' ').replace(/\n{3,}/g, '\n\n').trim();
  }
  return {
    url,
    title: (metaContent(doc, 'og:title') || doc.title || '').trim().slice(0, MAX_TITLE_CHARS),
    description: metaContent(doc, 'og:description', 'description').slice(0, MAX_DESC_CHARS),
    body_text: text.slice(0, MAX_BODY_CHARS),
    keywords: collectKeywords(doc),
  };
}

/**
 * captureOnce는 한 번만 캡처하고 결과를 재사용한다 — 아래 두 규약이 같은 결과를 쓴다.
 *
 * **캡처가 실패해도 URL은 반드시 돌려준다.** 저장의 단위가 URL이라, 본문 추출이 나쁜
 * 날을 보냈다고 저장 자체가 사라지면 안 된다.
 *
 * 이게 이론이 아닌 이유: 2026-08-03에 공유 저장이 `URL을 찾을 수 없습니다`로 끝나는
 * 것을 봤다. 확장 코드에는 `UTType.url` 폴백이 있지만, **JS 전처리가 선언돼 있으면
 * Safari는 propertyList 하나만 준다** — 폴백이 쓸 재료 자체가 오지 않는다. 그래서
 * 폴백은 여기, JS 안에 있어야 한다.
 *
 * 다만 이 방어에도 한계가 있다: JS가 **시작조차 못 하는** 경우(페이지가 모달을 띄운
 * 채인 경우 등)에는 이 `try`도 돌지 않는다. 그때는 구할 방법이 없다(04 §7.3.2).
 */
let captured = null;
function captureOnce() {
  if (captured !== null) return captured;
  try {
    captured = capture(document, location.href);
  } catch (e) {
    // 본문 없이 URL만 보낸다. 서버가 스크레이핑을 시도하므로 완전한 손실은 아니다 —
    // 아무것도 안 보내는 것과는 다르다.
    captured = { url: location.href, title: (document.title || '').slice(0, 300),
                 description: '', body_text: '', keywords: '' };
  }
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
