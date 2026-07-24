package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/coby/push-point/backend/internal/api/gen"
)

// bufLogger는 지정 레벨의 slog를 버퍼에 쓴다 — 로그 내용을 검증하기 위한 테스트용.
func bufLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level})), &buf
}

func serveReq(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }
}

// 5xx는 운영 레벨(info)에서도 Warn으로 올라와 보여야 한다.
func TestRequestLogEscalatesOn5xx(t *testing.T) {
	logger, buf := bufLogger(slog.LevelInfo)
	serveReq(requestLog(logger)(status(http.StatusInternalServerError)), http.MethodGet, "/api/v1/links")
	out := buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "status=500") {
		t.Fatalf("5xx는 Warn+status=500으로 로그되어야: %q", out)
	}
}

// 정상 200은 Debug라 info에선 조용하고, debug에서만 보이며 결코 Warn이 아니다.
func TestRequestLogNormalRequestIsQuietAtInfo(t *testing.T) {
	logger, buf := bufLogger(slog.LevelInfo)
	serveReq(requestLog(logger)(status(http.StatusOK)), http.MethodGet, "/healthz")
	if buf.Len() != 0 {
		t.Fatalf("info에서 정상 200은 아무 로그도 없어야: %q", buf.String())
	}

	logger, buf = bufLogger(slog.LevelDebug)
	serveReq(requestLog(logger)(status(http.StatusOK)), http.MethodGet, "/healthz")
	out := buf.String()
	if !strings.Contains(out, "level=DEBUG") || !strings.Contains(out, "status=200") {
		t.Fatalf("debug에서 정상은 Debug+status=200: %q", out)
	}
	if strings.Contains(out, "level=WARN") {
		t.Fatalf("정상 200이 Warn이면 안 됨: %q", out)
	}
}

// RequestID 미들웨어와 함께 걸면 접근 로그에 비어있지 않은 request_id가 붙는다.
func TestRequestLogIncludesRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := middleware.RequestID(requestLog(logger)(status(http.StatusOK)))
	serveReq(h, http.MethodGet, "/healthz")

	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("로그 JSON 파싱 실패: %v (%q)", err, buf.String())
	}
	if id, _ := m["request_id"].(string); id == "" {
		t.Fatalf("request_id가 비어있음: %q", buf.String())
	}
}

// 핸들러 panic은 500 JSON 계약 에러로 복구되고 Error+스택으로 로그된다(크래시 없음).
func TestRecoverPanicReturns500AndLogsError(t *testing.T) {
	logger, buf := bufLogger(slog.LevelInfo)
	h := recoverPanic(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := serveReq(h, http.MethodGet, "/api/v1/links")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic 후 500이어야, got %d", rec.Code)
	}
	if got := errCode(t, rec); got != string(gen.ErrorErrorCodeInternal) {
		t.Errorf("에러 코드 internal이어야, got %q", got)
	}
	out := buf.String()
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "boom") || !strings.Contains(out, "stack=") {
		t.Fatalf("panic은 Error+메시지+스택으로 로그되어야: %q", out)
	}
}

// http.ErrAbortHandler은 의도적 중단이므로 로그 없이 다시 던져야 한다(net/http 관례).
func TestRecoverPanicRethrowsAbortHandler(t *testing.T) {
	logger, buf := bufLogger(slog.LevelError)
	h := recoverPanic(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Fatalf("ErrAbortHandler는 다시 던져져야, got %v", rec)
		}
		if buf.Len() != 0 {
			t.Errorf("ErrAbortHandler는 로그를 남기지 않아야: %q", buf.String())
		}
	}()
	serveReq(h, http.MethodGet, "/api/v1/links")
	t.Fatal("panic이 전파됐어야 하는데 도달함")
}
