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

// 경계 동점 판정 — topK 밖을 봐야 알 수 있다.
//
// 3위와 4위 점수가 같으면 순위가 **태그 이름 알파벳순**으로 갈린다. 그건 품질이 아니라
// 우연인데 hit@3는 3위 안에만 있으면 통과라 그 우연을 못 본다. 가중치를 건드리면 이
// 덩어리가 재배열되면서 Recall@3는 거의 안 움직인다 — 지표가 조타를 못 하는 자리다.
func TestTiedAtCut(t *testing.T) {
	cases := []struct {
		name string
		rs   []ranked
		want bool
	}{
		{"경계 밖에 후보 없음", []ranked{{"a", 0.9}, {"b", 0.8}, {"c", 0.7}}, false},
		{"3위와 4위 동점", []ranked{{"a", 0.9}, {"b", 0.8}, {"c", 0.7}, {"d", 0.7}}, true},
		{"4위가 더 낮음", []ranked{{"a", 0.9}, {"b", 0.8}, {"c", 0.7}, {"d", 0.6}}, false},
		{"후보가 topK 미만", []ranked{{"a", 0.9}}, false},
		{"비었음", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tiedAtCut(tc.rs); got != tc.want {
				t.Errorf("tiedAtCut = %v, want %v", got, tc.want)
			}
		})
	}
}

// 미스 해부 — 0점인가 밀렸는가.
//
// 이 구분이 어떤 개선이 유효한지를 정한다. 0점은 순위를 아무리 바꿔도 못 고치고
// 승격이 필요한데, 승격은 오탐 대량 유입이라는 다른 위험이다. 실측으로 동결 test
// 미스 7건 중 6건이 0점이라 재랭킹 상한이 +1.6pp에 갇힌다 — 그 사실을 이 함수가 만든다.
func TestZeroScored(t *testing.T) {
	exp := map[string]bool{"book": true}
	cases := []struct {
		name string
		rs   []ranked
		want bool
	}{
		{"정답이 점수를 못 받음", []ranked{{"article", 0.9}, {"culture", 0.8}}, true},
		{"정답이 점수는 받았지만 밀림", []ranked{{"article", 0.9}, {"culture", 0.8}, {"book", 0.3}}, false},
		{"정답이 1위", []ranked{{"book", 0.9}}, false},
		{"아무 태그도 없음", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := zeroScored(tc.rs, exp); got != tc.want {
				t.Errorf("zeroScored = %v, want %v", got, tc.want)
			}
		})
	}
}
