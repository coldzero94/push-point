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
	if err := os.MkdirAll(filepath.Join(dataDir, "thumbs", "aa"), 0o755); err != nil {
		t.Fatal(err)
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
