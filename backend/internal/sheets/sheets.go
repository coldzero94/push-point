// Package sheets는 Google Sheets에 값을 쓰는 최소 클라이언트다.
//
// **공식 클라이언트(`google.golang.org/api/sheets/v4`)를 쓰지 않는다.** 실측하면 grpc·
// protobuf·opentelemetry·s2a-go까지 **모듈 75개**를 끌고 온다. 이 백엔드의 직접 의존성이
// 11개이고 배포물이 단일 정적 바이너리라는 점을 생각하면, 스프레드시트에 행을 쓰는 일
// 하나에 치를 값이 아니다. CGO-free sqlite를 고집한 것과 같은 판단이다.
//
// 대신 필요한 것만 표준 라이브러리로 만든다:
//   - 서비스 계정 JSON에서 RSA 개인키를 읽어 RFC 7523 JWT를 만들고 서명한다
//   - 그 JWT를 access token으로 바꾼다 (OAuth 2.0 JWT bearer grant)
//   - Sheets REST v4의 values.update / values.clear를 부른다
//
// 서비스 계정을 쓰는 이유는 사용자 OAuth 흐름이 리다이렉트·토큰 갱신·동의 화면을 요구해
// 단일 사용자 셀프호스트에 과하기 때문이다. 서비스 계정이면 키 파일 하나를 두고 시트를
// 그 계정 이메일에 공유하는 것으로 끝난다. **키는 기기 밖으로 나가지 않는다.**
package sheets

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	tokenURL = "https://oauth2.googleapis.com/token"
	apiBase  = "https://sheets.googleapis.com/v4/spreadsheets"
	// scope는 쓰기까지 필요하다. 읽기 전용 스코프로는 values.update가 403이다.
	//
	// drive.file을 함께 요청하는 이유는 **우리가 시트를 만들어 사용자에게 공유**하기
	// 위해서다(사용자가 시트를 만들고 ID를 복사해 오는 단계를 없앤다). drive.file은
	// 전체 Drive가 아니라 **이 앱이 만든 파일에만** 권한을 준다 — 사용자의 나머지
	// 드라이브는 이 키로 건드릴 수 없다. 넓은 `auth/drive`를 쓰지 않는 이유가 그것이다.
	scope     = "https://www.googleapis.com/auth/spreadsheets https://www.googleapis.com/auth/drive.file"
	driveBase = "https://www.googleapis.com/drive/v3/files"
	// tokenTTL은 JWT의 유효 기간. 구글이 1시간을 상한으로 둔다.
	tokenTTL = time.Hour
	// requestTimeout은 요청당 상한. 동기화는 사람이 기다리는 명령이라 짧게 잡는다.
	requestTimeout = 30 * time.Second
)

// serviceAccount는 서비스 계정 JSON 키에서 우리가 쓰는 필드만 담는다.
type serviceAccount struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// Client는 시트 하나에 값을 쓰는 클라이언트.
type Client struct {
	http *http.Client
	acct serviceAccount
	key  *rsa.PrivateKey
	// base는 Sheets API 루트. 테스트가 갈아 끼울 수 있게 필드로 둔다 — 상수로 두면
	// "비우고 쓴다"는 순서를 실제 호출로 확인할 방법이 없어 그 테스트가 가짜가 된다.
	base string
	// drive는 Drive API 루트. 시트를 만든 뒤 사용자에게 공유할 때만 쓴다.
	drive string
	token string
	// exp는 캐시된 토큰의 만료 시각. 한 번의 동기화에서 여러 요청을 보내므로
	// 매번 토큰을 새로 받지 않는다.
	exp time.Time
}

// New는 서비스 계정 JSON 키 바이트로 클라이언트를 만든다.
func New(keyJSON []byte) (*Client, error) {
	var acct serviceAccount
	if err := json.Unmarshal(keyJSON, &acct); err != nil {
		return nil, fmt.Errorf("sheets: 서비스 계정 키 파싱 실패: %w", err)
	}
	if acct.Type != "service_account" {
		// 사용자 OAuth 클라이언트 JSON을 잘못 넣는 실수가 흔하다. 그 경우 아래
		// 개인키 파싱이 "PEM 아님"으로 실패하는데, 원인을 짐작하기 어렵다.
		return nil, fmt.Errorf("sheets: 서비스 계정 키가 아닙니다 (type=%q) — "+
			"OAuth 클라이언트 JSON이 아니라 서비스 계정 키를 받으세요", acct.Type)
	}
	if acct.ClientEmail == "" || acct.PrivateKey == "" {
		return nil, fmt.Errorf("sheets: 키에 client_email 또는 private_key가 없습니다")
	}
	key, err := parseKey(acct.PrivateKey)
	if err != nil {
		return nil, err
	}
	if acct.TokenURI == "" {
		acct.TokenURI = tokenURL
	}
	return &Client{http: &http.Client{Timeout: requestTimeout}, acct: acct, key: key, base: apiBase, drive: driveBase}, nil
}

