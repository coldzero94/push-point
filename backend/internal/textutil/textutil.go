// Package textutil은 신뢰할 수 없는 텍스트 입력(클라이언트 캡처·스크랩 결과)을 저장 전에
// 다듬는 공통 규칙을 담는다. 스크래퍼와 저장 API가 **같은 상한·같은 정제**를 쓰게 해서
// "어디로 들어왔느냐"에 따라 저장 형태가 달라지지 않게 한다.
package textutil

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// 저장 상한. 초과는 거부가 아니라 절단이다 — 클라이언트가 룬/바이트 경계를 서버와 똑같이
// 맞출 방법이 없으므로, 경계에서 400을 내면 정상 캡처가 조용히 실패하는 UX가 된다.
const (
	// MaxBodyText는 links.body_text 상한(32KB). 보일러플레이트를 제거한 본문은 대개 훨씬
	// 작아 병적 outlier만 자른다 — DB 비대화를 막되 태깅·요약 입력은 온전히 유지한다.
	MaxBodyText = 32 << 10
	// MaxTitle/MaxDescription은 목록·카드에 쓰이는 짧은 필드의 상한.
	MaxTitle       = 512
	MaxDescription = 2048
)

// CapRunes는 s를 최대 limit 바이트로 자르되 UTF-8 룬 경계에서 끊는다(멀티바이트 깨짐 방지).
func CapRunes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// SanitizeText는 저장 전 정제: 유효하지 않은 UTF-8 바이트를 버리고 C0 제어문자를 없앤다.
// allowNewline이면 개행(\n)은 남긴다 — 요약은 문장 구분에 개행을 쓴다.
// 탭·캐리지리턴은 공백으로 접는다(로그·표 오염 방지).
func SanitizeText(s string, allowNewline bool) string {
	s = strings.ToValidUTF8(s, "")
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n':
			if allowNewline {
				return r
			}
			return ' '
		case r == '\t' || r == '\r':
			return ' '
		case r == unicode.ReplacementChar:
			return -1
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}

// Clean은 정제 + 절단을 한 번에 한다 (저장 API·스크래퍼 공용 진입점).
func Clean(s string, limit int, allowNewline bool) string {
	return CapRunes(strings.TrimSpace(SanitizeText(s, allowNewline)), limit)
}
