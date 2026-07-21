package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fixture HTML — og 태그가 온전한 문서.
const htmlFull = `<!doctype html>
<html lang="ko">
<head>
<title>페이지 타이틀 (무시됨)</title>
<meta property="og:title" content="고루틴과 채널 완벽 가이드">
<meta property="og:description" content="Go 동시성의 핵심 개념을 예제로 설명한다.">
<meta property="og:site_name" content="벨로그">
<meta name="author" content="홍길동">
<meta name="keywords" content="go,goroutine,channel">
<meta property="og:image" content="/img/cover.png">
<meta property="article:published_time" content="2026-03-15T09:30:00Z">
</head>
<body><p>본문</p></body>
</html>`

// og 태그가 없고 <title>만 있는 문서.
const htmlTitleOnly = `<!doctype html>
<html><head><title>  제목만 있는 문서  </title></head><body></body></html>`

// 메타·타이틀이 전무한 문서.
const htmlEmpty = `<!doctype html><html><head></head><body>내용</body></html>`

func TestDefaultParserFetch(t *testing.T) {
	pages := map[string]string{
		"/full":  htmlFull,
		"/title": htmlTitleOnly,
		"/empty": htmlEmpty,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	parser := NewDefaultParser(srv.Client(), "test-agent")

	tests := []struct {
		name        string
		path        string
		wantTitle   string
		wantDesc    string
		wantAuthor  string
		wantSite    string
		wantLang    string
		wantImage   string // 상대 경로가 절대 URL로 해석되는지 확인 (srv.URL + /img/cover.png)
		wantPubUnix int64  // 0이면 nil 기대
	}{
		{
			name: "og 태그 완비", path: "/full",
			wantTitle: "고루틴과 채널 완벽 가이드", wantDesc: "Go 동시성의 핵심 개념을 예제로 설명한다.",
			wantAuthor: "홍길동", wantSite: "벨로그", wantLang: "ko",
			wantImage: srv.URL + "/img/cover.png", wantPubUnix: 1773567000,
		},
		{
			name: "title만", path: "/title",
			wantTitle: "제목만 있는 문서",
		},
		{
			name: "전무", path: "/empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parser.Fetch(context.Background(), mustURL(t, srv.URL+tc.path))
			if err != nil {
				t.Fatalf("Fetch 에러: %v", err)
			}
			if m.Title != tc.wantTitle {
				t.Errorf("Title = %q, want %q", m.Title, tc.wantTitle)
			}
			if m.Description != tc.wantDesc {
				t.Errorf("Description = %q, want %q", m.Description, tc.wantDesc)
			}
			if m.Author != tc.wantAuthor {
				t.Errorf("Author = %q, want %q", m.Author, tc.wantAuthor)
			}
			if m.SiteName != tc.wantSite {
				t.Errorf("SiteName = %q, want %q", m.SiteName, tc.wantSite)
			}
			if m.Lang != tc.wantLang {
				t.Errorf("Lang = %q, want %q", m.Lang, tc.wantLang)
			}
			if m.ImageURL != tc.wantImage {
				t.Errorf("ImageURL = %q, want %q", m.ImageURL, tc.wantImage)
			}
			if tc.wantPubUnix == 0 {
				if m.PublishedAt != nil {
					t.Errorf("PublishedAt = %v, want nil", *m.PublishedAt)
				}
			} else {
				if m.PublishedAt == nil || *m.PublishedAt != tc.wantPubUnix {
					t.Errorf("PublishedAt = %v, want %d", m.PublishedAt, tc.wantPubUnix)
				}
			}
			// 기본 파서는 임의 호스트 → content_type=article.
			if m.ContentType != "article" {
				t.Errorf("ContentType = %q, want article", m.ContentType)
			}
		})
	}
}

// 호스트가 바뀌는 리다이렉트(A→B) 후, 최종 페이지가 상대 og:image(/img/x.png)를 주면
// finalURL(B 호스트) 기준으로 절대화돼야 한다 — 원래 요청 호스트(A)가 아니라.
func TestDefaultParserResolvesRelativeImageAgainstRedirectedHost(t *testing.T) {
	// B: 최종 페이지 — 상대 og:image를 준다. 호스트(포트)가 A와 다르다.
	const finalHTML = `<!doctype html><html lang="ko"><head>
<meta property="og:title" content="리다이렉트 최종 페이지">
<meta property="og:image" content="/img/x.png">
</head><body></body></html>`
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(finalHTML))
	}))
	defer srvB.Close()

	// A: B의 /final로 302 리다이렉트.
	srvA := httptest.NewServer(http.RedirectHandler(srvB.URL+"/final", http.StatusFound))
	defer srvA.Close()

	// http.DefaultClient(가드 없음)로 A→B 리다이렉트를 따라간다 (둘 다 127.0.0.1 httptest).
	parser := NewDefaultParser(nil, "test-agent")
	m, err := parser.Fetch(context.Background(), mustURL(t, srvA.URL+"/start"))
	if err != nil {
		t.Fatalf("Fetch 에러: %v", err)
	}
	want := srvB.URL + "/img/x.png"
	if m.ImageURL != want {
		t.Fatalf("ImageURL = %q, want %q (finalURL=B 호스트 기준 절대화)", m.ImageURL, want)
	}
	// 원래 요청 호스트(A) 기준으로 잘못 절대화되지 않았음을 명시적으로 확인.
	if strings.Contains(m.ImageURL, srvA.Listener.Addr().String()) {
		t.Fatalf("ImageURL이 원래 호스트(A) 기준으로 절대화됨: %q", m.ImageURL)
	}
}

// 2xx가 아니면 에러여야 한다 (재시도 유발).
func TestDefaultParserNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	parser := NewDefaultParser(srv.Client(), "test-agent")
	if _, err := parser.Fetch(context.Background(), mustURL(t, srv.URL+"/x")); err == nil {
		t.Fatal("500 응답인데 에러가 nil")
	}
}

func TestContentTypeFor(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"www.youtube.com", "video"},
		{"youtube.com", "video"},
		{"youtu.be", "video"},
		{"m.youtube.com", "video"},
		{"player.vimeo.com", "video"},
		{"twitter.com", "post"},
		{"x.com", "post"},
		{"www.instagram.com", "post"},
		{"medium.com", "article"},
		{"blog.naver.com", "article"},
		{"example.com:8080", "article"}, // 포트 포함
	}
	for _, tc := range tests {
		if got := contentTypeFor(tc.host); got != tc.want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", tc.host, got, tc.want)
		}
	}
}
