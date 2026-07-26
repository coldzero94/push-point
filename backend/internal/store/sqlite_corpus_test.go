package store

import (
	"context"
	"errors"
	"testing"
)

// corpus_df가 세는 것은 **문서 수**여야 한다 — 태깅이 돈 횟수가 아니라.
//
// 이게 link_terms 원장이 존재하는 유일한 이유다. 태깅은 재시도·본문 보충·undelete로
// 여러 번 도는데, 올리기만 하면 df가 조용히 부풀어 오래 쓸수록 통계가 실제와 멀어진다.
// 그리고 부풀었다는 사실은 화면 어디에도 안 보인다.
func TestCorpusDF_retaggingDoesNotDoubleCount(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://c.example/a"})

	for i := 0; i < 3; i++ { // 같은 링크를 세 번 태깅
		if err := s.ApplyTags(ctx, id, nil, []string{"golang", "쿠버네티스"}); err != nil {
			t.Fatal(err)
		}
	}
	docs, df, err := s.CorpusDF(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if docs != 1 {
		t.Errorf("문서 수가 1이어야 함: %d", docs)
	}
	for _, term := range []string{"golang", "쿠버네티스"} {
		if df[term] != 1 {
			t.Errorf("%s의 df가 1이어야 함 (재태깅 3회): %d", term, df[term])
		}
	}
}

// 재태깅으로 표면이 바뀌면 사라진 표면의 df는 줄어야 한다. 되돌리기가 없으면 한 번 오른
// df는 영원히 남아, 본문이 교체된 링크가 옛 주제의 통계를 계속 떠받친다.
func TestCorpusDF_retaggingRemovesGoneTerms(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://c.example/b"})

	if err := s.ApplyTags(ctx, id, nil, []string{"golang", "docker"}); err != nil {
		t.Fatal(err)
	}
	// 본문이 보충돼 다시 태깅했더니 docker는 더 이상 안 나온다.
	if err := s.ApplyTags(ctx, id, nil, []string{"golang", "kubernetes"}); err != nil {
		t.Fatal(err)
	}
	_, df, err := s.CorpusDF(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := df["docker"]; ok {
		t.Errorf("사라진 표면이 df에 남음: docker=%d", df["docker"])
	}
	if df["golang"] != 1 || df["kubernetes"] != 1 {
		t.Errorf("df가 어긋남: golang=%d kubernetes=%d", df["golang"], df["kubernetes"])
	}
}

// 여러 문서가 같은 표면을 쓰면 df는 문서 수만큼 오른다 — DF의 정의 그 자체다.
func TestCorpusDF_countsDistinctDocuments(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	for i, u := range []string{"https://c.example/1", "https://c.example/2", "https://c.example/3"} {
		id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: u})
		terms := []string{"golang"}
		if i == 0 {
			terms = append(terms, "쿠버네티스")
		}
		if err := s.ApplyTags(ctx, id, nil, terms); err != nil {
			t.Fatal(err)
		}
	}
	docs, df, err := s.CorpusDF(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if docs != 3 {
		t.Errorf("문서 수 3이어야 함: %d", docs)
	}
	if df["golang"] != 3 {
		t.Errorf("golang df 3이어야 함: %d", df["golang"])
	}
	if df["쿠버네티스"] != 1 {
		t.Errorf("쿠버네티스 df 1이어야 함: %d", df["쿠버네티스"])
	}
}

// terms에 중복이 와도 한 문서는 df에 1만 기여해야 한다. 호출자가 집합을 넘기는 것이
// 계약이지만, 계약을 어겼을 때 통계가 조용히 틀리는 것보다 여기서 막는 편이 낫다.
func TestCorpusDF_duplicateTermsCountOnce(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://c.example/dup"})

	if err := s.ApplyTags(ctx, id, nil, []string{"golang", "golang", "golang"}); err != nil {
		t.Fatal(err)
	}
	if _, df, _ := s.CorpusDF(ctx); df["golang"] != 1 {
		t.Errorf("중복 표면이 df를 부풀림: %d", df["golang"])
	}
}

// 삭제한 링크는 코퍼스에서 빠져야 한다. 검색에서만 빼고 통계에 남겨 두면, 지운 주제가
// 계속 "흔한 낱말"로 취급돼 앞으로 저장할 링크의 태깅을 눈에 안 보이게 끌어내린다.
func TestCorpusDF_deleteWithdrawsContribution(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	keep, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://c.example/keep"})
	gone, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://c.example/gone"})
	if err := s.ApplyTags(ctx, keep, nil, []string{"golang"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyTags(ctx, gone, nil, []string{"golang", "docker"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLink(ctx, gone); err != nil {
		t.Fatal(err)
	}
	docs, df, err := s.CorpusDF(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if docs != 1 {
		t.Errorf("삭제 후 문서 수 1이어야 함: %d", docs)
	}
	if df["golang"] != 1 {
		t.Errorf("golang df 1이어야 함 (남은 링크 1건): %d", df["golang"])
	}
	if _, ok := df["docker"]; ok {
		t.Errorf("삭제된 링크의 표면이 남음: docker=%d", df["docker"])
	}
}

// 삭제된 링크에는 태그·기여를 쓰지 않아야 한다.
//
// tagjob은 콘텐츠 조회와 분류를 마친 **뒤에** writer를 잡는다. 그 사이에 삭제가 커밋되면
// (DeleteLink는 running 잡을 일부러 남긴다) 방금 회수한 기여가 곧바로 다시 쌓이고,
// 두 번째 DeleteLink는 ErrNotFound로 빠져 자가 치유 경로가 없다. 지운 주제가 계속
// "흔한 낱말"로 남아 앞으로의 태깅을 끌어내린다 — 회수하는 이유와 정확히 반대다.
func TestApplyTags_refusesDeletedLink(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://c.example/race"})
	if err := s.ApplyTags(ctx, id, nil, []string{"golang"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatal(err)
	}

	// 진행 중이던 tagjob이 뒤늦게 커밋하려는 상황.
	err := s.ApplyTags(ctx, id, nil, []string{"golang"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("삭제된 링크에 태깅이 통과했다: %v", err)
	}
	docs, df, err := s.CorpusDF(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if docs != 0 || len(df) != 0 {
		t.Errorf("삭제된 링크의 기여가 되살아났다: docs=%d df=%v", docs, df)
	}
}
