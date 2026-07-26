package tagger

import (
	"reflect"
	"slices"
	"testing"

	"golang.org/x/text/unicode/norm"
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

// 한글 NFD 입력이 NFC와 같게 처리돼야 한다.
//
// "한"은 U+D55C 한 글자로도, "ᄒ+ᅡ+ᆫ"(U+1112 U+1161 U+11AB) 세 코드포인트로도 쓴다.
// 눈에는 똑같고 바이트는 다르다. 정규화가 없으면 사전 매칭이 통째로 빗나가는데,
// **증상이 조용하다** — 태그가 덜 붙을 뿐 오류도 로그도 없다.
//
// 실측(2026-07-26): golden 전 필드를 NFD로 바꾸면 dev 0.952 → 0.710,
// test 0.885 → 0.689. 나빠지는 정도가 아니라 한국어 태깅의 3분의 1이 사라진다.
// macOS 파일명이 NFD이고 일부 웹 폼·클립보드·iOS 공유 경로가 그대로 넘긴다.
func TestNormalize_composesHangulNFD(t *testing.T) {
	cases := []string{"쿠버네티스", "개발자", "머신러닝을", "책"}
	for _, nfc := range cases {
		nfd := norm.NFD.String(nfc)
		if nfd == nfc {
			t.Fatalf("%q가 NFD에서 안 바뀐다 — 테스트 픽스처가 잘못됐다", nfc)
		}
		if got, want := Normalize(nfd), Normalize(nfc); got != want {
			t.Errorf("NFD와 NFC가 다르게 정규화된다: %q → %q, %q → %q", nfd, got, nfc, want)
		}
	}
}

// 토크나이즈 전체 경로에서도 같아야 한다 — Normalize만 맞고 Tokenize가 다르게 쪼개면
// 매칭은 여전히 빗나간다.
func TestTokenize_nfdMatchesNfc(t *testing.T) {
	nfc := "쿠버네티스 클러스터에서 파드를 배포한다"
	nfd := norm.NFD.String(nfc)
	a, b := Tokenize(nfc), Tokenize(nfd)
	if len(a) != len(b) {
		t.Fatalf("토큰 수가 다르다: NFC %d개, NFD %d개", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("토큰 %d이 다르다: %q vs %q", i, a[i], b[i])
		}
	}
}

// 사전 매칭까지 이어져야 실제로 태그가 붙는다 — 여기가 사용자에게 보이는 지점이다.
func TestClassify_nfdInputStillMatches(t *testing.T) {
	d := testDict()
	nfc := Classify(Content{Title: "쿠버네티스 운영"}, d)
	nfd := Classify(Content{Title: norm.NFD.String("쿠버네티스 운영")}, d)
	if len(nfc) == 0 {
		t.Fatal("NFC에서도 태그가 안 붙는다 — 테스트 전제가 틀렸다")
	}
	if len(nfd) != len(nfc) {
		t.Errorf("NFD 입력에서 태그가 %d개 → %d개로 줄었다", len(nfc), len(nfd))
	}
}
