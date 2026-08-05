package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// 하네스가 런타임과 **같은 것**을 색인·채점하는지.
//
// 왜 소스를 읽어서 비교하는가: 두 코드는 서로 다른 패키지에 있고 각자 잘 돈다. 갈라져도
// 아무것도 실패하지 않는다 — 그냥 eval이 출하되지 않는 제품을 재게 된다. 실제로
// 그랬다(2026-08-04): 열 이름은 같은데 런타임은 description 열에 `description + summary`를
// 넣고 하네스는 description만 넣어서, `just eval-search`가 hit@1을 4.0점 낮게 쟀다.
// 그 상태로 "본문을 색인하면 나아질 것"이라는 잘못된 결론까지 갔다.
//
// 여기서 값을 비교할 수는 없다(다른 프로세스, 다른 DB). 대신 **두 쪽이 같은 재료를
// 언급하는지**를 본다 — 완벽하지 않지만, 한쪽만 고치는 순간 빨개진다는 것이 요점이다.
func TestEvalHarnessIndexesLikeRuntime(t *testing.T) {
	runtime := readFile(t, "../../internal/store/sqlite.go")
	harness := readFile(t, "search_eval.go")

	runtimeInsert := between(t, runtime, "INSERT INTO links_fts", "return nil")
	harnessInsert := between(t, harness, "INSERT INTO links_fts", "eval-search: FTS 색인 실패")

	for _, ingredient := range []string{"summary", "description", "title"} {
		inRuntime := strings.Contains(runtimeInsert, ingredient)
		inHarness := strings.Contains(harnessInsert, ingredient)
		if inRuntime != inHarness {
			t.Errorf("FTS 색인 재료 %q — 런타임 %v, 하네스 %v. 한쪽만 고쳤다면 "+
				"`just eval-search`는 출하되지 않는 제품을 재게 된다.",
				ingredient, inRuntime, inHarness)
		}
	}
}

// 태거의 상위 k가 런타임과 채점에서 같은지.
//
// 다르면 앱이 붙이는데 채점되지 않는 태그가 생긴다. 실제로 런타임 5 / 채점 3이었고,
// 그 두 칸이 태그 필터·통계 facet·FTS tags 열로 나가면서 정답률 16%짜리 태그를
// 검색과 개수에 섞고 있었다.
func TestEvalTopKMatchesRuntime(t *testing.T) {
	runtimeK := intConst(t, readFile(t, "../../internal/tagger/classify.go"), `topK\s+=\s+(\d+)`)
	evalK := intConst(t, readFile(t, "eval.go"), `evalTopK = (\d+)`)
	if runtimeK != evalK {
		t.Fatalf("런타임 topK=%d, evalTopK=%d — 앱이 붙이는 태그와 채점하는 태그가 다르다",
			runtimeK, evalK)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s 읽기 실패: %v", path, err)
	}
	return string(b)
}

func between(t *testing.T, s, from, to string) string {
	t.Helper()
	i := strings.Index(s, from)
	if i < 0 {
		t.Fatalf("%q를 못 찾았다 — 코드가 바뀌었으면 이 테스트도 같이 고쳐야 한다", from)
	}
	j := strings.Index(s[i:], to)
	if j < 0 {
		t.Fatalf("%q 뒤에서 %q를 못 찾았다", from, to)
	}
	return s[i : i+j]
}

func intConst(t *testing.T, s, pattern string) int {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("%q를 못 찾았다", pattern)
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n
}
