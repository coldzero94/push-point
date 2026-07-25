package main

// summary-eval 서브커맨드 — 추출식 요약(M5 Phase A)의 회귀 탐지 하네스.
//
//	pushpoint summary-eval [golden-dir]        # 지표 출력 + dev 상대 게이트(실패 시 exit 1)
//	pushpoint summary-eval -dump [golden-dir]  # dev 10건의 요약 텍스트를 스팟체크용으로 출력
//
// **정직한 한계**: golden에 정답 요약이 없어 ROUGE를 못 낸다. 아래 지표는 전부 회귀
// 탐지기이지 품질 판정기가 아니다 — 가독성·논지 포착·문맥 단절은 사람이 -dump를 읽어야
// 판정된다. 게이트도 절대 임계값이 아니라 lead-3 베이스라인 대비 상대 조건만 건다.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/coby/push-point/backend/internal/summarizer"
	"github.com/coby/push-point/backend/internal/tagger"
)

const (
	summaryLeadN     = 3    // 베이스라인 문장 수
	tagPreserveSlack = 0.02 // 태그 보존율이 베이스라인보다 이만큼까지 낮은 건 허용
	dumpCount        = 10   // 스팟체크로 덤프할 dev 건수
)

// summaryMetrics는 한 시스템(TextRank 또는 lead-3)의 측정 결과다.
type summaryMetrics struct {
	n           int     // 전체 건수
	produced    int     // 요약이 나온 건수
	descDup     float64 // description과의 평균 겹침 (낮을수록 좋음)
	intraDup    float64 // 선택 문장 간 최대 겹침 평균 (낮을수록 좋음)
	compression float64 // len(요약)/len(본문) 평균
	tagRecall   float64 // 요약만 본문으로 준 태거의 Recall@3 (두 시스템 공통 산출분 기준)
	tagRecallN  int     // ④의 분모 — 두 시스템이 **모두** 요약을 낸 건수
	stable      float64 // 같은 입력 2회 → 바이트 동일 비율
}

func (m summaryMetrics) coverage() float64 { return ratio(m.produced, m.n) }

func runSummaryEval(args []string) error {
	dump := false
	if len(args) > 0 && args[0] == "-dump" {
		dump, args = true, args[1:]
	}
	dir := "nlu/golden"
	if len(args) > 0 {
		dir = args[0]
	}
	dict, id2name, err := loadEvalDict()
	if err != nil {
		return err
	}

	if dump {
		entries, err := loadGolden(filepath.Join(dir, "dev.jsonl"))
		if err != nil {
			return err
		}
		dumpSummaries(entries)
		return nil
	}

	fmt.Println("요약 eval — 정답 요약이 없어 ROUGE는 불가하다. 아래는 회귀 탐지기이지 품질 판정기가 아니며,")
	fmt.Println("가독성·논지 포착·문맥 단절은 `summary-eval -dump` 출력을 사람이 읽어야 판정된다.")

	var gateErr error
	devMeasured := false
	for _, name := range []string{"dev", "test"} {
		entries, err := loadGolden(filepath.Join(dir, name+".jsonl"))
		if err != nil {
			// dev가 없으면 게이트를 잴 수 없다 — 조용한 통과는 게이트가 아니다.
			// (test는 동결 세트라 출력 전용이므로 부재를 허용한다.)
			if os.IsNotExist(err) && name == "test" {
				fmt.Printf("\n(test 없음 — 건너뜀)\n")
				continue
			}
			return fmt.Errorf("dev golden을 읽지 못해 게이트를 판정할 수 없다 (%s): %w",
				filepath.Join(dir, name+".jsonl"), err)
		}
		if len(entries) == 0 && name == "dev" {
			return fmt.Errorf("dev golden이 비어 있어 게이트를 판정할 수 없다")
		}
		cmp := comparableMask(entries)
		tr := measureSummaries(entries, dict, id2name, false, cmp)
		lead := measureSummaries(entries, dict, id2name, true, cmp)
		reportSummary(strings.ToUpper(name), entries, tr, lead)
		reportByLang(entries, dict, id2name)
		reportExtraction(entries)
		if name == "dev" {
			devMeasured = true
			gateErr = checkSummaryGates(tr, lead)
		}
	}
	if gateErr != nil {
		return gateErr
	}
	if !devMeasured {
		return fmt.Errorf("dev 세트를 측정하지 못했다 — 게이트 미판정")
	}
	fmt.Println("\ndev 게이트 통과 (test는 동결 — 출력만).")
	return nil
}

