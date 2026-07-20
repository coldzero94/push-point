-- 0001_init 롤백: 생성 역순으로 제거 (인덱스는 테이블과 함께 삭제됨)
DROP TABLE IF EXISTS links_fts;
DROP TABLE IF EXISTS tag_embeddings;
DROP TABLE IF EXISTS corpus_df;
DROP TABLE IF EXISTS tag_feedback;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS link_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS links;
