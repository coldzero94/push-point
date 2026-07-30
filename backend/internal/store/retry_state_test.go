package store

import (
	"context"
	"testing"
)

// 목록과 상세가 **같은 retry_state를 내야** 한다.
//
// 이 테스트가 있는 이유는 API 페이크가 이 결함을 가렸기 때문이다. `fakeStore.addLink`가
// `RetryState: "none"`을 기본값으로 주는 바람에 핸들러 테스트는 전부 통과했는데, 실제
// 상세 쿼리는 자기 컬럼 목록을 갖고 있어서 파생 컬럼을 안 읽었고 빈 문자열을 냈다.
// enum 밖 값이라 iOS가 디코드에서 터졌고, **화면을 열어 보고서야 알았다**(2026-07-30).
//
// 페이크는 계약의 모양을 검사하고, 이 테스트는 두 SQL이 갈라지지 않는지를 검사한다.
func TestRetryState_listAndDetailAgree(t *testing.T) {
	st, db, _ := newTestStore(t)
	ctx := context.Background()

	id, _, _, err := st.SaveLink(ctx, SaveInput{URL: "https://a.example/1"})
	if err != nil {
		t.Fatal(err)
	}

	check := func(what string) {
		t.Helper()
		page, _, err := st.ListLinks(ctx, "", 10, "", "", false)
		if err != nil {
			t.Fatal(err)
		}
		var fromList string
		for _, l := range page {
			if l.ID == id {
				fromList = l.RetryState
			}
		}
		d, err := st.GetLink(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if fromList == "" {
			t.Errorf("%s: 목록의 retry_state가 빈 문자열이다 — enum 밖 값은 클라이언트를 터뜨린다", what)
		}
		if d.RetryState == "" {
			t.Errorf("%s: 상세의 retry_state가 빈 문자열이다", what)
		}
		if fromList != d.RetryState {
			t.Errorf("%s: 목록=%q 상세=%q — 두 SQL이 갈라졌다", what, fromList, d.RetryState)
		}
	}

	check("저장 직후")

	// 백오프 상태를 만든다: 잡을 미래로 밀고 시도 횟수를 올린다.
	if _, err := db.Writer.ExecContext(ctx,
		`UPDATE jobs SET status='pending', attempts=1, run_after=unixepoch()+600 WHERE link_id=?`,
		id); err != nil {
		t.Fatal(err)
	}
	check("백오프 중")

	page, _, _ := st.ListLinks(ctx, "", 10, "", "", false)
	for _, l := range page {
		if l.ID == id && l.RetryState != "waiting" {
			t.Errorf("백오프 중인데 retry_state=%q — 기다리는 중과 일하는 중이 구분되지 않는다",
				l.RetryState)
		}
	}
}
