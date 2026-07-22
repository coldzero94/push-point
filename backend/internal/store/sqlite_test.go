package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/coby/push-point/backend/internal/queue"
)

// fakeQueue는 queue.Queue 계약을 흉내 낸다 — EnqueueTx는 실제로 jobs 행을
// 삽입해 SaveLink의 "한 트랜잭션" 불변식을 검증할 수 있게 한다.
type fakeQueue struct {
	wakes int
}

// 컴파일 타임 인터페이스 검증 (.claude/rules/backend.md 인터페이스 계약).
var _ queue.Queue = (*fakeQueue)(nil)

func (f *fakeQueue) EnqueueTx(tx *sql.Tx, kind queue.Kind, linkID int64) error {
	_, err := tx.Exec(`INSERT INTO jobs (kind, link_id) VALUES (?, ?)`, string(kind), linkID)
	return err
}
func (f *fakeQueue) Claim(context.Context) (*queue.Job, error)   { return nil, nil }
func (f *fakeQueue) Complete(context.Context, int64) error       { return nil }
func (f *fakeQueue) Fail(context.Context, int64, error) error    { return nil }
func (f *fakeQueue) RecoverStale(context.Context) (int64, error) { return 0, nil }
func (f *fakeQueue) Wake()                                       { f.wakes++ }
func (f *fakeQueue) Notify() <-chan struct{}                     { return nil }

func newTestStore(t *testing.T) (Store, *DB, *fakeQueue) {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	fq := &fakeQueue{}
	return New(db, fq), db, fq
}

// countRows는 임의 조건의 행 수를 센다 (테스트 단언용).
func countRows(t *testing.T, db *DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Reader.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("countRows(%q) 실패: %v", query, err)
	}
	return n
}

// setCreatedAt은 커서 경계 테스트용으로 created_at을 강제 설정한다.
func setCreatedAt(t *testing.T, db *DB, id, createdAt int64) {
	t.Helper()
	if _, err := db.Writer.Exec(`UPDATE links SET created_at = ? WHERE id = ?`, createdAt, id); err != nil {
		t.Fatalf("created_at 설정 실패: %v", err)
	}
}

func TestSaveLink(t *testing.T) {
	s, db, fq := newTestStore(t)
	ctx := context.Background()

	id, createdAt, dup, err := s.SaveLink(ctx, "https://example.com/a", "메모")
	if err != nil {
		t.Fatalf("SaveLink 실패: %v", err)
	}
	if dup {
		t.Fatal("첫 저장이 duplicate=true")
	}
	// created_at은 DB가 기록한 실제 값 (RETURNING)
	if got := countRows(t, db, `SELECT created_at FROM links WHERE id = ?`, id); got != createdAt {
		t.Fatalf("created_at = %d, DB 값 %d와 불일치", createdAt, got)
	}
	if fq.wakes != 1 {
		t.Fatalf("Wake 호출 수 = %d, want 1", fq.wakes)
	}
	// 같은 트랜잭션 enqueue — scrape 잡 1건
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE link_id = ? AND kind = 'scrape'`, id); n != 1 {
		t.Fatalf("scrape 잡 수 = %d, want 1", n)
	}
	// FTS 색인도 같은 트랜잭션에서 생성
	if n := countRows(t, db, `SELECT COUNT(*) FROM links_fts WHERE rowid = ?`, id); n != 1 {
		t.Fatalf("links_fts 행 수 = %d, want 1", n)
	}
	// domain 자동 채움
	d, err := s.GetLink(ctx, id)
	if err != nil {
		t.Fatalf("GetLink 실패: %v", err)
	}
	if d.Domain != "example.com" || d.Note != "메모" || d.Status != "pending" {
		t.Fatalf("상세 불일치: domain=%q note=%q status=%q", d.Domain, d.Note, d.Status)
	}
	if d.Jobs.Scrape != "pending" {
		t.Fatalf("jobs.scrape = %q, want pending", d.Jobs.Scrape)
	}

	// 중복 저장 — 기존 id + duplicate=true, 잡·Wake 추가 없음 (멱등)
	id2, _, dup2, err := s.SaveLink(ctx, "https://example.com/a", "다른 메모")
	if err != nil {
		t.Fatalf("중복 SaveLink 실패: %v", err)
	}
	if !dup2 || id2 != id {
		t.Fatalf("중복 저장 결과 = (id=%d, dup=%v), want (id=%d, dup=true)", id2, dup2, id)
	}
	// 중복 저장은 멱등 — "다른 메모"로 재저장해도 기존 note는 불변이어야 한다.
	if d, err := s.GetLink(ctx, id); err != nil {
		t.Fatalf("중복 저장 후 GetLink 실패: %v", err)
	} else if d.Note != "메모" {
		t.Fatalf("중복 저장 후 note = %q, want %q (불변)", d.Note, "메모")
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs`); n != 1 {
		t.Fatalf("중복 저장 후 잡 수 = %d, want 1", n)
	}
	if fq.wakes != 1 {
		t.Fatalf("중복 저장 후 Wake 수 = %d, want 1", fq.wakes)
	}
}

