package tagger

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// 스코어 가중치·컷 — 미측정 베이스라인. Stage3 golden(dev.jsonl)에서 튜닝한다(설계 결정).
const (
	wDomain  = 3.0 // 도메인맵 히트: 태그당 (가장 강한 신호)
	wTitle   = 2.0 // 제목 매칭: 매칭당
	wDesc    = 1.0 // 설명 매칭: 매칭당
	wNote    = 1.0 // 개인 메모 매칭: 매칭당
	wBody    = 1.0 // 본문 매칭: 매칭당 (matchCap이 본문 반복 스터핑을 억제)
	matchCap = 3   // 한 필드에서 한 태그의 기여 상한 (키워드 스터핑 방지)
	// minBodyCorroborated는 제목·설명·도메인이 이미 같은 태그를 가리킬 때 본문에
	// 요구하는 최소 횟수. 다른 신호가 뒷받침하므로 두 번이면 충분하다.
	minBodyCorroborated = 2
	// minBodyMatches는 본문 단독 신호가 인정받는 최소 횟수.
	//
	// 설명은 200자라 한 번 나오면 글의 주제일 가능성이 높지만, 본문은 32KB까지 가므로
	// 어떤 단어든 한 번쯤은 스친다. 그 둘을 같게 취급하면 threshold=1.0에서 "본문에 한 번
	// 언급"만으로 태그가 붙는다 — 실사용에서 월급 블로그에 travel·news가, 주가 기사에
	// dev가 그렇게 붙었다.
	//
	// 무게(wBody)를 깎는 대신 최소 횟수를 두는 이유: 무게를 깎으면 3회 이상 나온
	// **강한** 본문 신호까지 같이 약해져 도메인 히트에 밀린다. 여기서 걸러야 할 것은
	// 약한 신호 하나뿐이다.
	//
	// **본문만이 유일한 근거일 때는 더 요구한다.** 제목과 설명은 글이 스스로를 요약한
	// 것이라, 거기에 한 번도 안 나오는 단어는 대개 스쳐 지나간 것이다 — 이강인 이적
	// 기사에 ai가 붙은 실사용 사례가 그랬다. 본문의 두 번이 "AI로 만든 티저 영상"과
	// 네이버의 안내 문구("본문의 검색 링크는 AI 자동 인식으로 제공됩니다")였는데,
	// 둘 다 글의 주제와 무관하다. 특정 사이트의 문구를 지목해 지우는 대신, 어디에서
	// 왔든 뒷받침 없는 본문 언급의 기준을 올린다.
	minBodyMatches = 3
	// longBodyRunes는 위 규칙이 적용되기 시작하는 길이. 근거는 "긴 문서"라는 조건 자체다 —
	// 짧은 본문에서는 한 번 언급이 곧 주제이므로(설명 필드와 다를 바 없다) 걸러선 안 된다.
	// 설명 상한(2048바이트)과 같은 자릿수로 잡아, 그보다 짧은 본문은 설명처럼 취급한다.
	longBodyRunes = 2000
	threshold     = 1.0 // 총점이 이 미만인 태그는 컷 (설명 1매치 = 통과)
	topK          = 5   // 링크당 최대 태그 수
)

// Classify는 콘텐츠를 사전에 대해 분류해 상위 태그(≤topK, threshold 이상)를 돌려준다.
// 신호 = 도메인맵 + title/description/note 필드별 사전 매칭의 가법 스코어. 필드마다 따로
// 토큰화해 구문이 필드 경계를 넘지 못하게 한다. 출력은 결정적(score desc, 동점 name asc).
func Classify(c Content, d *Dictionary) []ScoredTag {
	score := map[int64]float64{}

	// 도메인 신호 — 호스트에 매핑된 각 태그에 +wDomain.
	for _, name := range DomainTags(c.Domain) {
		if id, ok := d.nameToID[strings.ToLower(name)]; ok {
			score[id] += wDomain
		}
	}
	addField(score, d, c.Title, wTitle)
	addField(score, d, c.Description, wDesc)
	addField(score, d, c.Note, wNote)
	// 본문은 길 때만 최소 매칭 횟수를 요구하고, 그 기준은 다른 신호의 뒷받침 여부로
	// 갈린다. 여기까지의 score에 있는 태그 = 도메인·제목·설명이 이미 가리킨 태그다.
	if utf8.RuneCountInString(c.Body) > longBodyRunes {
		corroborated := make(map[int64]bool, len(score))
		for id := range score {
			corroborated[id] = true
		}
		addBody(score, d, c.Body, corroborated)
	} else {
		addFieldMin(score, d, c.Body, wBody, 1)
	}

	out := make([]ScoredTag, 0, len(score))
	for id, s := range score {
		if s >= threshold {
			// confidence = 1 - 1/(1+score): score 1→0.5, 3→0.75, 5→0.833. corpus 무의존·결정적.
			out = append(out, ScoredTag{TagID: id, Confidence: 1 - 1/(1+s)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		si, sj := score[out[i].TagID], score[out[j].TagID]
		if si != sj {
			return si > sj
		}
		return d.idToName[out[i].TagID] < d.idToName[out[j].TagID]
	})
	if len(out) > topK {
		out = out[:topK]
	}
	return out
}

// addField는 한 필드를 토큰화·매칭해 태그별 (상한 적용) 매칭 수 × weight를 score에 더한다.
func addField(score map[int64]float64, d *Dictionary, text string, weight float64) {
	addFieldMin(score, d, text, weight, 1)
}

// addBody는 본문 매칭을 더한다. 뒷받침이 있는 태그와 없는 태그에 서로 다른 최소
// 횟수를 적용한다 — 같은 두 번이라도 제목이 거드는 두 번과 본문에만 있는 두 번은
// 증거로서 무게가 다르다.
func addBody(score map[int64]float64, d *Dictionary, text string, corroborated map[int64]bool) {
	if text == "" {
		return
	}
	for id, n := range d.matchField(Tokenize(text)) {
		min := minBodyMatches
		if corroborated[id] {
			min = minBodyCorroborated
		}
		if n < min {
			continue
		}
		if n > matchCap {
			n = matchCap
		}
		score[id] += wBody * float64(n)
	}
}

// addFieldMin은 min회 미만 매칭은 무시한다 — 긴 필드에서 한 번 스친 언급을 걸러낸다.
func addFieldMin(score map[int64]float64, d *Dictionary, text string, weight float64, min int) {
	if text == "" {
		return
	}
	for id, n := range d.matchField(Tokenize(text)) {
		if n < min {
			continue
		}
		if n > matchCap {
			n = matchCap
		}
		score[id] += weight * float64(n)
	}
}
