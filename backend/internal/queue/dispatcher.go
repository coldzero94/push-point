package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"
)

// Handler는 kind별 잡 처리 함수. 에러를 반환하면 dispatcher가 Fail을 기록한다
// (재시도/확정 실패 판정은 큐 몫).
type Handler func(ctx context.Context, job *Job) error

// DispatchQueue는 dispatcher가 필요로 하는 큐 능력. *SQLite가 만족한다.
type DispatchQueue interface {
	Queue
	// ClaimKinds는 주어진 kind만 대상으로 claim한다 (비어 있으면 nil, nil).
	ClaimKinds(ctx context.Context, kinds []Kind) (*Job, error)
	// Release는 셧다운 취소로 중단된 running 잡을 attempts 소진 없이 pending으로
	// 되돌린다 — "셧다운은 실패가 아님" (RecoverStale과 동일 의미론).
	Release(ctx context.Context, id int64) error
}

// Dispatcher는 notify 채널 + 1초 폴링 티커를 병행하며 Claim 루프를 돌리고,
// 집은 잡을 kind별 핸들러에 전달한다.
//
// M1에는 등록된 핸들러가 없을 수 있다 — 미등록 kind는 SELECT 대상에서 제외되어
// pending에 그대로 남고, M2에서 스크래퍼 핸들러가 등록되면 자연히 소비된다.
type Dispatcher struct {
	q        DispatchQueue
	log      *slog.Logger
	handlers map[Kind]Handler
	kinds    []Kind // 등록된 kind 목록 (claim 필터)
}

// NewDispatcher는 dispatcher를 만든다. Register는 Run 호출 전에 끝내야 한다.
func NewDispatcher(q DispatchQueue, log *slog.Logger) *Dispatcher {
	return &Dispatcher{
		q:        q,
		log:      log,
		handlers: make(map[Kind]Handler),
	}
}

// Register는 kind 핸들러를 등록한다. Run 이후 호출은 지원하지 않는다 (동시성 없음).
func (d *Dispatcher) Register(kind Kind, h Handler) {
	if _, dup := d.handlers[kind]; dup {
		d.log.Warn("queue: 핸들러 중복 등록 — 덮어씀", "kind", kind)
	} else {
		d.kinds = append(d.kinds, kind)
	}
	d.handlers[kind] = h
}

// Run은 시작 시 stale 잡을 복구한 뒤 claim 루프를 돈다. ctx 취소 시
// 진행 중인 핸들러가 끝날 때까지 기다렸다가 nil을 반환한다 (graceful).
func (d *Dispatcher) Run(ctx context.Context) error {
	// 크래시 복구: running → pending (kill -9 후에도 미처리 잡 재개).
	if n, err := d.q.RecoverStale(ctx); err != nil {
		return err
	} else if n > 0 {
		d.log.Info("queue: stale 잡 복구", "count", n)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			d.drain(gctx, g)
			select {
			case <-gctx.Done():
				return nil // 취소는 정상 종료
			case <-d.q.Notify(): // enqueue 커밋 직후 Wake
			case <-ticker.C: // run_after 도래 감지 (백오프 재시도)
			}
		}
	})
	return g.Wait()
}

// drain은 집을 잡이 없어질 때까지 claim하고, 잡마다 goroutine으로 핸들러를 실행한다.
// 동시성 상한은 핸들러 내부(semaphore 등)가 책임진다.
func (d *Dispatcher) drain(ctx context.Context, g *errgroup.Group) {
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := d.q.ClaimKinds(ctx, d.kinds)
		if err != nil {
			// 일시 오류(SQLITE_BUSY 등)일 수 있음 — 죽지 않고 다음 틱에 재시도.
			d.log.Error("queue: claim 실패", "err", err)
			return
		}
		if job == nil {
			return // 등록된 kind의 pending 잡 없음
		}
		g.Go(func() error {
			// 핸들러 실패는 잡 단위 Fail로 흡수되지만, 결과 기록이 재시도 후에도
			// 실패하면 process가 에러를 반환한다 — errgroup을 통해 Run이 fail-stop된다.
			return d.process(ctx, job)
		})
	}
}

