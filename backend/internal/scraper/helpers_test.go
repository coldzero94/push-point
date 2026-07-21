package scraper

import (
	"net/http"
	"net/url"
	"testing"
)

// mustURL은 rawURL을 파싱한다 (테스트 편의).
func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("URL 파싱 실패 %q: %v", raw, err)
	}
	return u
}

// rewriteTransport는 모든 요청을 target(테스트 서버)으로 돌려보내 실제 외부 호스트
// (youtube.com 등)를 부르지 않고도 어댑터를 결정적으로 테스트하게 한다. 경로·쿼리는 보존된다.
// wantHost가 non-nil이면 재작성 직전의 요청 호스트를 기록한다(네이버 재작성 검증용).
type rewriteTransport struct {
	target   *url.URL
	seenHost *string
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.seenHost != nil {
		*t.seenHost = req.URL.Host
	}
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	req.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newRewriteClient는 모든 요청을 base로 돌리는 http.Client를 만든다.
func newRewriteClient(t *testing.T, base string, seenHost *string) *http.Client {
	t.Helper()
	return &http.Client{Transport: rewriteTransport{target: mustURL(t, base), seenHost: seenHost}}
}
