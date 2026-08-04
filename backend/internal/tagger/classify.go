package tagger

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// 스코어 가중치·컷 — 미측정 베이스라인. Stage3 golden(dev.jsonl)에서 튜닝한다(설계 결정).
const (
	wDomain = 3.0 // 도메인맵 히트: 태그당 (가장 강한 신호)
	wTitle  = 2.0 // 제목 매칭: 매칭당
	// wKeywords는 발행자가 붙인 분류의 무게. 제목과 같은 급이다 — 우리가 본문에서
	// 추론한 값이 아니라 **사이트가 이 글을 뭐라고 분류했는지**이고, 대개 서너 낱말뿐이라
	// 스쳐 지나갈 여지가 없다. 도메인 맵과 같은 성격의 강한 신호이면서, 사이트별 등록이
	// 필요 없다는 점에서 그보다 일반적이다 — 등록되지 않은 도메인에서도 동작한다.
	wKeywords = 2.0
	wDesc     = 1.0 // 설명 매칭: 매칭당
	wNote     = 1.0 // 개인 메모 매칭: 매칭당
	wBody     = 1.0 // 본문 매칭: 매칭당 (matchCap이 본문 반복 스터핑을 억제)
	matchCap  = 3   // 한 필드에서 한 태그의 기여 상한 (키워드 스터핑 방지)
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
	// 링크당 최대 태그 수.
	//
	// **5였다가 3으로 줄였다(2026-08-04).** 4·5위는 링크를 하나도 더 구하지 못한다 —
	// dev/test/wild 세 세트 모두 링크 단위 Recall@3과 Recall@5가 **완전히 같다**(1.000 /
	// 0.905 / 0.821). 그러면서 링크마다 태그를 둘씩 더 붙였다: 189건 전체에서 4·5위가 낸
	// 207개 중 라벨과 맞는 것은 34개(16.4%)뿐이고, 1~3위는 54.5%다. 실사용 DB에서도
	// 19건 중 12건이 태그 4~5개를 달고 있었다.
	//
	// 이게 화면 세 곳에서 값을 치른다: 태그 필터의 후보 목록, 통계의 facet 개수, 그리고
	// FTS의 tags 열. 카드가 `tags.slice(0, 3)`으로 가려 주는 것은 목록뿐이다.
	topK = 3
)

// Classify는 콘텐츠를 사전에 대해 분류해 상위 태그(≤topK, threshold 이상)를 돌려준다.
// 신호 = 도메인맵 + title/keywords/description/note/body 필드별 사전 매칭의 가법 스코어. 필드마다 따로
// 토큰화해 구문이 필드 경계를 넘지 못하게 한다. 출력은 결정적(score desc, 동점 name asc).
func Classify(c Content, d *Dictionary) []ScoredTag {
	score := map[int64]float64{}
	// uncapped는 **상한(capN)만 뺀 같은 점수**다. 순위에는 쓰지 않고 **동점 파괴에만** 쓴다.
	uncapped := map[int64]float64{}

	// 도메인 신호 — 호스트에 매핑된 각 태그에 +wDomain.
	for _, name := range DomainTags(c.Domain) {
		if id, ok := d.nameToID[strings.ToLower(name)]; ok {
			score[id] += wDomain
		}
	}
	addField(score, uncapped, d, c.Title, wTitle)
	addField(score, uncapped, d, c.Keywords, wKeywords)
	addField(score, uncapped, d, c.Description, wDesc)
	addField(score, uncapped, d, c.Note, wNote)
	// 본문 요구치는 길이에 비례하고, 뒷받침이 있으면 한 단계 낮춘다.
	// 여기까지의 score에 있는 태그 = 도메인·제목·설명이 이미 가리킨 태그다.
	corroborated := make(map[int64]bool, len(score))
	for id := range score {
		corroborated[id] = true
	}
	addBody(score, uncapped, d, c.Body, corroborated)

	out := make([]ScoredTag, 0, len(score))
	for id, s := range score {
		if s >= threshold {
			// confidence = 1 - 1/(1+score): score 1→0.5, 3→0.75, 5→0.833. corpus 무의존·결정적.
			out = append(out, ScoredTag{TagID: id, Confidence: 1 - 1/(1+s)})
		}
	}
	// 동점 파괴: 점수 → **상한 없는 점수** → 태그 이름.
	//
	// **왜 상한 없는 점수인가.** `capN`은 키워드 스터핑이 **점수를 부풀리는 것**을 막는
	// 장치다. 그런데 상한에 걸린 태그가 여럿이면 그 태그들의 점수가 전부 같아져서,
	// 상한이 **누가 위인지까지** 정해 버린다 — 정확히는 알파벳순이 정하게 된다.
	// 그건 상한이 하려던 일이 아니다.
	//
	// 실측(2026-07-27, `ruliweb.com` 본문 8,946자): `game`이 **45회** 매칭인데
	// `finance`(4회)와 점수가 같았다. 상한이 11배 차이를 지우고 정답을 알파벳순 5위로
	// 보냈다. 상한 없는 점수는 이미 계산돼 있고 버려지던 값이라, 그걸 2차 키로만 쓴다.
	//
	// **점수 자체는 건드리지 않는다** — 스터핑 방지는 그대로다. 순위가 갈리지 않을 때만
	// 개입하므로 동점이 아닌 쌍의 순서는 절대 바뀌지 않는다.
	//
	// 이름 비교는 마지막에 남긴다 — 상한 없는 점수까지 같으면 결정적 순서가 필요하다.
	sort.Slice(out, func(i, j int) bool {
		si, sj := score[out[i].TagID], score[out[j].TagID]
		if si != sj {
			return si > sj
		}
		ui, uj := uncapped[out[i].TagID], uncapped[out[j].TagID]
		if ui != uj {
			return ui > uj
		}
		return d.idToName[out[i].TagID] < d.idToName[out[j].TagID]
	})
	if len(out) > topK {
		out = out[:topK]
	}
	return out
}

