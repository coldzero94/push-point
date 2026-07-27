package scraper

import "strings"

// ErrBlockedPage는 **응답은 200인데 내용이 페이지가 아니라 벽**일 때 반환된다.
//
// 봇 차단·인증 대기·로그인 벽은 HTTP 오류로 오지 않는다. 200과 함께 안내문 몇 줄을 준다.
// 그걸 그대로 저장하면 세 가지가 한꺼번에 벌어진다:
//
//  1. **벽의 문구가 태그가 된다.** imdb와 dribbble이 똑같이 166자짜리
//     `"JavaScript is disabled … not a robot"`을 주는데, 태거는 거기서 `javascript`를
//     뽑았다. 차단 페이지가 자기 이름을 태그로 만든 것이다.
//  2. **상태가 `done`이다.** 오류도 로그도 없어서 사용자도 eval도 정상 저장과 구분하지 못한다.
//  3. **재수집해도 영원히 같다.** 태거를 아무리 고쳐도 신호가 없다 — 이건 태거 문제가 아니다.
//
// 그래서 성공이 아니라 실패로 낸다. 큐가 `links.error`에 사유를 남기고 status를 `failed`로
// 두므로 화면에서 보이고, `body_source='client'`인 링크(확장·공유 시트가 본문을 준 링크)는
// `done`을 유지한다 — **해법이 클라이언트 캡처라는 사실이 데이터 모델에 이미 들어 있다.**
var ErrBlockedPage error = blockedErr{}

// blockedErr는 queue.Permanent를 만족한다 — 벽은 30초 뒤에도 벽이므로 재시도가 무의미하고,
// 재시도는 이미 우리를 막은 사이트를 더 두드리는 일이다. 큐가 이 인터페이스만 보므로
// scraper와 queue 사이에 import 의존이 생기지 않는다.
type blockedErr struct{}

func (blockedErr) Error() string {
	return "scraper: 봇 차단·로그인 벽이라 서버가 페이지를 읽지 못했습니다 — 브라우저 확장이나 공유 시트로 저장하면 본문이 함께 들어옵니다"
}
func (blockedErr) Permanent() bool { return true }

// blockedMaxRunes는 벽 판정에 걸리는 최대 신호 길이.
//
// **문구만으로 판정하지 않는 이유**가 이 상수다. 봇 차단을 다룬 진짜 기사도 "not a robot"을
// 쓸 수 있는데, 그런 글은 길다. 실측된 벽은 전부 훨씬 짧다 — imdb 166자, Reddit 74자,
// threads 219자. 길이 문턱을 함께 걸면 오탐이 사실상 사라진다.
const blockedMaxRunes = 400

// blockedPhrases는 벽이 스스로를 드러내는 문구들. 소문자로 비교한다.
//
// 이 목록은 **완전할 수 없다** — 사이트마다 문구가 다르고 바뀐다. 목표는 전수 차단이 아니라
// 실측으로 확인된 것을 잡는 것이고, 못 잡은 벽은 예전처럼 빈약한 스냅샷으로 남아
// `just eval`의 "신호 200자 미만" 집계에 잡힌다. 즉 놓쳐도 조용해지지는 않는다.
var blockedPhrases = []string{
	// 봇 차단 (imdb·dribbble에서 실측)
	"javascript is disabled",
	"enable javascript",
	"not a robot",
	"checking your browser",
	"attention required",
	"unusual traffic",
	"captcha",
	// 인증 대기 (reddit에서 실측)
	"please wait for verification",
	// 접근 거부
	"access denied",
	"access to this page has been denied",
	// 한국어 로그인 벽 (threads에서 실측)
	"로그인 또는 가입",
	"로그인이 필요",
	"자바스크립트를 활성화",
	"자바스크립트를 사용",
}

// isBlockedPage는 추출된 내용이 페이지가 아니라 벽인지 본다.
//
// 판정은 **길이 × 문구** 두 조건의 곱이다. 어느 한쪽만으로는 안 된다 — 길이만 보면 진짜
// 짧은 글(스포티파이 곡 페이지는 143자다)이 걸리고, 문구만 보면 차단을 다룬 기사가 걸린다.
//
// 세 필드를 합쳐서 본다. 벽은 어디에든 나타난다 — Reddit은 **제목**이
// `"Reddit - Please wait for verification"`이고, imdb는 제목이 비고 **본문**에만 있다.
func isBlockedPage(title, description, bodyText string) bool {
	combined := title + " " + description + " " + bodyText
	if len([]rune(combined)) >= blockedMaxRunes {
		return false
	}
	lower := strings.ToLower(combined)
	for _, p := range blockedPhrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
