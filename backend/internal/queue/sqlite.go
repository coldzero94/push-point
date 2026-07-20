package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// SQLite는 jobs 테이블 기반 Queue 구현체. store와 같은 writer *sql.DB를 공유한다
// (MaxOpenConns=1 — 쓰기 직렬화는 커넥션 풀 수준에서 보장됨).
type SQLite struct {
	writer *sql.DB
	notify chan struct{} // 버퍼 1 — Wake는 논블로킹 send
}

// 컴파일 타임 인터페이스 검증 (.claude/rules/backend.md 인터페이스 계약).
var (
	_ Queue         = (*SQLite)(nil)
	_ DispatchQueue = (*SQLite)(nil)
)

// NewSQLite는 writer 커넥션을 받아 큐를 만든다.
func NewSQLite(writer *sql.DB) *SQLite {
	return &SQLite{
		writer: writer,
		notify: make(chan struct{}, 1),
	}
}

// EnqueueTx는 호출자의 트랜잭션 안에서 잡을 삽입한다. 커밋·Wake는 호출자 책임.
func (q *SQLite) EnqueueTx(tx *sql.Tx, kind Kind, linkID int64) error {
	if _, err := tx.Exec(
		`INSERT INTO jobs (kind, link_id) VALUES (?, ?)`, string(kind), linkID,
	); err != nil {
		return fmt.Errorf("queue: enqueue(kind=%s, link=%d) 실패: %w", kind, linkID, err)
	}
	return nil
}

// claimColumns는 claim RETURNING 절과 scanJob의 순서를 맞추는 컬럼 목록.
const claimColumns = `id, kind, link_id, status, attempts, max_attempts, run_after, error, claimed_at, finished_at, created_at`

// Claim은 kind 무관하게 pending 잡 하나를 원자적으로 집는다. 없으면 (nil, nil).
func (q *SQLite) Claim(ctx context.Context) (*Job, error) {
	return q.claim(ctx, nil)
}

// ClaimKinds는 주어진 kind들만 대상으로 claim한다. dispatcher가 핸들러 등록된
// kind만 집도록 사용 — 미등록 kind는 pending에 남아 M2에서 자연 활성화된다.
// kinds가 비어 있으면 아무것도 집지 않는다 (nil, nil).
func (q *SQLite) ClaimKinds(ctx context.Context, kinds []Kind) (*Job, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	return q.claim(ctx, kinds)
}

// claim은 스펙 §4의 원자적 UPDATE ... WHERE id=(SELECT ... LIMIT 1) RETURNING.
// writer 커넥션(직렬)에서 실행되므로 동시 claim이 같은 잡을 집을 수 없다.
func (q *SQLite) claim(ctx context.Context, kinds []Kind) (*Job, error) {
	kindCond := ""
	args := []any{}
	if kinds != nil {
		kindCond = " AND kind IN (?" + strings.Repeat(",?", len(kinds)-1) + ")"
		for _, k := range kinds {
			args = append(args, string(k))
		}
	}
	//nolint:gosec // kindCond는 플레이스홀더만 조립 — 값은 전부 바인딩.
	query := `
		UPDATE jobs SET status='running', claimed_at=unixepoch(), attempts=attempts+1
		WHERE id = (
			SELECT id FROM jobs
			WHERE status='pending' AND run_after <= unixepoch()` + kindCond + `
			ORDER BY id LIMIT 1
		)
		RETURNING ` + claimColumns
	job, err := scanJob(q.writer.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // 집을 잡 없음 — 정상
	}
	if err != nil {
		return nil, fmt.Errorf("queue: claim 실패: %w", err)
	}
	return job, nil
}

