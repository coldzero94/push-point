package scraper

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestExtractBodyText(t *testing.T) {
	html := `<!DOCTYPE html><html lang="ko"><head><title>테스트 문서</title></head>
<body>
<nav>메뉴 홈 로그인 회원가입</nav>
<article>
<h1>쿠버네티스 오토스케일링 완벽 가이드</h1>
<p>이 글에서는 쿠버네티스 환경에서 오토스케일링을 어떻게 구성하는지 처음부터 끝까지 자세히 설명한다.
수평 파드 오토스케일러(HPA)와 수직 파드 오토스케일러(VPA)의 근본적인 차이를 먼저 짚고 넘어간다.</p>
<p>수평 확장은 트래픽이 늘면 파드 개수를 늘려 부하를 분산하고, 수직 확장은 개별 파드의 CPU·메모리
리소스 요청량을 조정한다. 실제 운영 환경에서는 이 두 방식을 상황에 맞게 조합하는 사례가 대부분이다.</p>
<p>마지막으로 커스텀 메트릭 기반 오토스케일링과 클러스터 오토스케일러까지 다루며, 프로덕션에서
자주 겪는 함정과 그 해결책을 정리한다.</p>
</article>
<footer>저작권 2026 테스트회사</footer>
</body></html>`

	got := extractBodyText([]byte(html), nil)
	if !strings.Contains(got, "오토스케일링") {
		t.Errorf("본문에 '오토스케일링'이 있어야: %q", got)
	}
	// 보일러플레이트(nav/footer)는 제거돼야 한다.
	if strings.Contains(got, "로그인") || strings.Contains(got, "저작권") {
		t.Errorf("보일러플레이트가 본문에 섞임: %q", got)
	}
}

func TestCapRunes(t *testing.T) {
	if got := capRunes("hello world", 5); got != "hello" {
		t.Errorf("capRunes ASCII = %q, want hello", got)
	}
	if got := capRunes("hi", 10); got != "hi" {
		t.Errorf("capRunes 짧음 = %q, want hi", got)
	}
	// 멀티바이트(한글 3바이트): 룬 경계에서 잘라 유효 UTF-8 유지.
	got := capRunes("한국어", 4) // 4바이트 → 첫 글자(3바이트) 경계에서 컷
	if !utf8.ValidString(got) {
		t.Errorf("capRunes 결과가 유효 UTF-8이 아님: %q", got)
	}
	if got != "한" {
		t.Errorf("capRunes(한국어, 4) = %q, want 한", got)
	}
}
