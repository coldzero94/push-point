package queue

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// waitJobStatus는 잡 status가 want가 될 때까지 폴링한다 (최대 3s).
func waitJobStatus(t *testing.T, db *sql.DB, jobID int64, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var got string
		if err := db.QueryRow(`SELECT status FROM jobs WHERE id=?`, jobID).Scan(&got); err != nil {
			t.Fatalf("잡 조회 실패: %v", err)
		}
		if got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("잡 %d가 %s 상태가 되지 않음 (3s 초과)", jobID, want)
}

// startDispatcher는 dispatcher를 백그라운드로 실행하고 종료 함수를 반환한다.
func startDispatcher(t *testing.T, d *Dispatcher) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	return func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run 종료 에러: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Run이 ctx 취소 후 3s 내에 종료되지 않음 (graceful 종료 실패)")
		}
	}
}

func TestDispatcherProcessesRegisteredKind(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	d := NewDispatcher(q, testLogger())

	handled := make(chan int64, 1)
	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		handled <- job.LinkID
		return nil
	})
	stop := startDispatcher(t, d)
	defer stop()

	linkID := insertLink(t, db, "https://h.example")
	enqueue(t, db, q, KindScrape, linkID)
	q.Wake() // enqueue 커밋 후 notify — 티커(1s) 없이 즉시 처리돼야 한다

	select {
	case got := <-handled:
		if got != linkID {
			t.Fatalf("핸들러 link_id = %d, want %d", got, linkID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("핸들러가 호출되지 않음")
	}
	waitJobStatus(t, db, 1, "done")
}

func TestDispatcherLeavesUnregisteredKindPending(t *testing.T) {
	// M1 핵심 계약: 핸들러 미등록 kind(scrape/tag/thumb 전부)는 claim되지 않고
	// pending에 남는다 — M2에서 핸들러 등록 시 자연 활성화.
	db := newTestDB(t)
	q := NewSQLite(db)
	d := NewDispatcher(q, testLogger())

	d.Register(KindTag, func(ctx context.Context, job *Job) error { return nil })
	stop := startDispatcher(t, d)
	defer stop()

	linkID := insertLink(t, db, "https://i.example")
	enqueue(t, db, q, KindScrape, linkID) // scrape 핸들러 없음
	enqueue(t, db, q, KindTag, linkID)
	q.Wake()

	waitJobStatus(t, db, 2, "done") // tag 잡은 처리됨
	// 티커 몇 회가 지나도 scrape 잡은 그대로 pending.
	time.Sleep(1500 * time.Millisecond)
	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status, attempts FROM jobs WHERE kind='scrape'`).
		Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempts != 0 {
		t.Fatalf("미등록 kind 잡 = (%s, attempts=%d), want (pending, 0)", status, attempts)
	}
}

func TestDispatcherFailsJobOnHandlerError(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	d := NewDispatcher(q, testLogger())

	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		return fmt.Errorf("스크랩 실패")
	})
	stop := startDispatcher(t, d)
	defer stop()

	linkID := insertLink(t, db, "https://j.example")
	enqueue(t, db, q, KindScrape, linkID)
	q.Wake()

	// 1차 시도 실패 → pending 복귀 + 백오프 (dispatcher가 재claim하지 못하는 미래 시각).
	waitJobStatus(t, db, 1, "pending")
	r := readJob(t, db, 1)
	if r.attempts != 1 || r.errMsg != "스크랩 실패" {
		t.Fatalf("Fail 기록 불일치: %+v", r)
	}
	var now int64
	if err := db.QueryRow(`SELECT unixepoch()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	if r.runAfter <= now {
		t.Fatalf("백오프 미적용: run_after=%d, now=%d", r.runAfter, now)
	}
}

func TestDispatcherRecoversStaleOnStart(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)

	// 크래시 시뮬레이션: running으로 남은 잡.
	linkID := insertLink(t, db, "https://k.example")
	enqueue(t, db, q, KindScrape, linkID)
	if job, err := q.Claim(context.Background()); err != nil || job == nil {
		t.Fatalf("사전 claim 실패: (%v, %v)", job, err)
	}

	d := NewDispatcher(q, testLogger())
	handled := make(chan struct{}, 1)
	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		handled <- struct{}{}
		return nil
	})
	stop := startDispatcher(t, d)
	defer stop()

	// Run 시작 시 RecoverStale → 첫 drain에서 재claim되어 처리된다.
	select {
	case <-handled:
	case <-time.After(3 * time.Second):
		t.Fatal("복구된 잡이 처리되지 않음")
	}
	waitJobStatus(t, db, 1, "done")
}

