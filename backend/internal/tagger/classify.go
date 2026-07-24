package tagger

import (
	"sort"
	"strings"
)

// 스코어 가중치·컷 — 미측정 베이스라인. Stage3 golden(dev.jsonl)에서 튜닝한다(설계 결정).
const (
	wDomain   = 3.0 // 도메인맵 히트: 태그당 (가장 강한 신호)
	wTitle    = 2.0 // 제목 매칭: 매칭당
	wDesc     = 1.0 // 설명 매칭: 매칭당
	wNote     = 1.0 // 개인 메모 매칭: 매칭당
	matchCap  = 3   // 한 필드에서 한 태그의 기여 상한 (키워드 스터핑 방지)
	threshold = 1.0 // 총점이 이 미만인 태그는 컷 (설명 1매치 = 통과)
	topK      = 5   // 링크당 최대 태그 수
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
	if text == "" {
		return
	}
	for id, n := range d.matchField(Tokenize(text)) {
		if n > matchCap {
			n = matchCap
		}
		score[id] += weight * float64(n)
	}
}
