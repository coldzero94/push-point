-- 발행자가 스스로 붙인 분류 (docs/v2/05-DATA-SCHEMA.md §2 links)
--
-- 도메인 맵은 사이트마다 손으로 등록해야 하고 등록되지 않은 도메인에는 효과가 0이다 —
-- 확장은 되지만 일반화가 아니다. 반면 meta[keywords]·article:section·JSON-LD
-- articleSection은 **발행자가 직접 붙인 값**이라 등록 없이도 동작한다.
--
-- 태깅 입력이므로 links에 둔다(tag 잡이 GetLinkContent로 읽는다). 화면에는 노출하지
-- 않으므로 Link/LinkDetail 응답에는 넣지 않았다 — 필요해지면 그때 계약에 더한다.
ALTER TABLE links ADD COLUMN keywords TEXT NOT NULL DEFAULT '';
