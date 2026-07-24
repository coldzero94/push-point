package tagger

import (
	"strings"
	"unicode/utf8"
)

// matchField는 한 필드의 토큰열에서 매칭된 tagID별 히트 수를 센다(1패스).
func (d *Dictionary) matchField(docToks []string) map[int64]int {
	hits := map[int64]int{}
	for i, dt := range docToks {
		// 1) 정확일치 (라틴/숫자/1룬) — 토큰 동등이라 단어경계가 공짜.
		//    "ai"/"ml" 같은 <3자 라틴은 부분문자열이 아니라 토큰 동등만 매칭('email'↛ai).
		for _, id := range d.exactLatin[dt] {
			hits[id]++
		}
		// 2) 한글 prefix — dt가 surface로 시작. Normalize가 조사를 이미 벗겼고, 복합명사는 흡수.
		for _, p := range d.koPrefix {
			if strings.HasPrefix(dt, p.surface) {
				hits[p.tagID]++
			}
		}
		// 3) 다중어 구문 — firstTok 히트 시에만 tail 연속 검증(비용 최소).
		for _, ph := range d.phrases[dt] {
			if matchTail(docToks, i+1, ph.tail) {
				hits[ph.tagID]++
			}
		}
	}
	return hits
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
