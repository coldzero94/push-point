// Every word on the landing page, in one place.
//
// Two files would drift. This project has already been bitten three times by the
// same shape of mistake — the streak rule, the cover pattern, and a hand-copied
// iOS golden — where two sides of one truth diverged and both kept looking fine.
// So the page is one document and the copy is one object; `scripts/site_copy_check.py`
// fails if the two locales stop having identical key sets.
//
// English is the default because the repo is a public GitHub landing page
// (CLAUDE.md language policy). Korean is a first-class second, not an afterthought:
// the app's own UI is Korean, so the screenshots are Korean either way.
export const COPY = {
  en: {
    'html.lang': 'en',
    'meta.title': 'Push-Point — save a link, it tags itself',
    'meta.desc':
      'A personal link archive that tags what you save without an LLM. One Go binary, one SQLite file, nothing leaves your machine.',

    'nav.repo': 'GitHub',
    'nav.lang': '한국어',

    'hero.tag': 'Personal link archive',
    'hero.title': 'Save a link.\nIt tags itself.',
    'hero.lede':
      'Share a page from Safari, Chrome, or anywhere else. Push-Point pulls the title, the image and the text, then files it under tags it worked out on its own — no LLM, no API key, no account. One Go binary and one SQLite file, on your machine.',
    'hero.cta': 'Read the source',
    'hero.cta2': 'How it works',
    'hero.note': 'Apache-2.0 · self-hosted · single user by design',

    'demo.title': 'Reading something. Two taps. It is filed.',
    'demo.body':
      'Watch where it ends: you never leave the page. A notification says it is saved and shows the tags it already worked out — frontend, javascript, dev — and Safari is exactly where you left it. There is no app to open, no form, and nothing to come back to later. The extension writes straight to the shared database, so it finishes in airplane mode too. This save took 76 ms.',
    'demo.caption': 'A real simulator recording, not a mockup. The list and stats beside it are the same archive, opened separately.',

    'num.title': 'Numbers, measured',
    'num.body':
      'Every figure here comes from a command in the repo, not from an impression. Re-run them yourself: they are the same commands CI uses.',
    'num.1.v': '1.96 ms',
    'num.1.k': 'Save API p99',
    'num.1.n': 'just bench-http · 2,000 requests · gate is 50 ms',
    'num.2.v': '48 ms',
    'num.2.k': 'Real share-sheet save',
    'num.2.n': 'Korean news article, scrape + tag + summary included',
    'num.3.v': '0.905',
    'num.3.k': 'Tagging recall@3',
    'num.3.n': 'just eval · frozen 84-link test set · domain-only baseline 0.250',
    'num.4.v': '0.480',
    'num.4.k': 'Search hit@1',
    'num.4.n': 'just eval-search · 25 queries · MRR@10 0.547',

    'how.title': 'How it works',
    'how.1.t': 'Capture',
    'how.1.b':
      'Safari runs the same extraction rule the browser extension uses, so the body text comes along — pages that block servers still get saved with their content. Everywhere else sends the URL and the server fetches it.',
    'how.2.t': 'Tag',
    'how.2.b':
      'A rule-based NLU reads the domain, the title and the body against a 42-tag dictionary with synonyms in both languages. It runs in-process in a few hundred milliseconds and costs nothing, because nothing is called.',
    'how.3.t': 'Find again',
    'how.3.b':
      'SQLite FTS5 over title, description, summary, notes and tags, with a LIKE fallback for queries too short to tokenize. Filter by tag, by status, by date.',

    'shot.list.t': 'The board',
    'shot.list.b':
      'Links without a thumbnail get a cover generated from the domain — the same domain always draws the same mark, so a source you save from often becomes recognizable.',
    'shot.tags.t': 'The tag dictionary',
    'shot.tags.b':
      'Tags are editable, and so are their synonyms. This is the whole model — there is no hidden one.',

    'honest.title': 'What it does not do',
    'honest.body':
      'The interesting part of a tool is usually where it stops. These are decisions, not gaps waiting to be filled.',
    'honest.1':
      'No accounts, no multi-user, no sync service. One person, one API key, one file to back up.',
    'honest.2':
      'No LLM API. That constraint is the point of the project — the tagger has to earn its accuracy from rules.',
    'honest.3':
      'It will not break into a page for you. A site behind a bot wall or a login is reported as failed with a plain reason, not saved as an empty shell. If you can see it in your own browser, the extension can save it.',
    'honest.4':
      'Not on the App Store. Build it and run it yourself; that is the deployment story.',

    'start.title': 'Run it',
    'start.body':
      'Go 1.25 and just. The single binary serves the API, the worker and the web UI from one process, and applies its own migrations on startup.',
    'start.note':
      'That is the whole setup. `just release` produces one binary with the web UI embedded.',

    'foot.license': 'Apache-2.0',
    'foot.built': 'Built in the open',
    'foot.docs': 'Docs (Korean)',
  },

  ko: {
    'html.lang': 'ko',
    'meta.title': 'Push-Point — 링크를 저장하면, 알아서 태그가 붙는다',
    'meta.desc':
      'LLM 없이 저장한 링크에 태그를 붙이는 개인용 링크 아카이브. Go 바이너리 하나, SQLite 파일 하나, 밖으로 나가는 것은 없다.',

    'nav.repo': 'GitHub',
    'nav.lang': 'English',

    'hero.tag': '개인용 링크 아카이브',
    'hero.title': '링크를 저장하면,\n알아서 태그가 붙는다.',
    'hero.lede':
      '사파리든 크롬이든 어디서든 공유하면 됩니다. Push-Point가 제목과 이미지와 본문을 끌어와, 스스로 판단한 태그로 정리합니다. LLM도 API 키도 계정도 없습니다. Go 바이너리 하나와 SQLite 파일 하나가 내 컴퓨터 안에서 전부 합니다.',
    'hero.cta': '소스 보기',
    'hero.cta2': '어떻게 동작하나',
    'hero.note': 'Apache-2.0 · 직접 호스팅 · 단일 사용자가 설계 전제',

    'demo.title': '보다가, 두 번 누르면, 정리까지 끝',
    'demo.body':
      '끝나는 지점을 보세요 — 페이지를 떠나지 않습니다. 알림이 저장됐다고 알려주면서 이미 붙은 태그(frontend · javascript · dev)까지 보여주고, 사파리는 있던 그대로입니다. 열 앱도, 채울 입력란도, 나중에 돌아와서 할 일도 없습니다. 확장이 공유 데이터베이스에 직접 쓰므로 비행기 모드에서도 끝납니다. 이 저장은 76 ms 걸렸습니다.',
    'demo.caption': '목업이 아니라 실제 시뮬레이터 녹화입니다. 옆의 목록·통계는 같은 아카이브를 따로 연 것입니다.',

    'num.title': '숫자는 재서 넣었다',
    'num.body':
      '여기 있는 값은 전부 저장소의 명령으로 나온 것이지 인상이 아닙니다. 직접 다시 재 보세요 — CI가 쓰는 것과 같은 명령입니다.',
    'num.1.v': '1.96 ms',
    'num.1.k': '저장 API p99',
    'num.1.n': 'just bench-http · 요청 2,000건 · 게이트는 50 ms',
    'num.2.v': '48 ms',
    'num.2.k': '실제 공유 저장',
    'num.2.n': '한국어 기사, 스크랩 + 태깅 + 요약까지 포함',
    'num.3.v': '0.905',
    'num.3.k': '태깅 recall@3',
    'num.3.n': 'just eval · 동결된 84건 test 세트 · 도메인만 쓰는 기준선은 0.250',
    'num.4.v': '0.480',
    'num.4.k': '검색 hit@1',
    'num.4.n': 'just eval-search · 질의 25건 · MRR@10 0.547',

    'how.title': '어떻게 동작하나',
    'how.1.t': '캡처',
    'how.1.b':
      '사파리에서는 브라우저 확장과 같은 추출 규칙이 돌아 본문까지 함께 옵니다. 서버가 막히는 페이지도 내용을 담은 채로 저장됩니다. 그 밖에서는 URL만 오고 서버가 가져옵니다.',
    'how.2.t': '태깅',
    'how.2.b':
      '규칙 기반 NLU가 도메인·제목·본문을 42개 태그 사전과 대조합니다. 사전에는 한국어와 영어 동의어가 함께 들어 있습니다. 프로세스 안에서 수백 밀리초에 끝나고, 부르는 곳이 없으니 비용도 0입니다.',
    'how.3.t': '다시 찾기',
    'how.3.b':
      'SQLite FTS5가 제목·설명·요약·메모·태그를 훑습니다. 토큰화하기엔 너무 짧은 질의는 LIKE로 떨어집니다. 태그·상태·날짜로 좁힐 수 있습니다.',

    'shot.list.t': '보드',
    'shot.list.b':
      '썸네일이 없는 링크에는 도메인으로 만든 커버가 붙습니다. 같은 도메인은 늘 같은 그림이라, 자주 저장하는 출처가 눈에 익습니다.',
    'shot.tags.t': '태그 사전',
    'shot.tags.b': '태그도 동의어도 고칠 수 있습니다. 이게 모델의 전부고, 숨겨진 것은 없습니다.',

    'honest.title': '하지 않는 것',
    'honest.body':
      '도구에서 재미있는 부분은 대개 어디서 멈추느냐입니다. 아래는 아직 못 채운 구멍이 아니라 결정입니다.',
    'honest.1': '계정도, 멀티유저도, 동기화 서비스도 없습니다. 한 사람, API 키 하나, 백업할 파일 하나.',
    'honest.2': 'LLM API를 쓰지 않습니다. 그 제약이 이 프로젝트의 요점이라, 태거는 정확도를 규칙으로 벌어야 합니다.',
    'honest.3':
      '페이지를 뚫어 주지 않습니다. 봇 차단이나 로그인 벽 뒤의 사이트는 이유를 밝히고 실패로 남지, 빈 껍데기로 저장되지 않습니다. 내 브라우저에서 보이는 것이라면 확장이 저장할 수 있습니다.',
    'honest.4': 'App Store에 없습니다. 직접 빌드해서 직접 돌리는 것이 배포 방식입니다.',

    'start.title': '실행',
    'start.body':
      'Go 1.25와 just가 있으면 됩니다. 바이너리 하나가 API·워커·웹 UI를 한 프로세스에서 서빙하고, 마이그레이션은 시작할 때 알아서 적용됩니다.',
    'start.note': '이게 전부입니다. `just release`는 웹 UI가 내장된 바이너리 하나를 만듭니다.',

    'foot.license': 'Apache-2.0',
    'foot.built': '공개된 채로 만들고 있습니다',
    'foot.docs': '문서 (한국어)',
  },
}
