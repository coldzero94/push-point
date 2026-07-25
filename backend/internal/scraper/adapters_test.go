package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// YouTube: oEmbed(title/author/thumbnail) + watch 페이지 og:description 병합, content_type=video.
func TestYouTubeAdapter(t *testing.T) {
	const oembedJSON = `{"title":"Go 동시성 강의","author_name":"채널이름","thumbnail_url":"https://i.ytimg.com/vi/abc/hq.jpg"}`
	// shortDescription(전체 설명)은 페이지 JSON에 있고 og:description보다 길다 — body_text 소스.
	const watchHTML = `<html lang="en"><head>
		<meta property="og:description" content="이 영상은 고루틴을 다룬다.">
		</head><body><script>var x = {"shortDescription":"고루틴과 채널을 처음부터 설명합니다.\n\n00:00 인트로\n05:00 채널","isLive":false};</script></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oembed":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(oembedJSON))
		case "/watch":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(watchHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// rewrite transport로 oEmbed·watch 요청 모두 fixture로 보낸다 (실제 youtube 미접속).
	client := newRewriteClient(t, srv.URL, nil)
	parser := NewDefaultParser(client, "test-agent")
	a := newYouTubeAdapter(parser, "https://www.youtube.com/oembed")

	if !a.Match(mustURL(t, "https://youtu.be/abc")) {
		t.Fatal("youtu.be가 Match되지 않음")
	}
	m, err := a.Fetch(context.Background(), mustURL(t, "https://www.youtube.com/watch?v=abc"))
	if err != nil {
		t.Fatalf("Fetch 에러: %v", err)
	}
	if m.Title != "Go 동시성 강의" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Author != "채널이름" {
		t.Errorf("Author = %q", m.Author)
	}
	if m.ImageURL != "https://i.ytimg.com/vi/abc/hq.jpg" {
		t.Errorf("ImageURL = %q", m.ImageURL)
	}
	if m.Description != "이 영상은 고루틴을 다룬다." {
		t.Errorf("Description(watch 병합) = %q", m.Description)
	}
	if m.ContentType != "video" {
		t.Errorf("ContentType = %q, want video", m.ContentType)
	}
	// 영상 본문 = 전체 설명(shortDescription). og:description(짧은 blurb)과 별개 필드다.
	if !strings.Contains(m.BodyText, "고루틴과 채널을 처음부터") || !strings.Contains(m.BodyText, "05:00 채널") {
		t.Errorf("BodyText(전체 설명) = %q", m.BodyText)
	}
}

func TestYouTubeDescription(t *testing.T) {
	cases := []struct {
		name, page, want string
	}{
		{"기본", `{"shortDescription":"안녕하세요","x":1}`, "안녕하세요"},
		{"이스케이프 개행", `{"shortDescription":"1줄\n2줄"}`, "1줄\n2줄"},
		{"이스케이프 따옴표", `{"shortDescription":"그는 \"안녕\"이라 했다"}`, `그는 "안녕"이라 했다`},
		// 설명 안에 `","`가 들어가도 깨지지 않아야 한다(정규식 축약 매칭의 실패 모드).
		{"본문에 따옴표-쉼표", `{"shortDescription":"a\",\"b 계속","author":"x"}`, `a","b 계속`},
		{"유니코드 이스케이프", `{"shortDescription":"가나"}`, "가나"},
		{"키 없음", `{"other":"x"}`, ""},
		{"닫는 따옴표 없음(잘린 페이지)", `{"shortDescription":"열린 채로`, ""},
	}
	for _, c := range cases {
		if got := youtubeDescription([]byte(c.page)); got != c.want {
			t.Errorf("%s: youtubeDescription = %q, want %q", c.name, got, c.want)
		}
	}
}

