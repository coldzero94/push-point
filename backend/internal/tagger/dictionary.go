package tagger

import (
	"strings"
	"unicode/utf8"
)

// Content는 태거 입력 — 런타임에 links에서 읽는 필드. Body는 go-trafilatura 추출 본문
// (비-아티클/추출실패면 빈 값 — 그때는 title/description으로 graceful degrade).
type Content struct {
	Domain      string
	Title       string
	Description string
	// Keywords는 발행자가 스스로 붙인 분류(meta keywords·article:section)다.
	// 우리가 추측한 값이 아니라 사이트가 알려준 값이라 신호가 강하다.
	Keywords string
	Note     string
	Body     string
}

// TagEntry는 사전 한 항목. store가 DB tags 테이블에서 읽어 넘긴다(태거는 store를 모른다).
type TagEntry struct {
	ID      int64
	Name    string
	Aliases []string
	Facet   string
}

// ScoredTag는 분류 결과 한 건. confidence는 (0,1) 단조.
type ScoredTag struct {
	TagID      int64
	Confidence float64
}

type koEntry struct {
	surface string // 한글 ≥2룬 — prefix 매칭
	tagID   int64
}

type phraseEntry struct {
	tail  []string // firstTok 이후 토큰들 — 연속 일치 검증
	tagID int64
}

// Dictionary는 매칭용으로 컴파일된 사전 인덱스.
type Dictionary struct {
	exactLatin map[string][]int64       // 라틴/숫자/1룬 surface 토큰 → 정확일치 tagID들
	koPrefix   []koEntry                // 한글 ≥2룬 surface
	phrases    map[string][]phraseEntry // 다중어 surface: firstTok → tail+tagID
	nameToID   map[string]int64         // 소문자 이름 → tagID (도메인맵 해소용)
	idToName   map[int64]string         // 동점 정렬(name asc)용
	// idToSurfaces는 태그가 가진 표면형 전부(이름 제외, 기재 순). **검색 확장이 쓴다** —
	// 한국어 표면이 매칭되면 형제인 영어 표면을 얹어 제목을 직접 때리기 위해서다
	// (match.go의 TermsInQuery). 매칭 인덱스 셋과 달리 토큰화하지 않은 원문을 담는다:
	// FTS에 넣을 것이라 `machine learning`처럼 공백이 있는 것도 그대로 필요하다.
	idToSurfaces map[int64][]string
	corpus       CorpusStats // 자체 코퍼스 DF — 제로값이면 IDF 비활성(idf.go)
}

// WithCorpus는 자체 코퍼스 통계를 붙인 **새 사전**을 돌려준다. 사전 컴파일과 코퍼스 통계는
// 수명이 다르다 — 사전은 시드에서 오고 코퍼스는 저장할 때마다 자란다 — 그래서
// BuildDictionary의 인자가 아니라 별도 단계다.
//
// 원본을 고치지 않고 복사하는 이유: 인덱스(맵·슬라이스)는 읽기 전용으로 공유해도 되지만,
// 제자리에서 코퍼스를 갈아 끼우면 그 사전을 이미 들고 있는 쪽의 결과가 말없이 바뀐다.
func (d *Dictionary) WithCorpus(c CorpusStats) *Dictionary {
	cp := *d
	cp.corpus = c
	return &cp
}

// BuildDictionary는 태그 항목을 매칭 인덱스로 컴파일한다. 각 태그의 name 자체 + 모든 alias를
// surface로 삼아 Tokenize한 뒤 토큰 수·한글 유무로 3분류한다:
//   - 1토큰 & (라틴/숫자 또는 1룬 한글) → exactLatin (정확일치. 라틴<3 부분문자열 금지가 공짜)
//   - 1토큰 & 한글 ≥2룬             → koPrefix (조사잔여·복합명사 흡수)
//   - 2토큰 이상                     → phrases (machine learning, ci/cd, 대규모 언어 모델)
func BuildDictionary(entries []TagEntry) *Dictionary {
	d := &Dictionary{
		exactLatin:   map[string][]int64{},
		phrases:      map[string][]phraseEntry{},
		nameToID:     map[string]int64{},
		idToName:     map[int64]string{},
		idToSurfaces: map[int64][]string{},
	}
	for _, e := range entries {
		d.nameToID[strings.ToLower(e.Name)] = e.ID
		d.idToName[e.ID] = e.Name
		surfaces := append([]string{e.Name}, e.Aliases...)
		d.idToSurfaces[e.ID] = append([]string(nil), e.Aliases...)
		for _, s := range surfaces {
			toks := Tokenize(s)
			switch {
			case len(toks) == 0:
				// surface가 전부 구분자/빈 값 — 스킵
			case len(toks) == 1:
				t := toks[0]
				if hasHangul(t) && utf8.RuneCountInString(t) >= 2 {
					d.koPrefix = append(d.koPrefix, koEntry{surface: t, tagID: e.ID})
				} else {
					d.exactLatin[t] = append(d.exactLatin[t], e.ID)
				}
			default:
				d.phrases[toks[0]] = append(d.phrases[toks[0]], phraseEntry{tail: toks[1:], tagID: e.ID})
			}
		}
	}
	return d
}

// hasHangul은 문자열에 한글 음절(가~힣)이 하나라도 있는지 본다.
func hasHangul(s string) bool {
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 {
			return true
		}
	}
	return false
}
