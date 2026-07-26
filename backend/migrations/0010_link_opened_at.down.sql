DROP INDEX IF EXISTS idx_links_unopened;
ALTER TABLE links DROP COLUMN opened_at;
