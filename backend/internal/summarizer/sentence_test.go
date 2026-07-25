package summarizer

import (
	"slices"
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
	}{
		{
			"한국어 마침표 종결",
			"쿠버네티스는 컨테이너 오케스트레이터다. 파드가 최소 배포 단위다. 서비스가 트래픽을 분산한다.",
			[]string{"쿠버네티스는 컨테이너 오케스트레이터다.", "파드가 최소 배포 단위다.", "서비스가 트래픽을 분산한다."},
		},
		{
			"영어 + 물음표·느낌표",
			"What is a neural network? It is a function. Really!",
			[]string{"What is a neural network?", "It is a function.", "Really!"},
		},
		{
			"URL·도메인은 자르지 않는다", // G3: 마침표 뒤에 공백이 없다
			"자세한 건 example.com 문서를 보라. 끝이다.",
			[]string{"자세한 건 example.com 문서를 보라.", "끝이다."},
		},
		{
			"소수점", // G2
			"버전 3.14 릴리스가 나왔다. 다음은 4.0이다.",
			[]string{"버전 3.14 릴리스가 나왔다.", "다음은 4.0이다."},
		},
		{
			"약어 e.g.는 문장 끝이 아니다", // G4
			"여러 언어, e.g. Go와 Rust를 쓴다. 둘 다 빠르다.",
			[]string{"여러 언어, e.g. Go와 Rust를 쓴다.", "둘 다 빠르다."},
		},
		{
			"이니셜", // G4
			"저자는 J. Smith 이다. 그는 유명하다.",
			[]string{"저자는 J. Smith 이다.", "그는 유명하다."},
		},
		{
			"말줄임", // G1
			"글쎄... 잘 모르겠다. 다음에 보자.",
			[]string{"글쎄... 잘 모르겠다.", "다음에 보자."},
		},
		{
			"닫는 따옴표·괄호는 현재 문장이 흡수",
			`그는 "안녕." 이라 했다. 나는 웃었다.`,
			[]string{`그는 "안녕." 이라 했다.`, "나는 웃었다."},
		},
		{
			"소문자로 이어지면 문장 끝이 아니다", // G5
			"파일은 main.go 다. 실행하면 된다.",
			[]string{"파일은 main.go 다.", "실행하면 된다."},
		},
		{
			"개행은 무조건 경계", // R0
			"첫 줄\n둘째 줄",
			[]string{"첫 줄", "둘째 줄"},
		},
		{"빈 입력", "", nil},
		{"종결부호 없는 한 덩어리", "종결부호가 전혀 없는 텍스트", []string{"종결부호가 전혀 없는 텍스트"}},
	}
	for _, c := range cases {
		got := Split(c.in)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s:\n  Split(%q)\n  = %q\n  want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestIsProse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"정상 한국어 산문", "쿠버네티스는 컨테이너를 오케스트레이션하는 시스템으로 널리 쓰인다.", true},
		{"정상 영어 산문", "A neural network is a function that maps inputs to outputs by layers.", true},
		{"너무 짧음", "짧다.", false},
		{"종결부호 없음", strings.Repeat("가나다라마바사", 5), false},
		{"코드 덤프", "if (x == 1) { return y; } else { return z; } // done.", false},
		{"숫자 표", "1. 10 2. 20 3. 30 4. 40 5. 50 6. 60 7. 70 8. 80 9. 90 10.", false},
		{"너무 김", strings.Repeat("아주 긴 문장이 계속 이어진다. ", 40), false},
	}
	for _, c := range cases {
		if got := IsProse(c.in); got != c.want {
			t.Errorf("%s: IsProse(%.40q) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}
