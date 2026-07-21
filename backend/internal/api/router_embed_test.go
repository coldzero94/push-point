//go:build embed_frontend

package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/coby/push-point/backend/internal/api/gen"
)

// SPA가 실제로 마운트된 릴리스 구성(embed_frontend)에서의 라우팅 경계.
// 실행: just web-test. 태그 없는 router_test.go는 SPA 미마운트 쪽을 본다.

// 계약 표면은 SPA가 붙어도 앱 셸을 반환하지 않는다 — 이게 리뷰 지적 #1의 회귀 방지선.
func TestContractSurfaceNotSwallowedBySPA(t *testing.T) {
	_, h, _ := newTestRouter(t)

	cases := []struct {
		method string
		target string
		key    string
	}{
		{http.MethodGet, "/api/v1/nope", ""},
		{http.MethodPut, "/api/v1/links", testKey},
		{http.MethodPost, "/healthz", ""},
		{http.MethodGet, "/thumbs", ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.target, "", tc.key)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body=%.120q)", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json (앱 셸 HTML 금지)", ct)
			}
			var body gen.Error
			decodeJSON(t, rec, &body)
			if body.Error.Code != gen.ErrorErrorCodeNotFound {
				t.Fatalf("error.code = %q, want %q", body.Error.Code, gen.ErrorErrorCodeNotFound)
			}
		})
	}
}

// 계약 밖 경로는 인증 없이 앱 셸을 받는다 — 브라우저는 최초 내비게이션에 Bearer를
// 실을 수 없으므로 셸은 공개여야 한다(frontend.md).
func TestClientRoutesServeShellWithoutAuth(t *testing.T) {
	_, h, _ := newTestRouter(t)

	for _, target := range []string{"/", "/links/123", "/settings"} {
		t.Run(target, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, target, "", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Fatalf("Content-Type = %q, want text/html", ct)
			}
		})
	}
}

// 계약 라우트는 SPA 마운트 뒤에도 정상 동작해야 한다(회귀 방지 — catch-all 제거로
// 라우팅이 바뀌었다). 인증 경계도 그대로.
func TestContractRoutesStillWorkWithSPA(t *testing.T) {
	_, h, _ := newTestRouter(t)

	if rec := do(t, h, http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/links", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/links (토큰 없음) = %d, want 401", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/links", "", testKey); rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/links (토큰 있음) = %d, want 200", rec.Code)
	}
}
