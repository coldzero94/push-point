package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const poolHTML = `<html lang="ko"><head><meta property="og:title" content="풀 테스트"></head><body></body></html>`

// Pool은 Registry 라우팅을 그대로 통과시킨다 — instagram(어댑터)·일반 URL(fallback) 라우팅 확인.
func TestPoolRoutes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(poolHTML))
	}))
	defer srv.Close()

	p := New(WithHTTPClient(newRewriteClient(t, srv.URL, nil)), WithRateInterval(0))

	// 일반 URL → 기본 파서 fallback.
	m, err := p.Fetch(context.Background(), "https://example.com/article")
	if err != nil {
		t.Fatalf("fallback Fetch 에러: %v", err)
	}
	if m.Title != "풀 테스트" || m.ContentType != "article" {
		t.Errorf("fallback 결과 예상 밖: %+v", m)
	}

	// instagram → 어댑터(네트워크 없이 빈 메타·post).
	ig, err := p.Fetch(context.Background(), "https://www.instagram.com/p/x/")
	if err != nil {
		t.Fatalf("instagram Fetch 에러: %v", err)
	}
	if ig.ContentType != "post" || ig.Title != "" {
		t.Errorf("instagram 결과 예상 밖: %+v", ig)
	}
}

// singleflight: 동일 URL 동시 2 fetch가 실제 HTTP는 1회여야 한다.
func TestPoolSingleflightDedup(t *testing.T) {
	var hits int64
	release := make(chan struct{})
	started := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		select {
		case started <- struct{}{}: // 첫 in-flight 요청 신호
		default:
		}
		<-release // 두 번째 호출이 sf.Do에 합류할 시간을 벌기 위해 블록
		_, _ = w.Write([]byte(poolHTML))
	}))
	defer srv.Close()

	p := New(WithHTTPClient(newRewriteClient(t, srv.URL, nil)), WithRateInterval(0))
	const url = "https://example.com/same"

	var wg sync.WaitGroup
	results := make([]Metadata, 2)
	errs := make([]error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0], errs[0] = p.Fetch(context.Background(), url)
	}()

	<-started // g1이 핸들러(HTTP in-flight)에 진입했음을 확인

	wg.Add(1)
	go func() {
		defer wg.Done()
		results[1], errs[1] = p.Fetch(context.Background(), url)
	}()

	// g2가 sf.Do에 합류할 여유를 준 뒤 핸들러를 풀어준다.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("Fetch 에러: %v / %v", errs[0], errs[1])
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Errorf("HTTP 히트 = %d, want 1 (singleflight 중복 제거 실패)", got)
	}
	if results[0].Title != "풀 테스트" || results[1].Title != "풀 테스트" {
		t.Errorf("두 결과가 공유되지 않음: %+v / %+v", results[0], results[1])
	}
}

// F3 교차 취소: singleflight로 합쳐진 리더의 ctx 취소가 살아 있는 다른 호출자를
// 실패시키면 안 된다. 리더 A가 취소돼도, 자기 ctx가 멀쩡한 B는 재시도로 성공해야 한다.
func TestPoolSingleflightCrossCancel(t *testing.T) {
	var hits int64
	firstStarted := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&hits, 1) == 1 {
			// 첫(공유) 요청: 리더 A의 ctx가 취소돼 연결이 끊길 때까지 블록한다.
			close(firstStarted)
			<-r.Context().Done()
			return
		}
		// 재시도(B의 살아 있는 자기 ctx): 정상 응답.
		_, _ = w.Write([]byte(poolHTML))
	}))
	defer srv.Close()

	p := New(WithHTTPClient(newRewriteClient(t, srv.URL, nil)), WithRateInterval(0))
	const url = "https://example.com/cross"

	ctxA, cancelA := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var errA error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, errA = p.Fetch(ctxA, url)
	}()

	<-firstStarted // A가 리더로 HTTP in-flight (singleflight 키 점유)

	// B는 살아 있는 자기 ctx로 같은 URL을 요청 — 진행 중인 A의 실행에 합류한다.
	var mB Metadata
	var errB error
	wg.Add(1)
	go func() {
		defer wg.Done()
		mB, errB = p.Fetch(context.Background(), url)
	}()
	time.Sleep(50 * time.Millisecond) // B가 sf.Do에 합류할 여유

	cancelA() // A 취소 → 공유 실행이 context 에러로 죽는다
	wg.Wait()

	// A는 자기 취소이므로 실패가 정상. B는 살아 있는 ctx라 재시도로 성공해야 한다.
	if errA == nil {
		t.Error("A는 자기 ctx 취소로 에러를 기대")
	}
	if errB != nil {
		t.Fatalf("B가 A의 취소에 휩쓸려 실패 (교차 취소 미방어): %v", errB)
	}
	if mB.Title != "풀 테스트" {
		t.Fatalf("B 결과 예상 밖: %+v", mB)
	}
	if got := atomic.LoadInt64(&hits); got != 2 {
		t.Errorf("HTTP 히트 = %d, want 2 (A 공유 1 + B 재시도 1)", got)
	}
}

// 도메인 rate limit: 같은 도메인 다른 URL 2회는 최소 interval만큼 간격이 벌어진다.
func TestPoolDomainRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(poolHTML))
	}))
	defer srv.Close()

	const interval = 80 * time.Millisecond
	p := New(WithHTTPClient(newRewriteClient(t, srv.URL, nil)), WithRateInterval(interval))

	start := time.Now()
	if _, err := p.Fetch(context.Background(), "https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Fetch(context.Background(), "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < interval {
		t.Errorf("두 요청 소요 %v < interval %v (rate limit 미적용)", elapsed, interval)
	}
}

// rate limit 대기 중 ctx 취소는 에러로 전파된다.
func TestPoolRateLimitContextCancel(t *testing.T) {
	l := newDomainLimiter(time.Hour) // 사실상 무한 대기
	// 첫 슬롯 소비.
	if err := l.wait(context.Background(), "h"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if err := l.wait(ctx, "h"); err == nil {
		t.Error("취소됐는데 wait가 nil 반환")
	}
}