// Email은 서비스 계정 주소. 시트를 이 주소에 공유해야 쓰기가 되므로,
// 안내 메시지에서 그대로 보여 줄 수 있어야 한다.
func (c *Client) Email() string { return c.acct.ClientEmail }

// parseKey는 PEM(PKCS#8 또는 PKCS#1) RSA 개인키를 읽는다.
// 구글 서비스 계정 키는 PKCS#8이지만 둘 다 받아 둔다.
func parseKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("sheets: private_key가 PEM이 아닙니다")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("sheets: RSA 키가 아닙니다 (%T)", k)
		}
		return rsaKey, nil
	}
	k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("sheets: 개인키 파싱 실패: %w", err)
	}
	return k, nil
}

// accessToken은 유효한 토큰을 돌려준다. 만료 1분 전부터 새로 받는다 —
// 경계에서 요청이 401로 죽는 것을 피하려는 여유다.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	if c.token != "" && time.Now().Before(c.exp.Add(-time.Minute)) {
		return c.token, nil
	}
	assertion, err := c.signedJWT()
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.acct.TokenURI,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("sheets: 토큰 요청 생성 실패: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("sheets: 토큰 요청 실패: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sheets: 토큰 발급 거부 (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("sheets: 토큰 응답 파싱 실패: %w", err)
	}
	c.token = out.AccessToken
	c.exp = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return c.token, nil
}

