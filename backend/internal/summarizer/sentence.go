// Package summarizer는 본문(body_text)에서 핵심 문장 2~3개를 골라내는 추출식 요약을 한다.
// 생성이 아니라 **원문 문장 선택**이라 환각이 0이고, LLM·외부 API·모델 파일이 전혀 없다
// (M5 Phase A. Phase B에서 문장 유사도만 임베딩으로 교체될 자리다 — 골격은 그대로 남는다).
//
// 형태소 분석기 없이 한국어+영어 혼합 텍스트를 다룬다: 문장 분리는 종결부호 + 뒤따르는
// 문자 종류 규칙, 토큰화는 tagger.Tokenize(조사 접미 제거) 재사용.
package summarizer

import (
	"strings"
	"unicode"
)

// 문장 분리·산문 판정 상수. 값의 근거는 golden 100건 실측이다(재측정은 just eval-summary).
const (
	// minSentRunes/maxSentRunes는 산문 문장의 길이 범위. 아래는 목차 항목·캡션, 위는
	// trafilatura가 블록을 납작하게 만들며 붙여 놓은 표·코드 덩어리다.
	minSentRunes = 25
	maxSentRunes = 400
	// letterRatioMin은 문장에서 글자가 차지해야 할 최소 비율 — 코드·표·이메일 목록을 거른다.
	letterRatioMin = 0.55
	digitRatioMax  = 0.12
	symbolRatioMax = 0.12
	// codeCharMax는 { } < > = ; | \ ( ) 의 허용 개수. 이걸 넘으면 산문이 아니라 코드다.
	codeCharMax = 2
)

// terminators는 문장 종결부호(한·영·전각).
func isTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', '…', '。', '！', '？':
		return true
	}
	return false
}

// isCloser는 종결부호 뒤에 붙어 **현재 문장에 흡수되어야 할** 닫는 기호다.
func isCloser(r rune) bool {
	switch r {
	case ')', ']', '}', '"', '\'', '»', '”', '’':
		return true
	}
	return false
}

// isOpener는 다음 문장의 시작으로 인정하는 여는 기호.
func isOpener(r rune) bool {
	switch r {
	case '(', '[', '{', '"', '\'', '«', '“', '‘':
		return true
	}
	return false
}

// abbreviations는 마침표로 끝나지만 문장을 끝내지 않는 약어(소문자 비교, 끝점 제거 후).
var abbreviations = map[string]bool{
	"e.g": true, "i.e": true, "etc": true, "vs": true, "cf": true, "al": true,
	"fig": true, "no": true, "vol": true, "ch": true, "ref": true, "approx": true,
	"dr": true, "mr": true, "mrs": true, "ms": true, "jr": true, "sr": true,
	"st": true, "inc": true, "ltd": true,
}

// Split은 텍스트를 문장으로 자른다. 형태소 분석기 없이 종결부호 + **뒤따르는 문자 종류**로
// 판정한다 — 이 조합이 URL·도메인·버전·파일명(뒤에 공백이 없다)을 통째로 막아 준다.
//
// 한국어 종결어미 사전은 쓰지 않는다: 실측상 한국어 문서도 마침표 종결이 압도적이라
// (`다.` 38~101회 vs 마침표 없는 `다 ` 0~7회) 사전은 정밀도만 깎는다.
func Split(text string) []string {
	rs := []rune(text)
	n := len(rs)
	var out []string
	start := 0
	cut := func(end int) {
		if s := strings.TrimSpace(string(rs[start:end])); s != "" {
			out = append(out, s)
		}
	}

	for i := 0; i < n; i++ {
		// R0: 개행은 무조건 경계. 현재 trafilatura 출력엔 개행이 없지만, 스크래퍼가
		// 블록 경계를 살리도록 개선되면 그날 바로 이득을 본다(1줄 비용의 선행 대비).
		if rs[i] == '\n' {
			cut(i)
			start = i + 1
			continue
		}
		r := rs[i]
		if !isTerminator(r) {
			continue
		}
		// G1 말줄임(… / ..): 문장 끝이 아니다.
		if r == '…' || (r == '.' && ((i > 0 && rs[i-1] == '.') || (i+1 < n && rs[i+1] == '.'))) {
			continue
		}
		// G2 소수점: 3.14의 점.
		if r == '.' && i > 0 && i+1 < n && unicode.IsDigit(rs[i-1]) && unicode.IsDigit(rs[i+1]) {
			continue
		}
		// 닫는 기호는 현재 문장이 가져간다: `...했다.")` 처럼.
		j := i + 1
		for j < n && isCloser(rs[j]) {
			j++
		}
		if j >= n { // 텍스트 끝
			cut(n)
			start = n
			break
		}
		// G3 종결부호 뒤가 공백이 아니면 문장 끝이 아니다 — URL·도메인·버전·파일명을
		// 정규식 없이 한 줄로 막는 규칙이다(example.com, v1.2.3, main.go).
		if !unicode.IsSpace(rs[j]) {
			continue
		}
		// G4 약어·이니셜: 직전 토큰이 e.g/Dr/vs 등이거나 라틴 대문자 1글자면 문장 끝이 아니다.
		if isAbbrevBefore(rs, i) {
			continue
		}
		k := j
		for k < n && unicode.IsSpace(rs[k]) {
			k++
		}
		// G4.5 인용 귀속: 닫는 부호를 흡수했고 뒤에 인용 조사가 오면 바깥 문장이 계속된다.
		if j > i+1 && k < n && continuesAfterQuote(rs, k) {
			continue
		}
		// G5 다음 문장의 첫 글자가 문장 시작다워야 한다(한글·대문자·숫자·여는 기호).
		if k < n && !isSentenceStart(rs[k]) {
			continue
		}
		cut(j)
		start = j
	}
	if start < n {
		cut(n)
	}
	return out
}

