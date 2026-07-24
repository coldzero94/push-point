-- 0004 롤백 — body_text 컬럼 제거 (SQLite 3.35+ DROP COLUMN, modernc 드라이버 지원).
ALTER TABLE links DROP COLUMN body_text;
