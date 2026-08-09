package tagger

import (
	"slices"
	"testing"
)

// 질의에서 사전 태그를 뽑는 규칙. **이게 검색의 언어 다리다** — 한국어 질의가 영어
// 문서에 닿는 경로가 여기 하나뿐이고, 조용히 빈손이 되면 검색은 그냥 조금 나빠질 뿐
// 아무것도 실패하지 않는다.
func TestTagsInQuery(t *testing.T) {
	d := BuildDictionary([]TagEntry{
		{ID: 1, Name: "golang", Aliases: []string{"go", "고랭", "고언어"}},
		{ID: 2, Name: "kubernetes", Aliases: []string{"쿠버네티스", "쿠버", "k8s"}},
		{ID: 3, Name: "productivity", Aliases: []string{"습관", "생산성", "habit"}},
		{ID: 4, Name: "llm", Aliases: []string{"machine learning", "머신러닝"}},
	})

	for _, c := range []struct {
		name string
		q    string
		want []string
	}{
		// 음차 — 미발견 7건 중 5건이 이 부류다(판다스/pandas, 쿠버네티스/Kubernetes …)
		{"음차 한국어가 영어 태그로", "쿠버네티스 하드웨이", []string{"kubernetes"}},
		{"별칭도 같은 태그로", "고랭 제네릭 언제 쓰나", []string{"golang"}},

		// **2음절이 살아남는다.** trigram의 3룬 하한이 버리는 바로 그 길이인데,
		// 사전 표면 매칭은 그 하한을 지나가지 않는다.
		{"2음절 주어", "습관 만드는 법", []string{"productivity"}},

		// 다중어 표면
		{"구문 표면", "machine learning 입문", []string{"llm"}},

		// 없는 건 없다고 해야 한다 — 아무 태그나 붙이면 확장이 잡음이 된다
		{"사전에 없으면 빈손", "어제 본 그 영상", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := d.TagsInQuery(c.q)
			if !slices.Equal(got, c.want) {
				t.Errorf("TagsInQuery(%q) = %v, want %v", c.q, got, c.want)
			}
		})
	}
}

// 같은 질의는 같은 확장을 내야 한다 — 순서가 흔들리면 캐시도 테스트도 성립하지 않는다.
func TestTagsInQuery_isOrdered(t *testing.T) {
	d := BuildDictionary([]TagEntry{
		{ID: 7, Name: "kubernetes", Aliases: []string{"쿠버네티스"}},
		{ID: 2, Name: "golang", Aliases: []string{"고랭"}},
	})
	want := []string{"golang", "kubernetes"} // ID 순
	for i := 0; i < 20; i++ {
		if got := d.TagsInQuery("고랭 쿠버네티스"); !slices.Equal(got, want) {
			t.Fatalf("%d회차: %v, want %v", i, got, want)
		}
	}
}

// TermsInQuery는 **건너편 문자체계의 표면형만** 얹는다. 이게 검색이 언어 경계를 건너는
// 방식이고, 좁히지 않으면 오히려 나빠진다 — 별칭을 전부 얹어 재 봤을 때 hit@1이
// 0.640에서 0.600으로 내려갔다. 그 사실이 이 테스트가 지키는 내용이다.
func TestTermsInQuery_crossScriptOnly(t *testing.T) {
	d := BuildDictionary([]TagEntry{
		{ID: 3, Name: "productivity", Aliases: []string{"습관", "생산성", "habit", "routine"}},
	})

	got := d.TermsInQuery("습관 만드는 법")

	// 이름은 문자체계와 무관하게 유지된다 — FTS의 tags 열을 때리는 경로다.
	// 영어 형제는 얹히고(제목을 직접 때린다), 한국어 형제는 빠진다(다리가 아니라 소음이다).
	want := []string{"productivity", "habit", "routine"}
	if !slices.Equal(got, want) {
		t.Fatalf("한국어 질의: got %v, want %v", got, want)
	}

	// 반대 방향도 같은 규칙이다.
	got = d.TermsInQuery("habit tracker")
	want = []string{"productivity", "습관", "생산성"}
	if !slices.Equal(got, want) {
		t.Fatalf("영어 질의: got %v, want %v", got, want)
	}
}
