package main

// eval·golden-capture 서브커맨드 — M3 태깅 품질 측정 하네스.
//
//	pushpoint eval [golden-dir]        # dev/test/wild.jsonl로 Recall@3 + 태그별 P/R + 베이스라인 (네트워크 0)
//	pushpoint golden-capture urls.tsv  # url<TAB>tag,tag 목록을 프로덕션 스크랩 경로로 스냅샷 캡처 → JSONL
//	pushpoint golden-refill in.jsonl   # 기존 golden의 **빈 필드만** 재fetch로 채움 → JSONL
//
// eval은 golden 스냅샷만 입력으로 태거를 돌린다(네트워크 0 → 시점 무관 재현). 사전은 런타임과
// 동일하게 fresh 마이그레이션 DB에서 읽어(결정적 시드) golden==runtime을 보장한다. golden 스냅샷은
// 태거 Content와 정확히 일치한다 — 태거가 쓰는 필드가 늘면 스냅샷도 늘어야 한다(train/serve skew).

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/scraper"
	"github.com/coby/push-point/backend/internal/store"
	"github.com/coby/push-point/backend/internal/tagger"
)

const evalTopK = 3 // Recall@3 및 태그별 P/R의 예측 상위 k

// goldenSnapshot은 태거 입력 필드(런타임 links에서 읽는 것과 동일 표면).
type goldenSnapshot struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	BodyText    string `json:"body_text"`
	// Keywords는 발행자 분류(meta keywords·article:section 등). 나중에 더해진 필드라
	// 없는 항목이 있을 수 있고, 그때는 "" — 그 항목은 Δkeywords에 기여하지 않는다.
	Keywords string `json:"keywords,omitempty"`
}

// goldenEntry는 golden JSONL 한 줄.
type goldenEntry struct {
	URL          string         `json:"url"`
	Snapshot     goldenSnapshot `json:"snapshot"`
	ExpectedTags []string       `json:"expected_tags"`
}

// runEval은 golden 디렉터리(기본 nlu/golden/)의 dev/test/wild를 평가해 리포트를 출력한다.
func runEval(args []string) error {
	dir := "nlu/golden"
	if len(args) > 0 {
		dir = args[0]
	}
	dict, id2name, err := loadEvalDict()
	if err != nil {
		return err
	}
	fmt.Printf("사전 %d개 태그 로드 (fresh 마이그레이션 시드)\n", len(id2name))

	ran := false
	// wild는 **dev/test와 합치지 않고 따로 낸다.** 합치면 하나의 평균 뒤로 숨는데,
	// 이 세트의 존재 이유가 정확히 그 평균이 가리던 격차를 보이게 하는 것이다
	// 수치는 여기 적지 않는다 — 결함을 고칠 때마다 움직이는데 주석은 실패하는 테스트가
	// 없어 조용히 낡는다(이 줄의 이전 판이 실제로 그랬다). 최신 값은 `just eval`이 낸다.
	for _, name := range []string{"dev", "test", "wild"} {
		path := filepath.Join(dir, name+".jsonl")
		entries, err := loadGolden(path)
		if err != nil {
			if os.IsNotExist(err) {
				// stdout이 아니라 stderr로 낸다 — `just eval | grep Recall`처럼 거르면
				// stdout에 섞인 안내는 사라지고 세트 하나가 빠진 채로 완전한 보고서처럼 보인다.
				fmt.Fprintf(os.Stderr, "(%s 없음 — 건너뜀: %s)\n", name, path)
				continue
			}
			return err
		}
		// **비어 있지만 존재하는 파일은 건너뛸 일이 아니라 결함이다.** 캡처가 실패했거나
		// 파일이 잘렸다는 뜻인데, 예전에는 이것도 `ran = true`로 세어서 "세 세트를 다 쟀다"는
		// 모양의 보고서가 exit 0으로 나왔다. 아무것도 안 재고도 성공하는 것이 가장 나쁘다.
		if len(entries) == 0 {
			return fmt.Errorf("eval: %s가 비어 있습니다 (%s) — 캡처 실패이거나 파일이 잘렸습니다", name, path)
		}
		// expected_tags가 사전에 없는 이름이면 **구조적으로 맞힐 수 없는 정답**이 된다.
		// 그러면 태거 실패처럼 보이는 수치가 나오는데 원인은 오타다. README가 "사전에 존재하는
		// name만"이라고 규정만 하고 강제하는 코드가 없었다.
		if err := validateExpectedTags(name, entries, id2name); err != nil {
			return err
		}
		ran = true
		// IDF는 **기본으로 꺼져 있다** — 런타임에서도 꺼져 있기 때문이다(tagger/idf.go).
		// `just eval`이 내는 수는 언제나 실제로 출하되는 동작이어야 한다.
		//
		// PP_EVAL_IDF=1이면 그 세트를 코퍼스로 삼아 켠다. 켜도 될지 판정하려면 이 실행과
		// 기본 실행의 차이를 봐야 하고, 2026-07-26 기준 그 차이는 개선이 아니었다
		// (Recall@3 동일, 최대 태그 article·tutorial 하락). 원인은 golden에 편중이 없다는
		// 것이다 — PP_DF_DUMP=1로 DF 분포를 보면 어떤 표면도 문서의 38%를 넘지 않는다.
		d := dict
		if os.Getenv("PP_EVAL_IDF") == "1" {
			d = dict.WithCorpus(goldenCorpus(entries, dict))
		}
		evalSet(strings.ToUpper(name), entries, d, id2name)
	}
	if !ran {
		return fmt.Errorf("eval: golden 파일이 없습니다 (%s/{dev,test,wild}.jsonl) — golden-capture로 만드세요", dir)
	}
	fmt.Println("\n측정치는 기록용이다 (M3엔 게이트 없음 — 게이트는 M5 진입, 동결 test 기준).")
	return nil
}

