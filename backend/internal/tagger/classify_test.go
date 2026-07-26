package tagger

import (
	"slices"
	"strings"
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
		{"본문 신호 — body의 kubernetes", Content{Body: "이 문서는 kubernetes 클러스터 운영을 처음부터 다룬다"}, 3, 0},
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

// 발행자 분류(keywords)는 제목과 같은 무게로 단독 통과해야 한다.
//
// 이 신호를 쓰는 이유가 여기 다 들어 있다: 제목이 "이강인, 극적 결승골"이면 어떤 사전으로도
// 축구를 못 맞히지만, 발행자는 `article:section=스포츠`로 이미 알려주고 있었다. 본문에서
// 추론한 값이 아니라 사이트가 선언한 값이라 한 번 나와도 믿을 수 있다.
func TestClassify_keywordsSignal(t *testing.T) {
	d := testDict()
	// 제목·설명·본문 어디에도 단서가 없고 분류만 있는 경우.
	got := Classify(Content{Title: "새 릴리스 소식", Keywords: "golang, 개발"}, d)
	if !slices.Contains(ids(got), int64(2)) {
		t.Errorf("keywords만으로 golang이 안 붙음: %v", ids(got))
	}
	if !slices.Contains(ids(got), int64(1)) {
		t.Errorf("keywords만으로 dev가 안 붙음: %v", ids(got))
	}
}

// keywords 한 번 = 제목 한 번. 도메인(3.0)보다는 약하고 설명(1.0)보다는 강하다는 순서가
// 유지되는지 본다 — 가중치를 만지다 순서가 뒤집히면 조용히 품질이 바뀐다.
func TestClassify_keywordsWeighsLikeTitle(t *testing.T) {
	d := testDict()
	byKeywords := Classify(Content{Keywords: "쿠버네티스"}, d)
	byTitle := Classify(Content{Title: "쿠버네티스"}, d)
	byDesc := Classify(Content{Description: "쿠버네티스"}, d)
	if len(byKeywords) != 1 || len(byTitle) != 1 || len(byDesc) != 1 {
		t.Fatalf("각 필드가 태그 하나씩을 내야 한다: kw=%v title=%v desc=%v", byKeywords, byTitle, byDesc)
	}
	if byKeywords[0].Confidence != byTitle[0].Confidence {
		t.Errorf("keywords와 title의 무게가 다르다: %v vs %v", byKeywords[0].Confidence, byTitle[0].Confidence)
	}
	if byKeywords[0].Confidence <= byDesc[0].Confidence {
		t.Errorf("keywords가 description보다 약하다: %v vs %v", byKeywords[0].Confidence, byDesc[0].Confidence)
	}
}

// capN은 한 필드에서 한 태그의 기여를 matchCap으로 자른다 — **키워드 스터핑 방지**의 실체다.
// 이걸 없애도 지금까지 아무 테스트가 실패하지 않았다(2026-07-27 실측). 상한이 없으면 본문에
// 같은 낱말을 스무 번 박은 페이지가 내용이 실한 페이지를 점수로 이긴다.
func TestCapNLimitsKeywordStuffing(t *testing.T) {
	if capN(1) != 1 {
		t.Errorf("상한 아래는 그대로: capN(1)=%d", capN(1))
	}
	if capN(matchCap) != matchCap {
		t.Errorf("상한과 같으면 그대로: capN(%d)=%d", matchCap, capN(matchCap))
	}
	if got := capN(matchCap + 50); got != matchCap {
		t.Errorf("상한을 넘으면 잘라야 한다: capN(%d)=%d, want %d", matchCap+50, got, matchCap)
	}
}

// 위 단위 규칙이 실제 점수까지 이어지는지 — 스터핑한 문서가 정직한 문서를 못 이겨야 한다.
func TestStuffingDoesNotOutscore(t *testing.T) {
	dict := BuildDictionary([]TagEntry{
		{ID: 1, Name: "python", Aliases: []string{"파이썬"}},
	})
	body := func(n int) string {
		out := ""
		for range n {
			out += "파이썬 "
		}
		return out + strings.Repeat("가 ", 200) // 길이 요구치를 넘기기 위한 채움
	}
	honest := Classify(Content{Body: body(matchCap)}, dict)
	stuffed := Classify(Content{Body: body(matchCap * 20)}, dict)

	if len(honest) == 0 || len(stuffed) == 0 {
		t.Fatalf("둘 다 태그가 나와야 비교가 된다: honest=%v stuffed=%v", honest, stuffed)
	}
	if stuffed[0].Confidence > honest[0].Confidence {
		t.Errorf("스무 배로 반복한 문서가 더 높은 점수를 받았다 — 상한이 안 걸린다: %v vs %v",
			stuffed[0].Confidence, honest[0].Confidence)
	}
}
