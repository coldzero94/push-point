-- 알아봄 원장 (2026-08-11)
--
-- **이 분야에서 아무도 만든 적 없는 표다.** 30년치 선행 사례(Remembrance Agent 1996부터
-- Evernote Context, Heyday까지)를 훑어도 "제안을 보여줬는데 사람이 무시했다"를 기록한
-- 제품이 없다. 그래서 "임계값만 잘 잡으면 된다"는 논쟁이 30년째 취향으로 결판난다.
--
-- 이 표의 목적은 기능이 아니라 **다음 결정을 반증 가능하게 만드는 것**이다. 알아봄이
-- 값을 하는지, 어느 단이 값을 하는지, 임계를 올려야 하는지를 숫자로 답한다.
--
-- `tag_feedback`을 본떴다 — append-only, 갱신 없음. 다만 그 표가 남긴 교훈도 함께 받는다:
-- **읽는 코드 없이 쌓기만 하는 원장은 죽은 자산이다.** `tag_feedback`은 1년 가까이
-- INSERT만 됐고 테스트 밖 SELECT가 0이었다. 그래서 이 표는 읽는 명령(`just recognition`)과
-- **같은 변경에서** 들어온다.
CREATE TABLE recognition_events (
  id       INTEGER PRIMARY KEY,
  link_id  INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  -- rung 0 = 중복 저장(그때 뭐라고 썼는지), 1 = 도메인 재조우.
  -- 정수인 이유: 단이 늘어날 때 문자열 오타가 조용히 새 부류를 만드는 것을 막는다.
  rung     INTEGER NOT NULL,
  shown_at INTEGER NOT NULL DEFAULT (unixepoch()),
  -- 사람이 그 알림을 눌러 링크로 갔는가. NULL이면 **무시됐다** — 이 열이 이 표의 요점이다.
  tapped_at INTEGER
);

-- 조회는 언제나 "최근 N일" 형태다.
CREATE INDEX idx_recognition_shown ON recognition_events(shown_at);
