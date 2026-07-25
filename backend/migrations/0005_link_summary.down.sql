-- 0005 롤백 — summary 컬럼 제거 (SQLite 3.35+ DROP COLUMN, modernc 드라이버 지원).
ALTER TABLE links DROP COLUMN summary;
