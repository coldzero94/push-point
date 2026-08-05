package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/coby/push-point/backend/internal/api/gen"
)

// writeJSONError는 공통 에러 형식 {error:{code,message}}를 기록한다.
func writeJSONError(w http.ResponseWriter, status int, code gen.ErrorErrorCode, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// 인코딩 실패는 이미 헤더가 나간 뒤라 복구 불가 — 무시해도 안전 (고정 구조체).
	_ = json.NewEncoder(w).Encode(apiErr(code, msg))
}

// authExempt는 인증 면제 경로 여부. 계약상 면제는 GET /healthz, GET /thumbs/* 2개뿐
// (docs/v2/ko/06 §1). /debug/pprof는 계약 밖 — 인증 대신 pprofLoopbackOnly가 루프백을
// 강제하므로 여기서도 통과시킨다.
func authExempt(r *http.Request) bool {
	if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
		return true
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/thumbs/") {
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/debug/pprof") {
		return true
	}
	return false
}

// BearerAuth는 정적 API 키 1개에 대한 Bearer 인증 미들웨어.
// 키를 SHA-256으로 접어 crypto/subtle 상수시간 비교 — 해시 길이가 같아
// 토큰 길이가 달라도 비교 시간이 일정하다 (타이밍 부채널 제거).
func BearerAuth(apiKey string) func(http.Handler) http.Handler {
	keyHash := sha256.Sum256([]byte(apiKey))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authExempt(r) {
				next.ServeHTTP(w, r)
				return
			}
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				writeJSONError(w, http.StatusUnauthorized, gen.ErrorErrorCodeUnauthorized, "missing bearer token")
				return
			}
			tokenHash := sha256.Sum256([]byte(token))
			if subtle.ConstantTimeCompare(tokenHash[:], keyHash[:]) != 1 {
				writeJSONError(w, http.StatusUnauthorized, gen.ErrorErrorCodeUnauthorized, "invalid api key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// pprofLoopbackOnly는 /debug/pprof 접근을 루프백 원격주소로 제한한다.
// 근거: pprof는 API 계약(openapi.yaml) 밖의 진단 도구다. Bearer 인증을 걸면
// `go tool pprof`가 헤더를 못 실어 마찰이 생기므로 인증 대신 "서버 로컬에서만"
// 이라는 더 강한 경계를 쓴다. Tailscale 등 외부 주소에서는 존재 자체를 숨기는 404.
func pprofLoopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// thumbsCacheControl은 /thumbs/* 성공 응답에 캐시 헤더를 붙인다.
// 썸네일 경로가 url_hash 기반이라 내용이 불변 — immutable 장기 캐시가 안전하다.
// 404까지 캐시되지 않도록 200일 때만 헤더를 세팅한다.
func thumbsCacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/thumbs/") {
			next.ServeHTTP(&cacheControlWriter{ResponseWriter: w}, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type cacheControlWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *cacheControlWriter) WriteHeader(code int) {
	if !w.wrote {
		w.wrote = true
		if code == http.StatusOK {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *cacheControlWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// maxRequestBody는 요청 바디를 n바이트로 제한한다. 클라이언트 캡처가 본문을 실어 오면서
// 바디가 커졌으므로(32KB 캡 + 여유), 무제한 읽기로 메모리를 태우지 못하게 막는다.
// 인증 그룹 **밖**에 둬서 미인증 요청도 상한을 받는다.
func maxRequestBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}
