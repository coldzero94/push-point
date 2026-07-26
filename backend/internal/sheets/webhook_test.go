package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 웹훅은 **기본 전송 경로**인데 커버리지가 0%였다. 폐기 방향인 서비스 계정 경로는
// 80%대인데 정작 CLI와 웹 버튼이 타는 쪽이 한 번도 실행되지 않았다.
//
// 여기서 지켜야 할 진짜 계약은 Go가 보내는 JSON 키와 사용자 시트에 붙어 있는
// 스크립트가 읽는 프로퍼티가 같은지다. 어느 한쪽만 바꿔도 컴파일되고 테스트가 통과하며,
// **이미 배포된 사용자 스크립트와 어긋나 모든 동기화가 깨진다.**

// newTestWebhook은 https 강제를 우회한다 — 같은 패키지라 구조체를 직접 만든다.
func newTestWebhook(url, token string) *Webhook {
	return &Webhook{http: http.DefaultClient, url: url, token: token}
}

// 스크립트에 토큰이 실제로 박히는지. 안 박히면 `__TOKEN__` 리터럴이 그대로 배포되어
// 모든 요청이 토큰 불일치로 거부된다.
func TestAppsScript_embedsToken(t *testing.T) {
	got := AppsScript("abc123")
	if strings.Contains(got, "__TOKEN__") {
		t.Error("치환되지 않은 자리표시자가 남았다 — 배포하면 모든 요청이 거부된다")
	}
	if !strings.Contains(got, "const TOKEN = 'abc123';") {
		t.Error("토큰이 스크립트에 없다")
	}
	// 스크립트가 자기 시트만 만지는지 — 권한의 경계다.
	if !strings.Contains(got, "getActiveSpreadsheet") {
		t.Error("스크립트가 활성 시트가 아닌 것을 열 수 있다")
	}
	if strings.Contains(got, "openById") || strings.Contains(got, "DriveApp") {
		t.Error("스크립트가 다른 파일에 접근할 수단을 갖고 있다")
	}
}

// Go가 보내는 키와 스크립트가 읽는 키가 같아야 한다. 이 단언이 그 둘을 묶는다.
func TestReplace_payloadMatchesScript(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		fmt.Fprint(w, `{"ok":true,"rows":2}`)
	}))
	defer srv.Close()

	wh := newTestWebhook(srv.URL, "tok")
	rows := [][]any{{"id", "url", "title"}, {1, "https://a", "제목"}}
	if err := wh.Replace(context.Background(), "links", rows); err != nil {
		t.Fatal(err)
	}

	script := AppsScript("tok")
	for _, key := range []string{"token", "action", "tab", "values", "width"} {
		if _, ok := got[key]; !ok {
			t.Errorf("페이로드에 %q가 없다", key)
		}
		if !strings.Contains(script, "body."+key) {
			t.Errorf("스크립트가 body.%s를 읽지 않는다 — 키가 갈라졌다", key)
		}
	}
	if got["action"] != "replace" {
		t.Errorf("action: %v", got["action"])
	}
	// width는 우리가 소유한 열 수다. 틀리면 스크립트가 지우는 범위가 어긋나
	// 사용자 열을 지우거나 옛 데이터를 남긴다.
	if w, _ := got["width"].(float64); int(w) != 3 {
		t.Errorf("width가 헤더 폭과 다르다: %v", got["width"])
	}
}

// 토큰 불일치는 흔한 상태다(스크립트를 다시 붙여넣으면 토큰이 바뀐다).
// 진단이 없으면 "스크립트 오류: token mismatch"만 보이고 무엇을 할지 알 수 없다.
func TestCall_tokenMismatchExplainsItself(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":false,"error":"token mismatch"}`)
	}))
	defer srv.Close()

	_, _, err := newTestWebhook(srv.URL, "wrong").Ping(context.Background())
	if err == nil {
		t.Fatal("토큰 불일치인데 성공했다")
	}
	if !strings.Contains(err.Error(), "sheets-setup") {
		t.Errorf("다시 붙여넣으라는 안내가 없다: %v", err)
	}
}

// 배포 설정에서 "액세스 권한"을 모든 사용자로 안 두면 구글 로그인 HTML이 온다.
// 가장 흔한 설정 실수이고, 그때 "JSON 파싱 실패"만 보이면 원인을 짐작할 수 없다.
func TestCall_loginPageExplainsDeploySetting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>로그인</title></head><body>accounts.google.com</body></html>`)
	}))
	defer srv.Close()

	_, _, err := newTestWebhook(srv.URL, "t").Ping(context.Background())
	if err == nil {
		t.Fatal("로그인 페이지를 성공으로 읽었다")
	}
	if !strings.Contains(err.Error(), "모든 사용자") {
		t.Errorf("배포 설정 안내가 없다: %v", err)
	}
}

// 없는 탭을 읽으면 빈 결과여야 한다 — 첫 동기화 전에는 정상 상태다.
func TestRead_returnsScriptValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true,"values":[["id","url"],["1","https://a"]]}`)
	}))
	defer srv.Close()

	rows, err := newTestWebhook(srv.URL, "t").Read(context.Background(), "links")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1][1] != "https://a" {
		t.Errorf("값이 어긋남: %v", rows)
	}
}

// https가 아닌 배포 URL은 거부한다 — 토큰이 평문으로 나간다.
func TestNewWebhook_requiresHTTPS(t *testing.T) {
	if _, err := NewWebhook("http://script.google.com/x", "t"); err == nil {
		t.Error("http URL을 받아들였다 — 토큰이 평문으로 나간다")
	}
}
