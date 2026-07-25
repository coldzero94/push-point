// 옵션 화면 — 서버 주소·API 키를 확장 저장소에 넣고, **그 origin에 대해서만** 호스트 권한을
// 요청한다. manifest에 <all_urls>를 정적으로 선언하지 않는 이유다(필요한 최소 권한만 갖는다).

const $ = (id) => document.getElementById(id);
const show = (msg, ok) => {
  $('status').textContent = msg;
  $('status').style.color = ok ? '#197459' : '#B3261E';
};

chrome.storage.local.get(['origin', 'apiKey']).then((s) => {
  $('origin').value = s.origin || '';
  $('key').value = s.apiKey || '';
});

$('save').addEventListener('click', async () => {
  const raw = $('origin').value.trim();
  const apiKey = $('key').value.trim();
  if (!raw || !apiKey) return show('서버 주소와 API 키를 모두 입력하세요', false);

  let origin;
  try {
    origin = new URL(raw).origin;
  } catch {
    return show('서버 주소가 URL 형식이 아닙니다', false);
  }

  // 이 origin으로만 요청할 수 있게 권한을 받는다 — 사용자 클릭 컨텍스트에서만 가능하다.
  const granted = await chrome.permissions.request({ origins: [`${origin}/*`] });
  if (!granted) return show('권한이 거부돼 저장 요청을 보낼 수 없습니다', false);

  await chrome.storage.local.set({ origin, apiKey });

  // 실제로 붙는지 확인해 준다 — 오타를 나중에 배지 '!'로 발견하지 않게.
  try {
    const res = await fetch(new URL('/api/v1/tags', origin), {
      headers: { Authorization: `Bearer ${apiKey}` },
    });
    if (res.ok) return show('저장됐고 서버 연결도 확인했습니다', true);
    if (res.status === 401) return show('저장됐지만 API 키가 서버와 다릅니다 (401)', false);
    show(`저장됐지만 서버가 ${res.status}를 돌려줬습니다`, false);
  } catch {
    show('저장됐지만 서버에 연결하지 못했습니다 (주소를 확인하세요)', false);
  }
});
