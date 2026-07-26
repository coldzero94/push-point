-- link_tags는 FK CASCADE로 함께 지워진다.
DELETE FROM tags WHERE name IN
  ('sports','football','politics','economy','realestate','health','food','culture','game','education','law','startup');
