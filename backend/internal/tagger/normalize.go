// Package tagger는 Push-Point의 런타임 태깅 추론(Phase A 규칙 태거, M5 ONNX)을 담는다.
// nlu/ 워크스페이스는 오프라인 자산(사전·golden)만 두고, 추론 코드는 전부 여기 Go에 있다.
//
// 이 파일은 그 토대인 한국어 정규화·토크나이즈다. 형태소 분석기 없이 조사(을/를/…)
// 접미를 벗겨 "쿠버네티스를" → "쿠버네티스"로 만든다. corpus_df 누적과 사전 매칭에
// **동일하게** 적용돼야(같은 함수) 두 경로의 토큰이 어긋나지 않는다 (계획 08 M3 Week1).
package tagger

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// particles는 벗겨낼 한국어 조사. 긴 것부터 검사해야 "자동으로"에서 '으로'를 벗겨
// "자동"이 되지(짧은 '로'를 먼저 벗기면 "자동으"가 된다). 전부 한글이라 영어 토큰은
// 자동으로 안전하다(접미가 절대 일치하지 않음). 정밀 우선 — 애매한 종결형은 뺀다.
var particles = []string{
	// 2자 이상 (긴 것 우선)
	"에서는", "에게서", "으로써", "으로서", "이라고",
	"에서", "에게", "한테", "으로", "부터", "까지", "처럼", "보다", "만큼",
	"밖에", "에는", "에도", "이나", "이란", "이라", "라고", "께서",
	// 1자
	"은", "는", "이", "가", "을", "를", "의", "에", "도", "만",
	"과", "와", "로", "나", "랑",
}

// stopHead는 접미가 조사여도 벗기면 안 되는 걸 거르기 위한 최소 어간 길이(룬).
// 한국어 내용어는 대개 2자 이상이라, 어간이 1자면(예: "평가"→"평") 벗기지 않는다.
const minStem = 2

// Normalize는 한 어절을 정규화한다: 앞뒤 문장부호 제거 → 소문자화 → 조사 접미 1회 제거.
// 조사는 어간이 minStem 룬 이상으로 남을 때만 벗긴다("평가"의 '가'는 보존). 영어·숫자
// 토큰은 조사 접미가 없으니 소문자화만 된다("Hello"→"hello", "said"→"said").
func Normalize(tok string) string {
	tok = strings.ToLower(strings.TrimFunc(tok, isTrimPunct))
	if tok == "" {
		return ""
	}
	for _, p := range particles {
		if !strings.HasSuffix(tok, p) {
			continue
		}
		stem := tok[:len(tok)-len(p)]
		if utf8.RuneCountInString(stem) >= minStem {
			return stem
		}
		break // 이 접미는 어간이 너무 짧다 — 더 짧은 다른 조사도 볼 필요 없이 원형 유지
	}
	return tok
}

// Tokenize는 텍스트를 어절로 쪼개(문자·숫자 외 구분자) 각각 Normalize한 결과를
// 순서대로 돌려준다. 빈 토큰은 버린다. 불용어·빈도 필터는 태거(소비자)의 몫이다.
func Tokenize(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if n := Normalize(f); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// isTrimPunct는 어절 가장자리에서 떼어낼 문자(문장부호·기호·공백). 내부 문자는
// FieldsFunc가 이미 나눴으므로 여기서는 경계 정리만 한다.
func isTrimPunct(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}
