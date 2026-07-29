package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/coby/push-point/backend/internal/api/gen"
)

// thumb_path는 있는데 파일이 없을 때 API가 URL을 광고하면 안 된다.
//
// **2026-07-29에 사용자가 먼저 발견한 결함이다.** 서버가 죽은 URL을 계속 주고, 클라이언트는
// 404를 맞은 뒤 조용히 생성 커버로 떨어진다 — 그런데 생성 커버는 "썸네일이 원래 없는 링크"의
// 정상 표시이기도 하다. **깨진 썸네일과 없는 썸네일이 화면에서 완전히 같아진다.**
//
// 이 프로젝트는 같은 부류를 이미 한 번 겪었다(상대 thumb_url 때문에 전부 비었던 것). 그때는
// 원인만 고치고 탐지 가능성을 0으로 남겼고, 그래서 두 번째 발생도 사람이 우연히 알아챘다.
// 이 테스트가 그 자리를 메운다.
func TestThumbURLOmittedWhenFileMissing(t *testing.T) {
	fs, h, dataDir := newTestRouter(t)

	present := fs.addLink("https://a.example/1", "done", 1000)
	missing := fs.addLink("https://a.example/2", "done", 900)
	fs.setThumb(present, "aa/present.jpg")
	fs.setThumb(missing, "bb/missing.jpg")

	// present만 디스크에 존재한다.
	//
	// **`bb/` 샤드는 만들되 파일은 안 만든다.** 없는 것이 파일 하나인지 샤드 통째인지가
	// 이 테스트의 강도를 가른다 — 썸네일은 해시 앞 두 자로 샤딩되므로 실제로 한 장이
	// 사라져도 그 샤드에는 다른 링크의 파일이 남아 있다. 샤드까지 없는 픽스처를 쓰면
	// "파일마다 stat" 대신 "샤드를 한 번 stat"으로 바꾸는 최적화가 테스트를 통과해 버리고,
	// 그건 2026-07-29에 사용자가 찾은 결함을 그대로 되돌리는 변경이다.
	for _, shard := range []string{"aa", "bb"} {
		if err := os.MkdirAll(filepath.Join(dataDir, "thumbs", shard), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dataDir, "thumbs", "aa", "present.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("목록", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/api/v1/links", "", testKey)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var page gen.LinkPage
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		got := map[int]*string{}
		for _, l := range page.Links {
			got[l.Id] = l.ThumbUrl
		}
		if u := got[int(present)]; u == nil || *u != "/thumbs/aa/present.jpg" {
			t.Errorf("있는 파일의 URL이 빠졌다: %v", u)
		}
		if u := got[int(missing)]; u != nil {
			t.Errorf("없는 파일의 URL을 광고했다: %q — 클라이언트는 404를 맞고 생성 커버로 떨어져, 깨진 것과 없는 것이 구분되지 않는다", *u)
		}
	})

	// 검색은 `toAPILink`의 **세 번째** 호출자이고, 그 결과를 gen.SearchResult로 필드마다
	// 손으로 옮긴다(handlers.go). 필드가 조용히 빠지기 딱 좋은 자리인데 단언이 없었다.
	t.Run("검색", func(t *testing.T) {
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
			switch r.Id {
			case int(present):
				if r.ThumbUrl == nil {
					t.Error("검색 결과에서 있는 파일의 URL이 빠졌다")
				}
			case int(missing):
				if r.ThumbUrl != nil {
					t.Errorf("검색 결과가 없는 파일의 URL을 광고했다: %q", *r.ThumbUrl)
				}
			}
		}
	})

	t.Run("상세", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, fmt.Sprintf("/api/v1/links/%d", missing), "", testKey)
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var d gen.LinkDetail
		if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		if d.ThumbUrl != nil {
			t.Errorf("상세가 없는 파일의 URL을 광고했다: %q", *d.ThumbUrl)
		}
	})
}

// stat이 ENOENT가 아닌 오류를 낼 때는 **없음으로 처리하지 않는다.**
//
// 파일은 있는데 읽을 수 없는 상태(권한, I/O, 마운트 해제)를 "원래 없다"로 접으면, 이
// 커밋이 고치려던 형태가 한 층 아래에서 되살아난다 — 게다가 URL을 아예 안 주므로
// 클라이언트의 실패 로그도 안 울려서 **어디에도 흔적이 남지 않는다.**
func TestThumbURLKeepsAdvertisingWhenStatFailsForAnotherReason(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root는 권한 검사를 우회한다")
	}
	fs, h, dataDir := newTestRouter(t)
	id := fs.addLink("https://a.example/1", "done", 1000)
	fs.setThumb(id, "cc/blocked.jpg")

	shard := filepath.Join(dataDir, "thumbs", "cc")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shard, "blocked.jpg"), []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 샤드를 못 읽게 만든다 → stat은 EACCES를 낸다(파일은 그대로 있다).
	if err := os.Chmod(shard, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shard, 0o755) })

	rec := do(t, h, http.MethodGet, "/api/v1/links", "", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var page gen.LinkPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Links) != 1 {
		t.Fatalf("링크 %d건", len(page.Links))
	}
	if page.Links[0].ThumbUrl == nil {
		t.Error("읽을 수 없는 썸네일을 '없음'으로 접었다 — " +
			"스토리지 고장이 생성 커버와 구별되지 않고, 클라이언트 로그도 울리지 않는다")
	}
}
