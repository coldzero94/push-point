package tagger

import "testing"

// 한국어 복합명사는 head-final이다 — 의미의 핵이 어절 뒤에 온다. prefix만 보던 시절
// `"대박식당처럼"`이 `식당`에 닿지 않아 김치찌개 레시피에서 `food`를 통째로 놓쳤다.
func TestMatchesKoSurface(t *testing.T) {
	cases := []struct {
		dt, surface string
		want        bool
		why         string
	}{
		{"식당가", "식당", true, "핵이 앞 — 예전에도 잡히던 경우"},
		{"대박식당", "식당", true, "핵이 뒤 — 이게 놓치던 경우"},
		{"쿠버네티스", "쿠버네티스", true, "완전 일치"},
		{"김치찌개", "찌개", true, "복합명사의 뒷머리"},
		{"대박식당가", "식당", false, "가운데 묻힌 것은 안 잡는다(Contains가 아니다)"},
		{"경기도", "경기", true, "이건 잡히는 게 맞다 — 오탐은 별칭 문제지 매칭 문제가 아니다"},
		{"파이썬", "자바", false, "무관"},
	}
	for _, c := range cases {
		if got := matchesKoSurface(c.dt, c.surface); got != c.want {
			t.Errorf("matchesKoSurface(%q, %q) = %v, want %v — %s", c.dt, c.surface, got, c.want, c.why)
		}
	}
}

// **matchField와 MatchedSurfaces가 같은 규칙을 써야 한다.** 갈라지면 corpus_df 누적 키와
// 조회 키가 어긋나 DF가 조용히 0에 머물고, IDF는 켜도 아무 일도 하지 않는다.
func TestMatchAndSurfacesAgreeOnSuffix(t *testing.T) {
	d := BuildDictionary([]TagEntry{{ID: 1, Name: "food", Aliases: []string{"식당"}}})

	// "대박식당처럼" → Normalize가 '처럼'을 벗겨 "대박식당" → suffix로 식당에 걸린다.
	hits := d.matchField(Tokenize("대박식당처럼 맛있게"))
	if len(hits) == 0 {
		t.Fatal("matchField가 suffix 매칭을 놓쳤다")
	}
	surfaces := d.MatchedSurfaces(Content{Title: "대박식당처럼 맛있게"})
	if !surfaces["식당"] {
		t.Errorf("MatchedSurfaces가 같은 표면을 못 냈다 — DF 누적 키가 갈라진다: %v", surfaces)
	}
}
