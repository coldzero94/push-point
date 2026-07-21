package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/coby/push-point/backend/internal/api/gen"
)

// 계약 표면(/api·/thumbs·/healthz)의 미등록 경로·메서드 불일치는 항상 JSON
// {error:{code,message}}여야 한다. 예전에는 웹 SPA가 "/*" 라우트로 얹혀 있어
// 이 요청들을 앱 셸 HTML 200으로 삼켰다 — 클라이언트는 성공으로 오진했다.
// 이 파일은 태그 없이(=SPA 미마운트) 도는 계약 보장이고, embed 빌드에서 SPA가
// 붙어도 같은 결과라는 것은 router_embed_test.go가 확인한다.
func TestContractSurfaceErrorsAreJSON(t *testing.T) {
	_, h, _ := newTestRouter(t)

	cases := []struct {
		name   string
		method string
		target string
		key    string
	}{
		{"미등록 API 경로 (토큰 없음)", http.MethodGet, "/api/v1/nope", ""},
		{"미등록 API 경로 (토큰 있음)", http.MethodGet, "/api/v1/nope", testKey},
		{"등록 경로 + 메서드 불일치", http.MethodPut, "/api/v1/links", testKey},
		{"healthz 메서드 불일치", http.MethodPost, "/healthz", ""},
		{"thumbs 루트 (파일 경로 아님)", http.MethodGet, "/thumbs", ""},
		{"thumbs 하위 미등록", http.MethodGet, "/thumbs/nope", ""},
		{"API 루트", http.MethodGet, "/api", testKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.target, "", tc.key)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body=%q)", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", ct)
			}
			var body gen.Error
			decodeJSON(t, rec, &body)
			if body.Error.Code != gen.ErrorErrorCodeNotFound {
				t.Fatalf("error.code = %q, want %q", body.Error.Code, gen.ErrorErrorCodeNotFound)
			}
			if body.Error.Message == "" {
				t.Fatal("error.message가 비어 있다")
			}
		})
	}
}

func TestIsContractPath(t *testing.T) {
	cases := map[string]bool{
		"/api":              true,
		"/api/":             true,
		"/api/v1/links":     true,
		"/thumbs":           true,
		"/thumbs/a/b.jpg":   true,
		"/healthz":          true,
		"/healthz/sub":      true,
		"/":                 false,
		"/links/123":        false,
		"/apidocs":          false, // 접두어만 같은 클라이언트 라우트는 셸이어야 한다
		"/healthzz":         false,
		"/assets/index.js":  false,
		"/thumbsnail/x.png": false,
	}
	for p, want := range cases {
		if got := isContractPath(p); got != want {
			t.Errorf("isContractPath(%q) = %v, want %v", p, got, want)
		}
	}
}
