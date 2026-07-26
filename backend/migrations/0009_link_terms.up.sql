-- corpus_df 누적 파이프라인 (M3 계획 항목).
--
-- corpus_df 테이블은 0001부터 있었지만 아무도 쓰지 않았다. 채우지 못한 이유는 멱등성이다:
-- 태깅은 재시도·본문 보충·undelete로 **여러 번 돌 수 있고**, 그때마다 df를 올리면 통계가
-- 문서 수가 아니라 태깅 횟수를 세게 된다.
--
-- link_terms가 그 문제를 푼다. "이 링크가 df에 무엇을 기여했는가"를 기록해 두면 재태깅 시
-- 정확히 그만큼 되돌릴 수 있다. 즉 link_terms는 원장(ledger)이고 corpus_df는 그 합계다.
CREATE TABLE link_terms (
  link_id INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  term    TEXT NOT NULL,
  PRIMARY KEY (link_id, term)
) WITHOUT ROWID;
