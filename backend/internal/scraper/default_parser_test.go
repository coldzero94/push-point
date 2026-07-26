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

// 발행자 분류 수집 — 네 출처를 다 모으고 합집합을 쓴다.
//
// 이게 태거 개선의 핵심이라 출처별로 따로 확인한다. 사이트마다 쓰는 태그가 다르므로
// 하나라도 조용히 빠지면 **그 사이트에서만** 신호가 사라지고, 다른 사이트는 멀쩡하니
// 눈으로는 안 잡힌다.
func TestParseKeywords(t *testing.T) {
	cases := []struct {
		name string
		html string
		want []string // 결과에 반드시 있어야 하는 조각들
	}{
		{
			name: "meta keywords",
			html: `<head><meta name="keywords" content="go, goroutine, channel"></head>`,
			want: []string{"go", "goroutine", "channel"},
		},
		{
			name: "news_keywords (Google News 규격)",
			html: `<head><meta name="news_keywords" content="이강인,축구"></head>`,
			want: []string{"이강인", "축구"},
		},
		{
			name: "article:section·article:tag 여러 개",
			html: `<head>
				<meta property="article:section" content="스포츠">
				<meta property="article:tag" content="해외축구">
				<meta property="article:tag" content="이적시장">
			</head>`,
			want: []string{"스포츠", "해외축구", "이적시장"},
		},
		{
			name: "JSON-LD articleSection",
			html: `<head><script type="application/ld+json">
				{"@type":"NewsArticle","articleSection":"정치","keywords":["국회","예산"]}
			</script></head>`,
			want: []string{"정치", "국회", "예산"},
		},
		{
			name: "JSON-LD @graph 중첩",
			html: `<head><script type="application/ld+json">
				{"@graph":[{"@type":"WebSite"},{"@type":"Article","articleSection":["경제","부동산"]}]}
			</script></head>`,
			want: []string{"경제", "부동산"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ParseHTML(strings.NewReader("<!doctype html><html>" + tc.html + "<body></body></html>"))
			if err != nil {
				t.Fatal(err)
			}
			got := parseKeywords(doc)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("분류 %q가 빠졌다: %q", w, got)
				}
			}
		})
	}
}

// 같은 값이 여러 출처에 중복돼 있어도 한 번만 남아야 한다 — 뉴스 사이트는 keywords와
// article:tag에 같은 낱말을 넣는 일이 흔한데, 그대로 두면 512바이트를 중복이 잡아먹는다.
func TestParseKeywords_dedupes(t *testing.T) {
	doc, err := ParseHTML(strings.NewReader(`<!doctype html><html><head>
		<meta name="keywords" content="축구, Football">
		<meta property="article:tag" content="축구">
		<meta property="article:section" content="football">
	</head><body></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	got := parseKeywords(doc)
	if n := strings.Count(got, "축구"); n != 1 {
		t.Errorf("중복 제거 실패 — %q에 축구가 %d번", got, n)
	}
	// 대소문자만 다른 값도 같은 것으로 본다.
	if n := strings.Count(strings.ToLower(got), "football"); n != 1 {
		t.Errorf("대소문자 중복 제거 실패: %q", got)
	}
}

// 깨진 JSON-LD는 흔하다(광고 스크립트가 끼어들거나 템플릿이 새는 경우). 보조 신호이므로
// 조용히 넘기고 나머지 출처는 그대로 살아야 한다 — 여기서 panic이 나면 스크랩 잡 전체가 죽는다.
func TestParseKeywords_brokenJSONLDDoesNotBreakOthers(t *testing.T) {
	doc, err := ParseHTML(strings.NewReader(`<!doctype html><html><head>
		<meta name="keywords" content="정상값">
		<script type="application/ld+json">{ 이건 JSON이 아니다 </script>
	</head><body></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := parseKeywords(doc); !strings.Contains(got, "정상값") {
		t.Errorf("깨진 JSON-LD가 다른 출처를 삼켰다: %q", got)
	}
}
