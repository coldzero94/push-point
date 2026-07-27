package store

import "testing"

// ftsMatchQueryAny는 같은 토큰을 OR로 잇는다 — AND가 빈손일 때의 재시도용.
func TestFtsMatchQueryAny(t *testing.T) {
	if got := ftsMatchQueryAny("쿠버네티스 하드웨이"); got != `"쿠버네티스" OR "하드웨이"` {
		t.Errorf("OR로 이어야 한다: %q", got)
	}
	// 토큰이 하나면 AND와 **글자까지 같아야** 한다 — 호출부가 그걸 보고 재시도를 건너뛴다.
	if a, o := ftsMatchQuery("제네릭"), ftsMatchQueryAny("제네릭"); a != o {
		t.Errorf("단일 토큰이면 AND와 OR가 같아야 재시도를 건너뛴다: %q vs %q", a, o)
	}
	// 3자 미만은 AND와 같은 규칙으로 버린다.
	if got := ftsMatchQueryAny("웹 취약점 top 10"); got != `"취약점" OR "top"` {
		t.Errorf("3자 미만 토큰은 빠져야 한다: %q", got)
	}
	// 주입 차단은 **두 겹**이다: 따옴표 이스케이프 + 3자 미만 제거.
	// `abc" OR "x`에서 `OR`와 `"x`는 2자라 먼저 버려지고, 남은 토큰은 따옴표가 이중화된다.
	if got := ftsMatchQueryAny(`abc" OR "x`); got != `"abc"""` {
		t.Errorf("주입 시도가 무력화돼야 한다: %q", got)
	}
}