// isAbbrevBefore는 rs[i](마침표) 직전 토큰이 약어이거나 대문자 이니셜인지 본다.
func isAbbrevBefore(rs []rune, i int) bool {
	if rs[i] != '.' {
		return false
	}
	s := i - 1
	for s >= 0 && !unicode.IsSpace(rs[s]) {
		s--
	}
	orig := strings.TrimRight(string(rs[s+1:i]), ".")
	if orig == "" {
		return false
	}
	if abbreviations[strings.ToLower(orig)] {
		return true
	}
	// "J. Smith"의 이니셜 — **라틴 대문자** 한 글자만. 원문 대소문자로 판정해야 한다:
	// 소문자화한 값으로 "글자 한 개"만 보면 한국어 종결(`…이다.`의 `다`)이 이니셜로 오인돼
	// 한국어 문장이 통째로 안 잘린다(대부분의 한국어 문장이 여기 걸린다).
	r := []rune(orig)
	return len(r) == 1 && unicode.IsUpper(r[0])
}

// quoteParticles는 닫는 인용부호 뒤에 붙어 **문장이 계속됨**을 뜻하는 한국어 인용 조사다.
// `그는 "안녕." 이라 했다.` 에서 자르면 한 문장이 조각난다.
var quoteParticles = []string{"이라고", "이라", "라고", "라며", "라는", "이란", "하고"}

// continuesAfterQuote는 닫는 인용부호를 흡수한 뒤(k = 다음 비공백 위치) 인용 조사가
// 이어지는지 본다 — 그렇다면 종결부호는 인용문의 것이고 바깥 문장은 계속된다.
func continuesAfterQuote(rs []rune, k int) bool {
	rest := string(rs[k:min(k+8, len(rs))])
	for _, p := range quoteParticles {
		if strings.HasPrefix(rest, p) {
			return true
		}
	}
	return false
}

// isSentenceStart는 문장 첫 글자로 인정되는 문자인지 본다.
func isSentenceStart(r rune) bool {
	return isHangul(r) || unicode.IsUpper(r) || unicode.IsDigit(r) || isOpener(r) ||
		// 한자·가나 등 대소문자 개념이 없는 문자도 문장 시작으로 인정한다.
		(unicode.IsLetter(r) && !unicode.IsLower(r))
}

func isHangul(r rune) bool { return r >= 0xAC00 && r <= 0xD7A3 }

// IsProse는 문장이 **사람이 읽는 산문**인지 판정한다. 이 게이트가 없으면 어떤 요약기든
// 목차·코드 덤프·이메일 목록·네비게이션 텍스트를 "핵심 문장"으로 뽑는다(실측: 문장의 15%가
// 이 부류). 알고리즘보다 먼저 오는 방어선이다.
func IsProse(s string) bool {
	rs := []rune(s)
	n := len(rs)
	if n < minSentRunes || n > maxSentRunes {
		return false
	}
	// 닫는 기호를 걷어낸 마지막 글자가 종결부호여야 한다 — 잘린 조각·표 셀을 거른다.
	last := n - 1
	for last >= 0 && isCloser(rs[last]) {
		last--
	}
	if last < 0 || !isTerminator(rs[last]) {
		return false
	}

	var letters, digits, symbols, codeChars int
	for _, r := range rs {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			digits++
		case isSymbolish(r):
			symbols++
		}
		if isCodeChar(r) {
			codeChars++
		}
	}
	f := float64(n)
	return float64(letters)/f >= letterRatioMin &&
		float64(digits)/f <= digitRatioMax &&
		float64(symbols)/f <= symbolRatioMax &&
		codeChars <= codeCharMax
}

// isSymbolish는 산문에서 흔한 문장부호를 제외한 기호 — 많으면 코드·표다.
func isSymbolish(r rune) bool {
	switch r {
	case ',', '.', '\'', '"', '?', '!', '(', ')', '-', ' ':
		return false
	}
	return unicode.IsSymbol(r) || unicode.IsPunct(r)
}

func isCodeChar(r rune) bool {
	switch r {
	case '{', '}', '<', '>', '=', ';', '|', '\\', '(', ')':
		return true
	}
	return false
}
