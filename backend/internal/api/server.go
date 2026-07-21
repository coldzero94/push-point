package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coby/push-point/backend/internal/api/gen"
	"github.com/coby/push-point/backend/internal/web"
)

// NewRouter는 chi 라우터를 조립한다: 공통 미들웨어(요청 로그 · thumbs 캐시) →
// Bearer 인증 아래 gen 계약 라우트 + 계약 밖 /debug/pprof(루프백 전용) →
// 미매칭 경로는 NotFound 훅에서 계약 표면이면 JSON 404, 아니면 인증 밖 웹 SPA
// 앱 셸(프로덕션 embed).
func NewRouter(s *Server, apiKey string, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(requestLog(logger))
	r.Use(thumbsCacheControl)

	strict := gen.NewStrictHandlerWithOptions(s, nil, gen.StrictHTTPServerOptions{
		// 요청 디코드 실패(JSON 파싱 등) → 400 invalid_input
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeJSONError(w, http.StatusBadRequest, gen.ErrorErrorCodeInvalidInput, err.Error())
		},
		// 핸들러가 error를 반환한 경우 — badRequest 센티널은 400, 나머지는 500
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			var br badRequest
			if errors.As(err, &br) {
				writeJSONError(w, http.StatusBadRequest, gen.ErrorErrorCodeInvalidInput, br.msg)
				return
			}
			logger.Error("핸들러 내부 오류", "method", r.Method, "path", r.URL.Path, "err", err)
			writeJSONError(w, http.StatusInternalServerError, gen.ErrorErrorCodeInternal, "internal server error")
		},
	})

	// 인증 경계: 계약(openapi.yaml) 라우트 + 계약 밖 /debug/pprof를 Bearer 인증 아래
	// 묶는다. healthz·thumbs는 authExempt로 면제, pprof는 loopback 전용(더 강한 경계).
	// BearerAuth를 전역 r.Use가 아니라 이 그룹에만 거는 이유: 아래 웹 SPA 앱 셸은
	// 인증 밖이어야 한다 — 브라우저는 최초 내비게이션에 Bearer를 못 싣는다. 앱 셸은
	// 공개, API 키는 설정 화면에서 입력해 /api 호출에만 붙는다(frontend.md 규칙).
	r.Group(func(ar chi.Router) {
		ar.Use(BearerAuth(apiKey))

		// /debug/pprof — 계약(openapi.yaml) 밖. 인증 불요(authExempt), 루프백만 허용
		// (근거는 middleware.go pprofLoopbackOnly 주석 참조).
		ar.Route("/debug/pprof", func(pr chi.Router) {
			pr.Use(pprofLoopbackOnly)
			pr.HandleFunc("/", pprof.Index)
			pr.HandleFunc("/cmdline", pprof.Cmdline)
			pr.HandleFunc("/profile", pprof.Profile)
			pr.HandleFunc("/symbol", pprof.Symbol)
			pr.HandleFunc("/trace", pprof.Trace)
			pr.HandleFunc("/{name}", pprof.Index) // heap, goroutine 등 이름 있는 프로파일
		})

		// gen 계약 라우트를 인증 그룹의 라우팅 트리에 등록한다. 반환값(=ar)은 버리고
		// 최상위 r을 서빙에 쓴다 — 라우트는 공유 트리에 얹혀 동일하게 매칭된다.
		gen.HandlerWithOptions(strict, gen.ChiServerOptions{
			BaseRouter: ar,
			// 파라미터 바인딩 실패(limit에 문자열 등) → 400 invalid_input
			ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
				writeJSONError(w, http.StatusBadRequest, gen.ErrorErrorCodeInvalidInput, err.Error())
			},
		})
	})

	// 미매칭 경로 처리 — 웹 SPA(프로덕션 embed)를 "/*" 라우트로 얹지 않고 chi의
	// NotFound/MethodNotAllowed 훅에 건다. "/*" 와일드카드는 라우트로 등록되는 순간
	// 메서드 불일치(PUT /api/v1/links)까지 매칭해 405를 삼키고 앱 셸 HTML 200을
	// 돌려줬다 — 계약상 모든 에러는 JSON {error:{code,message}}여야 한다.
	//
	// 훅에 걸면 계약 라우트가 먼저 매칭되고, 남은 것만 여기로 온다:
	//   - 계약 표면(/api·/thumbs·/healthz) → JSON 404 not_found
	//   - 그 외(클라이언트 라우트 /links/123 등) → 앱 셸
	// 빌드 태그 embed_frontend가 없으면 web.Handler()가 nil → 전부 JSON 404
	// (백엔드 전용 빌드·CI는 태그 없이 그린 유지).
	spa := web.Handler()
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if spa == nil || isContractPath(req.URL.Path) {
			writeJSONError(w, http.StatusNotFound, gen.ErrorErrorCodeNotFound, "no such endpoint")
			return
		}
		spa.ServeHTTP(w, req)
	})
	// 메서드 불일치는 정의상 계약 표면(또는 /debug/pprof)에서만 발생한다 — 앱 셸이
	// 아니라 JSON. 계약 error enum에 method_not_allowed가 없어(openapi.yaml components
	// Error) "그 (메서드, 경로) 조합은 계약에 없다"는 뜻의 404 not_found로 통일한다.
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		writeJSONError(w, http.StatusNotFound, gen.ErrorErrorCodeNotFound, "no such endpoint")
	})
	return r
}

// contractRoots는 앱 셸로 폴백하면 안 되는 서버 표면. 계약(openapi.yaml)의 /api·
// /thumbs·/healthz — 이 아래 미등록 경로는 HTML이 아니라 JSON 에러를 받아야
// 클라이언트가 "성공했는데 파싱이 이상함"으로 오진하지 않는다.
var contractRoots = [...]string{"/api", "/thumbs", "/healthz"}

// isContractPath는 경로가 계약 표면(루트 자신 또는 그 하위)인지 본다.
func isContractPath(p string) bool {
	for _, root := range contractRoots {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}

// requestLog는 경량 slog 접근 로그. 핫패스(p99 < 50ms) 부담을 피해 debug 레벨.
func requestLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !logger.Enabled(r.Context(), slog.LevelDebug) {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.Debug("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"dur_ms", float64(time.Since(start).Microseconds())/1000.0,
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