// YouTube: watch 페이지가 실패해도 oEmbed 결과는 유지된다 (best-effort 병합).
func TestYouTubeAdapterWatchFailsSoft(t *testing.T) {
	const oembedJSON = `{"title":"제목","author_name":"채널","thumbnail_url":"https://x/t.jpg"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oembed" {
			_, _ = w.Write([]byte(oembedJSON))
			return
		}
		http.Error(w, "blocked", http.StatusForbidden) // watch 페이지 차단
	}))
	defer srv.Close()

	client := newRewriteClient(t, srv.URL, nil)
	parser := NewDefaultParser(client, "test-agent")
	a := newYouTubeAdapter(parser, "https://www.youtube.com/oembed")

	m, err := a.Fetch(context.Background(), mustURL(t, "https://www.youtube.com/watch?v=z"))
	if err != nil {
		t.Fatalf("watch 실패는 에러가 아니어야 함: %v", err)
	}
	if m.Title != "제목" || m.ContentType != "video" {
		t.Errorf("oEmbed 결과가 유지되지 않음: %+v", m)
	}
}

// X/Twitter: publish.twitter.com/oembed의 author_name·본문(html) 추출, content_type=post.
// 실제 oEmbed html은 blockquote 안에 본문 <p>가 있고 그 뒤에 "&mdash; 작성자 (@handle)
// 날짜" 귀속이 붙는다 — description에는 <p> 본문만 담기고 귀속 접미사가 섞이면 안 된다.
func TestTwitterAdapter(t *testing.T) {
	// publish.twitter.com/oembed 대표 형태: blockquote > p(본문) + 뒤따르는 작성자·날짜 귀속.
	const oembedJSON = `{"author_name":"홍길동","html":"<blockquote class=\"twitter-tweet\"><p lang=\"ko\" dir=\"ltr\">트윗 본문 <a href=\"https://t.co/abc\">링크</a></p>&mdash; 홍길동 (@hong) <a href=\"https://twitter.com/hong/status/1\">2026년 3월 5일</a></blockquote>"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oembed" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(oembedJSON))
	}))
	defer srv.Close()

	client := newRewriteClient(t, srv.URL, nil)
	a := newTwitterAdapter(client, "test-agent", "https://publish.twitter.com/oembed")

	if !a.Match(mustURL(t, "https://x.com/u/status/1")) {
		t.Fatal("x.com이 Match되지 않음")
	}
	m, err := a.Fetch(context.Background(), mustURL(t, "https://twitter.com/u/status/1"))
	if err != nil {
		t.Fatalf("Fetch 에러: %v", err)
	}
	if m.Author != "홍길동" {
		t.Errorf("Author = %q", m.Author)
	}
	// blockquote 안 <p> 본문만 — 귀속 접미사(작성자·핸들·날짜)가 없어야 한다.
	if m.Description != "트윗 본문 링크" {
		t.Errorf("Description = %q, want %q (귀속 접미사 오염 없음)", m.Description, "트윗 본문 링크")
	}
	for _, banned := range []string{"@hong", "2026년", "홍길동 ("} {
		if strings.Contains(m.Description, banned) {
			t.Errorf("Description에 귀속 조각 %q 가 섞임: %q", banned, m.Description)
		}
	}
	if m.ContentType != "post" {
		t.Errorf("ContentType = %q, want post", m.ContentType)
	}
}

// 네이버 블로그: blog.naver.com → m.blog.naver.com 재작성 후 기본 파서.
func TestNaverAdapterRewritesHost(t *testing.T) {
	const blogHTML = `<html lang="ko"><head>
		<meta property="og:title" content="네이버 블로그 글 제목">
		</head><body></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(blogHTML))
	}))
	defer srv.Close()

	var seenHost string
	client := newRewriteClient(t, srv.URL, &seenHost)
	parser := NewDefaultParser(client, "test-agent")
	a := newNaverAdapter(parser)

	m, err := a.Fetch(context.Background(), mustURL(t, "https://blog.naver.com/someid/223456"))
	if err != nil {
		t.Fatalf("Fetch 에러: %v", err)
	}
	// 재작성으로 실제 요청 호스트가 m.blog.naver.com이어야 한다.
	if seenHost != "m.blog.naver.com" {
		t.Errorf("요청 호스트 = %q, want m.blog.naver.com", seenHost)
	}
	if m.Title != "네이버 블로그 글 제목" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.ContentType != "article" {
		t.Errorf("ContentType = %q, want article", m.ContentType)
	}
}

// Instagram: 메타 부재 허용 — 빈 메타데이터, 에러 없음, HTTP 요청조차 하지 않음.
func TestInstagramAdapterNoError(t *testing.T) {
	// 어떤 요청이든 오면 실패시키는 클라이언트 — 어댑터가 HTTP를 안 하는지 검증.
	failClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("instagram 어댑터가 HTTP 요청을 했다")
		return nil, nil
	})}
	_ = failClient // 어댑터는 client를 받지 않지만, 명시적으로 네트워크 미사용을 문서화.

	a := newInstagramAdapter()
	if !a.Match(mustURL(t, "https://www.instagram.com/p/xyz/")) {
		t.Fatal("instagram이 Match되지 않음")
	}
	m, err := a.Fetch(context.Background(), mustURL(t, "https://www.instagram.com/p/xyz/"))
	if err != nil {
		t.Fatalf("instagram은 에러가 아니어야 함: %v", err)
	}
	if m.Title != "" || m.Description != "" || m.ImageURL != "" {
		t.Errorf("빈 메타데이터 기대, got %+v", m)
	}
	if m.ContentType != "post" {
		t.Errorf("ContentType = %q, want post", m.ContentType)
	}
}

// roundTripFunc는 http.RoundTripper 어댑터.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
