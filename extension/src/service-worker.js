// 전송 계층 — **플랫폼(Chrome)에 종속되는 유일한 파일이다.**
// 캡처 규칙은 extract.js가 갖고 있고, 여기는 "언제 실행하고 어디로 보내는가"만 안다.
// 다른 플랫폼을 붙일 때 다시 쓰는 것은 extract.js이고 이 파일은 그 플랫폼 것으로 갈아탄다.
//
// **API 키는 여기(service worker)에서만 읽고 쓴다.** 페이지에 주입되는 것은 extract.js뿐이고
// 그 코드는 키를 보지 못한다 — 악성 페이지가 훔칠 표면이 없다.

const SETTINGS = ['origin', 'apiKey'];

async function loadSettings() {
  const s = await chrome.storage.local.get(SETTINGS);
  if (!s.origin || !s.apiKey) throw new Error('설정 필요 — 확장 옵션에서 서버 주소와 API 키를 넣으세요');
  return s;
}

/** 배지로 결과를 3초간 알린다 (팝업 없음 — 한 번 눌러 저장이 이 확장의 전부다). */
async function badge(tabId, text, color) {
  await chrome.action.setBadgeBackgroundColor({ tabId, color });
  await chrome.action.setBadgeText({ tabId, text });
  setTimeout(() => chrome.action.setBadgeText({ tabId, text: '' }), 3000);
}

async function savePage(tab) {
  if (!tab || !tab.id || !/^https?:/.test(tab.url || '')) return;
  try {
    const { origin, apiKey } = await loadSettings();

    // 페이지에 캡처 규칙만 주입한다. 반환값이 저장 계약 페이로드다.
    const [{ result: payload } = {}] = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      files: ['src/extract.js'],
    });
    if (!payload || !payload.url) throw new Error('페이지에서 콘텐츠를 얻지 못했습니다');

    const res = await fetch(new URL('/api/v1/links', origin), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${apiKey}` },
      body: JSON.stringify(payload),
    });

    if (res.status === 201) return badge(tab.id, '✓', '#197459');
    if (res.status === 200) return badge(tab.id, '=', '#8E6400'); // 이미 저장됨(본문은 보충될 수 있음)
    const detail = await res.text().catch(() => '');
    throw new Error(`서버 ${res.status} ${detail.slice(0, 200)}`);
  } catch (err) {
    console.error('[push-point] 저장 실패:', err);
    await badge(tab.id, '!', '#B3261E');
  }
}

chrome.action.onClicked.addListener(savePage);
chrome.commands.onCommand.addListener(async (cmd) => {
  if (cmd !== 'save-page') return;
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  await savePage(tab);
});
