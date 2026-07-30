package queue

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// `links.error`는 카드에 그려지는 **한 문장**이고, 카드는 두 줄에서 자른다.
//
// 감싼 사슬을 그대로 쓰면 래핑 접두사가 두 줄을 다 먹고 **정작 원인이 잘려 나간다** —
// 2026-07-30에 화면에서 그렇게 나왔다. 그때 남아 있던 두 줄은 카드가 커버·제목·메타에서
// 이미 세 번 보여 준 URL이었다.
func TestRootCause(t *testing.T) {
	dns := errors.New("lookup x.invalid: no such host")

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"감싸지 않은 오류는 그대로", dns, "lookup x.invalid: no such host"},
		{
			"실제 사슬에서 원인만 남는다",
			fmt.Errorf("scraper: GET 실패 https://x.invalid/a: %w",
				fmt.Errorf("Get %q: %w", "https://x.invalid/a", dns)),
			"lookup x.invalid: no such host",
		},
		{"한 겹", fmt.Errorf("바깥: %w", dns), "lookup x.invalid: no such host"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := rootCause(tc.err); got != tc.want {
				t.Errorf("rootCause = %q, want %q", got, tc.want)
			}
		})
	}

	// `errors.Join`은 사슬이 아니라 **목록**이다. 하나만 고르면 나머지를 숨기게 되므로
	// 벗기지 않는다 — Join된 오류의 Unwrap은 `[]error`를 내고 `errors.Unwrap`은 nil이다.
	t.Run("Join은 벗기지 않는다", func(t *testing.T) {
		joined := errors.Join(errors.New("첫째"), errors.New("둘째"))
		got := rootCause(joined)
		if got != joined.Error() {
			t.Errorf("rootCause = %q — Join을 벗겨 한쪽만 남겼다", got)
		}
	})
}

// **두 컬럼에 다른 것이 들어가야 한다.**
//
// `jobs.error`는 진단이라 감싼 사슬 전체가 필요하고, `links.error`는 카드에 그려지는
// 한 문장이라 가장 안쪽 원인만 있어야 한다. 이 구분이 없으면 카드가 두 줄을 래핑
// 접두사로 채우고 정작 원인을 잘라 낸다 — 2026-07-30에 화면에서 그렇게 나왔다.
//
// 이 테스트가 따로 있는 이유: `cause`를 `msg`로 되돌리는 변이를 **컴파일러가** 잡았는데
// (미사용 변수), 그건 커버리지가 아니다. `_ = cause` 한 줄이면 빠져나간다.
func TestFailWritesCauseToLinkAndChainToJob(t *testing.T) {
	db := newTestDB(t)
	q := NewSQLite(db)
	ctx := context.Background()
	linkID := insertLink(t, db, "https://x.invalid/a")
	enqueue(t, db, q, KindScrape, linkID)

	inner := errors.New("lookup x.invalid: no such host")
	wrapped := fmt.Errorf("scraper: GET 실패 https://x.invalid/a: %w", inner)

	for i := 1; i <= 3; i++ {
		if _, err := db.Exec(`UPDATE jobs SET run_after=unixepoch()-1`); err != nil {
			t.Fatal(err)
		}
		job, err := q.Claim(ctx)
		if err != nil || job == nil {
			t.Fatalf("시도 %d claim: (%v, %v)", i, job, err)
		}
		if err := q.Fail(ctx, job.ID, wrapped); err != nil {
			t.Fatal(err)
		}
	}

	var linkErr, jobErr string
	if err := db.QueryRow(`SELECT error FROM links WHERE id=?`, linkID).Scan(&linkErr); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT error FROM jobs WHERE link_id=?`, linkID).Scan(&jobErr); err != nil {
		t.Fatal(err)
	}

	if linkErr != inner.Error() {
		t.Errorf("links.error = %q\nwant %q — 래핑 접두사가 카드의 두 줄을 먹고 "+
			"정작 원인이 잘린다", linkErr, inner.Error())
	}
	if jobErr != wrapped.Error() {
		t.Errorf("jobs.error = %q\nwant 사슬 전체 — 진단에서 어느 단계인지가 정보다", jobErr)
	}
}
