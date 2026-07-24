package tagger

import "testing"

func TestBuildDictionary_classification(t *testing.T) {
	d := BuildDictionary([]TagEntry{
		{ID: 5, Name: "ai", Aliases: []string{"ml", "machine learning", "머신러닝", "딥러닝"}},
		{ID: 3, Name: "kubernetes", Aliases: []string{"k8s", "쿠버네티스"}},
		{ID: 8, Name: "book", Aliases: []string{"책", "독서"}},
	})

	// 라틴/숫자/1룬 → exactLatin. name 자체도 surface.
	for _, s := range []string{"ai", "ml", "k8s", "책"} {
		if _, ok := d.exactLatin[s]; !ok {
			t.Errorf("exactLatin에 %q 있어야", s)
		}
	}
	// 한글 ≥2룬 → koPrefix
	hasKo := func(surface string) bool {
		for _, k := range d.koPrefix {
			if k.surface == surface {
				return true
			}
		}
		return false
	}
	for _, s := range []string{"머신러닝", "딥러닝", "쿠버네티스", "독서"} {
		if !hasKo(s) {
			t.Errorf("koPrefix에 %q 있어야", s)
		}
	}
	// 다중어 → phrases (firstTok 인덱싱)
	if got, ok := d.phrases["machine"]; !ok || len(got) == 0 || got[0].tagID != 5 {
		t.Errorf("phrases[machine]에 ai(5) 있어야, got %v", d.phrases["machine"])
	}
	// nameToID / idToName
	if d.nameToID["kubernetes"] != 3 || d.idToName[3] != "kubernetes" {
		t.Errorf("name↔id 매핑 불일치")
	}
}
