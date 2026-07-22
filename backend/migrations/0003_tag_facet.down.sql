-- facet 컬럼 제거. SQLite 3.35+ ALTER TABLE DROP COLUMN을 쓴다
-- (실측: modernc.org/sqlite 번들 3.53.3에서 성공 — facet은 인덱스·뷰·트리거·생성열 어디에도
--  참조되지 않고, 자기 자신에만 걸린 CHECK 제약은 컬럼과 함께 사라진다).
ALTER TABLE tags DROP COLUMN facet;
