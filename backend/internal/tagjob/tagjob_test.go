package tagjob

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/store"
	"github.com/coby/push-point/backend/internal/tagger"
)

// body는 요약 가드(MinBodyRunes=200, MinProseSents=3)를 통과하고 시드 사전의
// kubernetes·golang에 걸리는 산문이다.
const body = "쿠버네티스 클러스터에서 파드를 배포하고 롤링 업데이트를 수행하는 방법을 설명한다. " +
	"golang으로 작성한 컨트롤러가 리소스 변화를 감지해 원하는 상태로 수렴시키는 과정을 살펴본다. " +
	"서비스 오브젝트가 파드 집합에 안정적인 네트워크 엔드포인트를 부여하는 원리를 다룬다. " +
	"오토스케일러가 관측 지표를 기준으로 레플리카 수를 조정하는 과정을 정리한다. " +
	"인그레스 컨트롤러가 외부 트래픽을 클러스터 내부로 라우팅하는 흐름을 따라간다."

func newStore(t *testing.T) store.Store {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open 실패: %v", err)
	}
	st := store.New(db, queue.NewSQLite(db.Writer))
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func saveLink(t *testing.T, st store.Store, url, bodyText string) int64 {
	t.Helper()
	id, _, _, err := st.SaveLink(context.Background(), store.SaveInput{
		URL: url, Title: "쿠버네티스 배포 전략", Description: "짧은 설명", BodyText: bodyText,
	})
	if err != nil {
		t.Fatalf("SaveLink 실패: %v", err)
	}
	return id
}

func TestRun_tagsAndSummarizes(t *testing.T) {
	st := newStore(t)
	id := saveLink(t, st, "https://example.com/k8s", body)

	res, err := Run(context.Background(), st, id)
	if err != nil {
		t.Fatalf("Run 실패: %v", err)
	}
	if res.Tags == 0 {
		t.Error("본문에 kubernetes·golang이 있으므로 태그가 붙어야 한다")
	}
	if res.SummaryLen == 0 {
		t.Error("산문 5문장이면 요약 가드를 통과해야 한다")
	}
	if res.SummaryErr != nil {
		t.Errorf("요약 기록이 실패하면 안 된다: %v", res.SummaryErr)
	}

	// 실제로 커밋됐는지 — Result만 보고 믿지 않는다.
	detail, err := st.GetLink(context.Background(), id)
	if err != nil {
		t.Fatalf("GetLink 실패: %v", err)
	}
	if len(detail.Tags) != res.Tags {
		t.Errorf("DB의 태그 수(%d)가 Result(%d)와 다르다", len(detail.Tags), res.Tags)
	}
	if len(detail.Summary) != res.SummaryLen {
		t.Errorf("DB의 요약 길이(%d)가 Result(%d)와 다르다", len(detail.Summary), res.SummaryLen)
	}
	// 추출식이므로 요약 문장은 원문에 그대로 있어야 한다(생성 금지).
	for _, line := range strings.Split(detail.Summary, "\n") {
		if line != "" && !strings.Contains(body, line) {
			t.Errorf("원문에 없는 문장이 요약에 있다: %q", line)
		}
	}
}

// 재실행이 태그를 중복시키면 안 된다 — 서버 워커의 재시도와 확장의 인라인 실행이
// 같은 링크에 겹칠 수 있으므로 멱등성이 전제다(ApplyTags가 source='rules'를 먼저 지운다).
func TestRun_isIdempotent(t *testing.T) {
	st := newStore(t)
	id := saveLink(t, st, "https://example.com/twice", body)

	first, err := Run(context.Background(), st, id)
	if err != nil {
		t.Fatalf("첫 Run 실패: %v", err)
	}
	second, err := Run(context.Background(), st, id)
	if err != nil {
		t.Fatalf("두 번째 Run 실패: %v", err)
	}
	if first.Tags != second.Tags {
		t.Errorf("재실행이 태그 수를 바꿨다: %d → %d", first.Tags, second.Tags)
	}
	detail, err := st.GetLink(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Tags) != first.Tags {
		t.Errorf("DB에 태그가 중복 적재됐다: %d (기대 %d)", len(detail.Tags), first.Tags)
	}
}

// 없는 링크는 에러여야 한다 — 워커는 이걸로 잡을 실패시키고, 확장은 tag_error에 담는다.
func TestRun_missingLink(t *testing.T) {
	st := newStore(t)
	res, err := Run(context.Background(), st, 99999)
	if err == nil {
		t.Fatalf("없는 링크는 에러여야 한다 (got %+v)", res)
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("ErrNotFound여야 한다: %v", err)
	}
}

// 본문이 없어도 실패하지 않고, 요약만 비어야 한다(가드 불통과는 정상 값).
// 확장이 URL만 받은 경우가 정확히 이 경로다.
func TestRun_noBodyStillSucceeds(t *testing.T) {
	st := newStore(t)
	id := saveLink(t, st, "https://example.com/urlonly", "")

	res, err := Run(context.Background(), st, id)
	if err != nil {
		t.Fatalf("본문이 없어도 실패하면 안 된다: %v", err)
	}
	if res.SummaryLen != 0 {
		t.Errorf("본문이 없으면 요약은 비어야 한다: %d", res.SummaryLen)
	}
	if res.SummaryErr != nil {
		t.Errorf("빈 요약을 쓰는 것도 정상 경로다: %v", res.SummaryErr)
	}
}

