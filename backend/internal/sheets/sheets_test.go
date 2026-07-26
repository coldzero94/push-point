package sheets

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"crypto"
)

// testKey는 서비스 계정 JSON 한 벌을 만든다. 실제 구글 키와 **같은 모양**(PKCS#8 PEM)이라
// 파싱 경로가 진짜와 같다 — 테스트용으로 모양을 단순화하면 정작 실키에서 깨진다.
func testKey(t *testing.T) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	acct := map[string]string{
		"type":         "service_account",
		"client_email": "pushpoint@example.iam.gserviceaccount.com",
		"private_key":  string(pemBytes),
	}
	b, err := json.Marshal(acct)
	if err != nil {
		t.Fatal(err)
	}
	return b, key
}

// JWT가 실제로 검증 가능한 서명을 갖는지. 이게 이 패키지에서 유일하게 조용히 틀릴 수 있는
// 부분이다 — 서명이 틀려도 코드는 돌고, 구글이 400을 줄 뿐이라 원인이 안 보인다.
func TestSignedJWT_verifies(t *testing.T) {
	keyJSON, key := testKey(t)
	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := c.signedJWT()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT는 세 조각이어야 한다: %d", len(parts))
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("서명이 base64url이 아니다: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("서명 검증 실패 — 구글이 400으로 거부할 값이다: %v", err)
	}

	// 클레임이 구글 요구사항을 만족하는지. aud/scope가 틀리면 토큰은 나오는데
	// 이후 요청이 403이 되어 원인이 두 단계 떨어진 곳에서 나타난다.
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["aud"] != tokenURL {
		t.Errorf("aud가 토큰 엔드포인트여야 한다: %v", claims["aud"])
	}
	if claims["scope"] != scope {
		t.Errorf("scope 불일치: %v", claims["scope"])
	}
	if claims["iss"] != "pushpoint@example.iam.gserviceaccount.com" {
		t.Errorf("iss가 서비스 계정 이메일이어야 한다: %v", claims["iss"])
	}
	if exp, iat := claims["exp"].(float64), claims["iat"].(float64); exp <= iat {
		t.Errorf("exp가 iat보다 뒤여야 한다: %v %v", iat, exp)
	}
}

// OAuth 클라이언트 JSON을 잘못 넣는 실수가 흔하다. 그때 "PEM 아님"으로 죽으면
// 원인을 짐작할 수 없으므로 진단이 먼저 나와야 한다.
func TestNew_rejectsNonServiceAccount(t *testing.T) {
	bad, _ := json.Marshal(map[string]any{"installed": map[string]string{"client_id": "x"}})
	_, err := New(bad)
	if err == nil {
		t.Fatal("서비스 계정이 아닌 키를 받아들였다")
	}
	if !strings.Contains(err.Error(), "서비스 계정 키가 아닙니다") {
		t.Errorf("진단이 불친절하다: %v", err)
	}
}

// Replace는 **비우고 쓴다**. clear를 빠뜨리면 이전 동기화의 꼬리가 남아, 지운 링크가
// 시트에 영원히 살아 있게 된다 — 그리고 그 사실은 시트를 끝까지 스크롤해야 보인다.
func TestReplace_clearsBeforeWriting(t *testing.T) {
	keyJSON, _ := testKey(t)
	var calls []string
	var wrote map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut {
			_ = json.NewDecoder(r.Body).Decode(&wrote)
		}
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.base = srv.URL

	rows := [][]any{{"id", "url"}, {1, "https://a.example"}}
	if err := c.Replace(context.Background(), "SHEET", "links", rows); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("clear + update 두 번이어야 한다: %v", calls)
	}
	if !strings.Contains(calls[0], "clear") {
		t.Errorf("첫 호출이 clear가 아니다 — 지운 링크가 시트에 남는다: %v", calls)
	}
	if !strings.HasPrefix(calls[1], "PUT") {
		t.Errorf("두 번째가 쓰기가 아니다: %v", calls)
	}
	if got := wrote["values"]; got == nil {
		t.Errorf("values를 보내지 않았다: %v", wrote)
	}
}

