package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

	// **깨진 줄은 건너뛰지 않고 에러다.** 조용히 건너뛰면 세트가 줄어든 채로 Recall이
	// 계산되고, 분모가 바뀐 것을 아무도 모른다. 잘린 파일이 오히려 더 좋은 수를 내기도 한다.
	bad := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(bad, []byte(`{"url":"https://a.com","expected_tags":["dev"]}
{"url": 깨진 JSON
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadGolden(bad); err == nil {
		t.Error("깨진 JSONL 줄을 조용히 건너뛰었다 — 에러여야 한다")
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
	// **컷은 리터럴 3으로 고정한다.** 예전에는 `len(got) > evalTopK`로 상수를 자기 자신과
	// 비교했고, 사전이 2태그뿐이라 3을 넘을 수가 없어서 **어떤 값을 넣어도 통과했다.**
	// 보고되는 모든 수가 "@3"인데 그 3을 고정하는 것이 아무것도 없었다.
	// 아래는 4태그가 전부 매치되는 입력이라, 자르기를 없애면 4가 나와 실패한다.
	wide := tagger.BuildDictionary([]tagger.TagEntry{
		{ID: 1, Name: "kubernetes", Aliases: []string{"쿠버네티스"}},
		{ID: 2, Name: "python", Aliases: []string{"파이썬"}},
		{ID: 3, Name: "golang", Aliases: []string{"고랭"}},
		{ID: 4, Name: "rust", Aliases: []string{"러스트"}},
	})
	wideNames := map[int64]string{1: "kubernetes", 2: "python", 3: "golang", 4: "rust"}
	all := classifyTop(tagger.Content{Title: "쿠버네티스 파이썬 고랭 러스트"}, wide, wideNames)
	if len(all) != 3 {
		t.Errorf("4개가 매치돼도 top-3으로 잘라야 한다: %d개 %v", len(all), all)
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

// validateExpectedTags가 없으면 라벨 오타가 **영구 miss**가 되어 태거 실패로 읽힌다.
// 태그별 표에서도 P=0.00 R=0.00 행이라 "태거가 못 맞히는 태그"와 눈으로 구분되지 않는다.
func TestValidateExpectedTags(t *testing.T) {
	id2name := map[int64]string{1: "career", 2: "ai"}
	entry := func(tags ...string) goldenEntry {
		return goldenEntry{URL: "https://a.com", ExpectedTags: tags}
	}

	if err := validateExpectedTags("wild", []goldenEntry{entry("career", "ai")}, id2name); err != nil {
		t.Errorf("사전에 있는 태그만 쓴 세트는 통과해야 한다: %v", err)
	}

	// 오타 — career를 carear로.
	err := validateExpectedTags("wild", []goldenEntry{entry("carear")}, id2name)
	if err == nil {
		t.Fatal("사전에 없는 태그명을 통과시켰다")
	}
	if !strings.Contains(err.Error(), "carear") {
		t.Errorf("어느 태그가 문제인지 말해야 한다: %v", err)
	}

	// 정답이 없는 항목은 자동 miss라 분모만 키운다.
	if err := validateExpectedTags("wild", []goldenEntry{entry()}, id2name); err == nil {
		t.Fatal("expected_tags가 빈 항목을 통과시켰다")
	}

	// 행 번호가 있어야 31줄짜리 파일에서 찾을 수 있다.
	err = validateExpectedTags("wild", []goldenEntry{entry("ai"), entry("nope")}, id2name)
	if err == nil || !strings.Contains(err.Error(), "2행") {
		t.Errorf("문제 행 번호를 알려야 한다: %v", err)
	}
}

// isThinSnapshot은 "태거를 고쳐서 얻을 수 있는 몫"의 경계를 긋는다. 세 필드를 합쳐서 보지
// 않으면 x.com처럼 본문 0자·설명 전문인 정상 캡처를 벽으로 오인한다.
func TestIsThinSnapshot(t *testing.T) {
	long := strings.Repeat("가", 300)
	short := strings.Repeat("가", 50)

	if isThinSnapshot("", "", long) {
		t.Error("본문이 충분하면 빈약이 아니다")
	}
	// x.com 어댑터: 본문 0자, 트윗 전문이 description에 온다.
	if isThinSnapshot("이재훈", long, "") {
		t.Error("설명에 내용이 있으면 본문이 비어도 빈약이 아니다")
	}
	// 봇 차단 페이지 — 제목이 그럴듯해도 내용이 없다.
	if !isThinSnapshot("Reddit - Please wait for verification", "", "") {
		t.Error("제목만 있고 내용이 없으면 빈약이다")
	}
	// 합산 경계 — 여기가 이 함수의 핵심이다. 어느 필드도 단독으로는 200자에 못 미치지만
	// 합치면 240자라 빈약이 아니다. 필드를 따로 보는 구현은 이 케이스에서 갈린다.
	part := strings.Repeat("가", 80)
	if isThinSnapshot(part, part, part) {
		t.Error("세 필드 합이 240자면 빈약이 아니다 — 필드별로 보면 안 된다")
	}
	// 반대쪽: 합이 150자면 어느 필드에 흩어져 있든 빈약이다.
	if !isThinSnapshot(short, short, short) {
		t.Error("합이 150자면 빈약이다")
	}
}

// measureSet은 리포트되는 **모든 수**의 출처다. 예전에는 계산이 Printf와 한 함수에 섞여
// 있어 테스트를 붙일 수 없었고, 그래서 분모 오프바이원·변형 배선 뒤바꿈·태그별 TP/FP/FN
// 오류가 전부 조용히 통과했다(2026-07-26 뮤테이션 확인).
//
// 픽스처는 손으로 세어 답을 아는 3건이다. 사전은 4태그이고, 도메인 규칙에 기대지 않도록
// 도메인 신호가 없는 URL을 쓴다.
func TestMeasureSet(t *testing.T) {
	dict := tagger.BuildDictionary([]tagger.TagEntry{
		{ID: 1, Name: "kubernetes", Aliases: []string{"쿠버네티스"}},
		{ID: 2, Name: "python", Aliases: []string{"파이썬"}},
		{ID: 3, Name: "golang", Aliases: []string{"고랭"}},
		{ID: 4, Name: "rust", Aliases: []string{"러스트"}},
	})
	id2name := map[int64]string{1: "kubernetes", 2: "python", 3: "golang", 4: "rust"}

	long := strings.Repeat("가", 300) // 빈약 판정에 걸리지 않을 만큼 긴 본문

	entries := []goldenEntry{
		// 1) 제목이 정답을 말한다 → 모든 변형에서 hit. 본문·분류가 없어도 맞는다.
		{
			URL:          "https://example.com/a",
			Snapshot:     goldenSnapshot{Title: "쿠버네티스 입문 " + long},
			ExpectedTags: []string{"kubernetes"},
		},
		// 2) **본문에만** 정답이 있다 → full은 hit, no-body는 miss.
		//    Δbody가 실제로 본문 기여를 재는지 여기서 갈린다.
		{
			URL:          "https://example.com/b",
			Snapshot:     goldenSnapshot{Title: "제목에는 단서가 없다 " + long, BodyText: "파이썬 " + long},
			ExpectedTags: []string{"python"},
		},
		// 3) 어디에도 정답 신호가 없다 → 전 변형 miss, 그리고 **정답 0점** 미스.
		{
			URL:          "https://example.com/c",
			Snapshot:     goldenSnapshot{Title: "고랭 이야기 " + long},
			ExpectedTags: []string{"rust"},
		},
	}

	m := measureSet(entries, dict, id2name)

	// Recall 분자 — 분모(len(entries))는 호출부가 쓰므로 여기서는 hit 수를 직접 고정한다.
	if m.fullHit != 2 {
		t.Errorf("full hit=2 여야 한다 (1·2번): %d", m.fullHit)
	}
	if m.noBodyHit != 1 {
		t.Errorf("no-body hit=1 이어야 한다 (2번이 본문을 잃고 miss): %d", m.noBodyHit)
	}
	if m.baseHit != 0 {
		t.Errorf("도메인만으로는 하나도 못 맞힌다: %d", m.baseHit)
	}
	// 분류(keywords)를 가진 항목이 없으므로 no-keywords는 full과, bare는 no-body와 같아야 한다.
	if m.noKWHit != m.fullHit {
		t.Errorf("분류가 없으면 no-keywords는 full과 같다: %d vs %d", m.noKWHit, m.fullHit)
	}
	if m.bareHit != m.noBodyHit {
		t.Errorf("분류가 없으면 bare는 no-body와 같다: %d vs %d", m.bareHit, m.noBodyHit)
	}
	if m.withKW != 0 {
		t.Errorf("분류를 가진 항목이 없다: %d", m.withKW)
	}

	// 미스 해부 — 3번은 정답 rust가 0점이므로 '순위 밀림'이 아니라 '정답 0점'이다.
	if m.missZero != 1 || m.missRank != 0 {
		t.Errorf("미스 해부: 정답 0점 1 · 순위 밀림 0 이어야 한다 (0점=%d 밀림=%d)", m.missZero, m.missRank)
	}

	// 태그별 집계 — golden 등장 수는 예측과 무관하게 라벨에서만 온다.
	for tag, want := range map[string]int{"kubernetes": 1, "python": 1, "rust": 1} {
		if m.goldN[tag] != want {
			t.Errorf("goldN[%s]=%d, 기대 %d", tag, m.goldN[tag], want)
		}
	}
	if m.tp["kubernetes"] != 1 {
		t.Errorf("kubernetes는 1번에서 TP: %d", m.tp["kubernetes"])
	}
	if m.fn["rust"] != 1 {
		t.Errorf("rust는 3번에서 FN(라벨엔 있는데 예측 안 됨): %d", m.fn["rust"])
	}
	// 3번에서 golang이 예측되지만 라벨은 rust다 → FP.
	if m.fp["golang"] != 1 {
		t.Errorf("golang은 3번에서 FP(예측됐지만 정답 아님): %d", m.fp["golang"])
	}
	if m.tp["golang"] != 0 {
		t.Errorf("golang은 어디서도 TP가 아니다: %d", m.tp["golang"])
	}

	// 빈약 스냅샷 — 셋 다 본문이 길어 해당 없음.
	if m.thin != 0 {
		t.Errorf("빈약한 스냅샷이 없어야 한다: %d", m.thin)
	}
}

// fill은 **비어 있는 필드만** 채운다. 이 한 줄이 golden-refill과 동결 test 사이에 서 있는
// 전부다 — 덮어쓰기로 바뀌면 refill 한 번에 test.jsonl의 스냅샷이 통째로 새 캡처로 바뀌고,
// 그 순간 이전 측정치와의 비교 가능성이 사라진다. 되돌릴 방법은 git뿐이다.
func TestFillPreservesExisting(t *testing.T) {
	existing := "원래 값"
	fill(&existing, "새 값")
	if existing != "원래 값" {
		t.Errorf("비어 있지 않은 필드는 보존해야 한다: %q", existing)
	}

	empty := ""
	fill(&empty, "새 값")
	if empty != "새 값" {
		t.Errorf("비어 있는 필드는 채워야 한다: %q", empty)
	}
}