// validateExpectedTags는 라벨이 전부 사전에 있는 태그 이름인지 확인한다.
//
// 사전에 없는 이름(오타·별칭 오용·사전에서 삭제된 태그)은 예측될 수가 없으므로 **영구 miss**가
// 된다. 그 결과는 Recall 하락으로 나타나고, 태그별 표에서는 P=0.00 R=0.00 행이 되어 "태거가
// 못 맞히는 태그"와 눈으로 구분되지 않는다. 라벨은 사람이 손으로 쓰므로 오타가 실제로 난다.
//
// 빈 expected_tags도 같이 막는다 — hit 판정이 교집합이라 정답이 없으면 자동 miss이고,
// 그 항목은 분모만 키운다.
func validateExpectedTags(setName string, entries []goldenEntry, id2name map[int64]string) error {
	known := make(map[string]bool, len(id2name))
	for _, n := range id2name {
		known[n] = true
	}
	var problems []string
	for i, e := range entries {
		if len(e.ExpectedTags) == 0 {
			problems = append(problems, fmt.Sprintf("%d행 정답 태그 없음: %s", i+1, e.URL))
			continue
		}
		for _, t := range e.ExpectedTags {
			if !known[t] {
				problems = append(problems, fmt.Sprintf("%d행 사전에 없는 태그 %q: %s", i+1, t, e.URL))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("eval: %s 라벨 오류 %d건\n  %s", setName, len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

// evalContent는 golden 항목을 태거 입력으로 바꾼다. 두 스위치로 변형을 만든다 —
// 각 신호의 기여를 **그것만 빼서** 재기 위한 것이고, 도메인·제목·설명은 항상 들어간다.
func evalContent(e goldenEntry, body, keywords bool) tagger.Content {
	c := tagger.Content{
		Domain:      hostOf(e.URL),
		Title:       e.Snapshot.Title,
		Description: e.Snapshot.Description,
	}
	if body {
		c.Body = e.Snapshot.BodyText
	}
	if keywords {
		c.Keywords = e.Snapshot.Keywords
	}
	return c
}

// goldenCorpus는 golden 세트 자체를 코퍼스로 삼아 DF를 센다.
//
// 런타임에서 corpus_df가 하는 일과 **같은 계산**이다 — 런타임은 저장된 링크가 코퍼스이고
// 여기서는 golden이 코퍼스다. 다른 코퍼스를 빌려 오면 실제로 켜질 통계와 다른 것을 재게 된다.
func goldenCorpus(entries []goldenEntry, dict *tagger.Dictionary) tagger.CorpusStats {
	df := map[string]int64{}
	withTerms := 0
	for _, e := range entries {
		surfaces := dict.MatchedSurfaces(evalContent(e, true, true))
		if len(surfaces) > 0 {
			withTerms++
		}
		for s := range surfaces {
			df[s]++
		}
	}
	// DF 분포 덤프 — IDF가 아무 일도 하지 않을 때 **왜 그런지**를 보는 유일한 창이다.
	// 지표만 보면 "IDF가 효과 없다"로 읽히지만, 실제 원인은 코퍼스에 편중이 없다는 것일 수 있다.
	if os.Getenv("PP_DF_DUMP") != "" {
		type kv struct {
			t string
			n int64
		}
		var rows []kv
		for t, n := range df {
			rows = append(rows, kv{t, n})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].n > rows[j].n })
		fmt.Printf("  DF 상위 (문서 %d건, 표면 %d개):\n", len(entries), len(df))
		for i, r := range rows {
			if i >= 15 {
				break
			}
			fmt.Printf("    %-24s df=%3d  ratio=%.2f\n", r.t, r.n, float64(r.n)/float64(len(entries)))
		}
	}
	// 분모는 **표면을 하나라도 낸 문서 수**다. 런타임의 `CorpusDF`가 link_terms에 행을
	// 남긴 링크만 세므로(표면 0인 링크는 원장에 아무것도 안 남긴다) 여기서도 같아야 한다.
	// len(entries)로 세면 eval과 런타임이 다른 N을 쓰게 되고, IDF를 켤 때 판정 도구와
	// 실제 동작이 어긋난다 — golden 123건 중 4건이 표면 0이라 3%쯤 벌어진다.
	return tagger.CorpusStats{Docs: int64(withTerms), DF: df}
}

// setMetrics는 한 세트에서 나오는 **모든 수**다. 출력 형식은 들어 있지 않다.
//
// 계산을 출력에서 떼어낸 이유는 하나다 — **잴 수 있게 하려고.** 예전에는 계산과 Printf가
// 한 함수에 섞여 있어서 evalSet에 테스트를 붙일 수가 없었고, 그래서 리포트되는 모든 수
// (Recall, Δ, 태그별 P/R)가 검증 없이 나갔다. 실제로 분모 오프바이원·변형 배선 뒤바꿈·
// 태그별 TP/FP/FN 오류가 전부 테스트를 통과했다(2026-07-26 뮤테이션 확인).
type setMetrics struct {
	// 변형별 hit 수. 변형은 **full에서 신호를 하나씩 뺀 것**이라 Δ가 그 신호만의 기여가 된다.
	// full=도메인+제목+설명+본문+분류 / no-body=full−본문 / no-keywords=full−분류 /
	// bare=본문도 분류도 없음 / base=도메인만.
	fullHit, noBodyHit, noKWHit, bareHit, baseHit int
	// tied: topK 경계에서 점수가 같아 **태그 이름 알파벳순으로** 갈린 링크 수.
	// missZero/missRank: 미스를 둘로 가른다 — 정답 태그가 0점인가, 점수는 있는데 밀렸는가.
	tied, missZero, missRank int
	// thin/thinHit: 스냅샷 자체가 빈약한 항목과, 그중 그래도 맞힌 항목.
	thin, thinHit int
	// withKW: 발행자 분류를 가진 항목 수 — Δkeywords를 몇 건 위에서 잰 것인지 없으면 해석 불가.
	withKW int
	// 태그별 집계 (full 기준).
	tp, fp, fn, goldN map[string]int
}

// measureSet은 세트 하나를 돌며 setMetrics를 채운다. 출력하지 않는다.
func measureSet(entries []goldenEntry, dict *tagger.Dictionary, id2name map[int64]string) setMetrics {
	m := setMetrics{
		tp:    map[string]int{},
		fp:    map[string]int{},
		fn:    map[string]int{},
		goldN: map[string]int{},
	}
	for _, e := range entries {
		exp := toSet(e.ExpectedTags)
		for t := range exp {
			m.goldN[t]++
		}
		if e.Snapshot.Keywords != "" {
			m.withKW++
		}

		base := classifyTop(tagger.Content{Domain: hostOf(e.URL)}, dict, id2name)
		noBody := classifyTop(evalContent(e, false, true), dict, id2name)
		noKW := classifyTop(evalContent(e, true, false), dict, id2name)
		full := classifyTop(evalContent(e, true, true), dict, id2name)
		// bare = 본문도 분류도 없는 변형. 분류가 **본문의 대체재**인지 보려면 필요하다.
		bare := classifyTop(evalContent(e, false, false), dict, id2name)

		m.baseHit += hit(base, exp)
		m.noBodyHit += hit(noBody, exp)
		m.noKWHit += hit(noKW, exp)
		m.fullHit += hit(full, exp)
		m.bareHit += hit(bare, exp)

		// 동점과 미스 해부는 full 기준으로 센다 — 실제로 출하되는 구성이다.
		full3 := classifyRanked(evalContent(e, true, true), dict, id2name)
		if tiedAtCut(full3) {
			m.tied++
		}
		if hit(full, exp) == 0 {
			if zeroScored(full3, exp) {
				m.missZero++
			} else {
				m.missRank++
			}
		}
		if isThinSnapshot(e.Snapshot.Title, e.Snapshot.Description, e.Snapshot.BodyText) {
			m.thin++
			m.thinHit += hit(full, exp)
		}

		// 태그별 P/R은 full 예측 기준.
		predicted := toSet(full)
		for t := range predicted {
			if exp[t] {
				m.tp[t]++
			} else {
				m.fp[t]++
			}
		}
		for t := range exp {
			if !predicted[t] {
				m.fn[t]++
			}
		}
	}
	return m
}

// evalSet은 한 세트(dev/test/wild)의 지표를 계산·출력한다: Recall@3와 full 기준 태그별 P/R.
// 계산은 measureSet이 하고, 여기는 형식만 맡는다.
func evalSet(name string, entries []goldenEntry, dict *tagger.Dictionary, id2name map[int64]string) {
	fmt.Printf("\n=== %s (%d건) ===\n", name, len(entries))
	if len(entries) == 0 {
		fmt.Println("(빈 세트 — 건너뜀)")
		return
	}

	m := measureSet(entries, dict, id2name)
	baseHit, noBodyHit, noKWHit, fullHit, bareHit := m.baseHit, m.noBodyHit, m.noKWHit, m.fullHit, m.bareHit
	tied, missZero, missRank := m.tied, m.missZero, m.missRank
	thin, thinHit, withKW := m.thin, m.thinHit, m.withKW
	tp, fp, fn, goldN := m.tp, m.fp, m.fn, m.goldN

	n := float64(len(entries))
	fmt.Printf("Recall@%d:  full=%.3f   no-body=%.3f (Δbody %+.3f)   baseline(도메인만)=%.3f (Δrules %+.3f)\n",
		evalTopK, float64(fullHit)/n, float64(noBodyHit)/n, float64(fullHit-noBodyHit)/n,
		float64(baseHit)/n, float64(fullHit-baseHit)/n)
	// 발행자 분류의 기여는 **두 축으로** 잰다. 본문이 있을 때와 없을 때가 다르기 때문이다 —
	// 분류는 본문이 이미 말해 주는 것을 반복하는 경우가 많아 본문이 있으면 Δ가 0에 가깝고,
	// 본문이 없을 때(스크랩 실패·SPA·봇 차단, 즉 클라이언트 캡처가 필요한 바로 그 페이지)
	// 비로소 값을 한다. 하나만 재면 "쓸모없다"와 "필수다" 중 아무 쪽으로나 읽힌다.
	fmt.Printf("           분류 기여: 본문 있을 때 %+.3f (no-keywords=%.3f) · 본문 없을 때 %+.3f (bare=%.3f) · 분류 있는 항목 %d/%d\n",
		float64(fullHit-noKWHit)/n, float64(noKWHit)/n,
		float64(noBodyHit-bareHit)/n, float64(bareHit)/n, withKW, len(entries))
	// 동점 — 지표가 못 보는 자리다. hit@3는 3위 안에 정답이 있으면 통과이므로,
	// 3위와 4위가 동점이라 알파벳순으로 갈린 경우를 구분하지 않는다. 가중치를 건드리면
	// 이 덩어리가 통째로 재배열되는데 Recall@3는 거의 움직이지 않는다.
	fmt.Printf("           경계 동점: %d/%d (%.0f%%) — 3위와 4위 점수가 같아 태그 이름 알파벳순으로 갈렸다\n",
		tied, len(entries), 100*float64(tied)/n)

	// 미스 해부 — 어떤 개선이 유효한지가 여기서 갈린다.
	// 0점 미스는 순위를 아무리 바꿔도 못 고친다(승격이 필요하고, 그건 오탐 위험이 다르다).
	if miss := len(entries) - fullHit; miss > 0 {
		fmt.Printf("           미스 %d건: 정답 0점 %d · 순위 밀림 %d → 재랭킹 상한 %.3f (+%.3f)\n",
			miss, missZero, missRank,
			float64(fullHit+missRank)/n, float64(missRank)/n)
	}
	// 빈약한 스냅샷 — **태거를 고쳐서 얻을 수 있는 몫의 상한**을 가른다.
	// 여기 걸린 항목은 봇 차단 페이지·로그인 벽·요청조차 안 한 어댑터의 결과물이라,
	// 태거를 아무리 고쳐도 신호가 없다. 해법은 태거가 아니라 클라이언트 캡처다.
	// 두 수를 같이 내지 않으면 캡처 결함이 태거 품질로 잘못 귀속된다.
	if thin > 0 && thin < len(entries) {
		usable := len(entries) - thin
		fmt.Printf("           신호 %d자 미만 %d건(맞힌 것 %d) — 캡처가 벽·빈 응답을 물어온 몫이라 태거로 못 고친다.\n",
			thinSignalRunes, thin, thinHit)
		fmt.Printf("             나머지 %d건 기준 Recall@%d=%.3f — **태거를 고쳐서 올릴 수 있는 상한**이다.\n",
			usable, evalTopK, float64(fullHit-thinHit)/float64(usable))
	}

	// 언어별 — 이 앱은 한국어와 영어를 대등하게 지원해야 하므로 한쪽만 잘 되는 회귀를 드러낸다.
	reportTagRecallByLang(entries, dict, id2name)

	// 태그별 표 — golden에 등장했거나 예측된 태그만, golden 빈도 내림차순.
	tags := map[string]bool{}
	for t := range goldN {
		tags[t] = true
	}
	for t := range tp {
		tags[t] = true
	}
	for t := range fp {
		tags[t] = true
	}
	var rows []string
	for t := range tags {
		rows = append(rows, t)
	}
	sort.Slice(rows, func(i, j int) bool {
		if goldN[rows[i]] != goldN[rows[j]] {
			return goldN[rows[i]] > goldN[rows[j]]
		}
		return rows[i] < rows[j]
	})
	fmt.Printf("태그별 (full, top-%d 예측 기준):\n", evalTopK)
	fmt.Printf("  %-13s %5s %5s %s\n", "tag", "P", "R", "golden")
	for _, t := range rows {
		p := ratio(tp[t], tp[t]+fp[t])
		r := ratio(tp[t], tp[t]+fn[t])
		fmt.Printf("  %-13s %5.2f %5.2f %6d\n", t, p, r, goldN[t])
	}
}

// reportTagRecallByLang은 한국어/영어 부분집합의 Recall@3를 나란히 낸다.
func reportTagRecallByLang(entries []goldenEntry, dict *tagger.Dictionary, id2name map[int64]string) {
	var ko, en []goldenEntry
	for _, e := range entries {
		if isKoreanEntry(e) {
			ko = append(ko, e)
		} else {
			en = append(en, e)
		}
	}
	line := func(label string, set []goldenEntry) {
		if len(set) == 0 {
			return
		}
		hits := 0
		for _, e := range set {
			pred := classifyTop(evalContent(e, true, true), dict, id2name)
			hits += hit(pred, toSet(e.ExpectedTags))
		}
		fmt.Printf("  %-8s %3d건  Recall@%d=%.3f\n", label, len(set), evalTopK, ratio(hits, len(set)))
	}
	fmt.Println("언어별:")
	line("한국어", ko)
	line("영어", en)
}

// classifyTop은 Content를 분류해 상위 evalTopK 태그 이름을 돌려준다.
func classifyTop(c tagger.Content, dict *tagger.Dictionary, id2name map[int64]string) []string {
	out := make([]string, 0, evalTopK)
	for _, r := range classifyRanked(c, dict, id2name) {
		if len(out) == evalTopK {
			break
		}
		out = append(out, r.name)
	}
	return out
}

// ranked는 이름과 confidence를 함께 들고 다닌다.
//
// `classifyTop`이 이름만 돌려주면서 점수가 버려지고 있었다. 그 결과 진단 두 가지가
// 구조적으로 불가능했다: **3위와 4위가 동점인지**(그러면 순위가 태그 이름 알파벳순으로
// 갈린다), 그리고 **미스가 0점인지 밀림인지**. 둘 다 아래에서 필요하다.
type ranked struct {
	name string
	conf float64
}

// classifyRanked는 컷 없이 전부 돌려준다 — topK 경계 **바깥**을 봐야 동점을 알 수 있다.
func classifyRanked(c tagger.Content, dict *tagger.Dictionary, id2name map[int64]string) []ranked {
	scored := tagger.Classify(c, dict)
	out := make([]ranked, 0, len(scored))
	for _, s := range scored {
		out = append(out, ranked{name: id2name[s.TagID], conf: s.Confidence})
	}
	return out
}

// hit은 예측 top-k와 expected가 하나라도 겹치면 1.
func hit(predicted []string, expected map[string]bool) int {
	for _, p := range predicted {
		if expected[p] {
			return 1
		}
	}
	return 0
}

// runGoldenCapture는 url<TAB>tag,tag 목록을 프로덕션 스크랩 경로로 돌려 golden JSONL을 stdout에
// 낸다. **네트워크 사용**(일회성 캡처) — eval 자체는 네트워크 0이지만 golden은 실제 추출로 만든다.
func runGoldenCapture(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("golden-capture: 사용법 pushpoint golden-capture <urls.tsv>  (한 줄: url<TAB>tag,tag,tag)")
	}
	f, err := os.Open(args[0])
	if err != nil {
		return fmt.Errorf("golden-capture: 입력 열기 실패: %w", err)
	}
	defer f.Close()

	sc := scraper.New() // 기본 SSRF 가드(공개 URL이라 무해)
	enc := json.NewEncoder(os.Stdout)
	ctx := context.Background()

	sccan := bufio.NewScanner(f)
	sccan.Buffer(make([]byte, 0, 1<<20), 1<<20)
	var ok, fail, skipped, thin int
	for sccan.Scan() {
		line := strings.TrimSpace(sccan.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rawURL, tagsField, _ := strings.Cut(line, "\t")
		rawURL = strings.TrimSpace(rawURL)
		var tags []string
		for t := range strings.SplitSeq(tagsField, ",") {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		if len(tags) == 0 { // 태그 없는 줄(탭 누락 등)은 golden에 넣지 않는다 — 항상 miss가 되어 지표를 왜곡.
			fmt.Fprintf(os.Stderr, "  건너뜀(태그 없음): %s\n", rawURL)
			skipped++
			continue
		}
		m, err := sc.Fetch(ctx, rawURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  실패: %s — %v\n", rawURL, err)
			fail++
			continue
		}
		e := goldenEntry{
			URL:          rawURL,
			Snapshot:     goldenSnapshot{Title: m.Title, Description: m.Description, BodyText: m.BodyText, Keywords: m.Keywords},
			ExpectedTags: tags,
		}
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("golden-capture: JSONL 쓰기 실패: %w", err)
		}
		ok++
		// **에러 없이 성공한 빈 캡처가 가장 위험하다.** 봇 차단 페이지·로그인 벽·요청조차
		// 하지 않는 어댑터(instagram)가 전부 err=nil로 돌아오고, 스냅샷만 보면 진짜 페이지와
		// 구분되지 않는다. 실제로 wild 초안에서 존재하지 않는 URL과 차단된 URL을 구분하지
		// 못한 원인이 이것이었다. 지우지는 않는다 — 사용자가 그 링크를 저장하면 실제로 그
		// 벽을 맞으므로 그것도 측정 대상이다. 대신 **세어서 눈에 띄게 한다.**
		if isThinSnapshot(m.Title, m.Description, m.BodyText) {
			thin++
			fmt.Fprintf(os.Stderr, "  ⚠ 빈약한 스냅샷(차단·로그인 벽 의심): %s (title=%dB desc=%dB body=%dB)\n",
				rawURL, len(m.Title), len(m.Description), len(m.BodyText))
			continue
		}
		fmt.Fprintf(os.Stderr, "  캡처: %s (title=%.30q body=%dB)\n", rawURL, m.Title, len(m.BodyText))
	}
	if err := sccan.Err(); err != nil {
		return fmt.Errorf("golden-capture: 입력 읽기 실패: %w", err)
	}
	fmt.Fprintf(os.Stderr, "완료: %d 캡처(빈약 %d), %d 실패, %d 건너뜀\n", ok, thin, fail, skipped)
	// **실패가 있으면 0으로 끝내지 않는다.** 예전에는 몇 건이 실패하든 nil을 돌려줘서
	// `golden-capture urls.tsv > wild.jsonl && git add wild.jsonl`이 잘린 세트에 대해
	// 성공했다. 사람이 stderr를 읽고 있을 때만 드러나는 실패는 스크립트에서는 없는 것과 같다.
	if fail > 0 || skipped > 0 {
		return fmt.Errorf("golden-capture: %d 실패 · %d 건너뜀 (성공 %d) — golden이 불완전합니다", fail, skipped, ok)
	}
	return nil
}

// thinSignalRunes는 "이 스냅샷으로 태깅할 만한 것이 있는가"를 가르는 글자 수다.
const thinSignalRunes = 200

// isThinSnapshot은 태거에게 줄 신호가 사실상 없는 스냅샷을 가려낸다.
//
// **벽인지 아닌지를 내용으로 판정하려 들지 않는다.** 봇 차단 페이지에도 제목이 있고
// ("Reddit - Please wait for verification", "Threads • 로그인"), 그럴듯한 제목을 근거로
// 벽을 알아내려는 규칙은 그 자체가 틀릴 수 있는 추론이다. 세는 것은 글자 수 하나다.
//
// 세 필드를 **합쳐서** 본다. 어느 한 필드가 비었다는 사실은 아무것도 뜻하지 않는다 —
// x.com 어댑터는 본문을 0자로 주면서 트윗 전문을 description에 담고(oembed), 그건 정상
// 캡처다. 도메인은 항상 있으므로 신호에 세지 않는다(그것만으로 맞히는 것이 baseline이다).
//
// 이건 **근사치이고 그렇게만 쓴다.** 200자가 넘는 벽도 있고 200자 미만의 진짜 짧은 글도 있다.
// 이 수로 판정하는 것은 "태거를 고쳐서 얻을 수 있는 몫의 상한"뿐이다.
func isThinSnapshot(title, description, bodyText string) bool {
	n := len([]rune(title)) + len([]rune(description)) + len([]rune(bodyText))
	return n < thinSignalRunes
}

// loadEvalDict는 fresh 마이그레이션 임시 DB에서 사전을 읽어(런타임과 동일 경로) 컴파일된
// Dictionary와 id→name 맵을 돌려준다. 시드는 결정적이라 eval이 완전히 재현된다.
func loadEvalDict() (*tagger.Dictionary, map[int64]string, error) {
	tmp, err := os.MkdirTemp("", "pp-eval-*")
	if err != nil {
		return nil, nil, fmt.Errorf("eval: 임시 디렉터리 실패: %w", err)
	}
	defer os.RemoveAll(tmp)

	db, err := store.Open(tmp) // 마이그레이션 자동 적용(시드 30태그)
	if err != nil {
		return nil, nil, fmt.Errorf("eval: DB 열기 실패: %w", err)
	}
	defer db.Close()

	st := store.New(db, queue.NewSQLite(db.Writer))
	entries, err := st.LoadTagDict(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("eval: 사전 로드 실패: %w", err)
	}
	tagEntries := make([]tagger.TagEntry, len(entries))
	id2name := make(map[int64]string, len(entries))
	for i, e := range entries {
		tagEntries[i] = tagger.TagEntry{ID: e.ID, Name: e.Name, Aliases: e.Aliases, Facet: e.Facet}
		id2name[e.ID] = e.Name
	}
	return tagger.BuildDictionary(tagEntries), id2name, nil
}

// loadGolden은 JSONL 파일을 goldenEntry 슬라이스로 읽는다.
func loadGolden(path string) ([]goldenEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []goldenEntry
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		var e goldenEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("golden 파싱 실패 (%s): %w", path, err)
		}
		out = append(out, e)
	}
	return out, scan.Err()
}

// hostOf는 URL에서 호스트를 뽑는다(태거 DomainTags가 www·서브도메인을 처리).
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// toSet은 문자열 슬라이스를 집합으로.
func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// ratio는 0 나눗셈을 0으로 처리한 num/den.
func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

// runGoldenRefill은 기존 golden JSONL을 다시 fetch해 **비어 있는 스냅샷 필드만** 채워 stdout에 낸다.
//
// 왜 golden-capture로 다시 뜨지 않는가: 태거가 새 필드를 쓰기 시작하면 그 필드가 없는 golden으로는
// **개선을 측정할 수 없다**(Δ가 항상 0). 그렇다고 전체를 다시 뜨면 그 사이 바뀐 페이지 때문에
// title·body_text까지 흔들려 **이전 측정치와 비교가 끊긴다** — 특히 test는 동결 세트라 그러면 안 된다.
// 그래서 채우기는 단방향이다: 이미 값이 있는 필드는 절대 건드리지 않는다.
//
// fetch 실패는 치명적이지 않다 — 그 항목은 원본 그대로 통과시킨다(빠진 필드는 "" 그대로).
// URL이 죽었다고 golden 항목을 잃으면 세트가 조용히 줄어 지표가 비교 불가능해진다.
func runGoldenRefill(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("golden-refill: 사용법 pushpoint golden-refill <golden.jsonl>")
	}
	entries, err := loadGolden(args[0])
	if err != nil {
		return fmt.Errorf("golden-refill: 입력 읽기 실패: %w", err)
	}

	sc := scraper.New()
	enc := json.NewEncoder(os.Stdout)
	ctx := context.Background()
	var filled, failed, already int

	for _, e := range entries {
		// 채울 것이 없으면 네트워크를 쓰지 않는다.
		if e.Snapshot.Title != "" && e.Snapshot.Description != "" && e.Snapshot.BodyText != "" && e.Snapshot.Keywords != "" {
			already++
			if err := enc.Encode(e); err != nil {
				return fmt.Errorf("golden-refill: JSONL 쓰기 실패: %w", err)
			}
			continue
		}
		m, err := sc.Fetch(ctx, e.URL)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "  실패(원본 유지): %s — %v\n", e.URL, err)
		} else {
			before := e.Snapshot
			fill(&e.Snapshot.Title, m.Title)
			fill(&e.Snapshot.Description, m.Description)
			fill(&e.Snapshot.BodyText, m.BodyText)
			fill(&e.Snapshot.Keywords, m.Keywords)
			if e.Snapshot != before {
				filled++
				fmt.Fprintf(os.Stderr, "  채움: %s (keywords=%.60q)\n", e.URL, e.Snapshot.Keywords)
			}
		}
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("golden-refill: JSONL 쓰기 실패: %w", err)
		}
	}
	fmt.Fprintf(os.Stderr, "완료: %d건 중 %d 채움, %d 이미 완전, %d fetch 실패\n",
		len(entries), filled, already, failed)
	return nil
}

