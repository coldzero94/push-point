package tagger

import (
	"slices"
	"testing"
)

// testDict는 검증에 쓰는 대표 태그들의 사전(실제 시드 aliases 미러). ID는 임의.
func testDict() *Dictionary {
	return BuildDictionary([]TagEntry{
		{ID: 1, Name: "dev", Aliases: []string{"개발", "programming", "coding", "코딩", "개발자"}},
		{ID: 2, Name: "golang", Aliases: []string{"go", "고랭", "고언어"}},
		{ID: 3, Name: "kubernetes", Aliases: []string{"k8s", "쿠버네티스", "쿠버"}},
		{ID: 4, Name: "opensource", Aliases: []string{"오픈소스", "open source", "github", "깃허브"}},
		{ID: 5, Name: "ai", Aliases: []string{"인공지능", "머신러닝", "machine learning", "ml", "딥러닝", "deep learning"}},
		{ID: 6, Name: "devops", Aliases: []string{"데브옵스", "ci/cd", "docker", "도커", "인프라", "infra"}},
		{ID: 7, Name: "video", Aliases: []string{"영상", "동영상", "유튜브", "youtube"}},
		{ID: 8, Name: "book", Aliases: []string{"책", "독서", "서평", "reading"}},
	})
}

func ids(tags []ScoredTag) []int64 {
	out := make([]int64, len(tags))
	for i, t := range tags {
		out[i] = t.TagID
	}
	return out
}

func TestClassify_matchExamples(t *testing.T) {
	d := testDict()
	cases := []struct {
		name   string
		c      Content
		want   int64 // 이 tagID가 결과에 있어야
		absent int64 // 이 tagID는 없어야 (0이면 검사 안 함)
	}{
		{"한글 조사 prefix — 쿠버네티스를→kubernetes", Content{Title: "쿠버네티스를 처음 배우는 사람"}, 3, 0},
		{"라틴<3 경계 — he said hello는 ai 아님", Content{Title: "he said hello"}, 0, 5},
		{"다중어 구문 — machine learning→ai", Content{Title: "a machine learning guide"}, 5, 0},
		{"다중어 구문 — open source→opensource", Content{Title: "open source project"}, 4, 0},
		{"다중어 구문 ci/cd→devops", Content{Title: "ci/cd pipeline"}, 6, 0},
		{"라틴<3 경계 — email은 ai(ml) 아님", Content{Title: "check your email now"}, 0, 5},
		{"라틴<3 — ai 토큰은 매칭", Content{Title: "ai is here"}, 5, 0},
		{"한글 prefix — 딥러닝을→ai", Content{Title: "딥러닝을 공부"}, 5, 0},
		{"한글 짧은 alias — 쿠버→kubernetes", Content{Title: "쿠버 입문"}, 3, 0},
		{"1룬 한글 정확 — 책→book", Content{Description: "이 책 추천"}, 8, 0},
	}
	for _, tc := range cases {
		got := ids(Classify(tc.c, d))
		if tc.want != 0 && !slices.Contains(got, tc.want) {
			t.Errorf("%s: tag %d 있어야, got %v", tc.name, tc.want, got)
		}
		if tc.absent != 0 && slices.Contains(got, tc.absent) {
			t.Errorf("%s: tag %d 없어야, got %v", tc.name, tc.absent, got)
		}
	}
}

func TestClassify_domainSignal(t *testing.T) {
	d := testDict()
	// github.com → opensource(4)+dev(1) 둘 다
	got := ids(Classify(Content{Domain: "github.com"}, d))
	if !slices.Contains(got, 4) || !slices.Contains(got, 1) {
		t.Errorf("github.com → opensource+dev 둘 다여야, got %v", got)
	}
	// 서브도메인 폴백 — m.youtube.com·www.youtube.com → video(7)
	for _, host := range []string{"m.youtube.com", "www.youtube.com", "youtube.com"} {
		if !slices.Contains(ids(Classify(Content{Domain: host}, d)), 7) {
			t.Errorf("%s → video여야", host)
		}
	}
}

func TestClassify_thresholdAndConfidence(t *testing.T) {
	d := testDict()
	// 매치 0 · 도메인 0 → 빈 결과
	if got := Classify(Content{Title: "완전히 무관한 잡담"}, d); len(got) != 0 {
		t.Errorf("매치 없으면 빈 결과여야, got %v", got)
	}
	// 도메인 단독(score=3) → confidence 0.75
	got := Classify(Content{Domain: "github.com"}, d)
	for _, tag := range got {
		if tag.Confidence <= 0 || tag.Confidence >= 1 {
			t.Errorf("confidence는 (0,1)이어야, got %v", tag.Confidence)
		}
		if tag.TagID == 4 || tag.TagID == 1 { // 도메인 단독 태그
			if tag.Confidence != 0.75 {
				t.Errorf("도메인 단독(score 3) confidence 0.75여야, got %v", tag.Confidence)
			}
		}
	}
}

func TestClassify_topKAndDeterministic(t *testing.T) {
	d := testDict()
	// 여러 태그가 매칭되게 — 상한 topK(5) 이하 + 결정적(반복 동일)
	c := Content{
		Title:       "kubernetes devops docker",
		Description: "open source golang python ai machine learning",
	}
	a := ids(Classify(c, d))
	b := ids(Classify(c, d))
	if len(a) > topK {
		t.Errorf("topK=%d 초과: %v", topK, a)
	}
	if !slices.Equal(a, b) {
		t.Errorf("결정적이어야: %v != %v", a, b)
	}
}
