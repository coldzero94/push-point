package store

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoResurface는 되살릴 링크가 없다는 신호. 오류가 아니라 상태다 — 아카이브가 작거나
// 다 읽은 사용자에게는 이게 정상이고, 호출부는 204로 옮긴다.
var ErrNoResurface = errors.New("store: 되살릴 링크가 없습니다")

// resurfaceMinAge는 저장 후 이만큼 지나야 "잊었다"고 본다(7일).
//
// 어제 저장한 것을 오늘 되살리면 그건 되살리기가 아니라 목록을 두 번 보여주는 것이다.
// 일주일은 "읽으려고 저장했는데 안 읽은" 상태가 확정되는 지점 — 그 안쪽은 아직 목록
// 상단에 있어서 사용자가 스스로 볼 수 있다.
const resurfaceMinAge = 7 * 24 * 60 * 60

// Resurfaced는 잊고 있던 링크 하나를 고른다. 후보가 없으면 ErrNoResurface.
//
// **하루 동안 같은 하나여야 한다.** 새로고침마다 바뀌면 추천이 아니라 슬롯머신이고,
// 오늘 넘긴 것이 내일 다시 오지 않으면 되살릴 이유가 없다. 그래서 무작위가 아니라
// **날짜로 결정되는 순서**를 쓴다: 링크 id와 그날의 일련번호를 섞은 값으로 정렬한다.
// 같은 날에는 같은 답, 다음 날에는 다른 답이 나오고, 상태를 새로 만들지 않는다.
//
// 후보 집합은 아직 한 번도 열지 않은 것으로 한정한다(`opened_at IS NULL`). 열어 본 것은
// 잊은 것이 아니라 본 것이고, 그때 후보에서 자연히 빠지므로 "봤음" 상태를 따로 둘 필요가
// 없다 — 되살리기 전용 열이 하나도 늘지 않는 이유다.
func (s *sqliteStore) Resurfaced(ctx context.Context, now int64) (Link, error) {
	day := now / 86400
	rows, err := s.db.Reader.QueryContext(ctx, `
		SELECT `+linkCols+`
		FROM links l
		WHERE l.deleted_at IS NULL
		  AND l.opened_at IS NULL
		  AND l.created_at <= ?
		ORDER BY ((l.id * 2654435761) % 4294967296 + ?) % 4294967296, l.id
		LIMIT 1`,
		now-resurfaceMinAge, day)
	if err != nil {
		return Link{}, fmt.Errorf("store: 되살리기 조회 실패: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Link{}, fmt.Errorf("store: 되살리기 조회 실패: %w", err)
		}
		return Link{}, ErrNoResurface
	}
	l, err := scanLink(rows)
	if err != nil {
		return Link{}, fmt.Errorf("store: 되살리기 스캔 실패: %w", err)
	}
	return l, nil
}
