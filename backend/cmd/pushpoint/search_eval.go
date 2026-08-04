package main

// eval-search 서브커맨드 — 검색 품질 측정 하네스.
//
//	pushpoint eval-search [golden-dir]   # nlu/golden/search.jsonl로 hit@1 · MRR@10
//
// **왜 필요한가.** 태깅에는 세 세트짜리 평가가 있는데 검색에는 아무것도 없었다. 성능 목표표에
// "검색(FTS5, 1만 링크) < 30ms"가 적혀 있는데 그걸 재는 커맨드조차 리포에 없었다. 그 상태로
// 검색을 건드리면 **측정 없는 품질 변경과 측정 없는 성능 회귀가 동시에** 난다.
//
// **코퍼스는 golden 스냅샷이다.** 별도 픽스처를 만들지 않는 이유는 두 가지다 — golden은
// 이미 커밋돼 있어 결과가 시점에 무관하게 재현되고, 내용이 프로덕션 스크랩 경로로 뜬 진짜
// 페이지라 "검색이 실제 저장물에서 어떻게 동작하나"를 잰다. 임시 DB에 마이그레이션을 적용해
// 채우므로 `links_fts` 색인·bm25 랭킹·LIKE 폴백이 전부 런타임과 같은 코드다.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/store"

	"github.com/coby/push-point/backend/internal/summarizer"
)

// searchEvalTopN은 MRR을 자르는 순위. hit@1과 함께 낸다 —
// **다시 찾기**는 1위에 오느냐가 대부분이고, MRR은 "몇 번째에 있었나"를 본다.
const searchEvalTopN = 10

// searchQuery는 search.jsonl 한 줄. 질의 하나에 정답 링크 하나.
type searchQuery struct {
	Query string `json:"query"`
	// URL은 정답 링크. golden 세트 어딘가에 있어야 하고, 없으면 하네스가 실패한다 —
	// 오타난 URL은 **구조적으로 못 맞히는 정답**이 되어 검색 실패처럼 보인다.
	URL string `json:"url"`
	// Why는 이 질의를 왜 넣었는지. 사람이 나중에 "이걸 왜 골랐지"를 답할 수 있어야 한다.
	Why string `json:"why,omitempty"`
}

// runSearchEval은 golden 스냅샷으로 임시 DB를 채우고 search.jsonl의 질의를 돌린다.
func runSearchEval(args []string) error {
	dir := "nlu/golden"
	if len(args) > 0 {
		dir = args[0]
	}
	queries, err := loadSearchQueries(filepath.Join(dir, "search.jsonl"))
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "pp-search-eval-")
	if err != nil {
		return fmt.Errorf("eval-search: 임시 디렉터리 실패: %w", err)
	}
	defer os.RemoveAll(tmp)

	db, err := store.Open(tmp)
	if err != nil {
		return fmt.Errorf("eval-search: DB 열기 실패: %w", err)
	}
	defer db.Close()
	// 큐는 검색 경로가 안 쓰지만 생성자 계약상 필요하다 — 잡을 돌리지 않는다.
	//
	// **사전을 넘긴다.** 이걸 빠뜨리면 하네스가 확장 없는 검색을 재고, 출하품과 다시
	// 갈라진다 — 열 이름만 맞추고 내용이 갈렸던 그 실수를 반복하는 것이다.
	dict, _, err := loadEvalDict()
	if err != nil {
		return fmt.Errorf("eval-search: 사전 로드 실패: %w", err)
	}
	st := store.New(db, queue.NewSQLite(db.Writer), store.WithQueryExpander(func(context.Context) store.QueryExpander { return dict }))

	corpus, err := seedFromGolden(context.Background(), db, dir)
	if err != nil {
		return err
	}
	fmt.Printf("코퍼스 %d건 (golden 스냅샷, fresh 마이그레이션 DB)\n", len(corpus))

	// **정답 URL이 코퍼스에 없으면 영원한 miss다.** 그 항목은 검색 실패처럼 보이지만 원인은
	// 오타이고, 태깅 쪽에서 정확히 같은 실수를 이미 겪었다(사전에 없는 expected_tags).
	// 조용히 0점을 주는 대신 여기서 멈춘다.
	var unknown []string
	for _, sq := range queries {
		if !corpus[sq.URL] {
			unknown = append(unknown, fmt.Sprintf("%q → %s", sq.Query, sq.URL))
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("eval-search: 정답 URL이 코퍼스에 없습니다 %d건 — golden에 있는 URL이어야 합니다\n  %s",
			len(unknown), strings.Join(unknown, "\n  "))
	}

	var hit1, found int
	var rrSum float64
	type miss struct {
		q, url, why string
		rank        int // 0 = 아예 없음
		top         string
	}
	var misses []miss

	for _, sq := range queries {
		res, _, mode, err := st.Search(context.Background(), sq.Query, "", nil, nil, "", searchEvalTopN)
		if err != nil {
			return fmt.Errorf("eval-search: 검색 실패 %q: %w", sq.Query, err)
		}
		rank := 0
		for i, r := range res {
			if r.URL == sq.URL {
				rank = i + 1
				break
			}
		}
		if rank == 1 {
			hit1++
		}
		if rank > 0 {
			found++
			rrSum += 1 / float64(rank)
		}
		if rank != 1 {
			top := "(결과 없음)"
			if len(res) > 0 {
				top = fmt.Sprintf("%.40s", res[0].Title)
			}
			misses = append(misses, miss{sq.Query, sq.URL, sq.Why, rank, top + " [" + string(mode) + "]"})
		}
	}

	q := float64(len(queries))
	fmt.Printf("\n질의 %d개\n", len(queries))
	fmt.Printf("  hit@1   = %.3f  (%d/%d)  — 1위에 정답이 온 비율\n", float64(hit1)/q, hit1, len(queries))
	fmt.Printf("  MRR@%d  = %.3f            — 정답 순위의 역수 평균(없으면 0)\n", searchEvalTopN, rrSum/q)
	fmt.Printf("  상위 %d 안 도달 = %.3f  (%d/%d)\n", searchEvalTopN, float64(found)/q, found, len(queries))

	if len(misses) > 0 {
		fmt.Printf("\n1위가 아닌 %d건 — 개선의 조준점이다:\n", len(misses))
		for _, m := range misses {
			pos := fmt.Sprintf("%d위", m.rank)
			if m.rank == 0 {
				pos = "**미발견**"
			}
			fmt.Printf("  [%s] %q\n        기대: %s\n        1위: %s\n", pos, m.q, m.url, m.top)
			if m.why != "" {
				fmt.Printf("        의도: %s\n", m.why)
			}
		}
	}
	fmt.Println("\n측정치는 기록용이다 — 검색을 golden에 맞추는 것은 허용, golden을 검색 결과에 맞추는 것은 금지.")
	return nil
}

