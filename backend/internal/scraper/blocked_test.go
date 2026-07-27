package scraper

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// 실측된 벽 세 개를 그대로 넣는다 — 문구를 지어내면 실제로 오는 것과 다른 것을 지키게 된다.
// 셋 다 2026-07-26 wild 캡처에서 나온 형태다.
func TestIsBlockedPage_RealWalls(t *testing.T) {
	cases := []struct {
		name              string
		title, desc, body string
	}{
		{
			"imdb·dribbble 봇 차단 (본문 166자, 제목 없음)",
			"", "",
			"<h1>JavaScript is disabled</h1> In order to continue, we need to verify that you're not a robot. This requires JavaScript. Enable JavaScript and then reload the page.",
		},
		{
			"Reddit 인증 대기 (제목에만 있다)",
			"Reddit - Please wait for verification", "",
			"Reddit - Please wait for verification",
		},
		{
			// **제목에만** 있는 경우를 따로 세운다. Reddit 실측은 제목과 본문에 같은 문구가
			// 있어서 본문만 봐도 잡히는데, 그러면 "제목을 보는가"가 시험되지 않는다.
			// 본문 추출이 실패해 제목만 남는 형태는 실제로 흔하다.
			"제목만 벽 (본문 추출 실패)",
			"Attention Required! | Cloudflare", "", "",
		},
		{
			"threads 로그인 벽 (한국어)",
			"Threads • 로그인",
			"Threads에 가입하여 아이디어를 나누고, 질문을 남기고, 떠오르는 생각을 게시해보세요.",
			"홈 검색 만들기 알림 프로필 고정 더 보기 홈 Threads에 로그인 또는 가입하기 사람들의 이야기를 확인하고 대화에 참여해보세요.",
		},
	}
	for _, c := range cases {
		if !isBlockedPage(c.title, c.desc, c.body) {
			t.Errorf("%s — 벽으로 판정하지 못했다", c.name)
		}
	}
}

// **오탐이 이 규칙의 진짜 위험이다.** 정상 페이지를 벽으로 보면 링크가 통째로 실패한다.
func TestIsBlockedPage_RealPages(t *testing.T) {
	long := strings.Repeat("본문이 실하게 이어진다. ", 60) // 400자 훌쩍 넘김

	cases := []struct {
		name              string
		title, desc, body string
	}{
		{
			"짧지만 진짜인 페이지 (스포티파이 곡, 실측 143자)",
			"Never Gonna Give You Up",
			"Rick Astley · Whenever You Need Somebody · Song · 1987",
			"Never Gonna Give You Up - song and lyrics by Rick Astley | Spotify",
		},
		{
			"봇 차단을 **다룬** 긴 기사 — 문구가 있어도 길면 벽이 아니다",
			"CAPTCHA는 어떻게 작동하는가",
			"not a robot 체크박스의 내부 동작을 뜯어본다",
			long,
		},
		{
			"로그인을 다룬 긴 글",
			"소셜 로그인 설계",
			"로그인이 필요한 화면을 어떻게 나눌 것인가",
			long,
		},
		{"전부 빈 페이지 — 벽은 아니다(다른 경로가 다룬다)", "", "", ""},
	}
	for _, c := range cases {
		if isBlockedPage(c.title, c.desc, c.body) {
			t.Errorf("%s — 정상 페이지를 벽으로 판정했다", c.name)
		}
	}
}

// 길이와 문구는 **곱**이다 — 어느 하나만으로는 판정하지 않는다.
func TestIsBlockedPage_NeedsBothConditions(t *testing.T) {
	// 짧지만 벽 문구가 없다 → 벽 아님
	if isBlockedPage("제목", "설명", "짧은 본문") {
		t.Error("문구 없이 길이만으로 벽 판정을 하면 안 된다")
	}
	// 벽 문구가 있지만 길다 → 벽 아님
	longWall := "javascript is disabled " + strings.Repeat("그리고 실제 내용이 길게 이어진다. ", 40)
	if isBlockedPage("", "", longWall) {
		t.Error("길면 문구가 있어도 벽으로 보면 안 된다")
	}
	// 경계: 문턱 바로 아래는 벽, 바로 위는 아님
	base := "javascript is disabled "
	justUnder := base + strings.Repeat("가", blockedMaxRunes-len([]rune(base))-3)
	if !isBlockedPage("", "", justUnder) {
		t.Errorf("문턱 아래(%d자)는 벽이어야 한다", len([]rune(justUnder))+2)
	}
	justOver := base + strings.Repeat("가", blockedMaxRunes)
	if isBlockedPage("", "", justOver) {
		t.Error("문턱 위는 벽이 아니어야 한다")
	}
}

// 판정이 실제로 Fetch를 실패시키는지 — 예전에는 이런 응답이 200 + `done`으로 저장됐다.
func TestFetchReturnsErrBlockedPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// 실제 imdb·dribbble 응답과 같은 형태: 200 + 짧은 안내문.
		_, _ = io.WriteString(w, `<!doctype html><html><head><title></title></head><body>`+
			`<h1>JavaScript is disabled</h1> In order to continue, we need to verify that `+
			`you're not a robot. This requires JavaScript.</body></html>`)
	}))
	defer srv.Close()

	p := NewDefaultParser(srv.Client(), "")
	u, _ := url.Parse(srv.URL)
	m, err := p.Fetch(context.Background(), u)
	if !errors.Is(err, ErrBlockedPage) {
		t.Fatalf("벽 응답이 ErrBlockedPage여야 한다: err=%v", err)
	}
	// **메타데이터를 남기면 안 된다** — 벽의 제목이 목록에 뜨는 것이 결함 C였다.
	if m.Title != "" || m.BodyText != "" {
		t.Errorf("벽의 내용이 반환됐다: title=%q body=%q", m.Title, m.BodyText)
	}
}

// 정상 페이지는 그대로 통과해야 한다 — 이 변경이 낼 수 있는 회귀가 정확히 그것이다.
func TestFetchPassesNormalPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><html><head><title>쿠버네티스 도입기</title>`+
			`<meta property="og:description" content="클러스터 마이그레이션 기록"></head><body><article><p>`+
			strings.Repeat("실제 본문이 이어진다. ", 60)+`</p></article></body></html>`)
	}))
	defer srv.Close()

	p := NewDefaultParser(srv.Client(), "")
	u, _ := url.Parse(srv.URL)
	m, err := p.Fetch(context.Background(), u)
	if err != nil {
		t.Fatalf("정상 페이지가 실패했다: %v", err)
	}
	if m.Title != "쿠버네티스 도입기" {
		t.Errorf("제목이 안 나왔다: %q", m.Title)
	}
}
