package textutil

import "testing"

// 이중 인코딩된 메타 필드가 화면에 &quot;로 새면 안 된다.
// 네이버 블로그 og:description이 `&amp;quot;`라서 한 번 디코딩해도 `&quot;`가 남는다.
func TestCleanMeta_unescapesDoubleEncodedEntities(t *testing.T) {
	got := CleanMeta("&quot;이번 달 실수령액&quot; 하지만", MaxDescription)
	want := `"이번 달 실수령액" 하지만`
	if got != want {
		t.Errorf("엔티티가 남았다:\n got=%q\nwant=%q", got, want)
	}
}

// 본문은 CleanMeta를 쓰지 않는다는 것이 전제다 — 여기서는 메타 필드가 코드처럼 보여도
// 한 번만 푼다는 것만 확인한다(무한 디코딩 금지).
func TestCleanMeta_decodesOnce(t *testing.T) {
	// &amp;amp;quot; → (호출 전 한 번 이미 풀렸다고 가정하면) &amp;quot; → &quot;
	if got := CleanMeta("&amp;quot;", MaxDescription); got != "&quot;" {
		t.Errorf("한 번만 풀어야 한다: %q", got)
	}
}
