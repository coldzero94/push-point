// Package queue는 SQLite jobs 테이블 기반의 내구성 있는 인프로세스 잡 큐 계약을 정의한다.
// (v1 Redis Streams 대체 — 스펙 docs/v2/05 §4, 04-DATA-FLOW 참조)
//
// 동작 계약 (구현체가 반드시 지킬 것):
//   - enqueue는 항상 호출자의 트랜잭션 안에서 일어난다 (EnqueueTx). 링크 저장과
//     잡 삽입이 한 트랜잭션 — 링크만 있고 잡이 없는 고아 상태가 불가능.
//   - 커밋은 호출자 책임이며, 커밋 성공 후 호출자가 Wake()로 dispatcher를 깨운다.
//   - Claim은 원자적 UPDATE ... WHERE id = (SELECT ... LIMIT 1) RETURNING 패턴.
//     집을 잡이 없으면 (nil, nil).
//   - dispatcher는 Notify() 채널 수신 + 1초 폴링 티커를 병행한다 (run_after 도래 감지).
package queue

import (
	"context"
	"database/sql"
)

// Kind는 잡 종류. jobs.kind CHECK 제약과 일치.
type Kind string

const (
	KindScrape Kind = "scrape"
	KindTag    Kind = "tag"
	KindThumb  Kind = "thumb"
)

// Status는 잡 상태. jobs.status CHECK 제약과 일치.
type Status string

const (
	StatusPending Status = "pending"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

// Job은 jobs 테이블 한 행. 시각은 unix epoch 초.
type Job struct {
	ID          int64
	Kind        Kind
	LinkID      int64
	Status      Status
	Attempts    int
	MaxAttempts int
	RunAfter    int64
	Error       string
	ClaimedAt   *int64 // 미클레임이면 nil
	FinishedAt  *int64 // 미완료면 nil
	CreatedAt   int64
}

// Queue는 잡 큐 인터페이스. sqlite 구현체는 store와 같은 writer *sql.DB를 공유한다.
type Queue interface {
	// EnqueueTx는 호출자의 트랜잭션 tx 안에서 jobs(kind, link_id)를 INSERT한다.
	// 커밋하지 않는다 — 커밋과 커밋 후 Wake() 호출은 호출자(store) 책임.
	EnqueueTx(tx *sql.Tx, kind Kind, linkID int64) error

	// Claim은 pending이고 run_after가 도래한 잡 하나를 원자적으로 running으로
	// 전이시키고(attempts+1, claimed_at 기록) 반환한다. 없으면 (nil, nil).
	Claim(ctx context.Context) (*Job, error)

	// Complete는 잡을 done으로 전이시키고 finished_at을 기록한다.
	Complete(ctx context.Context, id int64) error

	// Fail은 잡 실패를 기록한다.
	//   - attempts < max_attempts: status='pending'으로 되돌리고
	//     run_after = unixepoch() + 30*attempts (선형 백오프).
	//   - attempts >= max_attempts: status='failed' + finished_at 기록,
	//     그리고 같은 트랜잭션에서 links.status='failed' + links.error에 사유 기록.
	//     (단 kind='thumb'은 best-effort — 링크 상태를 건드리지 않는다.)
	Fail(ctx context.Context, id int64, jobErr error) error

	// RecoverStale은 프로세스 시작 시 running → pending 일괄 복구를 수행하고
	// 복구한 잡 수를 반환한다 (kill -9 크래시 복구).
	RecoverStale(ctx context.Context) (int64, error)

	// Wake는 dispatcher를 깨운다 (Notify 채널로 논블로킹 send — 가득 차 있으면 드롭).
	// enqueue 트랜잭션 커밋 성공 직후 호출한다.
	Wake()

	// Notify는 dispatcher가 수신 대기하는 알림 채널을 반환한다 (버퍼 1 권장).
	Notify() <-chan struct{}
}