// 빈 아카이브를 동기화하면 시트가 비워져야 한다 — 쓰기는 건너뛰되 clear는 해야 한다.
// 이걸 반대로 하면(아무것도 안 함) 지운 뒤 동기화해도 옛 내용이 그대로 남는다.
func TestReplace_emptyStillClears(t *testing.T) {
	keyJSON, _ := testKey(t)
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.base = srv.URL

	if err := c.Replace(context.Background(), "SHEET", "links", nil); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "clear") {
		t.Errorf("빈 입력에도 clear 한 번은 나가야 한다: %v", calls)
	}
}

// 403은 거의 항상 "시트를 서비스 계정에 공유하지 않음"이다. 그 진단이 붙지 않으면
// 사용자가 API 콘솔을 헤맨다.
func TestDo_forbiddenExplainsSharing(t *testing.T) {
	keyJSON, _ := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"caller does not have permission"}}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	err = c.do(context.Background(), http.MethodPut, srv.URL+"/values", map[string]any{})
	if err == nil {
		t.Fatal("403인데 에러가 없다")
	}
	if !strings.Contains(err.Error(), "공유했는지") {
		t.Errorf("403 진단에 공유 안내가 없다: %v", err)
	}
	if !strings.Contains(err.Error(), c.Email()) {
		t.Errorf("어느 계정에 공유해야 하는지 안 알려준다: %v", err)
	}
}

