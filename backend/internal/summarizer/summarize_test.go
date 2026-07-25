package summarizer

import (
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

// article은 n개의 서로 다른 산문 문장을 잇는다(각 문장이 IsProse를 통과할 만큼 길다).
func article(n int) string {
	topics := []string{
		"쿠버네티스 클러스터에서 파드를 배포하고 롤링 업데이트를 수행하는 방법을 설명한다",
		"서비스 오브젝트가 파드 집합에 안정적인 네트워크 엔드포인트를 부여하는 원리를 다룬다",
		"오토스케일러가 관측 지표를 기준으로 레플리카 수를 조정하는 과정을 살펴본다",
		"컨테이너 이미지를 빌드하고 레지스트리에 푸시하는 파이프라인을 구성해 본다",
		"네트워크 정책으로 파드 사이의 트래픽을 세밀하게 제어하는 방법을 정리한다",
		"영속 볼륨과 스토리지 클래스가 상태 저장 워크로드를 지원하는 구조를 본다",
		"인그레스 컨트롤러가 외부 트래픽을 클러스터 내부로 라우팅하는 흐름을 따라간다",
		"모니터링 스택을 구성해 클러스터의 상태를 지속적으로 관측하는 법을 익힌다",
		"백업과 복구 전략을 세워 장애 상황에서 데이터를 안전하게 지키는 법을 다룬다",
		"보안 컨텍스트를 설정해 컨테이너의 권한을 최소한으로 제한하는 방법을 본다",
	}
	var b strings.Builder
	for i := range n {
		b.WriteString(topics[i%len(topics)])
		b.WriteString(". ")
	}
	return b.String()
}

func TestSummarize_guards(t *testing.T) {
	long := article(10)
	cases := []struct {
		name, body, desc string
		wantEmpty        bool
	}{
		{"본문 없음", "", "", true},
		{"본문이 최소 길이 미만", strings.Repeat("가", MinBodyRunes-1), "", true},
		{"산문 문장 부족", strings.Repeat("코드 { x = 1; } 표 | 1 | 2 | ", 40), "", true},
		{"정상 본문", long, "무관한 설명", false},
		// description과 사실상 같으면 통째로 버린다(인스펙터에서 같은 말 반복 금지).
		{"description과 중복", long, long, true},
	}
	for _, c := range cases {
		got := Summarize(c.body, c.desc)
		if (got == "") != c.wantEmpty {
			t.Errorf("%s: Summarize 빈값=%v, want %v (got %.60q)", c.name, got == "", c.wantEmpty, got)
		}
	}
}

func TestSummarize_shape(t *testing.T) {
	body := article(10)
	got := Summarize(body, "짧은 설명")
	if got == "" {
		t.Fatal("정상 본문에서 요약이 나와야 한다")
	}
	lines := strings.Split(got, "\n")
	if len(lines) < 2 || len(lines) > 3 {
		t.Errorf("문장 수 = %d, want 2~3: %q", len(lines), got)
	}
	if n := utf8.RuneCountInString(got); n > maxSummaryRunes {
		t.Errorf("길이 %d룬 > 상한 %d", n, maxSummaryRunes)
	}
	// 추출식이므로 모든 문장이 **원문에 그대로** 있어야 한다(생성 금지 = 환각 0).
	for _, l := range lines {
		if !strings.Contains(body, l) {
			t.Errorf("원문에 없는 문장이 나옴(추출식 위반): %q", l)
		}
	}
	// 문서 등장 순서로 읽혀야 한다.
	prev := -1
	for _, l := range lines {
		i := strings.Index(body, l)
		if i < prev {
			t.Errorf("문서 순서가 뒤집힘: %q", got)
		}
		prev = i
	}
}

// 같은 입력은 항상 바이트 동일해야 한다 — 맵 순회가 새면 eval 회귀 탐지가 불가능해진다.
func TestSummarize_deterministic(t *testing.T) {
	body, desc := article(12), "설명"
	first := Summarize(body, desc)
	for range 50 {
		if got := Summarize(body, desc); got != first {
			t.Fatalf("비결정적 출력:\n%q\n%q", first, got)
		}
	}
}

// 반복되는 보일러플레이트가 요약 슬롯을 독식하면 안 된다(중심성 clique 문제).
func TestSummarize_noRepeatedBoilerplate(t *testing.T) {
	const cta = "이 글이 마음에 드셨다면 뉴스레터를 구독하고 소식을 받아보시기 바랍니다."
	var b strings.Builder
	for i, s := range strings.Split(strings.TrimSpace(article(8)), ". ") {
		if s == "" {
			continue
		}
		b.WriteString(s + ". ")
		if i%2 == 0 {
			b.WriteString(cta + " ") // 같은 문장을 반복 삽입
		}
	}
	got := Summarize(b.String(), "설명")
	if got == "" {
		t.Fatal("요약이 나와야 한다")
	}
	if n := strings.Count(got, cta); n > 1 {
		t.Errorf("반복 보일러플레이트가 %d번 뽑힘:\n%s", n, got)
	}
}

func TestContainment(t *testing.T) {
	a := contentTokens("쿠버네티스 클러스터 배포")
	b := contentTokens("쿠버네티스 클러스터 운영 전략")
	if c := Containment(nil, b); c != 0 {
		t.Errorf("빈 a면 0이어야, got %v", c)
	}
	if c := Containment(a, a); c != 1 {
		t.Errorf("자기 자신과는 1이어야, got %v", c)
	}
	// 비대칭이어야 한다: |a∩b|/|a| ≠ |a∩b|/|b| (길이가 다르면)
	if Containment(a, b) == Containment(b, a) && len(a) != len(b) {
		t.Errorf("Containment는 비대칭이어야 한다 (a=%v b=%v)", a, b)
	}
}

// contentTokens는 정렬·중복제거된 슬라이스를 내야 한다 — intersectCount의 병합 스캔이
// 이 불변식에 의존하므로 깨지면 모든 유사도가 조용히 틀린다.
func TestContentTokens_sortedUnique(t *testing.T) {
	for _, s := range []string{
		"쿠버네티스를 쿠버네티스와 쿠버네티스처럼 배포한다",
		"the the a an of Kubernetes cluster CLUSTER",
		"", "。 !! ??",
	} {
		got := contentTokens(s)
		if !sort.StringsAreSorted(got) {
			t.Errorf("정렬되지 않음 (%q): %v", s, got)
		}
		if len(slices.Compact(slices.Clone(got))) != len(got) {
			t.Errorf("중복이 남음 (%q): %v", s, got)
		}
		for _, tok := range got {
			if stopwords[tok] {
				t.Errorf("불용어가 남음 (%q): %q", s, tok)
			}
			if utf8.RuneCountInString(tok) < 2 {
				t.Errorf("1룬 토큰이 남음 (%q): %q", s, tok)
			}
		}
	}
}

func TestLeadN(t *testing.T) {
	body := article(6)
	got := LeadN(body, 3)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("3문장이어야, got %d: %q", len(lines), got)
	}
	// 앞에서부터 순서대로여야 한다(베이스라인의 정의).
	all := []string{}
	for _, s := range Split(body) {
		if IsProse(s) {
			all = append(all, s)
		}
	}
	for i, l := range lines {
		if l != all[i] {
			t.Errorf("%d번째 문장이 앞에서부터가 아님: %q != %q", i, l, all[i])
		}
	}
	if LeadN("", 3) != "" {
		t.Error("빈 본문은 빈 결과")
	}
}
