package ppcore

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const testKey = "test-key-abc"

func req(t *testing.T, addr, method, path, key, body string) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	r, err := http.NewRequest(method, "http://"+addr+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("%s %s 요청 실패: %v", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck // 테스트
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

// 자립 모드의 핵심 주장을 검증한다: 폰 안에서 띄운 인프로세스 서버가 서버 모드와
// **같은 계약**(api/openapi.yaml)을 서빙한다. 여기가 깨지면 Swift 앱이 두 모드에
// 코드 한 벌로 대응한다는 전제가 무너진다.
func TestInProcessServerServesContract(t *testing.T) {
	addr, err := Start(t.TempDir(), testKey)
	if err != nil {
		t.Fatalf("Start 실패: %v", err)
	}
	defer Stop() //nolint:errcheck // 테스트 정리

	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Errorf("루프백에 바인딩돼야 한다: %q", addr)
	}

	// 저장 — 클라이언트 캡처 필드까지 계약 그대로 넣는다.
	code, body := req(t, addr, http.MethodPost, "/api/v1/links", testKey,
		`{"url":"https://example.com/ios","title":"제목","body_text":"본문 텍스트"}`)
	if code != http.StatusCreated {
		t.Fatalf("저장이 201이어야 한다: %d %s", code, body)
	}
	var saved struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &saved); err != nil || saved.ID == 0 {
		t.Fatalf("저장 응답에 id가 있어야 한다 (%v): %s", err, body)
	}

	// 목록 — 방금 저장한 것이 보여야 한다.
	code, body = req(t, addr, http.MethodGet, "/api/v1/links", testKey, "")
	if code != http.StatusOK {
		t.Fatalf("목록이 200이어야 한다: %d %s", code, body)
	}
	if !strings.Contains(body, "https://example.com/ios") {
		t.Errorf("저장한 링크가 목록에 없다: %s", body)
	}
}

// 루프백은 iOS 앱 샌드박스를 넘어 공유되므로 인증이 실제로 걸려 있어야 한다 —
// 같은 기기의 다른 앱이 포트를 찾아내도 키 없이는 아무것도 못 해야 한다.
func TestInProcessServerRequiresAuth(t *testing.T) {
	addr, err := Start(t.TempDir(), testKey)
	if err != nil {
		t.Fatalf("Start 실패: %v", err)
	}
	defer Stop() //nolint:errcheck // 테스트 정리

	for _, c := range []struct{ name, key string }{
		{"키 없음", ""},
		{"틀린 키", "wrong-key"},
	} {
		if code, _ := req(t, addr, http.MethodGet, "/api/v1/links", c.key, ""); code != http.StatusUnauthorized {
			t.Errorf("%s: 401이어야 한다, got %d", c.name, code)
		}
	}
	// healthz는 계약상 인증 면제 — 앱이 서버 준비 여부를 키 없이 확인할 수 있어야 한다.
	if code, _ := req(t, addr, http.MethodGet, "/healthz", "", ""); code != http.StatusOK {
		t.Errorf("healthz는 인증 없이 200이어야 한다, got %d", code)
	}
}

// 앱은 포그라운드 복귀마다 Start를 부를 수 있다 — 두 번째 호출이 서버를 중복으로
// 띄우거나 실패하면 안 된다.
func TestStartIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	first, err := Start(dir, testKey)
	if err != nil {
		t.Fatalf("첫 Start 실패: %v", err)
	}
	defer Stop() //nolint:errcheck // 테스트 정리

	second, err := Start(dir, testKey)
	if err != nil {
		t.Fatalf("두 번째 Start 실패: %v", err)
	}
	if first != second {
		t.Errorf("같은 주소여야 한다: %q != %q", first, second)
	}
	if Addr() != first {
		t.Errorf("Addr()가 실행 중 주소를 돌려줘야 한다: %q != %q", Addr(), first)
	}
}

// 다른 인자로 재진입하면 조용히 무시하지 말고 실패해야 한다.
//
// 무시하면 이렇게 된다: 패키지 문서가 "실행마다 새 난수 키를 넘기라"고 하고 "재진입은
// 안전하다"고도 하는데, 둘을 그대로 따르면 앱은 새 키로 요청을 보내고 서버는 옛 키로
// 검사해 **모든 요청이 401**이 된다. 주소는 정상적으로 돌아오므로 원인이 드러나지 않는다.
func TestStartRejectsDifferentArgs(t *testing.T) {
	dir := t.TempDir()
	addr, err := Start(dir, testKey)
	if err != nil {
		t.Fatalf("첫 Start 실패: %v", err)
	}
	defer Stop() //nolint:errcheck // 테스트 정리

	if _, err := Start(dir, "another-key"); err == nil {
		t.Error("다른 apiKey로 Start하면 에러여야 한다 — 무시하면 401만 나고 원인이 안 보인다")
	}
	if _, err := Start(t.TempDir(), testKey); err == nil {
		t.Error("다른 dataDir로 Start하면 에러여야 한다 — 확장과 본체가 다른 DB를 보게 된다")
	}
	// 실패해도 실행 중이던 서버는 그대로여야 한다.
	if got := Addr(); got != addr {
		t.Errorf("거부된 Start가 실행 중 인스턴스를 건드렸다: %q → %q", addr, got)
	}
	if code, _ := req(t, addr, http.MethodGet, "/api/v1/links", testKey, ""); code != http.StatusOK {
		t.Errorf("원래 키로는 계속 200이어야 한다: %d", code)
	}
}

func TestStopWhenNotRunning(t *testing.T) {
	_ = Stop() // 앞선 테스트의 인스턴스가 남아 있으면 정리
	if err := Stop(); err == nil {
		t.Error("실행 중이 아닐 때 Stop은 에러여야 한다")
	}
	if Addr() != "" {
		t.Errorf("실행 중이 아니면 Addr()는 빈 문자열: %q", Addr())
	}
}
