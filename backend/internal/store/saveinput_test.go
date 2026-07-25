package store

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// Normalize는 **모든 진입점**(HTTP 핸들러 / 임베드 iOS의 로컬 큐 드레인)이 공유하는
// 검증·정제다. 여기가 깨지면 어떤 경로로 들어왔느냐에 따라 저장 형태가 갈라진다.
func TestSaveInputNormalize_url(t *testing.T) {
	bad := []string{"", "   ", "not a url", "ftp://x/y", "//host/path", "javascript:alert(1)", "https://"}
	for _, u := range bad {
		if _, err := (SaveInput{URL: u}).Normalize(); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("URL %q = %v, want ErrInvalidURL", u, err)
		}
	}
	good := []string{"https://a.example/x", "http://a.example", "  https://a.example/x  "}
	for _, u := range good {
		got, err := (SaveInput{URL: u}).Normalize()
		if err != nil {
			t.Errorf("URL %q = %v, want nil", u, err)
			continue
		}
		if got.URL != strings.TrimSpace(u) {
			t.Errorf("URL 공백 정리 = %q", got.URL)
		}
	}
}

func TestSaveInputNormalize_fields(t *testing.T) {
	in := SaveInput{
		URL:         "https://a.example/x",
		Title:       "제목\x00제어\n줄바꿈",
		Description: "설명\t탭",
		BodyText:    "첫 문장.\n둘째 문장.\n" + strings.Repeat("가", 50000),
	}
	got, err := in.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(got.Title, 0) || strings.Contains(got.Title, "\n") {
		t.Errorf("제목은 제어문자·개행이 없어야: %q", got.Title)
	}
	if strings.Contains(got.Description, "\t") {
		t.Errorf("설명의 탭은 접혀야: %q", got.Description)
	}
	if !strings.Contains(got.BodyText, "\n") {
		t.Error("body_text는 개행을 유지해야 한다(요약이 문장 구분에 쓴다)")
	}
	if len(got.BodyText) > 32<<10 || !utf8.ValidString(got.BodyText) {
		t.Errorf("body_text 절단 불량: %d바이트", len(got.BodyText))
	}
}

// SaveLink는 Normalize를 스스로 부른다 — 진입점이 이걸 우회할 수 없다는 게 이 구조의 요점이다.
func TestSaveLinkNormalizesAndRejectsBadURL(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := t.Context()
	if _, _, _, err := s.SaveLink(ctx, SaveInput{URL: "ftp://nope/x"}); !errors.Is(err, ErrInvalidURL) {
		t.Errorf("잘못된 URL = %v, want ErrInvalidURL", err)
	}
	id, _, _, err := s.SaveLink(ctx, SaveInput{
		URL: "  https://a.example/x  ", Title: "제목\x07", BodyText: strings.Repeat("나", 50000),
	})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := s.GetLinkContent(ctx, id)
	if strings.ContainsRune(c.Title, 7) {
		t.Errorf("제어문자가 저장됨: %q", c.Title)
	}
	if len(c.Body) > 32<<10 {
		t.Errorf("본문이 상한을 넘겨 저장됨: %d바이트", len(c.Body))
	}
}
