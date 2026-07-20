package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coby/push-point/backend/internal/api/gen"
)

// NewRouter는 chi 라우터를 조립한다: Bearer 인증 → 요청 로그 → gen 계약 라우트
// + 계약 밖 /debug/pprof (루프백 전용).
func NewRouter(s *Server, apiKey string, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(requestLog(logger))
	r.Use(BearerAuth(apiKey))
	r.Use(thumbsCacheControl)

	// /debug/pprof — 계약(openapi.yaml) 밖. 인증 불요, 루프백 원격주소만 허용
	// (근거는 middleware.go pprofLoopbackOnly 주석 참조).
	r.Route("/debug/pprof", func(pr chi.Router) {
		pr.Use(pprofLoopbackOnly)
		pr.HandleFunc("/", pprof.Index)
		pr.HandleFunc("/cmdline", pprof.Cmdline)
		pr.HandleFunc("/profile", pprof.Profile)
		pr.HandleFunc("/symbol", pprof.Symbol)
		pr.HandleFunc("/trace", pprof.Trace)
		pr.HandleFunc("/{name}", pprof.Index) // heap, goroutine 등 이름 있는 프로파일
	})

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
	return gen.HandlerWithOptions(strict, gen.ChiServerOptions{
		BaseRouter: r,
		// 파라미터 바인딩 실패(limit에 문자열 등) → 400 invalid_input
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeJSONError(w, http.StatusBadRequest, gen.ErrorErrorCodeInvalidInput, err.Error())
		},
	})
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
