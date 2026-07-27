package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 정답 URL이 코퍼스에 없으면 **영원한 miss**다. 검색 실패처럼 보이지만 원인은 오타이고,
// 태깅 쪽에서 같은 실수를 이미 겪었다(사전에 없는 expected_tags가 조용히 Recall을 깎았다).
func TestLoadSearchQueriesRejectsIncomplete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "search.jsonl")

	write := func(s string) { _ = os.WriteFile(path, []byte(s), 0o644) }

	write(`{"query":"쿠버네티스","url":"https://a.com"}` + "\n")
	got, err := loadSearchQueries(path)
	if err != nil || len(got) != 1 {
		t.Fatalf("정상 한 줄은 읽혀야 한다: %v %v", got, err)
	}

	write(`{"query":"","url":"https://a.com"}` + "\n")
	if _, err := loadSearchQueries(path); err == nil {
		t.Error("빈 질의를 통과시켰다 — 조용히 넘기면 분모만 줄어 수치가 좋아 보인다")
	}

	write(`{"query":"쿠버네티스","url":""}` + "\n")
	if _, err := loadSearchQueries(path); err == nil {
		t.Error("빈 URL을 통과시켰다")
	}

	write("")
	if _, err := loadSearchQueries(path); err == nil {
		t.Error("빈 파일을 통과시켰다")
	}

	write("{깨진 JSON\n")
	if _, err := loadSearchQueries(path); err == nil {
		t.Error("깨진 줄을 통과시켰다")
	}
}

// 커밋된 질의 파일 자체를 검사한다 — 정답 URL이 전부 golden 안에 있어야 한다.
// `just eval-search`가 런타임에도 같은 검사를 하지만, 그건 CI에서 돌지 않는다.
func TestCommittedSearchQueriesResolve(t *testing.T) {
	queries, err := loadSearchQueries("../../../nlu/golden/search.jsonl")
	if err != nil {
		t.Fatalf("search.jsonl: %v", err)
	}
	corpus := map[string]bool{}
	for _, set := range []string{"dev", "test", "wild"} {
		entries, err := loadGolden("../../../nlu/golden/" + set + ".jsonl")
		if err != nil {
			t.Fatalf("%s: %v", set, err)
		}
		for _, e := range entries {
			corpus[e.URL] = true
		}
	}
	var bad []string
	for _, q := range queries {
		if !corpus[q.URL] {
			bad = append(bad, q.Query+" → "+q.URL)
		}
	}
	if len(bad) > 0 {
		t.Errorf("정답 URL이 golden에 없다 %d건:\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}
