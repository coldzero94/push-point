package scraper

import "strings"

// footerMarkers는 **페이지 내용이 아니라 사이트 하단 고지**임을 드러내는 문구들.
// 소문자로 비교한다.
var footerMarkers = []string{
	"이용약관",
	"개인정보처리방침",
	"사업자등록번호",
	"통신판매업",
	"고객센터",
	"청소년보호정책",
	"저작권",
	"제휴문의",
	"대표이사",
	"all rights reserved",
}

const (
	// footerMinMarkers는 푸터로 보기 위해 필요한 **서로 다른** 표식 수.
	footerMinMarkers = 3
	// footerMaxRunes는 푸터 판정에 걸리는 최대 본문 길이.
	footerMaxRunes = 1000
)

// isFooterOnly는 추출된 본문이 사실상 사이트 하단 고지뿐인지 본다.
//
// **개수가 아니라 밀도로 판정한다.** golden 153건을 재 보면 표식이 둘 이상 나오는 본문이
// 셋인데 성격이 완전히 다르다(2026-07-27 실측):
//
//	멜론 곡 페이지     355자 · 표식 7  → 355자 전부가 회사 정보·약관이다
//	티스토리 홈        514자 · 표식 4  → 내비게이션 + 오늘의 인기글 제목
//	앱스토어(토스)   2,428자 · 표식 2  → **진짜 앱 설명**이고 표식은 곁다리다
//
// 개수만 보면 앱스토어가 걸리고, 길이만 보면 짧은 진짜 글이 걸린다. 둘을 곱해야 앞의 둘만
// 남는다. 벽 판정(blocked.go)과 같은 형태이고 같은 이유다.
//
// **왜 버리는가**: 푸터에서 뽑은 태그는 그 링크가 아니라 회사의 법적 고지를 설명한다.
// 실제로 멜론 푸터의 주소 `경기도 성남시`가 `sports`의 별칭 `경기`에 걸려 곡 페이지에
// `sports`가 붙었다. 본문을 버리면 그 태그가 사라지고, 제목·설명은 그대로라 곡 정보는 남는다.
// 실측: 버려도 Recall@3는 세 세트 모두 불변이고 `sports` 오탐 행이 사라진다.
func isFooterOnly(bodyText string) bool {
	if bodyText == "" || len([]rune(bodyText)) >= footerMaxRunes {
		return false
	}
	lower := strings.ToLower(bodyText)
	n := 0
	for _, m := range footerMarkers {
		if strings.Contains(lower, m) {
			n++
			if n >= footerMinMarkers {
				return true
			}
		}
	}
	return false
}