// fill은 dst가 비어 있을 때만 v를 넣는다 — 기존 값 보존이 golden-refill의 전부다.
func fill(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}

// tiedAtCut은 topK 경계에서 점수가 같은지 본다.
//
// 같으면 순위가 **태그 이름 알파벳순**으로 갈린다(Classify의 동점 정렬 규약). 그건 품질이
// 아니라 우연이고, hit@3는 그 우연을 못 본다 — 3위 안에만 있으면 통과이기 때문이다.
// 가중치를 건드리면 이 덩어리가 재배열되면서 Recall@3는 거의 안 움직인다.
func tiedAtCut(rs []ranked) bool {
	if len(rs) <= evalTopK {
		return false // 경계 밖에 후보가 없으면 갈릴 것도 없다
	}
	return rs[evalTopK-1].conf == rs[evalTopK].conf
}

// zeroScored는 정답 태그가 **하나도 점수를 못 받았는지** 본다.
//
// 이 구분이 개선의 종류를 정한다. 0점이면 순위를 아무리 바꿔도 못 고치고 — 태그를
// threshold 위로 **승격**시켜야 하는데 그건 오탐 대량 유입이라는 다른 위험이다.
// 점수가 있는데 밀린 것만 재랭킹으로 되찾을 수 있다.
func zeroScored(rs []ranked, expected map[string]bool) bool {
	for _, r := range rs {
		if expected[r.name] {
			return false // 점수는 받았다 — 밀린 것이다
		}
	}
	return true
}
