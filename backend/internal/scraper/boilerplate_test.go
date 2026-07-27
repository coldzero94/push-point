package scraper

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// 세 케이스 모두 2026-07-27 golden 실측에서 그대로 가져왔다 — 문구를 지어내면 실제로 오는
// 것과 다른 것을 지키게 된다. 셋의 성격이 다르다는 점이 이 규칙의 설계 근거다.
func TestIsFooterOnly(t *testing.T) {
	melon := "멜론 스튜디오 Windows 플레이어 Mac 플레이어 iPad 고객센터 이용약관 " +
		"위치기반서비스 이용약관 개인정보처리방침 청소년보호정책 제휴/프로모션문의 " +
		"이메일주소무단수집거부 파트너센터 (주)카카오엔터테인먼트 경기도 성남시 분당구 " +
		"판교역로 235 공동대표이사 : 고정희 사업자등록번호 : 220-88-02594 " +
		"통신판매업신고번호 : 2018-성남분당B-0004"
	if !isFooterOnly(melon) {
		t.Error("멜론: 355자 전부가 회사 정보·약관인데 푸터로 판정하지 못했다")
	}

	// **진짜 내용인데 표식이 곁다리로 섞인 경우** — 이걸 버리면 앱 설명이 통째로 사라진다.
	// 앱스토어 토스 페이지가 실측 2,428자에 표식 2개였다.
	appstore := "토스 금융이 쉬워진다 iPhone 전용 무료 · 앱 내 구입 " +
		strings.Repeat("내 금융 현황을 한눈에 확인하고 송금과 결제를 한 곳에서 처리합니다. ", 40) +
		" 고객센터 이용약관"
	if isFooterOnly(appstore) {
		t.Error("진짜 앱 설명을 푸터로 판정했다 — 길이를 안 보면 이렇게 된다")
	}

	// 길이와 개수는 **곱**이다.
	if isFooterOnly("고객센터 이용약관") {
		t.Errorf("표식 2개는 부족하다(문턱 %d개)", footerMinMarkers)
	}
	if isFooterOnly("") {
		t.Error("빈 본문은 푸터가 아니다 — 다른 경로가 다룬다")
	}
	long := strings.Repeat("실제 본문이 길게 이어진다. ", 100) + " 고객센터 이용약관 개인정보처리방침 사업자등록번호"
	if isFooterOnly(long) {
		t.Errorf("긴 본문은 표식이 많아도 푸터가 아니다(문턱 %d자)", footerMaxRunes)
	}
}

// 판정이 실제로 추출 경로에 걸려 있는지 — 단위 테스트만 있으면 호출을 지워도 통과한다.
func TestExtractBodyTextDropsFooterOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// 본문 영역에 하단 고지밖에 없는 페이지 — 멜론 곡 페이지가 이 형태였다.
		_, _ = io.WriteString(w, `<!doctype html><html><head>`+
			`<title>Marching Horns - 横山克</title>`+
			`<meta property="og:description" content="음악이 필요한 순간, 멜론"></head><body><article><p>`+
			`고객센터 이용약관 위치기반서비스 이용약관 개인정보처리방침 청소년보호정책 `+
			`제휴/프로모션문의 파트너센터 (주)카카오엔터테인먼트 경기도 성남시 분당구 판교역로 235 `+
			`공동대표이사 : 고정희 사업자등록번호 : 220-88-02594 통신판매업신고번호 : 2018-성남분당B-0004`+
			`</p></article></body></html>`)
	}))
	defer srv.Close()

	p := NewDefaultParser(srv.Client(), "")
	u, _ := url.Parse(srv.URL)
	m, err := p.Fetch(context.Background(), u)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if m.BodyText != "" {
		t.Errorf("하단 고지가 본문으로 저장됐다 (%d자): %.80q", len([]rune(m.BodyText)), m.BodyText)
	}
	// **제목·설명은 남아야 한다** — 곡 정보가 거기 있고, 링크가 쓸모없어지면 안 된다.
	if m.Title == "" {
		t.Error("제목까지 사라졌다")
	}
}
