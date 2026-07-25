package main

// eval·golden-capture 서브커맨드 — M3 태깅 품질 측정 하네스.
//
//	pushpoint eval [golden-dir]        # dev.jsonl/test.jsonl로 Recall@3 + 태그별 P/R + 베이스라인 (네트워크 0)
//	pushpoint golden-capture urls.tsv  # url<TAB>tag,tag 목록을 프로덕션 스크랩 경로로 스냅샷 캡처 → JSONL
//
// eval은 golden 스냅샷만 입력으로 태거를 돌린다(네트워크 0 → 시점 무관 재현). 사전은 런타임과
// 동일하게 fresh 마이그레이션 DB에서 읽어(결정적 시드) golden==runtime을 보장한다. golden 스냅샷은
// {title, description, body_text} — 태거 Content와 정확히 일치한다(meta_keywords는 태거가 안 써서 제외).

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
		evalSet(strings.ToUpper(name), entries, dict, id2name)
	}
	if !ran {
		return fmt.Errorf("eval: golden 파일이 없습니다 (%s/{dev,test}.jsonl) — golden-capture로 만드세요", dir)
	}
	fmt.Println("\n측정치는 기록용이다 (M3엔 게이트 없음 — 게이트는 M5 진입, 동결 test 기준).")
	return nil
}

// evalSet은 한 세트(dev/test)의 지표를 계산·출력한다: 세 변형(baseline=도메인만 /
// no-body=도메인+제목+설명 / full=+본문)의 Recall@3, 그리고 full 기준 태그별 P/R.
func evalSet(name string, entries []goldenEntry, dict *tagger.Dictionary, id2name map[int64]string) {
	fmt.Printf("\n=== %s (%d건) ===\n", name, len(entries))
	if len(entries) == 0 {
		fmt.Println("(빈 세트 — 건너뜀)")
		return
	}

	var baseHit, noBodyHit, fullHit int
	// 태그별 집계 (full 기준): TP/FP/FN + golden 등장 수.
	tp := map[string]int{}
	fp := map[string]int{}
	fn := map[string]int{}
	goldN := map[string]int{}

	for _, e := range entries {
		host := hostOf(e.URL)
		exp := toSet(e.ExpectedTags)
		for t := range exp {
			goldN[t]++
		}

		base := classifyTop(tagger.Content{Domain: host}, dict, id2name)
		noBody := classifyTop(tagger.Content{Domain: host, Title: e.Snapshot.Title, Description: e.Snapshot.Description}, dict, id2name)
		full := classifyTop(tagger.Content{Domain: host, Title: e.Snapshot.Title, Description: e.Snapshot.Description, Body: e.Snapshot.BodyText}, dict, id2name)

		baseHit += hit(base, exp)
		noBodyHit += hit(noBody, exp)
		fullHit += hit(full, exp)

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
			pred := classifyTop(tagger.Content{
				Domain: hostOf(e.URL), Title: e.Snapshot.Title,
				Description: e.Snapshot.Description, Body: e.Snapshot.BodyText,
			}, dict, id2name)
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
			Snapshot:     goldenSnapshot{Title: m.Title, Description: m.Description, BodyText: m.BodyText},
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
