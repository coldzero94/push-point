package main

// eval·golden-capture 서브커맨드 — M3 태깅 품질 측정 하네스.
//
//	pushpoint eval [golden-dir]        # dev.jsonl/test.jsonl로 Recall@3 + 태그별 P/R + 베이스라인 (네트워크 0)
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

// runEval은 golden 디렉터리(기본 nlu/golden/)의 dev/test를 평가해 리포트를 출력한다.
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
	for _, name := range []string{"dev", "test"} {
		path := filepath.Join(dir, name+".jsonl")
		entries, err := loadGolden(path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("\n(%s 없음 — 건너뜀: %s)\n", name, path)
				continue
			}
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
		return fmt.Errorf("eval: golden 파일이 없습니다 (%s/{dev,test}.jsonl) — golden-capture로 만드세요", dir)
	}
	fmt.Println("\n측정치는 기록용이다 (M3엔 게이트 없음 — 게이트는 M5 진입, 동결 test 기준).")
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

// evalSet은 한 세트(dev/test)의 지표를 계산·출력한다: Recall@3와 full 기준 태그별 P/R.
//
// 변형은 **full에서 신호를 하나씩 뺀 것**이다 — 그래야 Δ가 그 신호만의 기여가 된다.
// full=도메인+제목+설명+본문+분류 / no-body=full−본문 / no-keywords=full−분류 /
// baseline=도메인만(규칙 전체의 기여를 보는 기준선).
// goldenCorpus는 golden 세트 자체를 코퍼스로 삼아 DF를 센다.
//
// 런타임에서 corpus_df가 하는 일과 **같은 계산**이다 — 런타임은 저장된 링크가 코퍼스이고
// 여기서는 golden이 코퍼스다. 다른 코퍼스를 빌려 오면 실제로 켜질 통계와 다른 것을 재게 된다.
func goldenCorpus(entries []goldenEntry, dict *tagger.Dictionary) tagger.CorpusStats {
	df := map[string]int64{}
	for _, e := range entries {
		for s := range dict.MatchedSurfaces(evalContent(e, true, true)) {
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
	return tagger.CorpusStats{Docs: int64(len(entries)), DF: df}
}

func evalSet(name string, entries []goldenEntry, dict *tagger.Dictionary, id2name map[int64]string) {
	fmt.Printf("\n=== %s (%d건) ===\n", name, len(entries))
	if len(entries) == 0 {
		fmt.Println("(빈 세트 — 건너뜀)")
		return
	}

	var baseHit, noBodyHit, noKWHit, fullHit, bareHit int
	// 태그별 집계 (full 기준): TP/FP/FN + golden 등장 수.
	tp := map[string]int{}
	fp := map[string]int{}
	fn := map[string]int{}
	goldN := map[string]int{}
	// 발행자 분류를 가진 항목 수 — Δkeywords를 몇 건 위에서 잰 것인지 없으면 해석할 수 없다.
	withKW := 0

	for _, e := range entries {
		exp := toSet(e.ExpectedTags)
		for t := range exp {
			goldN[t]++
		}
		if e.Snapshot.Keywords != "" {
			withKW++
		}

		base := classifyTop(tagger.Content{Domain: hostOf(e.URL)}, dict, id2name)
		noBody := classifyTop(evalContent(e, false, true), dict, id2name)
		noKW := classifyTop(evalContent(e, true, false), dict, id2name)
		full := classifyTop(evalContent(e, true, true), dict, id2name)
		// bare = 본문도 분류도 없는 변형. 분류가 **본문의 대체재**인지 보려면 필요하다.
		bare := classifyTop(evalContent(e, false, false), dict, id2name)

		baseHit += hit(base, exp)
		noBodyHit += hit(noBody, exp)
		noKWHit += hit(noKW, exp)
		fullHit += hit(full, exp)
		bareHit += hit(bare, exp)

		// 태그별 P/R은 full 예측 기준.
		predicted := toSet(full)
		for t := range predicted {
			if exp[t] {
				tp[t]++
			} else {
				fp[t]++
			}
		}
		for t := range exp {
			if !predicted[t] {
				fn[t]++
			}
		}
	}

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
	scored := tagger.Classify(c, dict)
	if len(scored) > evalTopK {
		scored = scored[:evalTopK]
	}
	out := make([]string, 0, len(scored))
	for _, s := range scored {
		out = append(out, id2name[s.TagID])
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
	var ok, fail int
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
		fmt.Fprintf(os.Stderr, "  캡처: %s (title=%.30q body=%dB)\n", rawURL, m.Title, len(m.BodyText))
	}
	if err := sccan.Err(); err != nil {
		return fmt.Errorf("golden-capture: 입력 읽기 실패: %w", err)
	}
	fmt.Fprintf(os.Stderr, "완료: %d 캡처, %d 실패\n", ok, fail)
	return nil
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
