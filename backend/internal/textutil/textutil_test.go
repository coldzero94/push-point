package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapRunes(t *testing.T) {
	cases := []struct {
		in    string
		limit int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 3, "hel"},
		{"한국어", 4, "한"}, // 3바이트 룬 — 경계에서 끊는다
		{"한국어", 3, "한"},
		{"한국어", 2, ""},       // 첫 룬도 안 들어감
		{"anything", 0, ""},  // 상한 0
		{"anything", -1, ""}, // 음수여도 패닉하지 않는다
	}
	for _, c := range cases {
		got := CapRunes(c.in, c.limit)
		if got != c.want {
			t.Errorf("CapRunes(%q, %d) = %q, want %q", c.in, c.limit, got, c.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("CapRunes(%q, %d) 결과가 유효 UTF-8이 아님", c.in, c.limit)
		}
	}
}

func TestSanitizeText(t *testing.T) {
	// C0 제어문자는 제거, 탭·CR은 공백, 개행은 옵션.
	in := "정상\x00제어\x07문자\t탭\r캐리지\n개행"
	if got := SanitizeText(in, true); got != "정상제어문자 탭 캐리지\n개행" {
		t.Errorf("allowNewline=true → %q", got)
	}
	if got := SanitizeText(in, false); got != "정상제어문자 탭 캐리지 개행" {
		t.Errorf("allowNewline=false → %q", got)
	}
	// 불완전 UTF-8 바이트는 버린다(결과는 항상 유효 UTF-8).
	got := SanitizeText("정상"+string([]byte{0xff, 0xfe})+"끝", true)
	if !utf8.ValidString(got) {
		t.Errorf("유효 UTF-8이 아님: %q", got)
	}
	if !strings.Contains(got, "정상") || !strings.Contains(got, "끝") {
		t.Errorf("정상 문자가 사라짐: %q", got)
	}
}

func TestClean(t *testing.T) {
	// 정제 후 상한을 넘지 않고, 앞뒤 공백이 없어야 한다.
	long := strings.Repeat("가", 100)
	got := Clean("  "+long+"  ", 30, false)
	if len(got) > 30 {
		t.Errorf("상한 초과: %d바이트", len(got))
	}
	if !utf8.ValidString(got) || strings.TrimSpace(got) != got {
		t.Errorf("정제 결과 불량: %q", got)
	}
	// 개행 정책이 필드별로 다르다 — body_text만 개행을 남긴다.
	if strings.Contains(Clean("a\nb", 100, false), "\n") {
		t.Error("allowNewline=false인데 개행이 남음")
	}
	if !strings.Contains(Clean("a\nb", 100, true), "\n") {
		t.Error("allowNewline=true인데 개행이 사라짐")
	}
	if Clean("무엇이든", 0, true) != "" {
		t.Error("상한 0이면 빈 문자열")
	}
}