// signedJWT는 RFC 7523 assertion을 만든다: base64url(header).base64url(claims).base64url(RS256 서명).
func (c *Client) signedJWT() (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   c.acct.ClientEmail,
		"scope": scope,
		"aud":   c.acct.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(tokenTTL).Unix(),
	}
	seg := func(v any) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(b), nil
	}
	h, err := seg(header)
	if err != nil {
		return "", fmt.Errorf("sheets: JWT 헤더 인코딩 실패: %w", err)
	}
	p, err := seg(claims)
	if err != nil {
		return "", fmt.Errorf("sheets: JWT 클레임 인코딩 실패: %w", err)
	}
	signing := h + "." + p
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", fmt.Errorf("sheets: JWT 서명 실패: %w", err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Read는 탭 하나의 값을 읽는다. 없는 탭이면 빈 결과다(에러가 아니다 — 첫 동기화 전에는
// 탭이 비어 있는 것이 정상이다).
//
// 반환 행은 **뒤쪽 빈 셀이 잘려 온다.** Sheets가 그렇게 준다 — 마지막 열이 비어 있으면
// 그 행의 길이가 짧아진다. 호출자가 열 인덱스로 접근하면 범위를 벗어나므로 cell()을 쓴다.
func (c *Client) Read(ctx context.Context, spreadsheetID, tab string) ([][]string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	target := fmt.Sprintf("%s/%s/values/%s",
		c.base, url.PathEscape(spreadsheetID), url.PathEscape(tab))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("sheets: 읽기 요청 생성 실패: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sheets: 읽기 실패: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode == http.StatusBadRequest {
		// 탭이 없을 때 Sheets는 400에 "Unable to parse range"를 준다.
		// 첫 동기화 전에는 정상 상태이므로 에러로 올리지 않는다.
		if strings.Contains(string(body), "Unable to parse range") {
			return nil, nil
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		hint := ""
		if resp.StatusCode == http.StatusForbidden {
			hint = fmt.Sprintf(" — 시트를 %s 에 공유했는지 확인하세요", c.acct.ClientEmail)
		}
		return nil, fmt.Errorf("sheets: 읽기 거부 (%d)%s: %s",
			resp.StatusCode, hint, strings.TrimSpace(string(body)))
	}
	var out struct {
		Values [][]any `json:"values"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("sheets: 읽기 응답 파싱 실패: %w", err)
	}
	rows := make([][]string, 0, len(out.Values))
	for _, r := range out.Values {
		row := make([]string, 0, len(r))
		for _, v := range r {
			row = append(row, fmt.Sprint(v))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Cell은 행에서 i번째 열을 꺼낸다. 없으면 빈 문자열이다 — Sheets가 뒤쪽 빈 셀을
// 잘라 보내므로 인덱스 접근을 그대로 하면 패닉이 난다.
func Cell(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return row[i]
}

// Replace는 탭 하나의 내용을 rows로 통째로 바꾼다.
//
// **덧붙이기가 아니라 교체다.** 시트는 파생물이고 SQLite가 원본이라, 태그를 고치거나
// 링크를 지운 것이 시트에도 반영돼야 한다. 덧붙이기만 하면 시트가 원본과 조용히 갈라지고,
// 갈라졌다는 사실이 어디에도 안 보인다.
//
// 그 대가로 **시트에 손으로 적은 것은 지워진다.** 이건 버그가 아니라 계약이다.
func (c *Client) Replace(ctx context.Context, spreadsheetID, tab, lastCol string, rows [][]any) error {
	// 탭이 없으면 만든다. **구글이 새 스프레드시트에 만들어 주는 탭 이름은 `Sheet1`이고
	// 우리 기본 탭은 `links`다.** 이걸 빼먹으면 우리가 만든 시트에서 첫 동기화가 곧바로
	// 400 "Unable to parse range"로 죽는다 — 실제로 그랬고, httptest 목이 응답을 흉내낼 뿐
	// 구글의 의미론을 흉내내지 않아서 테스트가 통과하는 채로 남아 있었다.
	if err := c.ensureTab(ctx, spreadsheetID, tab); err != nil {
		return err
	}
	// 먼저 비운다. 이전 동기화가 더 많은 행을 썼다면 update만으로는 꼬리가 남는다.
	if err := c.clear(ctx, spreadsheetID, tab, lastCol); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	payload := map[string]any{"values": rows}
	// A1부터. 범위를 행 수로 지정하지 않는 이유는 values.update가 넘겨준 배열 크기대로
	// 쓰기 때문이고, 범위와 배열이 어긋날 때 조용히 잘리는 것을 피하기 위해서다.
	// USER_ENTERED다. RAW로 넣으면 날짜가 **텍스트 셀**이 되어 시트의 날짜 필터가
	// 안 걸리는데, 시트에서 거르려고 내보내는 것이므로 그러면 목적을 절반 잃는다.
	// 웹훅의 setValues도 파싱하므로 전송 방식에 따라 셀 타입이 달라지지도 않는다.
	//
	// 파싱을 켜는 이상 수식 주입이 열리므로 **EscapeRows가 선행 조건이다**(escape.go).
	target := fmt.Sprintf("%s/%s/values/%s!A1?valueInputOption=USER_ENTERED",
		c.base, url.PathEscape(spreadsheetID), url.PathEscape(tab))
	return c.do(ctx, http.MethodPut, target, payload)
}

// clear는 **우리 열만** 비운다. 탭 전체가 아니다.
//
// 이 한 줄이 계약을 바꾼다: "A~lastCol은 우리 것이고 매 동기화에 재생성된다.
// 그다음 열부터는 당신 것이고 우리는 영원히 건드리지 않는다."
//
// 원래는 탭을 통째로 비웠다. 그러면 사용자가 J열에 무엇을 적든 다음 동기화가 말없이
// 지운다 — 문서에 "계약"이라고 적어 뒀지만 그건 변명이지 해결이 아니었다. 손실을
// 감지하려 애쓰는 대신 **손실이 일어날 수 없는 구조**로 바꾸는 편이 싸고 확실하다.
// (감지는 애초에 불가능에 가깝다 — Sheets는 행·셀 수정 시각도 리비전 id도 주지 않는다.)
func (c *Client) clear(ctx context.Context, spreadsheetID, tab, lastCol string) error {
	target := fmt.Sprintf("%s/%s/values/%s:clear",
		c.base, url.PathEscape(spreadsheetID), url.PathEscape(tab+"!A:"+lastCol))
	return c.do(ctx, http.MethodPost, target, map[string]any{})
}

// ensureTab은 탭이 없으면 만든다. 이미 있으면 구글이 400을 주는데 그건 정상이다.
func (c *Client) ensureTab(ctx context.Context, spreadsheetID, tab string) error {
	target := fmt.Sprintf("%s/%s:batchUpdate", c.base, url.PathEscape(spreadsheetID))
	payload := map[string]any{"requests": []any{
		map[string]any{"addSheet": map[string]any{
			"properties": map[string]any{"title": tab},
		}},
	}}
	_, err := c.doJSON(ctx, http.MethodPost, target, payload)
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return nil
	}
	return err
}

func (c *Client) do(ctx context.Context, method, target string, payload any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sheets: 요청 본문 인코딩 실패: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sheets: 요청 생성 실패: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sheets: 요청 실패: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 403은 거의 항상 "시트를 서비스 계정에 공유하지 않음"이다. 그 진단을 여기서
		// 붙여 주지 않으면 사용자가 API 콘솔을 헤매게 된다.
		return fmt.Errorf("sheets: %s 거부 (%d)%s: %s",
			method, resp.StatusCode, c.sharingHint(resp.StatusCode), strings.TrimSpace(string(respBody)))
	}
	return nil
}

// Create는 새 스프레드시트를 만들고 ID를 돌려준다.
//
// **왜 우리가 만드는가.** 사용자에게 "시트를 만들고 URL에서 ID를 잘라 오세요"를 시키면
// 준비 단계가 하나 더 늘고, 그 단계가 틀리기도 쉽다(ID와 URL 전체를 헷갈리는 것이 흔하다).
// 만드는 쪽은 API 한 번이라 우리가 하는 편이 싸다.
//
// 만들어진 파일의 **소유자는 서비스 계정**이다. 그래서 Share를 반드시 이어서 불러야
// 사용자 눈에 보인다 — 안 그러면 시트는 존재하는데 아무도 열 수 없다.
func (c *Client) Create(ctx context.Context, title string) (string, error) {
	body, err := c.doJSON(ctx, http.MethodPost, c.base,
		map[string]any{"properties": map[string]any{"title": title}})
	if err != nil {
		return "", err
	}
	var out struct {
		SpreadsheetID string `json:"spreadsheetId"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("sheets: 생성 응답 파싱 실패: %w", err)
	}
	if out.SpreadsheetID == "" {
		return "", fmt.Errorf("sheets: 생성 응답에 spreadsheetId가 없습니다")
	}
	return out.SpreadsheetID, nil
}

// Share는 파일을 email에게 편집자로 공유한다.
//
// `sendNotificationEmail=false`인 이유: 자기 도구가 자기에게 보내는 알림 메일은 소음이고,
// 어차피 아래에서 URL을 출력한다. 다만 **개인 Gmail 계정에 공유할 때 구글이 알림 없는
// 공유를 거부하는 경우가 있어**, 실패하면 알림을 켜서 한 번 더 시도한다.
func (c *Client) Share(ctx context.Context, fileID, email string) error {
	target := fmt.Sprintf("%s/%s/permissions?sendNotificationEmail=false",
		c.drive, url.PathEscape(fileID))
	perm := map[string]any{"type": "user", "role": "writer", "emailAddress": email}
	if _, err := c.doJSON(ctx, http.MethodPost, target, perm); err == nil {
		return nil
	}
	retry := fmt.Sprintf("%s/%s/permissions?sendNotificationEmail=true",
		c.drive, url.PathEscape(fileID))
	if _, err := c.doJSON(ctx, http.MethodPost, retry, perm); err != nil {
		return fmt.Errorf("sheets: %s 에게 공유 실패: %w", email, err)
	}
	return nil
}

// URL은 사람이 열 수 있는 주소.
func URL(spreadsheetID string) string {
	return "https://docs.google.com/spreadsheets/d/" + spreadsheetID + "/edit"
}

// doJSON은 do와 같지만 응답 본문을 돌려준다.
func (c *Client) doJSON(ctx context.Context, method, target string, payload any) ([]byte, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sheets: 요청 본문 인코딩 실패: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sheets: 요청 생성 실패: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sheets: 요청 실패: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// do()와 같은 안내를 붙인다. Replace가 ensureTab(→ doJSON)을 먼저 부르므로,
		// 공유를 빼먹은 흔한 실수는 do()가 아니라 **여기서** 403으로 먼저 터진다 —
		// 안내가 do()에만 있으면 정작 사용자가 보는 오류에는 없다.
		return nil, fmt.Errorf("sheets: %s 거부 (%d)%s: %s",
			method, resp.StatusCode, c.sharingHint(resp.StatusCode), strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

// sharingHint는 403에 "어느 계정에 공유하라"를 붙인다. 403은 거의 항상 그 실수이고,
// 안내가 없으면 사용자가 API 콘솔을 헤맨다.
func (c *Client) sharingHint(status int) string {
	if status != http.StatusForbidden {
		return ""
	}
	return fmt.Sprintf(" — 시트를 %s 에 편집자로 공유했는지 확인하세요", c.acct.ClientEmail)
}
