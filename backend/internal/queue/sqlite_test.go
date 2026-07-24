package queue

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/coby/push-point/backend/migrations"
)

// newTestDB는 임시 파일 SQLite를 열고 embed 마이그레이션 *.up.sql을 순서대로 적용한다.
// (store.Open을 쓰지 않는 이유: 패키지 간 의존 최소화 — 큐는 writer *sql.DB만 필요.)
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("DB 오픈 실패: %v", err)
	}
	db.SetMaxOpenConns(1) // writer 커넥션 전략과 동일
	t.Cleanup(func() { db.Close() })

	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("마이그레이션 목록 실패: %v", err)
	}
	var ups []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".sql" && len(e.Name()) > 7 && e.Name()[len(e.Name())-7:] == ".up.sql" {
			ups = append(ups, e.Name())
		}
	}
	sort.Strings(ups)
	for _, name := range ups {
		b, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("마이그레이션 읽기 실패 %s: %v", name, err)
		}
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("마이그레이션 적용 실패 %s: %v", name, err)
		}
	}
	return db
}

// insertLink는 FK용 링크 한 행을 넣고 id를 반환한다.
func insertLink(t *testing.T, db *sql.DB, url string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO links (url, url_hash) VALUES (?, ?)`, url, "hash-"+url)
	if err != nil {
		t.Fatalf("링크 삽입 실패: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// enqueue는 EnqueueTx를 트랜잭션으로 감싸 잡을 넣는다.
func enqueue(t *testing.T, db *sql.DB, q *SQLite, kind Kind, linkID int64) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("트랜잭션 시작 실패: %v", err)
	}
	if err := q.EnqueueTx(tx, kind, linkID); err != nil {
		t.Fatalf("enqueue 실패: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("커밋 실패: %v", err)
	}
}

// jobRow는 검증용 잡 상태 스냅샷.
type jobRow struct {
	status     string
	attempts   int
	runAfter   int64
	errMsg     string
	finishedAt sql.NullInt64
}

func readJob(t *testing.T, db *sql.DB, id int64) jobRow {
	t.Helper()
	var r jobRow
	err := db.QueryRow(
		`SELECT status, attempts, run_after, error, finished_at FROM jobs WHERE id=?`, id,
	).Scan(&r.status, &r.attempts, &r.runAfter, &r.errMsg, &r.finishedAt)
	if err != nil {
		t.Fatalf("잡 조회 실패: %v", err)
	}
	return r
}

func TestClaimAtomicity(t *testing.T) {
	// 잡 1개에 동시 claim 8개 — 정확히 1개만 성공해야 한다.
	db := newTestDB(t)
	q := NewSQLite(db)
	linkID := insertLink(t, db, "https://a.example")
	enqueue(t, db, q, KindScrape, linkID)

	const workers = 8
	var wg sync.WaitGroup
	got := make([]*Job, workers)
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], errs[i] = q.Claim(context.Background())
		}()
	}
	wg.Wait()

	claimed := 0
	for i := range workers {
		if errs[i] != nil {
			t.Fatalf("claim %d 에러: %v", i, errs[i])
		}
		if got[i] != nil {
			claimed++
			if got[i].Kind != KindScrape || got[i].LinkID != linkID || got[i].Attempts != 1 {
				t.Errorf("claim 결과 불일치: %+v", got[i])
			}
			if got[i].Status != StatusRunning || got[i].ClaimedAt == nil {
				t.Errorf("running 전이 누락: %+v", got[i])
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("동시 claim 성공 수 = %d, want 1", claimed)
	}
}

func TestClaimEmptyQueue(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	job, err := q.Claim(context.Background())
	if err != nil || job != nil {
		t.Fatalf("빈 큐 claim = (%v, %v), want (nil, nil)", job, err)
	}
}

func TestFailBackoffAndNoClaimBeforeRunAfter(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	ctx := context.Background()
	linkID := insertLink(t, db, "https://b.example")
	enqueue(t, db, q, KindScrape, linkID)

	job, err := q.Claim(ctx)
	if err != nil || job == nil {
		t.Fatalf("claim 실패: (%v, %v)", job, err)
	}
	if err := q.Fail(ctx, job.ID, fmt.Errorf("네트워크 오류")); err != nil {
		t.Fatalf("Fail 실패: %v", err)
	}

	r := readJob(t, db, job.ID)
	if r.status != "pending" || r.attempts != 1 || r.errMsg != "네트워크 오류" {
		t.Fatalf("pending 복귀 실패: %+v", r)
	}
	// 선형 백오프: run_after ≈ now + 30*attempts(=1)
	var now int64
	if err := db.QueryRow(`SELECT unixepoch()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	if diff := r.runAfter - now; diff < 25 || diff > 35 {
		t.Fatalf("run_after 백오프 = now+%d초, want ≈30", diff)
	}

	// run_after 도래 전 — claim되면 안 된다.
	if got, err := q.Claim(ctx); err != nil || got != nil {
		t.Fatalf("백오프 중 claim = (%v, %v), want (nil, nil)", got, err)
	}

	// run_after를 과거로 당기면 다시 claim된다 (attempts=2).
	if _, err := db.Exec(`UPDATE jobs SET run_after=unixepoch()-1 WHERE id=?`, job.ID); err != nil {
		t.Fatal(err)
	}
	got, err := q.Claim(ctx)
	if err != nil || got == nil {
		t.Fatalf("백오프 후 claim 실패: (%v, %v)", got, err)
	}
	if got.ID != job.ID || got.Attempts != 2 {
		t.Fatalf("재claim 결과 불일치: %+v", got)
	}
}

