package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/coby/push-point/backend/internal/tagger"
)

func TestHit(t *testing.T) {
	exp := toSet([]string{"kubernetes", "devops"})
	if hit([]string{"kubernetes", "dev"}, exp) != 1 {
		t.Error("겹치면 hit=1")
	}
	if hit([]string{"python", "video"}, exp) != 0 {
		t.Error("안 겹치면 hit=0")
	}
	if hit(nil, exp) != 0 {
		t.Error("빈 예측은 hit=0")
	}
}

func TestRatio(t *testing.T) {
	if ratio(3, 4) != 0.75 {
		t.Error("3/4 != 0.75")
	}
	if ratio(0, 0) != 0 {
		t.Error("0/0은 0으로")
	}
}

func TestLoadGolden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "g.jsonl")
	content := `{"url":"https://a.com","snapshot":{"title":"t","description":"d","body_text":"b"},"expected_tags":["dev"]}

{"url":"https://b.com","snapshot":{"title":"t2","description":"","body_text":""},"expected_tags":["ai","ml"]}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadGolden(path)
	if err != nil {
		t.Fatalf("loadGolden: %v", err)
	}
	if len(got) != 2 { // 빈 줄은 건너뜀
		t.Fatalf("2건이어야, got %d", len(got))
	}
	if got[0].URL != "https://a.com" || got[0].Snapshot.Title != "t" || len(got[0].ExpectedTags) != 1 {
		t.Errorf("첫 항목 파싱 불일치: %+v", got[0])
	}
	if got[1].Snapshot.BodyText != "" || len(got[1].ExpectedTags) != 2 {
		t.Errorf("둘째 항목 파싱 불일치: %+v", got[1])
	}
}

func TestClassifyTop(t *testing.T) {
	dict := tagger.BuildDictionary([]tagger.TagEntry{
		{ID: 1, Name: "kubernetes", Aliases: []string{"k8s", "쿠버네티스"}},
		{ID: 2, Name: "python", Aliases: []string{"파이썬"}},
	})
	id2name := map[int64]string{1: "kubernetes", 2: "python"}
	got := classifyTop(tagger.Content{Title: "쿠버네티스 입문"}, dict, id2name)
	if !slices.Contains(got, "kubernetes") {
		t.Errorf("kubernetes 예측돼야: %v", got)
	}
	// top-k 상한(evalTopK) 이하
	if len(got) > evalTopK {
		t.Errorf("top-%d 초과: %v", evalTopK, got)
	}
	// 매치 없으면 빈 결과
	if got := classifyTop(tagger.Content{Title: "무관한 잡담"}, dict, id2name); len(got) != 0 {
		t.Errorf("매치 없으면 빈 결과: %v", got)
	}
}