// measureSummaries는 한 세트에 대해 TextRank(또는 lead-3 베이스라인) 지표를 계산한다.
// comparable은 TextRank와 lead-3가 **둘 다** 요약을 낸 항목의 마스크다. ④(태그 보존)의
// 분모는 반드시 이 집합이어야 한다 — lead-3에는 가드가 없어 커버리지가 구조적으로 높은데,
// 전체 건수로 나누면 "가드를 가진 쪽"이 가드 때문에 손해를 본다(그건 ①이 재는 축이다).
func comparableMask(entries []goldenEntry) []bool {
	out := make([]bool, len(entries))
	for i, e := range entries {
		b, d := e.Snapshot.BodyText, e.Snapshot.Description
		out[i] = summarizer.Summarize(b, d) != "" && summarizer.LeadN(b, summaryLeadN) != ""
	}
	return out
}

func measureSummaries(entries []goldenEntry, dict *tagger.Dictionary, id2name map[int64]string, useLead bool, comparable []bool) summaryMetrics {
	m := summaryMetrics{n: len(entries)}
	var descSum, intraSum, compSum float64
	var hits, stable int

	for i, e := range entries {
		body, desc := e.Snapshot.BodyText, e.Snapshot.Description
		s := summarizeWith(body, desc, useLead)
		if s == summarizeWith(body, desc, useLead) {
			stable++
		}
		if s == "" {
			continue
		}
		m.produced++

		sents := strings.Split(s, "\n")
		descSum += summarizer.Containment(evalTokens(s), evalTokens(desc))
		intraSum += maxPairContainment(sents)
		if n := utf8.RuneCountInString(body); n > 0 {
			compSum += float64(utf8.RuneCountInString(s)) / float64(n)
		}
		// 태그 신호 보존: 요약만 본문으로 준 태거가 expected_tags를 맞히는가.
		// 두 시스템이 모두 요약을 낸 항목에서만 센다(공정 비교 — comparableMask 주석 참고).
		if comparable[i] {
			m.tagRecallN++
			pred := classifyTop(tagger.Content{
				Domain: hostOf(e.URL), Title: e.Snapshot.Title, Description: desc, Body: s,
			}, dict, id2name)
			hits += hit(pred, toSet(e.ExpectedTags))
		}
	}

	if m.produced > 0 {
		p := float64(m.produced)
		m.descDup, m.intraDup, m.compression = descSum/p, intraSum/p, compSum/p
	}
	m.tagRecall = ratio(hits, m.tagRecallN)
	m.stable = ratio(stable, m.n)
	return m
}

func summarizeWith(body, desc string, useLead bool) string {
	if useLead {
		return summarizer.LeadN(body, summaryLeadN)
	}
	return summarizer.Summarize(body, desc)
}

// evalTokens는 요약·설명을 지표 계산용 토큰 집합으로 만든다. summarizer 내부와 같은
// 정규화(tagger.Tokenize)를 쓰되 여기서는 정렬·중복제거만 하면 된다.
func evalTokens(s string) []string {
	raw := tagger.Tokenize(s)
	sort.Strings(raw)
	out := raw[:0]
	for i, t := range raw {
		if i == 0 || t != raw[i-1] {
			out = append(out, t)
		}
	}
	return out
}

// maxPairContainment는 선택 문장들 사이의 최대 겹침 — MMR이 실제로 일하는지 본다.
func maxPairContainment(sents []string) float64 {
	if len(sents) < 2 {
		return 0
	}
	toks := make([][]string, len(sents))
	for i, s := range sents {
		toks[i] = evalTokens(s)
	}
	maxC := 0.0
	for i := range toks {
		for j := range toks {
			if i == j {
				continue
			}
			if c := summarizer.Containment(toks[i], toks[j]); c > maxC {
				maxC = c
			}
		}
	}
	return maxC
}

// guardBreakdown은 요약이 안 나온 이유를 센다 — 임계값이 안 보이는 채로 사람을 자르지 않게.
func guardBreakdown(entries []goldenEntry) (thinBody, fewProse, descDrop int) {
	for _, e := range entries {
		body := e.Snapshot.BodyText
		if utf8.RuneCountInString(body) < summarizer.MinBodyRunes {
			thinBody++
			continue
		}
		prose := 0
		for _, s := range summarizer.Split(body) {
			if summarizer.IsProse(s) {
				prose++
			}
		}
		if prose < summarizer.MinProseSents {
			fewProse++
			continue
		}
		if summarizer.Summarize(body, e.Snapshot.Description) == "" {
			descDrop++
		}
	}
	return
}

