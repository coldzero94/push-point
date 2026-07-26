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
	// 본문 매칭이 설명 매칭보다 약한 증거인 이유: 설명은 200자라 한 번 나오면 글의
	// 주제일 가능성이 높지만, 본문은 32KB까지 가므로 어떤 단어든 한 번쯤은 스친다.
	// 둘을 같게 취급하면 threshold=1.0에서 "본문에 한 번 언급"만으로 태그가 붙는다 —
	// 실사용에서 월급 블로그에 travel·news가, 주가 기사에 dev가, 이강인 이적 기사에
	// ai가 그렇게 붙었다(마지막 것은 "AI로 만든 티저 영상"과 네이버 안내 문구였다).
	//
	// 무게(wBody)를 깎는 대신 **횟수**를 요구하는 이유: 무게를 깎으면 여러 번 나온
	// 강한 본문 신호까지 같이 약해져 도메인 히트에 밀린다. 걸러야 할 것은 약한 신호다.
	// runesPerMatch는 "본문 몇 자마다 언급 한 번을 요구할 것인가"다.
	//
	// 계단이 아니라 비례로 두는 이유: 처음에는 2000자를 경계로 1회→3회를 요구했는데,
	// **같은 기사가 캡처 길이만 달라져 태그를 전부 잃는** 일이 실제로 벌어졌다
	// (1380자일 때 news·video, 2401자일 때 없음). 긴 문서에서 한 번의 언급이 약해지는
	// 것은 연속적인 관계이므로 요구치도 연속이어야 한다.
	//
	// 설명 상한(2048바이트)과 같은 자릿수 — 설명 한 편 분량마다 한 번씩을 기대한다.
	runesPerMatch = 2000
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
	// 본문 요구치는 길이에 비례하고, 뒷받침이 있으면 한 단계 낮춘다.
	// 여기까지의 score에 있는 태그 = 도메인·제목·설명이 이미 가리킨 태그다.
	corroborated := make(map[int64]bool, len(score))
	for id := range score {
		corroborated[id] = true
	}
	addBody(score, d, c.Body, corroborated)

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
	// 길이 비례 요구치. 상한은 matchCap과 같다 — 그보다 높이면 어떤 태그도 통과할 수 없다.
	need := 1 + utf8.RuneCountInString(text)/runesPerMatch
	if need > matchCap {
		need = matchCap
	}
	for id, n := range d.matchField(Tokenize(text)) {
		min := need
		if corroborated[id] && min > minBodyCorroborated {
			// 뒷받침이 있으면 한 단계 낮춘다 — 같은 두 번이라도 제목이 거드는 두 번은
			// 본문에만 있는 두 번보다 무겁다.
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
