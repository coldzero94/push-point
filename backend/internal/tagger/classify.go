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
	minBodyMatches = 2
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
	// 본문은 길 때만 최소 매칭 횟수를 요구한다.
	bodyMin := 1
	if utf8.RuneCountInString(c.Body) > longBodyRunes {
		bodyMin = minBodyMatches
	}
	addFieldMin(score, d, c.Body, wBody, bodyMin)

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
