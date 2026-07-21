package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- 픽스처 ----

// Netscape 북마크 export — http/https + 무시해야 할 javascript: 스킴 혼재.
const bookmarkFixture = `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
    <DT><H3>개발</H3>
    <DL><p>
        <DT><A HREF="https://github.com/coby/push-point" ADD_DATE="1700000000">push-point</A>
        <DT><A HREF="http://example.com/article">article</A>
        <DT><A HREF="javascript:void(0)">북마클릿(무시)</A>
    </DL><p>
    <DT><A HREF="https://www.youtube.com/watch?v=abc123">영상</A>
</DL><p>
`

// YouTube Takeout watch-history.json — titleUrl 필드, watch 아닌 항목 혼재.
const takeoutJSONFixture = `[
  {"header":"YouTube","title":"Watched 고루틴 강의","titleUrl":"https://www.youtube.com/watch?v=vid001","time":"2026-01-01T00:00:00Z"},
  {"header":"YouTube","title":"Watched 쿠버네티스","titleUrl":"https://www.youtube.com/watch?v=vid002","time":"2026-01-02T00:00:00Z"},
  {"header":"YouTube","title":"채널 방문(무시)","titleUrl":"https://www.youtube.com/channel/UC123","time":"2026-01-03T00:00:00Z"},
  {"header":"YouTube Music","title":"Watched"}
]`

// Takeout CSV — 헤더 + watch URL 셀 + youtu.be 단축 링크.
const takeoutCSVFixture = `Video Id,Video Title,Video URL
vid001,고루틴 강의,https://www.youtube.com/watch?v=vid001
vid002,쿠버네티스,https://youtu.be/vid002
,헤더행같은거 아님,https://youtube.com/watch?v=vid003
`

// YouTube Takeout watch-history.html — Takeout 기본 export. 영상 링크(watch)와
// 채널 링크가 섞여 있어 watch URL만 걸러야 한다 (채널·검색 링크는 무시).
const takeoutHTMLFixture = `<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"><title>YouTube</title></head>
<body><div class="mdl-grid">
  <div class="outer-cell">
    <a href="https://www.youtube.com/watch?v=vid001">고루틴 강의를 시청했습니다</a><br>
    <a href="https://www.youtube.com/channel/UC123">채널 이름</a>
  </div>
  <div class="outer-cell">
    <a href="https://www.youtube.com/watch?v=vid002">쿠버네티스 입문</a><br>
    <a href="https://www.youtube.com/channel/UC456">다른 채널</a>
  </div>
  <div class="outer-cell">
    <a href="https://youtu.be/vid003">단축 링크 영상</a>
  </div>
</div></body></html>
`

// ---- 파서 테스트 (테이블 주도) ----

func TestExtractBookmarkURLs(t *testing.T) {
	got, err := extractBookmarkURLs(strings.NewReader(bookmarkFixture))
	if err != nil {
		t.Fatalf("extractBookmarkURLs: %v", err)
	}
	want := []string{
		"https://github.com/coby/push-point",
		"http://example.com/article",
		"https://www.youtube.com/watch?v=abc123",
	}
	assertURLs(t, got, want)
}

func TestExtractTakeoutURLs(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		format string
		want   []string
	}{
		{
			name:   "json auto",
			input:  takeoutJSONFixture,
			format: "auto",
			want:   []string{"https://www.youtube.com/watch?v=vid001", "https://www.youtube.com/watch?v=vid002"},
		},
		{
			name:   "json explicit",
			input:  takeoutJSONFixture,
			format: "json",
			want:   []string{"https://www.youtube.com/watch?v=vid001", "https://www.youtube.com/watch?v=vid002"},
		},
		{
			name:   "csv auto",
			input:  takeoutCSVFixture,
			format: "auto",
			want:   []string{"https://www.youtube.com/watch?v=vid001", "https://youtu.be/vid002", "https://youtube.com/watch?v=vid003"},
		},
		{
			name:   "csv explicit",
			input:  takeoutCSVFixture,
			format: "csv",
			want:   []string{"https://www.youtube.com/watch?v=vid001", "https://youtu.be/vid002", "https://youtube.com/watch?v=vid003"},
		},
		{
			name:   "html auto",
			input:  takeoutHTMLFixture,
			format: "auto",
			want:   []string{"https://www.youtube.com/watch?v=vid001", "https://www.youtube.com/watch?v=vid002", "https://youtu.be/vid003"},
		},
		{
			name:   "html explicit",
			input:  takeoutHTMLFixture,
			format: "html",
			want:   []string{"https://www.youtube.com/watch?v=vid001", "https://www.youtube.com/watch?v=vid002", "https://youtu.be/vid003"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractTakeoutURLs(strings.NewReader(tc.input), tc.format)
			if err != nil {
				t.Fatalf("extractTakeoutURLs: %v", err)
			}
			assertURLs(t, got, tc.want)
		})
	}
}

func TestIsYouTubeWatchURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/watch?v=abc", true},
		{"https://youtube.com/watch?v=abc", true},
		{"https://m.youtube.com/watch?v=abc", true},
		{"https://youtu.be/abc", true},
		{"https://www.youtube.com/channel/UC123", false},
		{"https://www.youtube.com/watch", false}, // v 파라미터 없음
		{"https://youtu.be/", false},
		{"https://example.com/watch?v=abc", false},
		{"javascript:void(0)", false},
	}
	for _, tc := range tests {
		if got := isYouTubeWatchURL(tc.url); got != tc.want {
			t.Errorf("isYouTubeWatchURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestDedupeURLs(t *testing.T) {
	got := dedupeURLs([]string{"a", "b", "a", "c", "b"})
	assertURLs(t, got, []string{"a", "b", "c"})
}

// ---- 전송 테스트 (httptest 서버, 외부 네트워크 없음) ----

func TestSendLinks(t *testing.T) {
	// 저장/중복/실패를 URL로 분기하는 가짜 서버.
	var mu sync.Mutex
	var received []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.Copy(io.Discard, r.Body)

		mu.Lock()
		received = append(received, body.URL)
		mu.Unlock()

		switch {
		case strings.Contains(body.URL, "dup"):
			w.WriteHeader(http.StatusOK) // 중복
		case strings.Contains(body.URL, "boom"):
			w.WriteHeader(http.StatusInternalServerError) // 실패
		default:
			w.WriteHeader(http.StatusCreated) // 저장
		}
	}))
	defer srv.Close()

	urls := []string{
		"https://example.com/1",
		"https://example.com/2",
		"https://example.com/dup",
		"https://example.com/boom",
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	// interval=0 — 테스트는 rate limit 없이 즉시 전송.
	saved, dup, failed := sendLinks(context.Background(), srv.Client(), srv.URL, "test-key", urls, 0, logger)

	if saved != 2 || dup != 1 || failed != 1 {
		t.Fatalf("saved=%d dup=%d failed=%d, want 2/1/1", saved, dup, failed)
	}
	if len(received) != 4 {
		t.Fatalf("서버 수신 %d건, want 4", len(received))
	}
}

// 인터벌이 실제로 요청을 지연시키는지 (rate limit 경로) 최소 검증.
func TestSendLinksRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	urls := []string{"https://a.com/1", "https://a.com/2", "https://a.com/3"}
	interval := 20 * time.Millisecond

	start := time.Now()
	saved, _, _ := sendLinks(context.Background(), srv.Client(), srv.URL, "k", urls, interval, logger)
	elapsed := time.Since(start)

	if saved != 3 {
		t.Fatalf("saved=%d, want 3", saved)
	}
	// 3건이면 간격 2회 → 최소 2*interval 이상.
	if elapsed < 2*interval {
		t.Fatalf("elapsed %v < %v — rate limit 미적용", elapsed, 2*interval)
	}
}

// ---- 헬퍼 ----

func assertURLs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("URL 수 %d, want %d\n got=%v\nwant=%v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("URL[%d]=%q, want %q\n got=%v\nwant=%v", i, got[i], want[i], got, want)
		}
	}
}
