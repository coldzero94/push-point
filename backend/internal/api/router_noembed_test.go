//go:build !embed_frontend

package api

import (
	"net/http"
	"strings"
	"testing"
)

// SPA 미마운트(기본 빌드 = web.Handler()가 nil)에서는 클라이언트 라우트도 갈 곳이
// 없다 — chi 기본 평문 404가 아니라 계약 JSON 404로 답한다. embed 빌드에서 같은
// 경로가 앱 셸을 받는 것은 router_embed_test.go가 확인한다.
func TestUnmatchedPathIsJSONWithoutSPA(t *testing.T) {
	_, h, _ := newTestRouter(t)

	rec := do(t, h, http.MethodGet, "/links/123", "", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
}