func reportSummary(name string, entries []goldenEntry, tr, lead summaryMetrics) {
	fmt.Printf("\n=== %s (%d건) ===\n", name, tr.n)
	fmt.Printf("%-22s %10s %10s\n", "", "TextRank", "lead-3")
	fmt.Printf("%-22s %10.3f %10.3f   (요약이 나온 비율)\n", "① 커버리지", tr.coverage(), lead.coverage())
	fmt.Printf("%-22s %10.3f %10.3f   (낮을수록 좋음 — 설명과 같은 말 반복)\n", "② desc 중복도", tr.descDup, lead.descDup)
	fmt.Printf("%-22s %10.3f %10.3f   (낮을수록 좋음 — 문장끼리 중복)\n", "③ intra 중복도", tr.intraDup, lead.intraDup)
	fmt.Printf("%-22s %10.3f %10.3f   (요약만 본문으로 준 태거 Recall@3, 공통 산출 %d건)\n",
		"④ 태그 신호 보존", tr.tagRecall, lead.tagRecall, tr.tagRecallN)
	fmt.Printf("%-22s %10.3f %10.3f   (요약/본문 길이비)\n", "⑤ 압축률", tr.compression, lead.compression)
	fmt.Printf("%-22s %10.3f %10.3f   (같은 입력 2회 바이트 동일)\n", "⑦ 결정성", tr.stable, lead.stable)

	thin, few, drop := guardBreakdown(entries)
	fmt.Printf("⑥ 가드 발동: 본문<%d룬 %d건 · 산문<%d문장 %d건 · desc중복≥%.1f %d건 (합 %d = 요약 없음)\n",
		summarizer.MinBodyRunes, thin, summarizer.MinProseSents, few, summarizer.DescDropRatio, drop,
		thin+few+drop)
	// 항등식이 깨지면 분해가 Summarize와 어긋난 것이다 — 조용히 틀린 표를 내지 않는다.
	if want := tr.n - tr.produced; thin+few+drop != want {
		fmt.Printf("   ⚠️ 가드 분해 불일치: 합 %d ≠ 요약 없음 %d — guardBreakdown이 Summarize와 어긋났다\n",
			thin+few+drop, want)
	}
	fmt.Println("   ※ ④는 같은 사전·토크나이저로 만든 요약을 같은 태거로 재는 순환 지표다 — 최적화 대상으로 삼으면 망가진다.")
}

// reportExtraction은 **스크래퍼 건강도**를 도메인 단위로 낸다. 요약·태깅 품질의 상한은
// 본문 추출이 정하므로, 추출이 조용히 0이 되는 계통적 실패(문자셋 변환 오류·SPA·봇 차단)를
// 여기서 숫자로 드러낸다. 실제로 이 지표가 없던 동안 한국 기술블로그 다수가
// `transform: short internal buffer`로 본문 0B였는데 아무 테스트도 실패하지 않았다.
func reportExtraction(entries []goldenEntry) {
	type stat struct{ ok, total int }
	byDomain := map[string]*stat{}
	okAll := 0
	for _, e := range entries {
		d := hostOf(e.URL)
		d = strings.TrimPrefix(d, "www.")
		st := byDomain[d]
		if st == nil {
			st = &stat{}
			byDomain[d] = st
		}
		st.total++
		if utf8.RuneCountInString(e.Snapshot.BodyText) >= 200 {
			st.ok++
			okAll++
		}
	}
	fmt.Printf("본문 추출: %d/%d건 (%.0f%%) — 200룬 이상. **이 수치가 떨어지면 스크래퍼 회귀다**\n",
		okAll, len(entries), 100*ratio(okAll, len(entries)))

	var failed []string
	for d, st := range byDomain {
		if st.ok < st.total {
			failed = append(failed, fmt.Sprintf("%s %d/%d", d, st.total-st.ok, st.total))
		}
	}
	sort.Strings(failed)
	if len(failed) > 0 {
		fmt.Printf("  추출 실패 도메인: %s\n", strings.Join(failed, " · "))
	}
}