func TestListLinks_CursorBoundary(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	// 5건 저장 후 created_at을 겹치게 설정: id1,2,3 → 100 / id4,5 → 200
	var ids []int64
	for _, u := range []string{"https://a.com/1", "https://a.com/2", "https://a.com/3", "https://a.com/4", "https://a.com/5"} {
		id, _, _, err := s.SaveLink(ctx, u, "")
		if err != nil {
			t.Fatalf("SaveLink 실패: %v", err)
		}
		ids = append(ids, id)
	}
	for i, ca := range []int64{100, 100, 100, 200, 200} {
		setCreatedAt(t, db, ids[i], ca)
	}

	// created_at DESC, id DESC → 5,4,3,2,1 순서. limit 2로 3페이지 순회.
	wantPages := [][]int64{{ids[4], ids[3]}, {ids[2], ids[1]}, {ids[0]}}
	cursor := ""
	for pi, want := range wantPages {
		items, next, err := s.ListLinks(ctx, cursor, 2, "", "")
		if err != nil {
			t.Fatalf("페이지 %d 조회 실패: %v", pi, err)
		}
		if len(items) != len(want) {
			t.Fatalf("페이지 %d 크기 = %d, want %d", pi, len(items), len(want))
		}
		for j, w := range want {
			if items[j].ID != w {
				t.Fatalf("페이지 %d[%d] = id %d, want %d", pi, j, items[j].ID, w)
			}
		}
		last := pi == len(wantPages)-1
		if last && next != "" {
			t.Fatalf("마지막 페이지에 next_cursor %q", next)
		}
		if !last && next == "" {
			t.Fatalf("페이지 %d에 next_cursor 없음", pi)
		}
		cursor = next
	}

	// 잘못된 커서 → ErrInvalidCursor
	if _, _, err := s.ListLinks(ctx, "!!!invalid!!!", 2, "", ""); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("잘못된 커서 에러 = %v, want ErrInvalidCursor", err)
	}
}

func TestListLinks_Filters(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id1, _, _, _ := s.SaveLink(ctx, "https://f.com/1", "")
	id2, _, _, _ := s.SaveLink(ctx, "https://f.com/2", "")
	if _, err := s.UpdateLink(ctx, id1, nil, []string{"golang"}); err != nil {
		t.Fatalf("태그 부착 실패: %v", err)
	}
	if _, err := db.Writer.Exec(`UPDATE links SET status = 'done' WHERE id = ?`, id2); err != nil {
		t.Fatalf("status 변경 실패: %v", err)
	}

	// tag 필터 — 부착 태그도 함께 조회 (N+1 없이 IN 쿼리)
	items, _, err := s.ListLinks(ctx, "", 20, "golang", "")
	if err != nil {
		t.Fatalf("tag 필터 조회 실패: %v", err)
	}
	if len(items) != 1 || items[0].ID != id1 {
		t.Fatalf("tag 필터 결과 = %+v, want id %d 1건", items, id1)
	}
	if len(items[0].Tags) != 1 || items[0].Tags[0].Name != "golang" || items[0].Tags[0].Source != "manual" {
		t.Fatalf("tag 필터 항목의 tags = %+v", items[0].Tags)
	}

	// status 필터
	items, _, err = s.ListLinks(ctx, "", 20, "", "done")
	if err != nil {
		t.Fatalf("status 필터 조회 실패: %v", err)
	}
	if len(items) != 1 || items[0].ID != id2 {
		t.Fatalf("status 필터 결과 = %+v, want id %d 1건", items, id2)
	}
}

func TestSearch_ModeBranchAndEscape(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	idKube, _, _, _ := s.SaveLink(ctx, "https://s.com/kube", "쿠버네티스 입문 강의")
	idPct, _, _, _ := s.SaveLink(ctx, "https://s.com/pct", "progress 100% done")
	idNoPct, _, _, _ := s.SaveLink(ctx, "https://s.com/nopct", "progress 1000 done")

	tests := []struct {
		name     string
		q        string
		wantMode SearchMode
		wantIDs  []int64
	}{
		{"3자 이상 한국어 FTS", "쿠버네티스", SearchModeFTS, []int64{idKube}},
		{"조사 붙은 부분 문자열도 FTS 매칭", "버네티", SearchModeFTS, []int64{idKube}},
		{"2자 미만 LIKE 폴백", "0%", SearchModeLike, []int64{idPct}}, // %는 리터럴 — 이스케이프 검증
		{"2자 LIKE 폴백", "고", SearchModeLike, nil},
		{"토큰 전부 3자 미만이면 LIKE 폴백", "ab cd", SearchModeLike, nil},
		{"FTS 미스", "존재하지않는검색어", SearchModeFTS, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items, _, mode, err := s.Search(ctx, tc.q, "", nil, nil, "", 20)
			if err != nil {
				t.Fatalf("Search(%q) 실패: %v", tc.q, err)
			}
			if mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tc.wantMode)
			}
			var got []int64
			for _, it := range items {
				got = append(got, it.ID)
				if mode == SearchModeLike && it.Rank != nil {
					t.Fatal("LIKE 모드인데 Rank != nil")
				}
				if mode == SearchModeFTS && it.Rank == nil {
					t.Fatal("FTS 모드인데 Rank == nil")
				}
			}
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("결과 = %v, want %v", got, tc.wantIDs)
			}
			for i := range got {
				if got[i] != tc.wantIDs[i] {
					t.Fatalf("결과 = %v, want %v", got, tc.wantIDs)
				}
			}
		})
	}
	_ = idNoPct // "0%" 검색이 "1000"에 매칭되면 이스케이프 실패로 위에서 잡힌다
}

