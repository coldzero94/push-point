-- 열람 신호 — 코어 루프 5단계 중 마지막(재열람)만 계측이 0이었다.
--
-- 컬럼 하나뿐이다. `open_count`는 두지 않는다 — 이건 지표가 아니라 **링크별 사실**이고,
-- 횟수를 세는 순간 비율·집계로 쓰고 싶어지는데 이 신호는 푸시포인트를 통과한 열람만
-- 잡으므로(브라우저 히스토리·원본 앱 직접 열기는 0으로 남는다) 구조적으로 과소집계다.
-- 그 숫자를 믿고 "난 안 읽는다"고 결론 내리면 틀린 결론이 된다.
--
-- `updated_at`도 건드리지 않는다. 열람이 그걸 올리면 목록 정렬과 인스펙터의
-- "수정됨" 의미가 함께 흔들린다.
ALTER TABLE links ADD COLUMN opened_at INTEGER;

-- "안 연 것" 필터용 부분 인덱스. keyset 커서가 그대로 탄다.
CREATE INDEX idx_links_unopened ON links(created_at DESC, id DESC)
  WHERE deleted_at IS NULL AND opened_at IS NULL;
