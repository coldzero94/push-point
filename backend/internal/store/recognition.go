package store

import (
	"context"
	"fmt"
)

// Rung은 알아봄의 단계. 정수로 저장한다(마이그레이션 0013 주석 참조).
const (
	// RungDuplicate — 같은 링크를 다시 저장했다. 그때의 날짜와 메모를 돌려준다.
	RungDuplicate = 0
	// RungDomain — 같은 곳에서 여러 번 저장했다. 사전도 코퍼스도 안 쓴다.
	RungDomain = 1
)

// domainEncounterMin은 도메인 재조우를 말할 최소 횟수.
//
// 3이다. 2는 우연이고 — 같은 블로그를 두 번 저장하는 것은 아무것도 뜻하지 않는다 —
// 3부터 습관이다. 이 상수가 낮으면 배너가 거의 매번 무언가를 말하게 되고, 그러면 사람이
// 세 번째 줄을 읽는 것을 그만둔다. 그건 이 제품이 가진 몇 안 되는 표면을 태우는 일이다.
const domainEncounterMin = 3

// DomainEncounter는 이 도메인에서 저장한 링크가 **이 링크를 포함해** 몇 번째인지 돌려준다.
// domainEncounterMin 미만이면 0 — "말할 것이 없다"는 뜻이고, 호출자는 그때 조용하다.
//
// **사전도 코퍼스도 안 쓴다.** 알아봄 사다리에서 이 단이 값을 하는 이유가 그것이다:
// 42개 태그가 변별을 못 하는 6건짜리 아카이브에서도 "이 블로그 다섯 번째"는 참이다.
//
// 삭제된 링크는 세지 않는다. 지운 것은 없던 일로 하는 것이 이 앱의 다른 곳과 같은 규칙이다.
func (s *sqliteStore) DomainEncounter(ctx context.Context, linkID int64) (string, int, error) {
	var domain string
	if err := s.db.Reader.QueryRowContext(ctx,
		`SELECT domain FROM links WHERE id = ?`, linkID).Scan(&domain); err != nil {
		return "", 0, fmt.Errorf("store: 도메인 조회 실패: %w", err)
	}
	if domain == "" {
		return "", 0, nil
	}
	var n int
	if err := s.db.Reader.QueryRowContext(ctx,
		`SELECT count(*) FROM links WHERE domain = ? AND deleted_at IS NULL`, domain).Scan(&n); err != nil {
		return "", 0, fmt.Errorf("store: 도메인 횟수 조회 실패: %w", err)
	}
	if n < domainEncounterMin {
		return domain, 0, nil
	}
	return domain, n, nil
}

// RecordRecognition은 알아봄을 **보여줬다**는 사실을 남긴다.
//
// 이 한 줄이 원장의 전부이자 요점이다 — 보여준 것은 기록되는데 눌린 것만 기록되면,
// "무시당했다"가 데이터에서 사라지고 그 자리를 취향이 채운다. 30년간 이 분야가 그랬다.
func (s *sqliteStore) RecordRecognition(ctx context.Context, linkID int64, rung int) error {
	if _, err := s.db.Writer.ExecContext(ctx,
		`INSERT INTO recognition_events (link_id, rung) VALUES (?, ?)`, linkID, rung); err != nil {
		return fmt.Errorf("store: 알아봄 기록 실패: %w", err)
	}
	return nil
}

// MarkRecognitionTapped는 그 링크의 **가장 최근** 미탭 알아봄에 탭을 기록한다.
//
// 알림 id가 아니라 링크 id로 찾는 이유: 알림 페이로드에 이미 링크 id가 실려 있고
// (`SaveNotifier.linkIDKey`), 이벤트 id를 왕복시키려면 알림 payload와 라우터 양쪽을
// 고쳐야 하는데 얻는 것은 같다. 같은 링크의 알아봄이 하루에 두 번 뜨는 일은 사실상 없다.
func (s *sqliteStore) MarkRecognitionTapped(ctx context.Context, linkID int64) error {
	if _, err := s.db.Writer.ExecContext(ctx, `
		UPDATE recognition_events SET tapped_at = unixepoch()
		WHERE id = (
			SELECT id FROM recognition_events
			WHERE link_id = ? AND tapped_at IS NULL
			ORDER BY shown_at DESC, id DESC LIMIT 1
		)`, linkID); err != nil {
		return fmt.Errorf("store: 알아봄 탭 기록 실패: %w", err)
	}
	return nil
}

// RecognitionStat은 단별 집계 한 줄.
type RecognitionStat struct {
	Rung   int
	Shown  int
	Tapped int
}

// RecognitionStats는 최근 days일의 단별 집계.
//
// **읽는 코드 없이 쌓기만 하는 원장은 죽은 자산이다.** `tag_feedback`이 그랬고, 이 표는
// 같은 변경에서 읽는 명령과 함께 들어온다(`just recognition`).
func (s *sqliteStore) RecognitionStats(ctx context.Context, days int) ([]RecognitionStat, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := s.db.Reader.QueryContext(ctx, `
		SELECT rung, count(*), count(tapped_at)
		FROM recognition_events
		WHERE shown_at >= unixepoch() - ? * 86400
		GROUP BY rung ORDER BY rung`, days)
	if err != nil {
		return nil, fmt.Errorf("store: 알아봄 집계 실패: %w", err)
	}
	defer rows.Close()
	var out []RecognitionStat
	for rows.Next() {
		var st RecognitionStat
		if err := rows.Scan(&st.Rung, &st.Shown, &st.Tapped); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