func TestSearch_FTSSyncOnUpdate(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	id, _, _, _ := s.SaveLink(ctx, "https://s.com/sync", "임시 메모")

	// note 수정 → 같은 트랜잭션 재색인 → 즉시 검색 히트
	note := "쿠버네티스 네트워킹 정리"
	if _, err := s.UpdateLink(ctx, id, &note, nil); err != nil {
		t.Fatalf("UpdateLink 실패: %v", err)
	}
	items, _, mode, err := s.Search(ctx, "네트워킹", "", nil, nil, "", 20)
	if err != nil || mode != SearchModeFTS || len(items) != 1 || items[0].ID != id {
		t.Fatalf("note 재색인 검색 = (%v, %v, %v), want 1건 히트", items, mode, err)
	}

	// 태그 부착 → tags 컬럼(태그명 공백 연결) 재색인 → 태그명으로 검색 히트
	if _, err := s.UpdateLink(ctx, id, nil, []string{"golang"}); err != nil {
		t.Fatalf("태그 부착 실패: %v", err)
	}
	items, _, _, err = s.Search(ctx, "golang", "", nil, nil, "", 20)
	if err != nil || len(items) != 1 || items[0].ID != id {
		t.Fatalf("태그 재색인 검색 = (%v, %v), want 1건 히트", items, err)
	}
}

func TestSearch_FTSCursor(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	// 동일 텍스트 3건 — bm25 동점 → id 오름차순 tie-break로 커서 순회
	var ids []int64
	for _, u := range []string{"https://c.com/1", "https://c.com/2", "https://c.com/3"} {
		id, _, _, err := s.SaveLink(ctx, u, "쿠버네티스 강의 노트")
		if err != nil {
			t.Fatalf("SaveLink 실패: %v", err)
		}
		ids = append(ids, id)
	}
	seen := map[int64]bool{}
	cursor := ""
	for page := 0; ; page++ {
		items, next, mode, err := s.Search(ctx, "쿠버네티스", "", nil, nil, cursor, 1)
		if err != nil {
			t.Fatalf("페이지 %d 실패: %v", page, err)
		}
		if mode != SearchModeFTS {
			t.Fatalf("mode = %q, want fts", mode)
		}
		for _, it := range items {
			if seen[it.ID] {
				t.Fatalf("id %d 중복 등장", it.ID)
			}
			seen[it.ID] = true
		}
		if next == "" {
			break
		}
		cursor = next
		if page > 5 {
			t.Fatal("커서 순회가 끝나지 않음")
		}
	}
	if len(seen) != len(ids) {
		t.Fatalf("순회 결과 %d건, want %d건", len(seen), len(ids))
	}
}

func TestSearch_LikeCursor(t *testing.T) {
	// LIKE 폴백(q 3자 미만)도 (created_at, id) keyset으로 커서 순회한다 —
	// created_at이 전부 겹치는 다중 행에서 id DESC tie-break만으로 중복·누락 없이
	// 전량 순회하는지 검증 (searchLike keyset 커버).
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	var ids []int64
	for _, u := range []string{"https://lc.com/1", "https://lc.com/2", "https://lc.com/3", "https://lc.com/4", "https://lc.com/5"} {
		id, _, _, err := s.SaveLink(ctx, u, "메모 노트")
		if err != nil {
			t.Fatalf("SaveLink 실패: %v", err)
		}
		ids = append(ids, id)
	}
	// created_at을 전부 같은 값으로 — id DESC tie-break로만 페이지가 갈린다.
	for _, id := range ids {
		setCreatedAt(t, db, id, 500)
	}

	// q="메모"는 2 rune → LIKE 폴백. limit 2로 순회 (2,2,1).
	seen := map[int64]bool{}
	cursor := ""
	for page := 0; ; page++ {
		items, next, mode, err := s.Search(ctx, "메모", "", nil, nil, cursor, 2)
		if err != nil {
			t.Fatalf("페이지 %d 실패: %v", page, err)
		}
		if mode != SearchModeLike {
			t.Fatalf("페이지 %d mode = %q, want like", page, mode)
		}
		for _, it := range items {
			if seen[it.ID] {
				t.Fatalf("id %d 중복 등장", it.ID)
			}
			seen[it.ID] = true
			if it.Rank != nil {
				t.Fatal("LIKE 모드인데 Rank != nil")
			}
		}
		if next == "" {
			break
		}
		cursor = next
		if page > 10 {
			t.Fatal("LIKE 커서 순회가 끝나지 않음")
		}
	}
	if len(seen) != len(ids) {
		t.Fatalf("LIKE 순회 결과 %d건, want %d건 (누락)", len(seen), len(ids))
	}
}

