-- 본문 텍스트 컬럼 추가 (docs/v2/05-DATA-SCHEMA.md §2 links)
-- go-trafilatura가 추출한 본문(보일러플레이트 제거)을 담는다. 규칙 태거(M3)·추출식 요약(M5)의
-- 입력 전용 — links_fts에는 넣지 않고(trigram이 본문에서 폭증), API 계약에도 노출하지 않는다.
-- ALTER ADD COLUMN이라 맨 뒤에 붙는다(0001~0003 불변). 추출 실패·비-아티클이면 '' 기본값.
ALTER TABLE links ADD COLUMN body_text TEXT NOT NULL DEFAULT '';
