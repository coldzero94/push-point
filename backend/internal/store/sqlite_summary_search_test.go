package store

import (
	"context"
	"testing"
)

// 요약이 검색에 잡혀야 한다.
//
// **왜 이게 필요한가**: golden 123건 중 107건(87%)이 요약에만 있는 3-gram을 얻는다
// (nlu/golden/README.md의 B1 게이트 측정). 즉 요약은 제목·설명이 담지 못한 어휘를
// 대부분의 링크에서 들여오는데, 색인에 없으면 그 어휘로는 저장한 것을 못 찾는다.
//
// 조용히 깨지는 방식이 두 가지라 둘 다 고정한다: reindexFTS가 summary를 안 읽는 경우와,
// SetSummary가 재색인하지 않는 경우. 후자가 특히 위험한데, tag 잡이 ApplyTags로 재색인한
// **뒤에** SetSummary가 오므로 요약은 저장되지만 색인에는 영원히 안 들어간다.
func TestSearch_findsBySummary(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, err := s.SaveLink(ctx, SaveInput{
		URL:   "https://sum.example/a",
		Title: "어느 글",
		// 제목·설명에는 없는 낱말을 요약에만 둔다 — 그래야 검색이 요약을 봤다고 말할 수 있다.
		Description: "짧은 소개",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 요약 붙기 전에는 안 잡혀야 한다(그래야 아래 성공이 요약 덕분임이 확실하다).
	if res, _, _, _ := s.Search(ctx, "형광등", "", nil, nil, "", 10); len(res) != 0 {
		t.Fatalf("요약 전에 이미 잡힌다 — 테스트가 무의미하다: %d건", len(res))
	}

	if err := s.SetSummary(ctx, id, "복도의 형광등이 깜빡이고 있었다."); err != nil {
		t.Fatal(err)
	}
	res, _, _, err := s.Search(ctx, "형광등", "", nil, nil, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].ID != id {
		t.Errorf("요약의 낱말로 못 찾는다 — SetSummary가 재색인하지 않거나 "+
			"reindexFTS가 summary를 안 읽는다: %d건", len(res))
	}
}

// 요약을 지우면 색인에서도 빠져야 한다 — 재시도로 요약이 사라졌는데 검색에는 남아 있으면
// 없는 문장으로 링크가 잡힌다.
func TestSearch_clearedSummaryLeavesIndex(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://sum.example/b", Title: "어느 글"})
	if err := s.SetSummary(ctx, id, "복도의 형광등이 깜빡이고 있었다."); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSummary(ctx, id, ""); err != nil {
		t.Fatal(err)
	}
	if res, _, _, _ := s.Search(ctx, "형광등", "", nil, nil, "", 10); len(res) != 0 {
		t.Errorf("지운 요약이 색인에 남아 있다: %d건", len(res))
	}
}