func TestUpdateLink_TagReplaceAndFeedback(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id, _, _, _ := s.SaveLink(ctx, "https://u.com/1", "")

	// 전체 교체: dev + golang 추가 (seed 사전에 존재)
	d, err := s.UpdateLink(ctx, id, nil, []string{"dev", "golang"})
	if err != nil {
		t.Fatalf("태그 교체 실패: %v", err)
	}
	if len(d.Tags) != 2 {
		t.Fatalf("tags = %+v, want 2건", d.Tags)
	}
	for _, lt := range d.Tags {
		if lt.Source != "manual" || lt.Confidence != nil {
			t.Fatalf("태그 %q: source=%q confidence=%v, want manual/nil", lt.Name, lt.Source, lt.Confidence)
		}
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM tag_feedback WHERE link_id = ? AND action = 'added'`, id); n != 2 {
		t.Fatalf("added 피드백 = %d, want 2", n)
	}

	// golang 제거 → removed 피드백
	d, err = s.UpdateLink(ctx, id, nil, []string{"dev"})
	if err != nil {
		t.Fatalf("태그 축소 실패: %v", err)
	}
	if len(d.Tags) != 1 || d.Tags[0].Name != "dev" {
		t.Fatalf("tags = %+v, want dev 1건", d.Tags)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM tag_feedback WHERE link_id = ? AND action = 'removed'`, id); n != 1 {
		t.Fatalf("removed 피드백 = %d, want 1", n)
	}

	// 대소문자 무시 매칭 (NOCASE) — 변화 없으므로 피드백도 없음
	if _, err := s.UpdateLink(ctx, id, nil, []string{"DEV"}); err != nil {
		t.Fatalf("NOCASE 태그 교체 실패: %v", err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM tag_feedback WHERE link_id = ?`, id); n != 3 {
		t.Fatalf("피드백 총계 = %d, want 3 (무변화 교체는 기록 없음)", n)
	}

	// 사전에 없는 이름 → ErrUnknownTag
	if _, err := s.UpdateLink(ctx, id, nil, []string{"없는태그"}); !errors.Is(err, ErrUnknownTag) {
		t.Fatalf("미존재 태그 에러 = %v, want ErrUnknownTag", err)
	}
	// 없는 링크 → ErrNotFound
	if _, err := s.UpdateLink(ctx, 99999, nil, []string{"dev"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("미존재 링크 에러 = %v, want ErrNotFound", err)
	}
}

func TestDeleteLink_Soft(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id, _, _, _ := s.SaveLink(ctx, "https://d.com/1", "쿠버네티스 삭제 테스트")
	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatalf("DeleteLink 실패: %v", err)
	}

	// 행은 남고 deleted_at만 기록 (소프트)
	if n := countRows(t, db, `SELECT COUNT(*) FROM links WHERE id = ? AND deleted_at IS NOT NULL`, id); n != 1 {
		t.Fatal("소프트 삭제 행이 없음")
	}
	// 상세/목록/검색 전부 미노출
	if _, err := s.GetLink(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("삭제 후 GetLink = %v, want ErrNotFound", err)
	}
	items, _, err := s.ListLinks(ctx, "", 20, "", "")
	if err != nil || len(items) != 0 {
		t.Fatalf("삭제 후 목록 = %v (%v), want 빈 목록", items, err)
	}
	res, _, _, err := s.Search(ctx, "쿠버네티스", "", nil, nil, "", 20)
	if err != nil || len(res) != 0 {
		t.Fatalf("삭제 후 검색 = %v (%v), want 미히트", res, err)
	}
	// 재삭제 → ErrNotFound
	if err := s.DeleteLink(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("재삭제 = %v, want ErrNotFound", err)
	}
}

func TestDeleteLink_ClearsPendingFailedJobs(t *testing.T) {
	// 소프트 삭제는 links 행을 남기므로 jobs FK CASCADE가 발동하지 않는다 —
	// DeleteLink가 같은 트랜잭션에서 pending/failed 잡을 명시적으로 지우고,
	// running 잡(dispatcher가 처리 중)은 유지하는지 검증.
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id, _, _, err := s.SaveLink(ctx, "https://dj.com/1", "")
	if err != nil {
		t.Fatalf("SaveLink 실패: %v", err) // 저장이 pending scrape 잡 1건 생성
	}
	// failed 잡·running 잡을 추가로 심는다.
	if _, err := db.Writer.Exec(
		`INSERT INTO jobs (kind, link_id, status) VALUES ('tag', ?, 'failed')`, id); err != nil {
		t.Fatalf("failed 잡 삽입 실패: %v", err)
	}
	if _, err := db.Writer.Exec(
		`INSERT INTO jobs (kind, link_id, status, claimed_at) VALUES ('thumb', ?, 'running', unixepoch())`, id); err != nil {
		t.Fatalf("running 잡 삽입 실패: %v", err)
	}

	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatalf("DeleteLink 실패: %v", err)
	}

	// pending scrape + failed tag 는 삭제
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE link_id = ? AND status IN ('pending','failed')`, id); n != 0 {
		t.Fatalf("삭제 후 pending/failed 잡 = %d, want 0", n)
	}
	// running thumb 은 유지 (dispatcher 처리 중)
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE link_id = ? AND status = 'running'`, id); n != 1 {
		t.Fatalf("삭제 후 running 잡 = %d, want 1 (유지)", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE link_id = ?`, id); n != 1 {
		t.Fatalf("삭제 후 전체 잡 = %d, want 1 (running만)", n)
	}
}