// isKoreanEntry는 golden 항목이 한국어 콘텐츠인지 본다(제목·설명·본문의 한글 비율).
// 이 앱은 한국어와 영어를 **대등하게** 지원해야 하므로 두 언어를 갈라 재는 것이 필수다.
func isKoreanEntry(e goldenEntry) bool {
	s := e.Snapshot.Title + " " + e.Snapshot.Description + " " + e.Snapshot.BodyText
	hangul, letters := 0, 0
	for _, r := range s {
		if r >= 0xAC00 && r <= 0xD7A3 {
			hangul++
		}
		if r >= 0xAC00 && r <= 0xD7A3 || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letters++
		}
	}
	return letters > 0 && float64(hangul)/float64(letters) > 0.2
}

// reportByLang은 한국어/영어 부분집합의 핵심 지표를 나란히 낸다 — 한쪽 언어만 잘 되는
// 회귀(예: 문장 분리 규칙이 한 언어에 치우침)를 드러내는 유일한 축이다.
func reportByLang(entries []goldenEntry, dict *tagger.Dictionary, id2name map[int64]string) {
	var ko, en []goldenEntry
	for _, e := range entries {
		if isKoreanEntry(e) {
			ko = append(ko, e)
		} else {
			en = append(en, e)
		}
	}
	fmt.Printf("언어별 (TextRank):\n")
	fmt.Printf("  %-8s %5s %10s %10s %10s\n", "", "건수", "커버리지", "desc중복", "태그보존")
	for _, g := range []struct {
		name string
		set  []goldenEntry
	}{{"한국어", ko}, {"영어", en}} {
		if len(g.set) == 0 {
			continue
		}
		m := measureSummaries(g.set, dict, id2name, false, comparableMask(g.set))
		fmt.Printf("  %-8s %5d %10.3f %10.3f %10.3f\n", g.name, m.n, m.coverage(), m.descDup, m.tagRecall)
	}
}

// checkSummaryGates는 dev 세트의 상대 게이트를 판정한다. 절대 임계값은 쓰지 않는다 —
// 정답이 없어 "0.13이 좋다"고 말할 근거가 없고, 말할 수 있는 건 베이스라인 대비뿐이다.
func checkSummaryGates(tr, lead summaryMetrics) error {
	var fail []string
	if !(tr.descDup < lead.descDup) {
		fail = append(fail, fmt.Sprintf("desc 중복도가 lead-3보다 낮아야 함 (TextRank %.3f, lead-3 %.3f)", tr.descDup, lead.descDup))
	}
	if tr.tagRecall < lead.tagRecall-tagPreserveSlack {
		fail = append(fail, fmt.Sprintf("태그 보존율이 lead-3 대비 %.2f 이상 떨어짐 (TextRank %.3f, lead-3 %.3f)", tagPreserveSlack, tr.tagRecall, lead.tagRecall))
	}
	if tr.stable != 1.0 {
		fail = append(fail, fmt.Sprintf("결정성이 1.000이 아님 (%.3f) — 맵 순회 누출", tr.stable))
	}
	if len(fail) > 0 {
		return fmt.Errorf("dev 게이트 실패:\n  - %s", strings.Join(fail, "\n  - "))
	}
	return nil
}

// dumpSummaries는 사람 스팟체크용 텍스트를 출력한다 — 자동 지표가 못 잡는 가독성·문맥
// 단절·제목 접착을 판정할 유일한 경로. 출력을 커밋해 두면 알고리즘 변경이 PR diff에 보인다.
func dumpSummaries(entries []goldenEntry) {
	sorted := append([]goldenEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].URL < sorted[j].URL })
	if len(sorted) > dumpCount {
		sorted = sorted[:dumpCount]
	}
	fmt.Println("# 요약 스팟체크 (dev, URL 정렬 상위 10건)")
	fmt.Println("# 자동 지표가 못 잡는 것을 사람이 본다: 가독성 · 논지 포착 · 문맥 단절 · 제목 접착.")
	for _, e := range sorted {
		s := summarizer.Summarize(e.Snapshot.BodyText, e.Snapshot.Description)
		fmt.Printf("\n## %s\n", e.URL)
		fmt.Printf("[설명] %s\n", oneLine(e.Snapshot.Description))
		if s == "" {
			fmt.Printf("[요약] (없음 — 가드)\n")
			continue
		}
		for _, line := range strings.Split(s, "\n") {
			fmt.Printf("[요약] %s\n", line)
		}
	}
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > 160 {
		r := []rune(s)
		return string(r[:160]) + "…"
	}
	return s
}
