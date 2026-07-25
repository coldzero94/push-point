package tagjob

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/store"
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
