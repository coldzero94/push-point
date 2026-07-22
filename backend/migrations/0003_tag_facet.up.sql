-- 태그 facet 컬럼 추가 (docs/v2/05-DATA-SCHEMA.md §2 tags, api/openapi.yaml TagFacet)
-- facet은 "색"이 아니라 "분류 축"이다 — 서버는 의미(craft/media/life/neutral)만 저장하고,
-- 그 facet이 무슨 색인지는 각 클라이언트(web/iOS)가 자기 토큰 체계로 정한다.
-- 라이트/다크 2벌인 색 값을 계약에 넣으면 표현이 서버로 역전되므로 hex는 저장하지 않는다.
ALTER TABLE tags ADD COLUMN facet TEXT NOT NULL DEFAULT 'neutral'
  CHECK (facet IN ('craft','media','life','neutral'));

-- 시드 30개(0002)의 배정. 사전에 없는 새 태그는 DEFAULT로 neutral로 태어나고,
-- 사용자가 태그 관리 화면에서 facet을 고르면 색을 얻는다.
UPDATE tags SET facet='craft' WHERE name IN
  ('dev','golang','kubernetes','ios','swift','python','rust','javascript','frontend',
   'backend','database','devops','security','opensource','ai','llm','data','design');
UPDATE tags SET facet='media' WHERE name IN ('article','video','tutorial','book','podcast');
UPDATE tags SET facet='life'  WHERE name IN
  ('news','science','finance','career','productivity','travel','life');