// 태깅이 corpus_df를 채워야 한다 — 그리고 다시 돌려도 부풀지 않아야 한다.
//
// 이 경로 전체가 눈에 보이지 않는다: corpus_df는 API에도 화면에도 안 나온다. 태거가
// 표면을 넘기지 않거나 넘기는 키가 매칭과 달라지면 df는 조용히 0에 머물고, 나중에 IDF를
// 켜는 사람은 "IDF가 효과 없다"는 잘못된 결론을 얻는다. 그래서 여기서 못박는다.
func TestRun_accumulatesCorpusDF(t *testing.T) {
	st := newStore(t)
	// **띄어 쓰지 않은 복합명사**를 일부러 넣는다. 공유 픽스처(body)는 "쿠버네티스 클러스터"로
	// 띄어 써서 문서 토큰과 사전 표면이 같아지는데, 그러면 아래 "키가 사전 표면인가" 단언이
	// 무엇을 검사하든 통과한다 — 실측으로 그 상태였다. 한글 매칭은 전방일치라
	// "쿠버네티스클러스터"가 표면 "쿠버네티스"에 걸리고, 그때 둘이 달라진다.
	id := saveLink(t, st, "https://example.com/corpus",
		"쿠버네티스클러스터를 운영하며 겪은 일을 정리한다. "+
			"golang으로 작성한 컨트롤러가 리소스 변화를 감지해 원하는 상태로 수렴시킨다. "+
			"오토스케일러가 관측 지표를 기준으로 레플리카 수를 조정하는 과정을 살펴본다.")
	ctx := context.Background()

	if _, err := Run(ctx, st, id); err != nil {
		t.Fatalf("Run: %v", err)
	}
	docs, df, err := st.CorpusDF(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if docs != 1 {
		t.Fatalf("문서 수 1이어야 함: %d", docs)
	}
	if len(df) == 0 {
		t.Fatal("태깅이 corpus_df에 아무것도 남기지 않았다 — 표면 전달이 끊겼다")
	}
	// **키가 사전 표면인지**를 본다. df==1만 보던 이전 단언은 수학적으로 실패할 수 없었다 —
	// 문서 1건 + 중복 없는 집합이므로 무슨 키가 들어오든 df는 항상 1이다. 실측: match.go의
	// `out[p.surface]`를 `out[dt]`로 바꿔도 17개 패키지 전부 통과했다.
	//
	// 이게 중요한 이유: `MatchedSurfaces`는 `matchField`의 매칭 규칙을 손으로 복제한 두
	// 번째 구현이라 키가 갈라질 실제 위험이 있고, 누적 키와 조회 키가 어긋나면 df가
	// **조용히 0에 머문다**(idf.go가 없는 키를 df=0으로 읽는다).
	surfaces := map[string]bool{}
	for _, s := range dictSurfaces(t, st) {
		surfaces[s] = true
	}
	for term, n := range df {
		if n != 1 {
			t.Errorf("한 문서에서 %q의 df가 1이 아님: %d", term, n)
		}
		if !surfaces[term] {
			t.Errorf("사전 표면이 아닌 키가 corpus_df에 쌓였다: %q — 누적 키와 조회 키가 갈라지면 "+
				"df가 조용히 0에 머문다", term)
		}
	}

	if _, err := Run(ctx, st, id); err != nil {
		t.Fatalf("재Run: %v", err)
	}
	docs2, df2, err := st.CorpusDF(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if docs2 != docs || len(df2) != len(df) {
		t.Errorf("재태깅이 통계를 바꿈: docs %d→%d, 표면 %d→%d", docs, docs2, len(df), len(df2))
	}
	for term, n := range df2 {
		if n != 1 {
			t.Errorf("재태깅 후 %q의 df가 부풀었다: %d", term, n)
		}
	}
}

// 발행자 분류가 태거까지 **실제로 도달하는지**.
//
// 이 브랜치의 헤드라인 기능인데 테스트가 양 끝만 잡고 있었다 — 추출(scraper)과 저장(store)과
// 스코어링(tagger)에는 있는데 **잇는 지점**에 없었다. 실측: `toTaggerContent`에서
// `Keywords: c.Keywords`를 지워도 17개 패키지 전부 통과했다. keywords는 API·화면에
// 노출되지 않아 증상도 없다.
//
// 제목·본문을 비워 다른 신호를 배제한다 — 그래야 태그가 붙었을 때 분류 덕분이라고
// 말할 수 있다.
func TestRun_keywordsReachTheTagger(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	id, _, _, err := st.SaveLink(ctx, store.SaveInput{
		URL:      "https://example.com/kw-only",
		Keywords: "쿠버네티스",
		// 본문은 태그 잡이 저장 시점에 걸리게 하는 최소치만. 사전 표면은 넣지 않는다.
		BodyText: "이 문장에는 사전에 있는 낱말이 들어 있지 않다.",
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := Run(ctx, st, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(res.Names, "kubernetes") {
		t.Errorf("분류가 태거에 도달하지 않았다 — 붙은 태그: %v", res.Names)
	}
}

// dictSurfaces는 사전이 실제로 쓰는 표면 목록. 테스트가 "무엇이 정당한 키인가"를
// 손으로 적으면 사전이 바뀔 때마다 같이 틀리므로 사전에게 묻는다.
func dictSurfaces(t *testing.T, st store.Store) []string {
	t.Helper()
	entries, err := st.LoadTagDict(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	te := make([]tagger.TagEntry, len(entries))
	for i, e := range entries {
		te[i] = tagger.TagEntry{ID: e.ID, Name: e.Name, Aliases: e.Aliases, Facet: e.Facet}
	}
	return tagger.BuildDictionary(te).Surfaces()
}
