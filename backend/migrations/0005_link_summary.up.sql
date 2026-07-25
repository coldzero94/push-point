-- 추출식 요약 컬럼 추가 (docs/v2/05-DATA-SCHEMA.md §2 links)
-- tag 잡이 body_text에서 산문 문장 2~3개를 골라 '\n'으로 이어 쓴다(M5 Phase A, LLM 없음).
-- LinkDetail에만 노출하고 목록·검색에는 싣지 않으며, links_fts에도 넣지 않는다(stage 2 재검토).
-- 가드(본문 200룬 미만 · 산문 3문장 미만 · description과 0.8 이상 중복) 불통과면 '' —
-- 그때 UI는 요약 섹션을 아예 그리지 않는다. ALTER ADD COLUMN이라 맨 뒤에 붙는다(0001~0004 불변).
ALTER TABLE links ADD COLUMN summary TEXT NOT NULL DEFAULT '';
