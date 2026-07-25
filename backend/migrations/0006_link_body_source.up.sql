-- 본문 출처 표시 (docs/v2/05-DATA-SCHEMA.md §2 links)
-- '' = 아직 본문 없음, 'server' = 스크래퍼가 추출, 'client' = 브라우저 확장·Share Extension이
-- 렌더된 페이지에서 캡처해 저장 요청에 실어 보냄. 'client'면 이후 스크랩이 제목·설명·본문을
-- 덮어쓰지 않는다 — 서버가 못 가져오는 페이지(SPA·봇 차단·로그인 벽)라서 클라이언트가 준 것이므로
-- 서버 재시도 결과가 항상 더 나쁘다. FTS 무관 컬럼이라 재색인 규약에 영향이 없다.
ALTER TABLE links ADD COLUMN body_source TEXT NOT NULL DEFAULT ''
  CHECK (body_source IN ('', 'server', 'client'));
