package summarizer

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/coby/push-point/backend/internal/tagger"
)

// 요약 파이프라인 상수. 값은 golden 100건 실측에서 왔고, 재측정 경로는 just eval-summary다.
// 가드 임계값은 **공개**한다 — eval 하네스가 "왜 요약이 없었는가"를 같은 값으로 세야
// 하고(중복 리터럴은 조용히 어긋난다), 그 분해가 임계값을 사람에게 계속 보이게 한다.
const (
	// MinBodyRunes 미만이면 요약하지 않는다 — 본문이 사실상 없는 SPA 블로그(velog·toss·brunch)다.
	MinBodyRunes = 200
	// MinProseSents 미만이면 요약하지 않는다 — 고를 문장이 없다.
	MinProseSents = 3
	// rankThreshold 미만이면 그래프가 거의 완전그래프라 중심성이 무의미하다 → 문서 순서를 쓴다.
	rankThreshold = 5
	// k3Threshold 이상이면 3문장, 아니면 2문장.
	k3Threshold = 8
	// maxSents는 O(n²) 유사도 계산 방어선(실측 최대 351문장).
	maxSents = 200
	// maxSummaryRunes는 요약 총 길이 상한 — 인스펙터 폭 기준 약 6줄.
	maxSummaryRunes = 450
	// DescDropRatio: 요약이 description과 이만큼 겹치면 통째로 버린다. 같은 말을 두 번 하는
	// 화면을 막는 **제품 규칙**이며, 랭킹을 바꾸는 손잡이가 아니다(발동 건수는 eval이 노출).
	DescDropRatio = 0.8
)

// Summarize는 본문에서 핵심 문장 2~3개를 골라 개행으로 이어 돌려준다.
// 요약할 수 없거나 해서는 안 되면 **빈 문자열**이다 — 호출자·UI는 그때 아무것도 그리지 않는다.
// description은 중복 회피에만 쓰이고 요약 재료로는 쓰이지 않는다(요약은 본문에서만 나온다).
//
// 5겹 가드: 본문 길이 → 산문 게이트 → 산문 문장 수 → 길이 캡 → description 중복.
// 실측상 저장 링크의 약 71%에서 요약이 나오고 나머지는 빈 문자열이다(태거와 같은 graceful degrade).
func Summarize(body, description string) string {
	if utf8.RuneCountInString(body) < MinBodyRunes {
		return ""
	}
	// 완전히 같은 문장은 한 번만 담는다 — 반복되는 보일러플레이트가 서로 완전 유사한
	// clique를 만들어 중심성을 독식하는 것을 그래프 구성 전에 없앤다.
	var prose []string
	seen := make(map[string]bool)
	for _, s := range Split(body) {
		if !IsProse(s) || seen[s] {
			continue
		}
		seen[s] = true
		prose = append(prose, s)
		if len(prose) == maxSents {
			break
		}
	}
	if len(prose) < MinProseSents {
		return ""
	}

	k := 2
	if len(prose) >= k3Threshold {
		k = 3
	}

	tokens := make([][]string, len(prose))
	for i, s := range prose {
		tokens[i] = contentTokens(s)
	}
	descTok := contentTokens(description)

	var picked []int
	if len(prose) < rankThreshold {
		// 문장이 너무 적어 그래프가 정보를 못 준다 — 앞에서부터 k개(문서 순서).
		for i := 0; i < k; i++ {
			picked = append(picked, i)
		}
	} else {
		picked = selectMMR(tokens, centrality(tokens), descTok, k)
	}
	sort.Ints(picked) // 문서 등장 순서로 읽히게

	// 길이 캡 — 첫 문장은 무조건 넣고, 이후는 총 룬수가 상한 안일 때만.
	var out []string
	total := 0
	for _, i := range picked {
		n := utf8.RuneCountInString(prose[i])
		if len(out) > 0 && total+n > maxSummaryRunes {
			continue
		}
		out = append(out, prose[i])
		total += n
	}
	summary := strings.Join(out, "\n")

	// 최종 노출 게이트: description과 사실상 같은 말이면 보여주지 않는다.
	if Containment(contentTokens(summary), descTok) >= DescDropRatio {
		return ""
	}
	return summary
}

// LeadN은 앞에서부터 산문 n문장을 잇는다 — eval의 **상시 베이스라인**이다.
// lead 베이스라인은 뉴스·아티클에서 강력해서, TextRank가 이걸 못 이기면 복잡도가
// 정당화되지 않는다. 그 사실을 매 측정마다 코드로 확인한다.
func LeadN(body string, n int) string {
	var out []string
	for _, s := range Split(body) {
		if !IsProse(s) {
			continue
		}
		out = append(out, s)
		if len(out) == n {
			break
		}
	}
	return strings.Join(out, "\n")
}

// Containment는 비대칭 겹침 |a∩b| / |a| — "a의 토큰 중 b에도 있는 비율"이다.
// MMR 페널티·최종 노출 게이트·eval의 중복 지표가 전부 이 함수 하나를 쓴다.
// 두 인자는 contentTokens가 만든 **정렬·중복제거된** 슬라이스여야 한다.
func Containment(a, b []string) float64 {
	if len(a) == 0 {
		return 0
	}
	return float64(intersectCount(a, b)) / float64(len(a))
}

// contentTokens는 문장을 유사도 계산용 토큰 집합으로 만든다: tagger.Tokenize(조사 접미
// 제거·소문자) → 불용어·1룬 토큰 제거 → 정렬·중복 제거.
// **정렬 슬라이스로 보관해 맵 순회를 없앤 것이 출력 결정성의 근거다.**
func contentTokens(s string) []string {
	raw := tagger.Tokenize(s)
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		if stopwords[t] || utf8.RuneCountInString(t) < 2 {
			continue
		}
		out = append(out, t)
	}
	sort.Strings(out)
	return slicesCompact(out)
}

// slicesCompact은 정렬된 슬라이스에서 연속 중복을 제거한다(제자리).
func slicesCompact(s []string) []string {
	if len(s) < 2 {
		return s
	}
	j := 1
	for i := 1; i < len(s); i++ {
		if s[i] != s[j-1] {
			s[j] = s[i]
			j++
		}
	}
	return s[:j]
}

// intersectCount는 정렬된 두 슬라이스의 공통 원소 수를 병합 스캔으로 센다(맵 없음).
func intersectCount(a, b []string) int {
	n, i, j := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			n++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return n
}