func TestRetryLink(t *testing.T) {
	s, db, fq := newTestStore(t)
	ctx := context.Background()

	id, _, _, _ := s.SaveLink(ctx, "https://r.com/1", "")

	// failed가 아니면 거부
	if err := s.RetryLink(ctx, id); !errors.Is(err, ErrNotFailed) {
		t.Fatalf("pending 재시도 = %v, want ErrNotFailed", err)
	}
	if _, err := db.Writer.Exec(`UPDATE links SET status = 'failed', error = 'boom' WHERE id = ?`, id); err != nil {
		t.Fatalf("failed 상태 설정 실패: %v", err)
	}
	// 기존 scrape 잡도 확정 실패 상태로 — RetryLink가 같은 트랜잭션에서 정리해야 한다
	if _, err := db.Writer.Exec(
		`UPDATE jobs SET status = 'failed', error = 'boom', finished_at = unixepoch() WHERE link_id = ?`, id); err != nil {
		t.Fatalf("failed 잡 설정 실패: %v", err)
	}
	wakesBefore := fq.wakes
	if err := s.RetryLink(ctx, id); err != nil {
		t.Fatalf("RetryLink 실패: %v", err)
	}
	d, err := s.GetLink(ctx, id)
	if err != nil {
		t.Fatalf("GetLink 실패: %v", err)
	}
	if d.Status != "pending" || d.Error != "" {
		t.Fatalf("재시도 후 status=%q error=%q, want pending/빈 문자열", d.Status, d.Error)
	}
	// failed 잡은 삭제되고 새 pending scrape 잡 1건만 남는다 —
	// 잡 요약이 옛 failed를 최신으로 보여주지 않는다.
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE link_id = ? AND status = 'failed'`, id); n != 0 {
		t.Fatalf("재시도 후 failed 잡 = %d, want 0 (같은 트랜잭션 정리)", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE link_id = ? AND kind = 'scrape' AND status = 'pending'`, id); n != 1 {
		t.Fatalf("재시도 후 pending scrape 잡 = %d, want 1", n)
	}
	if d.Jobs.Scrape != "pending" {
		t.Fatalf("재시도 후 jobs.scrape = %q, want pending", d.Jobs.Scrape)
	}
	if fq.wakes != wakesBefore+1 {
		t.Fatalf("재시도 후 Wake 증가분 = %d, want 1", fq.wakes-wakesBefore)
	}
	// 없는 링크
	if err := s.RetryLink(ctx, 99999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("미존재 링크 재시도 = %v, want ErrNotFound", err)
	}
}

func TestTags_CRUD(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	// seed 30개 확인
	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags 실패: %v", err)
	}
	if len(tags) != 30 {
		t.Fatalf("seed 태그 수 = %d, want 30", len(tags))
	}

	// NOCASE 중복 — seed의 dev와 대소문자만 다름
	if _, err := s.CreateTag(ctx, "DEV", nil, ""); !errors.Is(err, ErrDuplicateTag) {
		t.Fatalf("NOCASE 중복 생성 = %v, want ErrDuplicateTag", err)
	}
	created, err := s.CreateTag(ctx, "테스트태그", []string{"testtag"}, "")
	if err != nil {
		t.Fatalf("CreateTag 실패: %v", err)
	}
	if created.LinkCount != 0 || len(created.Aliases) != 1 || created.Aliases[0] != "testtag" {
		t.Fatalf("생성 결과 = %+v", created)
	}

	// 개명 중복 검사
	if _, err := s.UpdateTag(ctx, created.ID, ptr("golang"), nil, nil); !errors.Is(err, ErrDuplicateTag) {
		t.Fatalf("중복 개명 = %v, want ErrDuplicateTag", err)
	}
	// aliases만 교체 (name nil → 유지)
	upd, err := s.UpdateTag(ctx, created.ID, nil, []string{"a", "b"}, nil)
	if err != nil {
		t.Fatalf("UpdateTag 실패: %v", err)
	}
	if upd.Name != "테스트태그" || len(upd.Aliases) != 2 {
		t.Fatalf("수정 결과 = %+v", upd)
	}
	// 미존재 태그
	if _, err := s.UpdateTag(ctx, 99999, nil, nil, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("미존재 수정 = %v, want ErrNotFound", err)
	}

	// 삭제 후 사전에서 사라짐
	if err := s.DeleteTag(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTag 실패: %v", err)
	}
	if err := s.DeleteTag(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("재삭제 = %v, want ErrNotFound", err)
	}
}

