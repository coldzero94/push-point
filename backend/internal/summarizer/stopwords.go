package summarizer

// stopwords는 문장 유사도 계산에서 제외할 기능어다. TextRank 원논문은 명사·형용사만
// 남기는 POS 필터를 쓰지만 형태소 분석기가 없으므로, 소형 stoplist + 1룬 토큰 제거로
// 대체한다. 기능어가 남으면 "모든 문장이 서로 비슷하다"가 되어 그래프가 무너진다.
//
// **일부러 tagger 패키지에 넣지 않았다.** 태거의 사전 매칭은 기능어를 만나도 해가 없고,
// 여기 리스트를 태거와 공유하면 태깅 정확도(Recall@3 dev 0.900 / test 0.880)에 회귀
// 위험이 생긴다. 요약 전용으로 격리해 두 시스템을 독립적으로 유지한다.
//
// 토큰은 tagger.Normalize를 거친 뒤(조사 제거·소문자) 비교되므로 여기도 그 형태로 적는다.
var stopwords = map[string]bool{
	// 한국어 — 지시·접속·의존명사·상투어
	"그리고": true, "그러나": true, "하지만": true, "그래서": true, "따라서": true,
	"또한": true, "그런데": true, "그러면": true, "그럼": true, "즉": true,
	"이것": true, "그것": true, "저것": true, "여기": true, "거기": true,
	"이런": true, "그런": true, "저런": true, "이렇게": true, "그렇게": true,
	"때문": true, "위해": true, "통해": true, "대해": true, "관해": true,
	"경우": true, "정도": true, "지금": true, "다음": true, "이번": true,
	"우리": true, "저희": true, "자신": true, "사람": true, "생각": true,
	"수도": true, "수가": true, "있다": true, "없다": true, "하다": true,
	"된다": true, "한다": true, "이다": true, "같다": true, "많다": true,

	// 영어 — 관사·전치사·조동사·대명사
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"if": true, "then": true, "than": true, "that": true, "this": true,
	"these": true, "those": true, "there": true, "here": true, "what": true,
	"which": true, "who": true, "when": true, "where": true, "how": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"being": true, "have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "can": true, "could": true,
	"should": true, "may": true, "might": true, "must": true, "shall": true,
	"of": true, "in": true, "on": true, "at": true, "to": true, "for": true,
	"with": true, "by": true, "from": true, "as": true, "into": true, "about": true,
	"it": true, "its": true, "we": true, "you": true, "your": true, "our": true,
	"they": true, "them": true, "their": true, "he": true, "she": true, "his": true,
	"her": true, "not": true, "no": true, "so": true, "up": true, "out": true,
	"more": true, "most": true, "some": true, "any": true, "all": true, "also": true,
}
