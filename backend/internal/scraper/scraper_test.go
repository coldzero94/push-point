package scraper

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
)

// **한국 웹에는 아직 EUC-KR/CP949 페이지가 있다.** `news.naver.com`의 레거시 랭킹 페이지가
// 그렇고, UTF-8로 읽으면 제목이 `���̹� ����`가 된다 — wild 세트 16행이 그 상태로 박제돼
// 있다. 깨진 글자는 사전의 어떤 표면과도 안 맞으므로 그 링크는 태깅에서 통째로 사라지는데,
// 상태는 `done`이고 오류도 로그도 없다.
func TestParseHTMLCharsetDecodesEUCKR(t *testing.T) {
	title := "네이버 뉴스"
	euckr, err := korean.EUCKR.NewEncoder().Bytes([]byte(
		`<!doctype html><html><head><meta http-equiv="Content-Type" content="text/html; charset=euc-kr">` +
			`<title>` + title + `</title></head><body></body></html>`))
	if err != nil {
		t.Fatalf("픽스처 인코딩 실패: %v", err)
	}
	// 픽스처가 정말 UTF-8이 아니어야 시험이 성립한다.
	if utf8.Valid(euckr) {
		t.Fatal("픽스처가 UTF-8로 유효하다 — EUC-KR 경로를 시험하지 못한다")
	}

	doc, err := ParseHTMLCharset(euckr, "")
	if err != nil {
		t.Fatalf("ParseHTMLCharset: %v", err)
	}
	if got := doc.Find("title").Text(); got != title {
		t.Errorf("EUC-KR 제목을 못 읽었다: %q, want %q", got, title)
	}

	// Content-Type 헤더만 있고 meta가 없어도 판정돼야 한다(레거시 페이지가 흔한 형태).
	noMeta, _ := korean.EUCKR.NewEncoder().Bytes([]byte(
		`<!doctype html><html><head><title>` + title + `</title></head><body></body></html>`))
	doc2, err := ParseHTMLCharset(noMeta, "text/html; charset=EUC-KR")
	if err != nil {
		t.Fatalf("ParseHTMLCharset(헤더): %v", err)
	}
	if got := doc2.Find("title").Text(); got != title {
		t.Errorf("Content-Type 헤더로 판정하지 못했다: %q, want %q", got, title)
	}
}

// **정상 UTF-8은 손상되지 않아야 한다.** 이 프로젝트는 이미 문자셋 변환기 때문에 한국
// 기술블로그의 본문이 통째로 비는 회귀를 겪었다(trafilatura의 `transform: short internal
// buffer`). 그래서 변환을 추가할 때 UTF-8 무손상이 같이 고정돼야 한다.
func TestParseHTMLCharsetPreservesUTF8(t *testing.T) {
	for _, ct := range []string{"", "text/html", "text/html; charset=utf-8"} {
		html := []byte(`<!doctype html><html><head><title>쿠버네티스 도입기 — 우아한형제들</title>` +
			`</head><body><p>본문 內容 🚀</p></body></html>`)
		doc, err := ParseHTMLCharset(html, ct)
		if err != nil {
			t.Fatalf("ct=%q: %v", ct, err)
		}
		if got := doc.Find("title").Text(); got != "쿠버네티스 도입기 — 우아한형제들" {
			t.Errorf("ct=%q 제목 손상: %q", ct, got)
		}
		if got := doc.Find("p").Text(); got != "본문 內容 🚀" {
			t.Errorf("ct=%q 본문 손상: %q", ct, got)
		}
	}
}

// 알 수 없는 charset 이름이 와도 **문서가 온전해야** 한다.
//
// 이건 에러 폴백 분기를 재는 게 아니다 — charset.NewReader는 알 수 없는 이름을 에러로
// 취급하지 않고 sniffing으로 넘긴다(그래서 그 분기는 도달하지 않는다, ParseHTMLCharset
// 주석 참조). 여기서 지키는 것은 **판정을 위해 미리 읽은 1024바이트가 유실되지 않는 것**이다.
// 프리뷰 뒤로 원본을 다시 잇지 않는 구현이면 제목(앞)이나 본문 끝(뒤) 중 하나가 사라진다.
func TestParseHTMLCharsetKeepsWholeDocument(t *testing.T) {
	html := []byte(`<!doctype html><html><head><title>온전한 제목</title></head>` +
		`<body><p>` + strings.Repeat("채움 ", 500) + `끝</p></body></html>`)
	doc, err := ParseHTMLCharset(html, "text/html; charset=x-불명-인코딩")
	if err != nil {
		t.Fatalf("폴백이 에러를 냈다: %v", err)
	}
	if got := doc.Find("title").Text(); got != "온전한 제목" {
		t.Errorf("앞부분(제목)이 유실됐다: %q", got)
	}
	if !strings.HasSuffix(doc.Find("p").Text(), "끝") {
		t.Error("프리뷰 뒤로 원본을 다시 잇지 않아 뒷부분이 유실됐다")
	}
}