func TestFailMaxAttempts(t *testing.T) {
	// 테이블 주도: kind별로 확정 실패 시 링크 상태 반영 여부가 다르다.
	cases := []struct {
		name           string
		kind           Kind
		wantLinkFailed bool
	}{
		{"scrape 확정 실패는 링크도 failed", KindScrape, true},
		{"thumb은 best-effort — 링크 상태 불변", KindThumb, false},
		{"tag도 best-effort — 링크 상태 불변", KindTag, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			q := NewSQLite(db)
			ctx := context.Background()
			linkID := insertLink(t, db, "https://c.example/"+string(tc.kind))
			enqueue(t, db, q, tc.kind, linkID)

			// max_attempts(기본 3)만큼 claim→Fail 반복.
			var jobID int64
			for i := 1; i <= 3; i++ {
				if _, err := db.Exec(`UPDATE jobs SET run_after=unixepoch()-1`); err != nil {
					t.Fatal(err)
				}
				job, err := q.Claim(ctx)
				if err != nil || job == nil {
					t.Fatalf("시도 %d claim 실패: (%v, %v)", i, job, err)
				}
				jobID = job.ID
				if err := q.Fail(ctx, job.ID, fmt.Errorf("실패 %d", i)); err != nil {
					t.Fatalf("시도 %d Fail 실패: %v", i, err)
				}
			}

			r := readJob(t, db, jobID)
			if r.status != "failed" || r.attempts != 3 || !r.finishedAt.Valid {
				t.Fatalf("확정 실패 상태 불일치: %+v", r)
			}
			var linkStatus, linkErr string
			if err := db.QueryRow(`SELECT status, error FROM links WHERE id=?`, linkID).
				Scan(&linkStatus, &linkErr); err != nil {
				t.Fatal(err)
			}
			if tc.wantLinkFailed {
				if linkStatus != "failed" || linkErr != "실패 3" {
					t.Fatalf("링크 상태 = (%s, %q), want (failed, 실패 3)", linkStatus, linkErr)
				}
			} else if linkStatus != "pending" || linkErr != "" {
				t.Fatalf("thumb 실패가 링크를 건드림: (%s, %q)", linkStatus, linkErr)
			}

			// 확정 실패한 잡은 다시 claim되지 않는다.
			if got, err := q.Claim(ctx); err != nil || got != nil {
				t.Fatalf("failed 잡 claim = (%v, %v), want (nil, nil)", got, err)
			}
		})
	}
}

func TestComplete(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	ctx := context.Background()
	linkID := insertLink(t, db, "https://d.example")
	enqueue(t, db, q, KindScrape, linkID)

	job, err := q.Claim(ctx)
	if err != nil || job == nil {
		t.Fatalf("claim 실패: (%v, %v)", job, err)
	}
	if err := q.Complete(ctx, job.ID); err != nil {
		t.Fatalf("Complete 실패: %v", err)
	}
	r := readJob(t, db, job.ID)
	if r.status != "done" || !r.finishedAt.Valid {
		t.Fatalf("done 전이 실패: %+v", r)
	}
	if err := q.Complete(ctx, 99999); err == nil {
		t.Fatal("없는 잡 Complete가 에러를 반환하지 않음")
	}
}

