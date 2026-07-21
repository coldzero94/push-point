//go:build embed_frontend

package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

// 이 파일은 embed_frontend 태그에서만 컴파일된다 — spa.go와 같은 조건이다.
// 실행: just web-test (= just web-build 후 go test -tags embed_frontend ./internal/web/...)

func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// 클라이언트 라우트(TanStack Router)는 하드 리로드·딥링크에서도 앱 셸을 받아야 한다.
func TestClientRouteServesShell(t *testing.T) {
	for _, target := range []string{"/", "/index.html", "/links/123", "/settings"} {
		t.Run(target, func(t *testing.T) {
			rec := get(t, target)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Fatalf("Content-Type = %q, want text/html", ct)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
				t.Fatalf("Cache-Control = %q, want no-cache (셸은 항상 재검증)", cc)
			}
			if !strings.Contains(rec.Body.String(), "<div id=\"root\"") {
				t.Fatalf("셸 본문에 마운트 지점이 없다: %.200q", rec.Body.String())
			}
		})
	}
}

// Vite 콘텐츠 해시 번들은 올바른 MIME + immutable 장기 캐시로 나가야 한다.
func TestAssetHeaders(t *testing.T) {
	entries, err := fs.Glob(dist, "dist/assets/*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	wantType := map[string]string{".js": "javascript", ".css": "text/css"}
	checked := 0
	for _, e := range entries {
		ext := path.Ext(e)
		want, ok := wantType[ext]
		if !ok {
			continue
		}
		target := strings.TrimPrefix(e, "dist")
		rec := get(t, target)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", target, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, want) {
			t.Errorf("%s: Content-Type = %q, want ~%q", target, ct, want)
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Errorf("%s: Cache-Control = %q, want immutable", target, cc)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("dist/assets에 검사할 js/css 번들이 없다 — web-build가 제대로 돌았는지 확인")
	}
}

// 확장자가 있는데 없는 파일은 셸(200 text/html)이 아니라 404다. 셸로 덮으면
// base 경로 오설정·깨진 번들 참조가 조용히 200으로 성공처럼 보인다.
func TestMissingAssetIsNotFound(t *testing.T) {
	for _, target := range []string{"/nope.js", "/assets/does-not-exist.abc123.js", "/favicon-missing.svg"} {
		t.Run(target, func(t *testing.T) {
			rec := get(t, target)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404 (body=%.120q)", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "<div id=\"root\"") {
				t.Fatal("셸을 반환했다 — 확장자 있는 미존재 경로는 404여야 한다")
			}
		})
	}
}

// 경로 탈출은 임베드 FS 밖 파일을 절대 노출하면 안 된다. path.Clean이 ".."를
// 접어 없애므로 결과는 셸(확장자 없음) 또는 404이며, 어느 쪽도 파일 내용이 아니다.
func TestTraversalExposesNothing(t *testing.T) {
	targets := []string{
		"/../../etc/passwd",
		"/..%2f..%2fetc%2fpasswd",
		"/assets/../../../../etc/passwd",
		"/../spa.go",
		"/%2e%2e/%2e%2e/etc/hosts",
	}
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			rec := get(t, target)
			if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 200(셸) 또는 404", rec.Code)
			}
			body := rec.Body.String()
			for _, leak := range []string{"root:", "package web", "localhost"} {
				if strings.Contains(body, leak) {
					t.Fatalf("본문에 %q 유출 (status=%d): %.200q", leak, rec.Code, body)
				}
			}
			if rec.Code == http.StatusOK && !strings.Contains(body, "<div id=\"root\"") {
				t.Fatalf("200인데 셸이 아니다: %.200q", body)
			}
		})
	}
}