// addField는 한 필드를 토큰화·매칭해 태그별 (상한 적용) 매칭 수 × weight를 score에 더한다.
// uncapped에는 **같은 계산을 상한 없이** 누적한다 — 동점 파괴에만 쓴다(sortScored 주석).
func addField(score, uncapped map[int64]float64, d *Dictionary, text string, weight float64) {
	addFieldMin(score, uncapped, d, text, weight, 1)
}

// addBody는 본문 매칭을 더한다. 뒷받침이 있는 태그와 없는 태그에 서로 다른 최소
// 횟수를 적용한다 — 같은 두 번이라도 제목이 거드는 두 번과 본문에만 있는 두 번은
// 증거로서 무게가 다르다.
func addBody(score, uncapped map[int64]float64, d *Dictionary, text string, corroborated map[int64]bool) {
	if text == "" {
		return
	}
	// 길이 비례 요구치. 상한은 matchCap과 같다 — 그보다 높이면 어떤 태그도 통과할 수 없다.
	need := 1 + utf8.RuneCountInString(text)/runesPerMatch
	if need > matchCap {
		need = matchCap
	}
	for id, h := range d.matchField(Tokenize(text)) {
		min := need
		if corroborated[id] && min > minBodyCorroborated {
			// 뒷받침이 있으면 한 단계 낮춘다 — 같은 두 번이라도 제목이 거드는 두 번은
			// 본문에만 있는 두 번보다 무겁다.
			min = minBodyCorroborated
		}
		if h.n < min {
			continue
		}
		// 횟수 요구(min)는 IDF보다 **앞에** 있다. 흔한 낱말이라고 횟수를 면제해 주면
		// 걸러야 할 약한 신호가 배율만 낮춘 채 그대로 들어온다.
		score[id] += wBody * float64(capN(h.n)) * h.mul
		uncapped[id] += wBody * float64(h.n) * h.mul
	}
}

// capN은 한 필드에서 한 태그의 기여를 matchCap으로 자른다 (키워드 스터핑 방지).
func capN(n int) int {
	if n > matchCap {
		return matchCap
	}
	return n
}

// addFieldMin은 min회 미만 매칭은 무시한다 — 긴 필드에서 한 번 스친 언급을 걸러낸다.
func addFieldMin(score, uncapped map[int64]float64, d *Dictionary, text string, weight float64, min int) {
	if text == "" {
		return
	}
	for id, h := range d.matchField(Tokenize(text)) {
		if h.n < min {
			continue
		}
		score[id] += weight * float64(capN(h.n)) * h.mul
		uncapped[id] += weight * float64(h.n) * h.mul
	}
}