func TestRecoverStale(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	ctx := context.Background()
	l1 := insertLink(t, db, "https://e1.example")
	l2 := insertLink(t, db, "https://e2.example")
	enqueue(t, db, q, KindScrape, l1)
	enqueue(t, db, q, KindScrape, l2)

	// 두 잡 다 running으로 만든 뒤 (크래시 시뮬레이션) 복구.
	for range 2 {
		if job, err := q.Claim(ctx); err != nil || job == nil {
			t.Fatalf("claim 실패: (%v, %v)", job, err)
		}
	}
	n, err := q.RecoverStale(ctx)
	if err != nil || n != 2 {
		t.Fatalf("RecoverStale = (%d, %v), want (2, nil)", n, err)
	}
	// 복구된 잡은 다시 claim 가능.
	job, err := q.Claim(ctx)
	if err != nil || job == nil {
		t.Fatalf("복구 후 claim 실패: (%v, %v)", job, err)
	}
	if job.ClaimedAt == nil {
		t.Fatal("재claim 시 claimed_at 미기록")
	}
	// 멱등: running이 없으면 0.
	if _, err := db.Exec(`UPDATE jobs SET status='pending', run_after=unixepoch()-1`); err != nil {
		t.Fatal(err)
	}
	if n, err := q.RecoverStale(ctx); err != nil || n != 0 {
		t.Fatalf("2차 RecoverStale = (%d, %v), want (0, nil)", n, err)
	}
}

func TestClaimKindsFilter(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	ctx := context.Background()
	linkID := insertLink(t, db, "https://f.example")
	enqueue(t, db, q, KindScrape, linkID)
	enqueue(t, db, q, KindTag, linkID)

	// 등록 kind 없음 → 아무것도 안 집는다.
	if got, err := q.ClaimKinds(ctx, nil); err != nil || got != nil {
		t.Fatalf("빈 kinds claim = (%v, %v), want (nil, nil)", got, err)
	}
	// tag만 등록 → scrape는 pending에 남는다.
	got, err := q.ClaimKinds(ctx, []Kind{KindTag})
	if err != nil || got == nil || got.Kind != KindTag {
		t.Fatalf("tag claim = (%+v, %v)", got, err)
	}
	if got, err := q.ClaimKinds(ctx, []Kind{KindTag}); err != nil || got != nil {
		t.Fatalf("tag 소진 후 claim = (%v, %v), want (nil, nil)", got, err)
	}
	var scrapeStatus string
	if err := db.QueryRow(`SELECT status FROM jobs WHERE kind='scrape'`).Scan(&scrapeStatus); err != nil {
		t.Fatal(err)
	}
	if scrapeStatus != "pending" {
		t.Fatalf("미등록 kind 잡 status = %s, want pending", scrapeStatus)
	}
}

func TestRelease(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	ctx := context.Background()
	linkID := insertLink(t, db, "https://g.example")
	enqueue(t, db, q, KindScrape, linkID)

	job, err := q.Claim(ctx)
	if err != nil || job == nil {
		t.Fatalf("claim 실패: (%v, %v)", job, err)
	}
	if job.Attempts != 1 {
		t.Fatalf("claim 후 attempts = %d, want 1", job.Attempts)
	}
	// Release — pending 복귀 + attempts 되감기 (셧다운은 실패가 아님).
	if err := q.Release(ctx, job.ID); err != nil {
		t.Fatalf("Release 실패: %v", err)
	}
	r := readJob(t, db, job.ID)
	if r.status != "pending" || r.attempts != 0 || r.errMsg != "" {
		t.Fatalf("release 결과 불일치: %+v", r)
	}
	// 곧바로 다시 claim 가능 — attempts는 다시 1 (소진 없음).
	got, err := q.Claim(ctx)
	if err != nil || got == nil || got.ID != job.ID || got.Attempts != 1 {
		t.Fatalf("release 후 claim = (%+v, %v), want attempts=1", got, err)
	}
	// running이 아닌 잡 Release는 에러.
	if err := q.Complete(ctx, job.ID); err != nil {
		t.Fatalf("Complete 실패: %v", err)
	}
	if err := q.Release(ctx, job.ID); err == nil {
		t.Fatal("done 잡 Release가 에러를 반환하지 않음")
	}
}

func TestWakeNonBlocking(t *testing.T) {
	q := NewSQLite(nil)
	// 버퍼(1)가 가득 차도 블록하지 않아야 한다.
	q.Wake()
	q.Wake()
	q.Wake()
	select {
	case <-q.Notify():
	default:
		t.Fatal("Wake 후 Notify 채널이 비어 있음")
	}
	select {
	case <-q.Notify():
		t.Fatal("Notify 채널에 신호가 2개 이상 쌓임 (버퍼 1이어야 함)")
	default:
	}
}
