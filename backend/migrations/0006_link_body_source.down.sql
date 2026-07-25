-- 0006 롤백 — body_source 컬럼 제거 (SQLite 3.35+ DROP COLUMN).
ALTER TABLE links DROP COLUMN body_source;