// recordBackoffs는 Fail/Complete/Release 결과 기록이 실패했을 때의 재시도 간격이다.
// 최초 호출 실패 후 이 간격들만큼(총 3회) 재시도한다.
var recordBackoffs = []time.Duration{
	100 * time.Millisecond,
	300 * time.Millisecond,
	900 * time.Millisecond,
}

// recordWithRetry는 결과 기록 연산 fn을 최초 1회 실행하고, 실패하면 recordBackoffs
// 간격으로 최대 3회 재시도한다. 모두 실패하면 마지막 에러를 반환한다. recCtx는
// WithoutCancel이라 셧다운 중에도 기록이 취소되지 않는다 (재시도 대기만큼만 지연).
func (d *Dispatcher) recordWithRetry(recCtx context.Context, op string, jobID int64, fn func(context.Context) error) error {
	err := fn(recCtx)
	for _, backoff := range recordBackoffs {
		if err == nil {
			return nil
		}
		d.log.Warn("queue: 결과 기록 재시도", "op", op, "job", jobID, "backoff", backoff.String(), "err", err)
		time.Sleep(backoff)
		err = fn(recCtx)
	}
	return err
}

// process는 핸들러 실행 후 결과를 기록한다. 셧다운 중에도 결과 기록은 유실되면
// 안 되므로 context.WithoutCancel로 DB 기록을 보장한다. 기록(Fail/Complete/Release)이
// 재시도 후에도 실패하면 에러를 반환한다 — errgroup을 통해 Run이 fail-stop되고,
// 잡이 running에 고착되는 대신 재시작의 RecoverStale이 복구 경로가 된다.
func (d *Dispatcher) process(ctx context.Context, job *Job) error {
	recCtx := context.WithoutCancel(ctx)
	if err := d.handlers[job.Kind](ctx, job); err != nil {
		// 셧다운 취소로 중단된 잡은 실패가 아니다 — attempts 소진 없이 pending 복귀
		// (Fail로 기록하면 재시작 몇 번만으로 정상 잡이 영구 실패가 된다).
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			d.log.Info("queue: 셧다운으로 잡 중단 — pending 복귀", "job", job.ID, "kind", job.Kind)
			// Release 실패 시 RecoverStale은 attempts를 되감지 않는다(의도: 크래시
			// 루프가 attempts를 무한 리셋하는 것 방지). 셧다운 반복으로 attempts가
			// 소진되는 엣지는 수용 — attempts를 되감는 Release 정상 경로가 흔한 쪽이다.
			if rerr := d.recordWithRetry(recCtx, "release", job.ID, func(c context.Context) error {
				return d.q.Release(c, job.ID)
			}); rerr != nil {
				d.log.Error("queue: 잡 복귀 실패 — fail-stop", "job", job.ID, "err", rerr)
				return fmt.Errorf("queue: release(job=%d) 재시도 소진: %w", job.ID, rerr)
			}
			return nil
		}
		d.log.Warn("queue: 잡 실패", "job", job.ID, "kind", job.Kind,
			"link", job.LinkID, "attempts", job.Attempts, "err", err)
		if ferr := d.recordWithRetry(recCtx, "fail", job.ID, func(c context.Context) error {
			return d.q.Fail(c, job.ID, err)
		}); ferr != nil {
			d.log.Error("queue: 실패 기록 실패 — fail-stop", "job", job.ID, "err", ferr)
			return fmt.Errorf("queue: fail(job=%d) 재시도 소진: %w", job.ID, ferr)
		}
		return nil
	}
	if err := d.recordWithRetry(recCtx, "complete", job.ID, func(c context.Context) error {
		return d.q.Complete(c, job.ID)
	}); err != nil {
		d.log.Error("queue: 완료 기록 실패 — fail-stop", "job", job.ID, "err", err)
		return fmt.Errorf("queue: complete(job=%d) 재시도 소진: %w", job.ID, err)
	}
	return nil
}