// TestTags_Facet은 0003 마이그레이션의 facet 컬럼을 검증한다:
// 시드 30개 배정(craft 18 / media 5 / life 7), 신규 태그 기본값 neutral,
// 생성 시 지정, 수정 시 교체·유지, CHECK 제약(enum 밖 값 거부).
func TestTags_Facet(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	// 시드 배정 — 0003의 UPDATE 3문이 30개를 전부 덮는다 (neutral로 남는 시드는 없다)
	tags, err := s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags 실패: %v", err)
	}
	byFacet := map[string]int{}
	byName := map[string]string{}
	for _, tg := range tags {
		byFacet[tg.Facet]++
		byName[tg.Name] = tg.Facet
	}
	want := map[string]int{FacetCraft: 18, FacetMedia: 5, FacetLife: 7}
	for f, n := range want {
		if byFacet[f] != n {
			t.Errorf("시드 facet %s = %d개, want %d개", f, byFacet[f], n)
		}
	}
	if byFacet[FacetNeutral] != 0 {
		t.Errorf("시드 neutral = %d개, want 0개 (30개 전부 배정)", byFacet[FacetNeutral])
	}
	for name, f := range map[string]string{
		"golang": FacetCraft, "design": FacetCraft,
		"podcast": FacetMedia, "video": FacetMedia,
		"travel": FacetLife, "life": FacetLife,
	} {
		if byName[name] != f {
			t.Errorf("시드 %q facet = %q, want %q", name, byName[name], f)
		}
	}

	// 신규 태그 기본값 = neutral (사전에 없는 태그는 색 없이 태어난다)
	def, err := s.CreateTag(ctx, "새태그", nil, "")
	if err != nil {
		t.Fatalf("CreateTag 실패: %v", err)
	}
	if def.Facet != FacetNeutral {
		t.Fatalf("기본 facet = %q, want %q", def.Facet, FacetNeutral)
	}

	// 생성 시 지정
	got, err := s.CreateTag(ctx, "지정태그", nil, FacetMedia)
	if err != nil {
		t.Fatalf("CreateTag(facet) 실패: %v", err)
	}
	if got.Facet != FacetMedia {
		t.Fatalf("생성 시 지정 facet = %q, want %q", got.Facet, FacetMedia)
	}

	// 목록에도 그대로 실린다 (조회 경로 커버)
	tags, err = s.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags 실패: %v", err)
	}
	byName = map[string]string{}
	for _, tg := range tags {
		byName[tg.Name] = tg.Facet
	}
	if byName["새태그"] != FacetNeutral || byName["지정태그"] != FacetMedia {
		t.Fatalf("목록 facet = 새태그:%q 지정태그:%q", byName["새태그"], byName["지정태그"])
	}

	// 수정 — facet nil이면 유지, 값이 있으면 교체
	upd, err := s.UpdateTag(ctx, got.ID, nil, []string{"오디오"}, nil)
	if err != nil {
		t.Fatalf("UpdateTag 실패: %v", err)
	}
	if upd.Facet != FacetMedia {
		t.Fatalf("facet nil 수정 후 = %q, want %q 유지", upd.Facet, FacetMedia)
	}
	upd, err = s.UpdateTag(ctx, got.ID, nil, nil, ptr(FacetLife))
	if err != nil {
		t.Fatalf("UpdateTag(facet) 실패: %v", err)
	}
	if upd.Facet != FacetLife {
		t.Fatalf("facet 교체 후 = %q, want %q", upd.Facet, FacetLife)
	}

	// CHECK 제약 — enum 밖 값은 DB가 막는다 (API 계층 검증의 백스톱)
	if _, err := s.CreateTag(ctx, "엉터리", nil, "rainbow"); err == nil {
		t.Fatal("enum 밖 facet 생성이 성공했다 — CHECK 제약 미동작")
	}
	if _, err := s.UpdateTag(ctx, got.ID, nil, nil, ptr("rainbow")); err == nil {
		t.Fatal("enum 밖 facet 수정이 성공했다 — CHECK 제약 미동작")
	}
}

