-- 시드 태그 30개 제거 (link_tags/tag_feedback/tag_embeddings는 FK CASCADE로 함께 삭제)
DELETE FROM tags WHERE name IN (
  'dev','golang','kubernetes','ios','swift','python','rust','javascript',
  'frontend','backend','database','devops','security','opensource','ai','llm',
  'data','design','article','video','tutorial','news','science','finance',
  'career','productivity','book','podcast','travel','life'
);