// loadSearchQueries는 search.jsonl을 읽는다. 빈 질의·빈 URL은 오류다 —
// 조용히 넘기면 분모만 줄어 수치가 좋아 보인다.
func loadSearchQueries(path string) ([]searchQuery, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eval-search: %s를 읽지 못했습니다: %w", path, err)
	}
	var out []searchQuery
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var sq searchQuery
		if err := json.Unmarshal([]byte(line), &sq); err != nil {
			return nil, fmt.Errorf("eval-search: %s:%d 파싱 실패: %w", path, i+1, err)
		}
		if sq.Query == "" || sq.URL == "" {
			return nil, fmt.Errorf("eval-search: %s:%d query와 url이 모두 있어야 합니다", path, i+1)
		}
		out = append(out, sq)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("eval-search: %s가 비어 있습니다", path)
	}
	return out, nil
}

// seedFromGolden은 dev/test/wild 스냅샷을 links에 넣고 FTS를 색인한다.
//
// store 인터페이스를 쓰지 않고 SQL로 직행하는 이유는 `SaveLink`가 제목·본문을 못 받기
// 때문이다(스크랩 잡이 채우는 구조). 여기서는 스냅샷이 이미 답이므로 잡을 돌릴 이유가 없다.
// **색인은 런타임과 같은 컬럼 구성**이어야 하므로 links_fts INSERT를 직접 맞춘다.
func seedFromGolden(ctx context.Context, db *store.DB, dir string) (map[string]bool, error) {
	seen := map[string]bool{}
	for _, name := range []string{"dev", "test", "wild"} {
		entries, err := loadGolden(filepath.Join(dir, name+".jsonl"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if seen[e.URL] {
				continue
			}
			seen[e.URL] = true
			sum := sha256.Sum256([]byte(e.URL))
			host := ""
			if u, err := url.Parse(e.URL); err == nil {
				host = strings.TrimPrefix(u.Host, "www.")
			}
			res, err := db.Writer.ExecContext(ctx, `
				INSERT INTO links (url, url_hash, domain, title, description, body_text, status)
				VALUES (?, ?, ?, ?, ?, ?, 'done')`,
				e.URL, hex.EncodeToString(sum[:]), host,
				e.Snapshot.Title, e.Snapshot.Description, e.Snapshot.BodyText)
			if err != nil {
				return nil, fmt.Errorf("eval-search: 링크 삽입 실패 %s: %w", e.URL, err)
			}
			id, _ := res.LastInsertId()
			// 색인 구성은 런타임(store.reindexFTS)과 **글자 그대로** 같아야 한다.
			//
			// 열 이름만 맞추고 내용이 갈렸던 자리다: 런타임은 description 열에
			// `description + " " + summary`를 넣는데 하네스는 description만 넣었다.
			// 그래서 `just eval-search`가 **출하품을 4.0점 낮게** 쟀다(hit@1 0.480 대
			// 실제 0.520). 요약은 본문에서 순수 함수로 나오므로 golden에 필드를 더할
			// 필요 없이 여기서 같은 함수를 부른다.
			//
			// 태그는 golden 라벨을 쓴다: 태거가 붙였을 값의 상한이다. 실제 태거 예측으로
			// 바꿔 재 봐도 hit@1·MRR이 **정확히 0.000** 움직였다 — 이 25개 질의는 태그
			// 열로 풀리지 않는다.
			summary := summarizer.Summarize(e.Snapshot.BodyText, e.Snapshot.Description)
			if _, err := db.Writer.ExecContext(ctx, `
				INSERT INTO links_fts (rowid, title, description, note, tags)
				VALUES (?, ?, ?, '', ?)`,
				id, e.Snapshot.Title,
				strings.TrimSpace(e.Snapshot.Description+" "+summary),
				strings.Join(e.ExpectedTags, " ")); err != nil {
				return nil, fmt.Errorf("eval-search: FTS 색인 실패 %s: %w", e.URL, err)
			}
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("eval-search: golden 스냅샷이 없습니다 (%s)", dir)
	}
	return seen, nil
}