// Complete는 잡을 done으로 전이시킨다.
func (q *SQLite) Complete(ctx context.Context, id int64) error {
	res, err := q.writer.ExecContext(ctx,
		`UPDATE jobs SET status='done', finished_at=unixepoch() WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("queue: complete(job=%d) 실패: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("queue: complete(job=%d) RowsAffected 실패: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("queue: complete(job=%d) — 잡 없음", id)
	}
	return nil
}

// Fail은 실패를 기록한다. attempts < max_attempts면 pending 복귀 + 선형 백오프
// (run_after = unixepoch() + 30*attempts), 초과면 failed 확정.
// kind='thumb'이 아닌 잡의 확정 실패는 links.status='failed' + error도 기록한다.
func (q *SQLite) Fail(ctx context.Context, id int64, jobErr error) error {
	msg := ""
	if jobErr != nil {
		msg = jobErr.Error()
	}
	tx, err := q.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("queue: fail(job=%d) 트랜잭션 시작 실패: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck // 커밋 후 no-op

	var (
		kind                  string
		linkID                int64
		attempts, maxAttempts int
	)
	err = tx.QueryRowContext(ctx,
		`SELECT kind, link_id, attempts, max_attempts FROM jobs WHERE id=?`, id,
	).Scan(&kind, &linkID, &attempts, &maxAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("queue: fail(job=%d) — 잡 없음", id)
	}
	if err != nil {
		return fmt.Errorf("queue: fail(job=%d) 조회 실패: %w", id, err)
	}

	if attempts < maxAttempts {
		// 재시도 여지 — pending 복귀 + 선형 백오프.
		_, err = tx.ExecContext(ctx,
			`UPDATE jobs SET status='pending', run_after=unixepoch()+30*attempts, error=? WHERE id=?`,
			msg, id)
		if err != nil {
			return fmt.Errorf("queue: fail(job=%d) 재시도 스케줄 실패: %w", id, err)
		}
	} else {
		// 재시도 소진 — 잡 확정 실패.
		_, err = tx.ExecContext(ctx,
			`UPDATE jobs SET status='failed', finished_at=unixepoch(), error=? WHERE id=?`,
			msg, id)
		if err != nil {
			return fmt.Errorf("queue: fail(job=%d) 확정 실패 기록 실패: %w", id, err)
		}
		// thumb은 best-effort — 링크 상태를 건드리지 않는다.
		if Kind(kind) != KindThumb {
			_, err = tx.ExecContext(ctx,
				`UPDATE links SET status='failed', error=?, updated_at=unixepoch() WHERE id=?`,
				msg, linkID)
			if err != nil {
				return fmt.Errorf("queue: fail(job=%d) 링크 %d 상태 기록 실패: %w", id, linkID, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("queue: fail(job=%d) 커밋 실패: %w", id, err)
	}
	return nil
}

// RecoverStale은 크래시로 남은 running 잡을 전부 pending으로 복구한다 (시작 시 1회).
func (q *SQLite) RecoverStale(ctx context.Context) (int64, error) {
	res, err := q.writer.ExecContext(ctx,
		`UPDATE jobs SET status='pending', claimed_at=NULL WHERE status='running'`)
	if err != nil {
		return 0, fmt.Errorf("queue: stale 잡 복구 실패: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("queue: stale 잡 복구 수 확인 실패: %w", err)
	}
	return n, nil
}

// Release는 셧다운 취소로 중단된 running 잡을 attempts 소진 없이 pending으로
// 되돌린다 (claim이 올린 attempts를 되감음). RecoverStale과 같은
// "셧다운은 실패가 아님" 의미론 — dispatcher가 graceful shutdown 시 호출한다.
func (q *SQLite) Release(ctx context.Context, id int64) error {
	res, err := q.writer.ExecContext(ctx, `
		UPDATE jobs SET status='pending', attempts=attempts-1, claimed_at=NULL
		WHERE id=? AND status='running'`, id)
	if err != nil {
		return fmt.Errorf("queue: release(job=%d) 실패: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("queue: release(job=%d) RowsAffected 실패: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("queue: release(job=%d) — running 잡 없음", id)
	}
	return nil
}

// Wake는 dispatcher를 깨운다. 채널이 이미 차 있으면 드롭 (곧 깨어날 예정이므로 충분).
func (q *SQLite) Wake() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Notify는 dispatcher가 대기하는 알림 채널을 반환한다.
func (q *SQLite) Notify() <-chan struct{} {
	return q.notify
}

// scanJob은 claimColumns 순서로 한 행을 Job으로 읽는다.
func scanJob(row *sql.Row) (*Job, error) {
	var (
		j                     Job
		kind, status          string
		claimedAt, finishedAt sql.NullInt64
	)
	err := row.Scan(&j.ID, &kind, &j.LinkID, &status, &j.Attempts, &j.MaxAttempts,
		&j.RunAfter, &j.Error, &claimedAt, &finishedAt, &j.CreatedAt)
	if err != nil {
		return nil, err
	}
	j.Kind = Kind(kind)
	j.Status = Status(status)
	if claimedAt.Valid {
		j.ClaimedAt = &claimedAt.Int64
	}
	if finishedAt.Valid {
		j.FinishedAt = &finishedAt.Int64
	}
	return &j, nil
}
