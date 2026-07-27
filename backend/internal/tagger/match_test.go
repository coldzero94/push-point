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

// fieldHit은 개수와 배율을 **따로** 들고 다닌다. 두 규칙 다 지금까지 아무 테스트가
// 지키지 않았다 — 둘 중 어느 쪽을 깨뜨려도 17개 패키지가 전부 통과했다(2026-07-27 실측).
//
//   - n은 **누적**해야 한다. 안 그러면 matchCap(한 필드에서 한 태그의 기여 상한,
//     `classify.go:21`)이 무력해져 키워드 스터핑을 막지 못한다.
//   - mul은 **최댓값**이어야 한다. 한 태그가 `go`(흔함)와 `golang`(희귀)에 동시에
//     걸렸다면 진짜 증거는 희귀한 쪽인데, 최솟값이나 평균을 쓰면 강한 증거가 약한
//     증거에 희석된다.
func TestFieldHitAccumulates(t *testing.T) {
	var h fieldHit
	h.add(1.0)
	h.add(3.0)
	h.add(2.0)

	if h.n != 3 {
		t.Errorf("매칭 횟수를 누적해야 matchCap이 걸린다: n=%d, want 3", h.n)
	}
	if h.mul != 3.0 {
		t.Errorf("가장 변별력 있는 표면의 배율을 써야 한다(최댓값): mul=%v, want 3.0", h.mul)
	}

	// 낮은 값이 나중에 와도 최댓값이 유지돼야 한다 — 순서에 의존하면 안 된다.
	var h2 fieldHit
	h2.add(5.0)
	h2.add(0.5)
	if h2.mul != 5.0 {
		t.Errorf("나중에 온 낮은 배율이 최댓값을 덮어썼다: %v", h2.mul)
	}
}

// matchTail은 구문(다중어 표면)의 꼬리가 **연속으로** 이어지는지 본다.
func TestMatchTail(t *testing.T) {
	toks := []string{"machine", "learning", "쿠버네티스에서"}

	if !matchTail(toks, 1, []string{"learning"}) {
		t.Error("바로 다음 토큰이 일치하면 매칭이어야 한다")
	}

	// **범위 검사** — 없으면 인덱스 초과로 panic한다. 지금까지 이걸 지키는 테스트가 없었다.
	if matchTail(toks, 2, []string{"쿠버네티스", "추가토큰"}) {
		t.Error("tail이 토큰열 끝을 넘으면 false여야 한다")
	}
	if matchTail(toks, 3, []string{"뭐든"}) {
		t.Error("start가 이미 끝이면 false여야 한다")
	}

	// **라틴은 정확 동등**이다. prefix로 완화하면 `learning`이 `learnings`에 걸린다 —
	// 짧은 라틴 표면(`go`, `ai`)이 무관한 단어에 붙기 시작하는 것이 이 규칙이 막는 것이다.
	if matchTail([]string{"machine", "learnings"}, 1, []string{"learning"}) {
		t.Error("라틴 꼬리는 정확 동등이어야 한다 — learnings는 learning이 아니다")
	}

	// 한글 꼬리는 어절 규칙(prefix)을 쓴다 — 조사가 붙어도 이어져야 한다.
	if !matchTail([]string{"머신", "러닝을"}, 1, []string{"러닝"}) {
		t.Error("한글 꼬리는 어절 안에서 이어져야 한다")
	}
}

// phraseKey는 구문 **전체**를 하나의 DF 키로 만든다.
//
// 첫 토큰만 키로 쓰면 `machine learning`의 DF 대신 `machine`의 DF를 보게 되고, 흔한 낱말의
// 빈도가 희귀한 구문에 적용돼 IDF가 거꾸로 간다. 지금까지 이걸 지키는 테스트가 없었다.
func TestPhraseKey(t *testing.T) {
	if got := phraseKey("machine", []string{"learning"}); got != "machine learning" {
		t.Errorf("phraseKey = %q, want %q", got, "machine learning")
	}
	if got := phraseKey("자연어", []string{"처리", "모델"}); got != "자연어 처리 모델" {
		t.Errorf("꼬리가 여럿이면 전부 이어야 한다: %q", got)
	}
	// Surfaces()가 내는 키와 matchField가 기록하는 키가 같아야 DF가 쌓인다.
	d := BuildDictionary([]TagEntry{{ID: 1, Name: "llm", Aliases: []string{"machine learning"}}})
	var found bool
	for _, s := range d.Surfaces() {
		if s == "machine learning" {
			found = true
		}
	}
	if !found {
		t.Errorf("Surfaces가 구문 키를 구문 전체로 내야 한다: %v", d.Surfaces())
	}
}
