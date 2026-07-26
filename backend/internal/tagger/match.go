package tagger

import (
	"strings"
	"unicode/utf8"
)

// fieldHit은 한 필드에서 한 태그가 얻은 증거. 개수와 배율을 **따로** 들고 다닌다.
//
// 왜 곱해서 하나로 합치지 않는가: 상한(matchCap)은 "몇 번 나왔나"에 걸어야 하는 값이다.
// 배율을 미리 곱해 버리면 흔한 낱말이 다섯 번 나온 것과 희귀한 낱말이 두 번 나온 것이
// 같은 수가 되어, 키워드 스터핑을 막으려던 상한이 그 둘을 구분하지 못한다.
type fieldHit struct {
	n int // 매칭 횟수 (matchCap이 걸리는 대상)
	// mul은 매칭된 표면들 중 **가장 변별력 있는 것**의 IDF 배율이다.
	// 최댓값을 쓰는 이유: 한 태그가 `go`(흔함)와 `golang`(희귀)에 동시에 걸렸다면
	// 실제 증거는 희귀한 쪽이다. 평균을 내면 강한 증거가 약한 증거에 희석된다.
	mul float64
}

// add는 표면 하나의 매칭을 누적한다.
func (h *fieldHit) add(mul float64) {
	h.n++
	if mul > h.mul {
		h.mul = mul
	}
}

// matchField는 한 필드의 토큰열에서 매칭된 tagID별 히트를 센다(1패스).
func (d *Dictionary) matchField(docToks []string) map[int64]fieldHit {
	hits := map[int64]fieldHit{}
	record := func(id int64, surface string) {
		h := hits[id]
		h.add(d.idfMul(surface))
		hits[id] = h
	}
	for i, dt := range docToks {
		// 1) 정확일치 (라틴/숫자/1룬) — 토큰 동등이라 단어경계가 공짜.
		//    "ai"/"ml" 같은 <3자 라틴은 부분문자열이 아니라 토큰 동등만 매칭('email'↛ai).
		for _, id := range d.exactLatin[dt] {
			record(id, dt)
		}
		// 2) 한글 prefix — dt가 surface로 시작. Normalize가 조사를 이미 벗겼고, 복합명사는 흡수.
		for _, p := range d.koPrefix {
			if strings.HasPrefix(dt, p.surface) {
				// DF는 사전 표면 기준으로 세므로 여기서도 문서 토큰(dt)이 아니라
				// **사전 표면**을 넘긴다 — 아니면 "쿠버네티스가"와 "쿠버네티스"가 다른
				// 통계로 갈라져 DF가 영원히 0에 머문다.
				record(p.tagID, p.surface)
			}
		}
		// 3) 다중어 구문 — firstTok 히트 시에만 tail 연속 검증(비용 최소).
		for _, ph := range d.phrases[dt] {
			if matchTail(docToks, i+1, ph.tail) {
				record(ph.tagID, phraseKey(dt, ph.tail))
			}
		}
	}
	return hits
}

// phraseKey는 다중어 표면의 DF 키 — 구문 전체가 하나의 낱말이다.
// ("machine"의 DF가 아니라 "machine learning"의 DF를 봐야 한다.)
func phraseKey(first string, tail []string) string {
	return strings.Join(append([]string{first}, tail...), " ")
}

// matchTail은 docToks[start:]가 tail과 연속(contiguous) 일치하는지 본다. tail 각 토큰은
// 자기 클래스 규칙으로 검증: 한글 ≥2룬 = prefix, 그 외(라틴/숫자/1룬) = 정확 동등.
func matchTail(docToks []string, start int, tail []string) bool {
	if start+len(tail) > len(docToks) {
		return false
	}
	for j, t := range tail {
		dt := docToks[start+j]
		if hasHangul(t) && utf8.RuneCountInString(t) >= 2 {
			if !strings.HasPrefix(dt, t) {
				return false
			}
		} else if dt != t {
			return false
		}
	}
	return true
}

// Surfaces는 사전의 모든 표면을 DF 키 형태로 돌려준다 — corpus_df 누적 파이프라인이
// "무엇을 세야 하는가"를 사전에게 묻기 위한 것이다. 매칭과 누적이 같은 키를 쓰지 않으면
// DF가 조용히 0에 머물고 IDF는 아무 일도 하지 않는다.
func (d *Dictionary) Surfaces() []string {
	out := make([]string, 0, len(d.exactLatin)+len(d.koPrefix)+len(d.phrases))
	for s := range d.exactLatin {
		out = append(out, s)
	}
	for _, e := range d.koPrefix {
		out = append(out, e.surface)
	}
	for first, entries := range d.phrases {
		for _, ph := range entries {
			out = append(out, phraseKey(first, ph.tail))
		}
	}
	return out
}

// MatchedSurfaces는 문서 하나에서 매칭된 **사전 표면의 집합**을 돌려준다(중복 없음).
// corpus_df 누적의 입력이다 — 한 문서가 한 표면을 몇 번 썼든 DF에는 1만 기여한다.
func (d *Dictionary) MatchedSurfaces(c Content) map[string]bool {
	out := map[string]bool{}
	for _, text := range []string{c.Title, c.Description, c.Keywords, c.Note, c.Body} {
		if text == "" {
			continue
		}
		toks := Tokenize(text)
		for i, dt := range toks {
			if len(d.exactLatin[dt]) > 0 {
				out[dt] = true
			}
			for _, p := range d.koPrefix {
				if strings.HasPrefix(dt, p.surface) {
					out[p.surface] = true
				}
			}
			for _, ph := range d.phrases[dt] {
				if matchTail(toks, i+1, ph.tail) {
					out[phraseKey(dt, ph.tail)] = true
				}
			}
		}
	}
	return out
}
