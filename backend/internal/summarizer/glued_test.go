package summarizer

import "testing"

// 상세 화면 요약 자리에 광고가 뜬 사고(2026-07-29)를 고정한다.
func TestIsProseRejectsGluedDOMText(t *testing.T) {
	ad := "이 제품 '시험 착용' 해보세요연구원16년차 / SponsoredSponsored뿌리 깊은 기미, " +
		"제일 후회되는 3가지 행동chunsooae / SponsoredSponsored남편이랑 어디가면 딸인 줄 " +
		"알아~ 아무도 몰랐던 그녀의 피부탄력 비결 '이것'More From NBC NewsNBC NEWS / " +
		"SHOPHow long should you keep towels before throwing them out?"
	if IsProse(ad) {
		t.Fatal("광고 위젯이 이어 붙은 문자열을 산문으로 통과시켰다")
	}
}

func TestGluedRepeatPrecision(t *testing.T) {
	// 통과해야 하는 것 — 실제 산문. 골든 9,028문장에서 이 패턴의 오탐은 0.01%였다.
	for _, s := range []string{
		"The branch predictor learns the pattern, so the pipeline stall disappears entirely.",
		"정렬된 배열에서는 분기 예측이 거의 항상 맞아떨어져 파이프라인이 비지 않는다.",
		"He said that that argument was weak.",         // 같은 단어 반복이지만 공백이 있다
		"We tested it on macOS and on iOS in the lab.", // 대소문자 전이는 흔하다
	} {
		if !IsProse(s) {
			t.Errorf("산문을 걸렀다: %q", s)
		}
	}
	// 걸러야 하는 것 — 구분자 없이 붙은 **연속 라틴 토큰**의 반복.
	for _, s := range []string{
		"NewsletterNewsletter sign up for our daily briefing on the markets today.",
		"SubscribeSubscribe to read this story ad-free and support our newsroom.",
	} {
		if IsProse(s) {
			t.Errorf("붙은 반복을 통과시켰다: %q", s)
		}
	}
}

// 알려진 한계: 반복 단위에 공백이 들어간 것("Read moreRead more")은 잡지 못한다.
//
// 공백을 허용하도록 넓히면 "He said that that argument was weak" 같은 **정상 문장**이
// 걸린다("that " 반복). 실제로 사고를 낸 문자열은 연속 토큰 형태였고, 골든 9,028문장으로
// 오탐률을 잰 것도 그 형태다. **재지 않은 범위로 규칙을 넓히지 않는다** — 넓히려면
// 같은 방식으로 다시 재고 나서 한다.
func TestGluedRepeatKnownLimitation(t *testing.T) {
	s := "Read moreRead more about the topic and then decide what to do next."
	if !IsProse(s) {
		t.Skip("공백을 포함한 반복까지 잡게 되었다면 위 주석과 함께 이 테스트를 지울 것")
	}
}
