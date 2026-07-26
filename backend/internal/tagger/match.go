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

// matchesKoSurface는 문서 토큰 dt가 한글 사전 표면 surface에 걸리는지 판정한다.
//
// **어절의 앞이나 뒤 어느 쪽이든 본다.** 예전에는 prefix만 봤고, 그래서 `"대박식당처럼"`이
// `식당`에 닿지 않았다. 한국어 복합명사는 **head-final**이다 — `대박식당`은 수식어+**식당**,
// `김치찌개`는 김치+**찌개**로 의미의 핵이 뒤에 온다. prefix만 보면 핵이 앞에 오는
// `식당가` 같은 경우만 잡고, 더 흔한 쪽을 통째로 놓친다.
//
// 실측(2026-07-26, 153건): prefix만 → prefix+suffix로 바꾸면
// **동결 test 0.885→0.902, wild 0.733→0.767, 한국어 wild 0.750→0.833.**
// dev는 0.952→0.935로 1건 내려가는데, 그 1건은 오매칭이 아니라 순위 밀림이다 —
// `유튜브 클론코딩 … 강의`에서 `클론코딩`이 `코딩`에 걸려 `dev`가 새로 붙었고(코딩 강의에
// 맞는 태그다) 라벨에 있던 `tutorial`을 3위 밖으로 밀었다.
//
// **`strings.Contains`가 아닌 이유**: 위 153건에서 두 규칙의 결과가 **완전히 동일했다.**
// 같은 값이면 좁은 쪽을 고른다 — 중간에 묻힌 표면까지 잡으면 나중에 오탐이 늘 여지가 크고,
// 그 이득이 있다는 증거는 아직 없다.
//
// **한계**: 이 규칙은 나쁜 별칭을 증폭한다. `경기`(sports 별칭)가 `불경기`에도 걸리게 된다.
// 사전 쪽 문제이고 별도 마이그레이션이 필요하다.
func matchesKoSurface(dt, surface string) bool {
	return strings.HasPrefix(dt, surface) || strings.HasSuffix(dt, surface)
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
		// 2) 한글 어절 경계 매칭 — dt의 앞이나 뒤가 surface. Normalize가 조사를 이미 벗겼고,
		//    복합명사는 어느 쪽에 붙든 흡수된다(matchesKoSurface 주석 참조).
		for _, p := range d.koPrefix {
			if matchesKoSurface(dt, p.surface) {
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
//
// **여기는 일부러 prefix로 남긴다.** 단일 토큰 매칭은 matchesKoSurface로 suffix까지 보게
// 바꿨지만, 구문 꼬리에 같은 규칙을 적용해도 153건에서 **어떤 수도 움직이지 않았다**
// (2026-07-26 실측). 이득의 증거가 없으면 넓히지 않는다. 구문은 이미 첫 토큰이 정확
// 동등이라(`d.phrases[dt]` 조회) 판정이 더 빡빡한 경로이기도 하다.
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
			// **matchField와 반드시 같은 규칙이어야 한다.** 누적 키와 조회 키가 갈라지면
			// DF가 조용히 0에 머물고 IDF는 아무 일도 하지 않는다. 그래서 두 곳 모두
			// matchesKoSurface를 부른다 — 규칙이 한 곳에만 있어야 어긋날 수가 없다.
			for _, p := range d.koPrefix {
				if matchesKoSurface(dt, p.surface) {
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
