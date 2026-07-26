package store

// 열람 신호 — 코어 루프 5단계 중 마지막(재열람)만 계측이 0이었다.

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MarkOpened는 이 링크를 열었다는 사실만 기록한다.
//
// **횟수를 세지 않는다.** 이건 지표가 아니라 링크별 사실이다 — 이 신호는 푸시포인트를
// 통과한 열람만 잡으므로(브라우저 히스토리·원본 앱에서 바로 여는 경우는 0으로 남는다)
// 구조적으로 과소집계이고, 비율로 쓰면 "난 안 읽는다"는 틀린 결론을 만든다. 링크별
// 사실로만 쓰면 과소집계는 "안 연 것" 필터에 이미 읽은 게 조금 섞이는 노이즈로 끝난다.
//
// **`updated_at`을 건드리지 않는다.** 열람이 그걸 올리면 목록 정렬과 인스펙터의
// "수정됨"이 함께 흔들린다 — 연 적 있다는 사실이 수정은 아니다.
func (s *sqliteStore) MarkOpened(ctx context.Context, linkID int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE links SET opened_at = ? WHERE id = ? AND deleted_at IS NULL`,
			time.Now().Unix(), linkID)
		if err != nil {
			return fmt.Errorf("store: 열람 기록 실패: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("store: RowsAffected 실패: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}