func TestDispatcherGracefulShutdownWaitsInflight(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	d := NewDispatcher(q, testLogger())

	started := make(chan struct{})
	release := make(chan struct{})
	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		close(started)
		<-release // 셧다운이 진행 중 핸들러를 기다리는지 확인
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	linkID := insertLink(t, db, "https://l.example")
	enqueue(t, db, q, KindScrape, linkID)
	q.Wake()
	<-started

	cancel()
	select {
	case <-done:
		t.Fatal("진행 중 핸들러를 기다리지 않고 Run이 종료됨")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 종료 에러: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("핸들러 종료 후에도 Run이 끝나지 않음")
	}
	// 취소 후에도 결과 기록(Complete)은 유실되지 않아야 한다 (WithoutCancel).
	waitJobStatus(t, db, 1, "done")
}

func TestDispatcherShutdownReleasesCanceledJob(t *testing.T) {
	// F3: 셧다운 취소로 핸들러가 context.Canceled를 반환하면 Fail(attempts 소진)이
	// 아니라 pending 복귀 — 재시작 몇 번만으로 정상 잡이 영구 실패가 되면 안 된다.
	db := newTestDB(t)
	q := NewSQLite(db)
	d := NewDispatcher(q, testLogger())

	started := make(chan struct{})
	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		close(started)
		<-ctx.Done() // 느린 잡 — 셧다운 취소를 기다린다
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	linkID := insertLink(t, db, "https://m.example")
	enqueue(t, db, q, KindScrape, linkID)
	q.Wake()
	<-started

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 종료 에러: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("셧다운이 끝나지 않음")
	}

	// pending 복귀 + attempts 되감김 (0) — Fail이 아니라 Release 경로.
	r := readJob(t, db, 1)
	if r.status != "pending" || r.attempts != 0 {
		t.Fatalf("셧다운 후 잡 = (%s, attempts=%d), want (pending, 0)", r.status, r.attempts)
	}
	var linkStatus string
	if err := db.QueryRow(`SELECT status FROM links WHERE id=?`, linkID).Scan(&linkStatus); err != nil {
		t.Fatal(err)
	}
	if linkStatus != "pending" {
		t.Fatalf("셧다운 후 링크 status = %s, want pending (실패 아님)", linkStatus)
	}
}

// failCompleteQueue는 Complete만 항상 실패시키는 큐 래퍼 — 나머지 능력은 *SQLite에
// 위임한다. 결과 기록이 재시도 후에도 실패할 때의 fail-stop 경로 검증용.
type failCompleteQueue struct {
	*SQLite
}

func (q *failCompleteQueue) Complete(context.Context, int64) error {
	return fmt.Errorf("주입된 Complete 실패")
}

func TestDispatcherFailStopsWhenRecordFails(t *testing.T) {
	// H1/M1: 핸들러는 성공했지만 Complete 기록이 재시도(100/300/900ms) 후에도 실패하면
	// dispatcher는 잡을 running에 고착시키는 대신 Run에서 에러를 반환해야 한다 —
	// 호출측(main)이 이를 감지해 fail-stop하고, 재시작의 RecoverStale이 복구한다.
	db := newTestDB(t)
	q := &failCompleteQueue{NewSQLite(db)}
	d := NewDispatcher(q, testLogger())

	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		return nil // 핸들러 성공 → Complete 경로 진입 → 주입된 기록 실패
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	linkID := insertLink(t, db, "https://o.example")
	enqueue(t, db, q.SQLite, KindScrape, linkID)
	q.Wake()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Complete 영구 실패에도 Run이 nil 반환 — fail-stop 안 됨")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run이 에러를 반환하지 않음 (재시도 소진 후 fail-stop 기대)")
	}
}

func TestDispatcherFailsOnHandlerTimeoutWithoutShutdown(t *testing.T) {
	// 셧다운이 아닌 핸들러 자체 타임아웃(DeadlineExceeded)은 여전히 Fail 경로다.
	db := newTestDB(t)
	q := NewSQLite(db)
	d := NewDispatcher(q, testLogger())

	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		return context.DeadlineExceeded // 핸들러 내부 타임아웃 — dispatcher ctx는 살아 있음
	})
	stop := startDispatcher(t, d)
	defer stop()

	linkID := insertLink(t, db, "https://n.example")
	enqueue(t, db, q, KindScrape, linkID)
	q.Wake()

	// Fail 경로 확인: attempts가 1로 오른 채 pending 복귀 (초기 pending과 구분).
	deadline := time.Now().Add(3 * time.Second)
	for {
		r := readJob(t, db, 1)
		if r.status == "pending" && r.attempts == 1 {
			if r.errMsg != context.DeadlineExceeded.Error() {
				t.Fatalf("Fail 기록 error = %q, want %q", r.errMsg, context.DeadlineExceeded.Error())
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("타임아웃 Fail이 기록되지 않음: %+v", r)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
