package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/coby/push-point/backend/internal/api/gen"
)

// 2026-07-30에 계약이 받은 세 필드가 실제로 값을 싣는지.
//
// 셋을 한 PR로 묶은 이유는 계약을 건드리면 **생성물 3종 재생성까지가 done**이라
// 필드 하나나 셋이나 같은 비용이기 때문이다(12 §5). 그래서 테스트도 함께 둔다 —
// 하나가 조용히 빠지면 나머지 둘이 통과하며 가려 준다.
func TestContractBundle_carriesItsNewFields(t *testing.T) {
	fs, h, _ := newTestRouter(t)

	ok := fs.addLink("https://a.example/ok", "done", 2000)
	bad := fs.addLink("https://a.example/bad", "failed", 1000)
	fs.setError(bad, "403 Forbidden")
	fs.setRetryState(bad, "exhausted")
	_ = ok

	t.Run("Link.error와 retry_state가 목록에 실린다", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/links", "", testKey)
		var page gen.LinkPage
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, l := range page.Links {
			if l.Id != int(bad) {
				continue
			}
			if l.Error != "403 Forbidden" {
				t.Errorf("실패 사유가 안 실렸다: %q — 그러면 모든 실패가 같은 문장이 되고 "+
					"무엇이 잘못됐는지 보려면 링크마다 상세를 열어야 한다", l.Error)
			}
			if l.RetryState != gen.Exhausted {
				t.Errorf("retry_state=%q — 기다리는 중과 죽은 것이 구분되지 않는다", l.RetryState)
			}
			return
		}
		t.Fatal("실패한 링크가 목록에 없다")
	})

	// **검색은 손으로 필드를 옮기는 세 번째 프로젝션이다.** 목록·상세가 통과해도 여기만
	// 빌 수 있고, 2026-07-30에 실제로 그랬다 — `retry_state: ""`가 enum 밖 값이라 iOS
	// 클라이언트의 디코드가 터졌는데, 목록 응답만 보고 있었으면 못 봤다.
	t.Run("검색 결과도 같은 두 필드를 싣는다", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/search?q=title", "", testKey)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		var page gen.SearchPage
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Links) == 0 {
			t.Fatal("검색 결과가 비었다 — 픽스처가 안 걸렸다")
		}
		for _, r := range page.Links {
			// enum 밖 값(빈 문자열)이면 생성된 클라이언트가 디코드에 실패한다.
			if !r.RetryState.Valid() {
				t.Errorf("#%d retry_state=%q — enum 밖 값이다", r.Id, r.RetryState)
			}
			if r.Id == int(bad) && r.Error != "403 Forbidden" {
				t.Errorf("검색 결과에 실패 사유가 안 실렸다: %q", r.Error)
			}
		}
	})

	// **세 프로젝션을 전부 본다.** 목록·검색·상세가 각각 필드를 손으로 옮기고, 빠뜨려도
	// 컴파일이 된다 — 구조체 필드가 제로값을 갖고 그 제로값이 enum 밖이라 **클라이언트에서만**
	// 터진다. 2026-07-30에 검색과 상세를 연달아 빠뜨렸고, 목록만 단언하는 테스트는 둘 다
	// 통과시켰다. 그래서 이 서브테스트 셋이 따로 있다.
	t.Run("상세도 같은 두 필드를 싣는다", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, fmt.Sprintf("/api/v1/links/%d", bad), "", testKey)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var d gen.LinkDetail
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		if !d.RetryState.Valid() {
			t.Errorf("상세의 retry_state=%q — enum 밖 값이면 생성된 클라이언트가 "+
				"디코드에서 터지고 **상세 화면이 아예 안 뜬다**", d.RetryState)
		}
		if d.Error != "403 Forbidden" {
			t.Errorf("상세의 error=%q", d.Error)
		}
	})

	t.Run("Stats.failed_links가 실제 수를 준다", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/stats", "", testKey)
		var st gen.Stats
		if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
			t.Fatal(err)
		}
		if st.FailedLinks != 1 {
			t.Errorf("failed_links=%d, want 1 — 이 수가 없으면 웹은 실패 목록으로 가는 "+
				"입구를 그릴 수 없다(13 §2)", st.FailedLinks)
		}
	})

	t.Run("Tag.last_saved_at이 값을 싣는다", func(t *testing.T) {
		// **양성 단언이 있어야 한다.** 처음엔 "링크 0건이면 null"만 봤는데, 그러면
		// 핸들러가 항상 nil을 줘도 통과한다 — 변이 검증에서 실제로 빠져나갔다.
		var id int64
		for tid := range fs.tags {
			id = tid
			break
		}
		if id == 0 {
			t.Skip("사전이 비어 있다")
		}
		fs.setTagLastSaved(id, 1_700_000_000)

		rec := do(t, h, http.MethodGet, "/api/v1/tags", "", testKey)
		var tags []gen.Tag
		if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
			t.Fatal(err)
		}
		for _, tg := range tags {
			if tg.Id != int(id) {
				continue
			}
			if tg.LastSavedAt == nil {
				t.Fatal("last_saved_at이 안 실렸다 — 끊긴 주제와 이번 주 주제를 " +
					"태그 목록에서 가를 수 없다")
			}
			if int64(*tg.LastSavedAt) != 1_700_000_000 {
				t.Errorf("last_saved_at=%d, want 1700000000", *tg.LastSavedAt)
			}
			return
		}
		t.Fatal("심은 태그가 응답에 없다")
	})

	t.Run("Tag.last_saved_at은 붙은 링크가 없으면 null이다", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/tags", "", testKey)
		var tags []gen.Tag
		if err := json.Unmarshal(rec.Body.Bytes(), &tags); err != nil {
			t.Fatal(err)
		}
		if len(tags) == 0 {
			t.Skip("사전이 비어 있다")
		}
		for _, tg := range tags {
			if tg.LinkCount == 0 && tg.LastSavedAt != nil {
				t.Errorf("%s: 링크 0건인데 last_saved_at=%v — 0으로 접으면 1970년에 "+
					"저장한 것처럼 보인다", tg.Name, *tg.LastSavedAt)
			}
		}
	})
}
