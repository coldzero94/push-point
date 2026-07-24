package tagger

import (
	"reflect"
	"slices"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"쿠버네티스를", "쿠버네티스"},  // 를 제거 (계획 08의 대표 예)
		{"자동으로", "자동"},       // 긴 조사 '으로' 우선 (짧은 '로'가 아니라)
		{"회의에서", "회의"},       // 에서 제거
		{"데이터가", "데이터"},      // 가 제거
		{"평가", "평가"},         // 어간 1자('평')이라 '가' 보존 — 과다 제거 방지
		{"나는", "나는"},         // 어간 1자('나')라 '는' 보존
		{"data를", "data"},    // 라틴+조사 혼합도 조사만 제거
		{"Hello", "hello"},   // 영어는 소문자화만
		{"said", "said"},     // 조사 접미 없음 — 그대로
		{"(쿠버네티스)", "쿠버네티스"}, // 가장자리 괄호 제거
		{"", ""},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTokenize(t *testing.T) {
	// "쿠버네티스를 처음 배우는 사람" → 조사 벗긴 어절, 순서 보존
	got := Tokenize("쿠버네티스를 처음 배우는 사람")
	if !slices.Contains(got, "쿠버네티스") {
		t.Errorf("Tokenize에 '쿠버네티스'가 있어야: %v", got)
	}

	// 영어 문장은 소문자 어절, 조사 제거 없음
	want := []string{"he", "said", "hello"}
	if got := Tokenize("He said, hello!"); !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize = %v, want %v", got, want)
	}

	// 구분자만 있으면 빈 슬라이스
	if got := Tokenize("!!! ,, --"); len(got) != 0 {
		t.Errorf("구분자만 있으면 빈 결과여야: %v", got)
	}
}