// 토큰은 캐시돼야 한다. 동기화 한 번에 clear + update 두 요청이 나가는데 매번 토큰을
// 새로 받으면 요청이 두 배가 되고, 구글의 토큰 발급에도 한도가 있다.
func TestAccessToken_isCached(t *testing.T) {
	keyJSON, _ := testKey(t)
	var tokenCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL
	for i := 0; i < 3; i++ {
		if _, err := c.accessToken(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls != 1 {
		t.Errorf("토큰을 %d번 받았다 — 캐시가 안 된다", tokenCalls)
	}
}

// Sheets는 **행 뒤쪽의 빈 셀을 잘라서 보낸다.** 마지막 열(메모·상태)이 비면 그 행만
// 짧아지는데, 열 인덱스로 그냥 접근하면 거기서 패닉이 난다 — 그리고 그 행은 "메모가
// 비어 있는 링크"라는 가장 흔한 경우다.
func TestRead_handlesRaggedRows(t *testing.T) {
	keyJSON, _ := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		fmt.Fprint(w, `{"values":[["id","url","note"],["1","https://a.example","메모"],["2","https://b.example"]]}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.base = srv.URL

	rows, err := c.Read(context.Background(), "SHEET", "links")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("행 수: %d", len(rows))
	}
	if got := Cell(rows[2], 2); got != "" {
		t.Errorf("잘린 셀은 빈 문자열이어야 한다: %q", got)
	}
	if got := Cell(rows[1], 2); got != "메모" {
		t.Errorf("있는 셀은 그대로여야 한다: %q", got)
	}
	if got := Cell(rows[1], 99); got != "" {
		t.Errorf("범위 밖도 빈 문자열이어야 한다: %q", got)
	}
}

// 첫 동기화 전에는 탭이 없다. Sheets는 그때 400 "Unable to parse range"를 주는데,
// 그걸 에러로 올리면 "아직 안 만들었다"가 "고장났다"로 보고된다.
func TestRead_missingTabIsNotAnError(t *testing.T) {
	keyJSON, _ := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"Unable to parse range: nope"}}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.base = srv.URL

	rows, err := c.Read(context.Background(), "SHEET", "nope")
	if err != nil {
		t.Fatalf("없는 탭은 에러가 아니어야 한다: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("없는 탭은 빈 결과여야 한다: %v", rows)
	}
}

// 진짜 400(범위 파싱 문제가 아닌 것)은 삼키면 안 된다 — 삼키면 "시트가 비었다"로
// 보이고, 그 상태로 Replace를 돌리면 멀쩡한 시트를 지운다.
func TestRead_otherBadRequestStillErrors(t *testing.T) {
	keyJSON, _ := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"Invalid spreadsheet id"}}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.base = srv.URL

	if _, err := c.Read(context.Background(), "SHEET", "links"); err == nil {
		t.Fatal("다른 400을 삼켰다 — 빈 시트로 오인하면 Replace가 멀쩡한 시트를 지운다")
	}
}

// 시트 생성 — 응답에서 ID를 꺼내야 한다. 못 꺼내면 빈 ID로 이후 요청이 전부
// 이상한 URL로 나가는데, 그 증상은 "404"라서 원인이 안 보인다.
func TestCreate_returnsID(t *testing.T) {
	keyJSON, _ := testKey(t)
	var gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if props, ok := body["properties"].(map[string]any); ok {
			gotTitle, _ = props["title"].(string)
		}
		fmt.Fprint(w, `{"spreadsheetId":"NEW123"}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.base = srv.URL

	id, err := c.Create(context.Background(), "제목")
	if err != nil {
		t.Fatal(err)
	}
	if id != "NEW123" {
		t.Errorf("ID: %q", id)
	}
	if gotTitle != "제목" {
		t.Errorf("제목이 전달되지 않았다: %q", gotTitle)
	}
}

// ID 없는 응답을 성공으로 읽으면 빈 ID가 흘러 다닌다. 그 상태로 상태 파일에 저장되면
// 다음 실행도 빈 ID를 재사용해 영원히 고장난다.
func TestCreate_emptyIDIsAnError(t *testing.T) {
	keyJSON, _ := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.base = srv.URL

	if _, err := c.Create(context.Background(), "x"); err == nil {
		t.Fatal("ID 없는 응답을 성공으로 읽었다")
	}
}

// 공유는 알림 없이 먼저 시도하고, 거부되면 알림을 켜서 재시도한다.
// 개인 Gmail에 알림 없는 공유를 구글이 거부하는 경우가 있어서다 — 재시도가 없으면
// 시트는 만들어졌는데 사용자가 볼 수 없는 상태로 끝난다.
func TestShare_retriesWithNotification(t *testing.T) {
	keyJSON, _ := testKey(t)
	var attempts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		notify := r.URL.Query().Get("sendNotificationEmail")
		attempts = append(attempts, notify)
		if notify == "false" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"Bad Request"}}`)
			return
		}
		fmt.Fprint(w, `{"id":"perm1"}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.drive = srv.URL

	if err := c.Share(context.Background(), "SHEET", "me@example.com"); err != nil {
		t.Fatalf("재시도로 성공해야 한다: %v", err)
	}
	if len(attempts) != 2 || attempts[0] != "false" || attempts[1] != "true" {
		t.Errorf("알림 없이 먼저, 그다음 알림 켜고여야 한다: %v", attempts)
	}
}

// 둘 다 실패하면 어느 계정에 공유하려 했는지가 오류에 있어야 한다 —
// 없으면 사용자가 콘솔에서 무엇을 고쳐야 할지 모른다.
func TestShare_bothFailuresAreReported(t *testing.T) {
	keyJSON, _ := testKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			fmt.Fprint(w, `{"access_token":"t","expires_in":3600}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"Drive API has not been used"}}`)
	}))
	defer srv.Close()

	c, err := New(keyJSON)
	if err != nil {
		t.Fatal(err)
	}
	c.acct.TokenURI = srv.URL + "/token"
	c.drive = srv.URL

	err = c.Share(context.Background(), "SHEET", "me@example.com")
	if err == nil {
		t.Fatal("둘 다 실패했는데 성공으로 보고했다")
	}
	if !strings.Contains(err.Error(), "me@example.com") {
		t.Errorf("어느 계정인지 없다: %v", err)
	}
}

// drive.file 스코프를 요청해야 시트를 만들고 공유할 수 있다. 넓은 auth/drive를
// 요청하면 사용자의 드라이브 전체가 이 키의 사정권에 들어온다 — 그건 이 도구가
// 필요로 하는 권한이 아니다.
func TestScope_isNarrow(t *testing.T) {
	if !strings.Contains(scope, "drive.file") {
		t.Error("drive.file 스코프가 없으면 시트 생성·공유가 403이다")
	}
	if strings.Contains(scope, "auth/drive ") || strings.HasSuffix(scope, "auth/drive") {
		t.Error("전체 Drive 스코프를 요청하고 있다 — drive.file로 충분하다")
	}
}
