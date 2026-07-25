package queue

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// waitJobStatus는 잡 status가 want가 될 때까지 폴링한다 (최대 3s).
// waitJobRetryScheduled는 "실패해서 pending으로 되돌아온" 상태를 기다린다. 초기 pending과
// 구분하려고 attempts와 error가 기록됐는지까지 본다.
func waitJobRetryScheduled(t *testing.T, db *sql.DB, jobID int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status, errMsg string
		var attempts int
		if err := db.QueryRow(
			`SELECT status, attempts, error FROM jobs WHERE id=?`, jobID,
		).Scan(&status, &attempts, &errMsg); err != nil {
			t.Fatalf("잡 조회 실패: %v", err)
		}
		if status == "pending" && attempts >= 1 && errMsg != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("잡 %d가 실패 후 pending으로 돌아오지 않음 (5s 초과)", jobID)
}

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
	d := NewDispatcher(q, testLogger(), 8)

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
	d := NewDispatcher(q, testLogger(), 8)

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
	d := NewDispatcher(q, testLogger(), 8)

	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		return fmt.Errorf("스크랩 실패")
	})
	stop := startDispatcher(t, d)
	defer stop()

	linkID := insertLink(t, db, "https://j.example")
	enqueue(t, db, q, KindScrape, linkID)
	q.Wake()

	// 1차 시도 실패 → pending 복귀 + 백오프 (dispatcher가 재claim하지 못하는 미래 시각).
	// **초기 pending과 구분해서 기다려야 한다** — enqueue 직후에도 status는 pending이라
	// 상태만 보고 기다리면 dispatcher가 claim하기도 전에 통과해, 아직 비어 있는 실패 기록을
	// 읽는다(느린 CI에서만 드러나던 경합).
	waitJobRetryScheduled(t, db, 1)
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

	d := NewDispatcher(q, testLogger(), 8)
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
	d := NewDispatcher(q, testLogger(), 8)

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
	d := NewDispatcher(q, testLogger(), 8)

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
	d := NewDispatcher(q, testLogger(), 8)

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
	d := NewDispatcher(q, testLogger(), 8)

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

func TestDispatcherBoundsInFlightClaims(t *testing.T) {
	// F2: maxInFlight보다 많은 잡을 enqueue해도 동시에 running으로 뒤집히는 잡 수는
	// maxInFlight 이하여야 한다 — claim이 워커 용량에 묶여 미실행 잡의 attempts가
	// 즉시 소진되지 않는다. 느린 핸들러로 잡을 붙잡아 두고 DB running 카운트를 관측한다.
	db := newTestDB(t)
	q := NewSQLite(db)
	const maxInFlight = 3
	d := NewDispatcher(q, testLogger(), maxInFlight)

	release := make(chan struct{})
	var running int64 // 현재 핸들러에 진입한 잡 수
	var peak int64    // 관측된 동시 최댓값
	d.Register(KindScrape, func(ctx context.Context, job *Job) error {
		cur := atomic.AddInt64(&running, 1)
		for {
			p := atomic.LoadInt64(&peak)
			if cur <= p || atomic.CompareAndSwapInt64(&peak, p, cur) {
				break
			}
		}
		<-release // 셧다운/해제 전까지 슬롯을 붙잡는다
		atomic.AddInt64(&running, -1)
		return nil
	})
	stop := startDispatcher(t, d)

	const nJobs = 12
	for i := 0; i < nJobs; i++ {
		linkID := insertLink(t, db, fmt.Sprintf("https://bound.example/%d", i))
		enqueue(t, db, q, KindScrape, linkID)
	}
	q.Wake()

	// 슬롯이 다 차서 안정될 시간을 준 뒤 DB running 카운트를 반복 관측한다.
	// (상한) 어느 순간에도 running 잡 수는 maxInFlight를 넘지 않는다.
	// (하한) 느린 핸들러로 포화시키면 정확히 maxInFlight개가 동시에 running이어야 한다 —
	// peak가 maxInFlight에 도달하면 관측을 마친다.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var dbRunning int
		if err := db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status='running'`).Scan(&dbRunning); err != nil {
			t.Fatalf("running 카운트 조회 실패: %v", err)
		}
		if dbRunning > maxInFlight {
			t.Fatalf("동시 running 잡 %d > maxInFlight %d (claim이 용량에 묶이지 않음)", dbRunning, maxInFlight)
		}
		if atomic.LoadInt64(&peak) == maxInFlight {
			break // 정확 포화 관측 — 더 기다릴 필요 없음
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 핸들러가 관측한 동시 진입 최댓값은 "정확히" maxInFlight여야 한다 — 상한(claim 폭주
	// 없음)뿐 아니라 하한도 단언한다. claim이 "Wake당 1건"으로 붕괴하면 peak가 maxInFlight에
	// 못 미쳐(예: 1) 이 단언이 회귀를 잡는다.
	if p := atomic.LoadInt64(&peak); p != maxInFlight {
		t.Fatalf("동시 핸들러 진입 최댓값 = %d, want %d (정확 포화 — 상·하한 동시 확인)", p, maxInFlight)
	}

	// 나머지 잡은 아직 pending으로 남아 attempts가 소진되지 않았어야 한다.
	var pendingZeroAttempts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status='pending' AND attempts=0`).
		Scan(&pendingZeroAttempts); err != nil {
		t.Fatalf("pending 카운트 조회 실패: %v", err)
	}
	if pendingZeroAttempts == 0 {
		t.Fatal("모든 잡이 claim됨 — claim이 용량에 묶이지 않았다 (attempts 소진 위험)")
	}

	close(release)
	stop()
}