func TestTagRenameAndDelete_ReindexFTS(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id, _, _, _ := s.SaveLink(ctx, "https://t.com/1", "")
	created, err := s.CreateTag(ctx, "옛이름", nil, "")
	if err != nil {
		t.Fatalf("CreateTag 실패: %v", err)
	}
	if _, err := s.UpdateLink(ctx, id, nil, []string{"옛이름"}); err != nil {
		t.Fatalf("태그 부착 실패: %v", err)
	}

	// 개명 → 부착 링크의 FTS tags 텍스트가 새 이름으로 재색인
	if _, err := s.UpdateTag(ctx, created.ID, ptr("새이름표"), nil, nil); err != nil {
		t.Fatalf("UpdateTag 실패: %v", err)
	}
	items, _, _, err := s.Search(ctx, "새이름표", "", nil, nil, "", 20)
	if err != nil || len(items) != 1 || items[0].ID != id {
		t.Fatalf("개명 후 새 이름 검색 = (%v, %v), want 히트", items, err)
	}
	items, _, _, err = s.Search(ctx, "옛이름", "", nil, nil, "", 20)
	if err != nil || len(items) != 0 {
		t.Fatalf("개명 후 옛 이름 검색 = (%v, %v), want 미히트", items, err)
	}

	// 삭제 → link_tags CASCADE + FTS에서 태그 텍스트 제거
	if err := s.DeleteTag(ctx, created.ID); err != nil {
		t.Fatalf("DeleteTag 실패: %v", err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM link_tags WHERE link_id = ?`, id); n != 0 {
		t.Fatalf("삭제 후 link_tags = %d, want 0 (CASCADE)", n)
	}
	items, _, _, err = s.Search(ctx, "새이름표", "", nil, nil, "", 20)
	if err != nil || len(items) != 0 {
		t.Fatalf("태그 삭제 후 검색 = (%v, %v), want 미히트", items, err)
	}
}

func TestStats(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id1, _, _, _ := s.SaveLink(ctx, "https://st.com/1", "")
	id2, _, _, _ := s.SaveLink(ctx, "https://st.com/2", "")
	id3, _, _, _ := s.SaveLink(ctx, "https://st.com/3", "")
	if _, err := s.UpdateLink(ctx, id1, nil, []string{"dev"}); err != nil {
		t.Fatalf("태그 부착 실패: %v", err)
	}
	if _, err := s.UpdateLink(ctx, id2, nil, []string{"dev", "golang"}); err != nil {
		t.Fatalf("태그 부착 실패: %v", err)
	}
	if err := s.DeleteLink(ctx, id3); err != nil {
		t.Fatalf("DeleteLink 실패: %v", err)
	}
	// id1을 8일 전으로 밀어 this_week에서 제외
	var now int64
	if err := db.Reader.QueryRow(`SELECT unixepoch()`).Scan(&now); err != nil {
		t.Fatalf("unixepoch 조회 실패: %v", err)
	}
	setCreatedAt(t, db, id1, now-8*86400)

	st, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats 실패: %v", err)
	}
	if st.TotalLinks != 2 {
		t.Fatalf("total_links = %d, want 2 (소프트 삭제 제외)", st.TotalLinks)
	}
	if st.LinksThisWeek != 1 {
		t.Fatalf("links_this_week = %d, want 1", st.LinksThisWeek)
	}
	// by_tag: dev 2건 > golang 1건 (삭제된 링크는 미집계)
	if len(st.ByTag) != 2 || st.ByTag[0].Name != "dev" || st.ByTag[0].Count != 2 ||
		st.ByTag[1].Name != "golang" || st.ByTag[1].Count != 1 {
		t.Fatalf("by_tag = %+v", st.ByTag)
	}
	if len(st.ByDay) == 0 {
		t.Fatal("by_day가 비어 있음")
	}
	// id1(8일 전)·id2(현재) 모두 최근 30일 이내, id3은 삭제 → 합계 2
	var dayTotal int64
	for _, d := range st.ByDay {
		dayTotal += d.Count
	}
	if dayTotal != 2 {
		t.Fatalf("by_day 합계 = %d, want 2", dayTotal)
	}
}

func TestSearch_Filters(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id1, _, _, _ := s.SaveLink(ctx, "https://sf.com/1", "쿠버네티스 노트 하나")
	id2, _, _, _ := s.SaveLink(ctx, "https://sf.com/2", "쿠버네티스 노트 둘")
	if _, err := s.UpdateLink(ctx, id1, nil, []string{"kubernetes"}); err != nil {
		t.Fatalf("태그 부착 실패: %v", err)
	}
	setCreatedAt(t, db, id1, 1000)
	setCreatedAt(t, db, id2, 2000)

	// tag 필터
	items, _, _, err := s.Search(ctx, "쿠버네티스", "kubernetes", nil, nil, "", 20)
	if err != nil || len(items) != 1 || items[0].ID != id1 {
		t.Fatalf("tag 필터 검색 = (%v, %v), want id1만", items, err)
	}
	// from/to 필터
	from, to := int64(1500), int64(2500)
	items, _, _, err = s.Search(ctx, "쿠버네티스", "", &from, &to, "", 20)
	if err != nil || len(items) != 1 || items[0].ID != id2 {
		t.Fatalf("from/to 검색 = (%v, %v), want id2만", items, err)
	}
}

func TestSaveLink_ResaveAfterDelete(t *testing.T) {
	// F1: 저장 → 삭제 → 같은 URL 재저장 → 같은 행 undelete + 신규처럼 처리 → 상세 200.
	s, db, fq := newTestStore(t)
	ctx := context.Background()

	id, createdAt, dup, err := s.SaveLink(ctx, "https://re.com/1", "원래 메모")
	if err != nil || dup {
		t.Fatalf("첫 저장 = (dup=%v, %v)", dup, err)
	}
	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatalf("DeleteLink 실패: %v", err)
	}

	// 재저장 — duplicate=false (201 신규처럼), 같은 행 재사용
	id2, createdAt2, dup2, err := s.SaveLink(ctx, "https://re.com/1", "새 메모")
	if err != nil {
		t.Fatalf("재저장 실패: %v", err)
	}
	if dup2 || id2 != id {
		t.Fatalf("재저장 = (id=%d, dup=%v), want (id=%d, dup=false)", id2, dup2, id)
	}
	if createdAt2 != createdAt {
		t.Fatalf("재저장 created_at = %d, want 원래 값 %d (행 유지)", createdAt2, createdAt)
	}
	if fq.wakes != 2 {
		t.Fatalf("Wake 수 = %d, want 2 (재저장도 신규처럼 깨움)", fq.wakes)
	}

	// 상세 200 — undelete 후 pending, note는 새 값, error 초기화
	d, err := s.GetLink(ctx, id)
	if err != nil {
		t.Fatalf("재저장 후 GetLink = %v, want 성공", err)
	}
	if d.Status != "pending" || d.Note != "새 메모" || d.Error != "" {
		t.Fatalf("재저장 상세: status=%q note=%q error=%q, want pending/새 메모/빈 문자열", d.Status, d.Note, d.Error)
	}
	// 삭제 시 첫 저장의 pending scrape 잡이 정리되고(DeleteLink) 재저장이 1건 재-enqueue —
	// 남는 scrape 잡은 1건, FTS 재색인.
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE link_id = ? AND kind = 'scrape'`, id); n != 1 {
		t.Fatalf("재저장 후 scrape 잡 = %d, want 1 (삭제가 옛 pending 잡 정리)", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM links_fts WHERE rowid = ?`, id); n != 1 {
		t.Fatalf("재저장 후 links_fts = %d, want 1 (재색인)", n)
	}
	// 재색인된 note로 검색 히트
	items, _, _, err := s.Search(ctx, "새 메모", "", nil, nil, "", 20)
	if err != nil || len(items) != 1 || items[0].ID != id {
		t.Fatalf("재저장 후 검색 = (%v, %v), want 1건 히트", items, err)
	}

	// 재저장된 링크에 다시 저장하면 평범한 중복 (멱등)
	id3, _, dup3, err := s.SaveLink(ctx, "https://re.com/1", "또 다른 메모")
	if err != nil || !dup3 || id3 != id {
		t.Fatalf("undelete 후 중복 저장 = (id=%d, dup=%v, %v), want (id=%d, true)", id3, dup3, err, id)
	}
}

func TestSaveLink_UndeleteResetsTagsAndNote(t *testing.T) {
	// F1 확정: undelete 재저장은 신규 저장과 동일 취급 —
	//   ① 부착 태그 전부 제거 (시스템 동작이라 tag_feedback 미기록),
	//   ② 태그 빈 상태로 FTS 재색인 → 옛 태그 텍스트 검색 미히트,
	//   ③ note는 새 값으로 교체 (빈 문자열이면 ''로).
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id, _, _, err := s.SaveLink(ctx, "https://ud.com/1", "옛 메모")
	if err != nil {
		t.Fatalf("첫 저장 실패: %v", err)
	}
	// seed 사전의 golang 부착 → tags 컬럼 재색인 → 태그명으로 FTS 히트
	if _, err := s.UpdateLink(ctx, id, nil, []string{"golang"}); err != nil {
		t.Fatalf("태그 부착 실패: %v", err)
	}
	if items, _, _, err := s.Search(ctx, "golang", "", nil, nil, "", 20); err != nil || len(items) != 1 {
		t.Fatalf("삭제 전 태그 검색 = (%d건, %v), want 1건", len(items), err)
	}
	feedbackBefore := countRows(t, db, `SELECT COUNT(*) FROM tag_feedback WHERE link_id = ?`, id)

	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatalf("DeleteLink 실패: %v", err)
	}
	// 빈 note로 재저장 — 신규 저장 취급
	id2, _, dup, err := s.SaveLink(ctx, "https://ud.com/1", "")
	if err != nil || dup || id2 != id {
		t.Fatalf("재저장 = (id=%d, dup=%v, %v), want (id=%d, dup=false)", id2, dup, err, id)
	}

	d, err := s.GetLink(ctx, id)
	if err != nil {
		t.Fatalf("재저장 후 GetLink 실패: %v", err)
	}
	if len(d.Tags) != 0 {
		t.Fatalf("재저장 후 tags = %+v, want 0개 (신규 저장 취급)", d.Tags)
	}
	if d.Note != "" {
		t.Fatalf("재저장 후 note = %q, want '' (빈 note로 교체)", d.Note)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM link_tags WHERE link_id = ?`, id); n != 0 {
		t.Fatalf("재저장 후 link_tags = %d, want 0", n)
	}
	// 태그 제거는 시스템 동작 — tag_feedback 추가 기록 없음
	if n := countRows(t, db, `SELECT COUNT(*) FROM tag_feedback WHERE link_id = ?`, id); n != feedbackBefore {
		t.Fatalf("재저장 후 tag_feedback = %d, want %d (시스템 동작이라 미기록)", n, feedbackBefore)
	}
	// FTS 재색인이 태그 빈 상태로 됐으므로 옛 태그명 검색 미히트
	if items, _, _, err := s.Search(ctx, "golang", "", nil, nil, "", 20); err != nil || len(items) != 0 {
		t.Fatalf("재저장 후 옛 태그 검색 = (%d건, %v), want 0건 (태그 빈 상태 재색인)", len(items), err)
	}
}

func TestCursor_ModeMismatch(t *testing.T) {
	// F2: 목록/LIKE 커서("c")와 FTS 커서("f")는 모드 판별자로 구분 —
	// 모드가 다른 커서는 ErrInvalidCursor(→ 400 invalid_input).
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	for _, u := range []string{"https://m.com/1", "https://m.com/2"} {
		if _, _, _, err := s.SaveLink(ctx, u, "쿠버네티스 커서 노트"); err != nil {
			t.Fatalf("SaveLink 실패: %v", err)
		}
	}

	// FTS 커서 획득 (limit 1 → next 존재)
	_, ftsCursor, mode, err := s.Search(ctx, "쿠버네티스", "", nil, nil, "", 1)
	if err != nil || mode != SearchModeFTS || ftsCursor == "" {
		t.Fatalf("FTS 1페이지 = (cursor=%q, mode=%q, %v)", ftsCursor, mode, err)
	}
	// 목록 커서 획득
	_, listCursor, err := s.ListLinks(ctx, "", 1, "", "")
	if err != nil || listCursor == "" {
		t.Fatalf("목록 1페이지 = (cursor=%q, %v)", listCursor, err)
	}

	// FTS 커서를 목록에 → 400
	if _, _, err := s.ListLinks(ctx, ftsCursor, 1, "", ""); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("목록에 FTS 커서 = %v, want ErrInvalidCursor", err)
	}
	// 목록 커서를 FTS 검색에 → 400
	if _, _, _, err := s.Search(ctx, "쿠버네티스", "", nil, nil, listCursor, 1); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("FTS에 목록 커서 = %v, want ErrInvalidCursor", err)
	}
	// FTS 커서를 LIKE 폴백 검색에 → 400 (LIKE는 "c" 모드)
	if _, _, _, err := s.Search(ctx, "고", "", nil, nil, ftsCursor, 1); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("LIKE에 FTS 커서 = %v, want ErrInvalidCursor", err)
	}
	// 정상 모드 조합은 계속 동작
	if _, _, err := s.ListLinks(ctx, listCursor, 1, "", ""); err != nil {
		t.Fatalf("목록 커서 재사용 실패: %v", err)
	}
	if _, _, _, err := s.Search(ctx, "쿠버네티스", "", nil, nil, ftsCursor, 1); err != nil {
		t.Fatalf("FTS 커서 재사용 실패: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }
