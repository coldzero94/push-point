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
	scope = "https://www.googleapis.com/auth/spreadsheets"
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
	base  string
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
	return &Client{http: &http.Client{Timeout: requestTimeout}, acct: acct, key: key, base: apiBase}, nil
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

// Replace는 탭 하나의 내용을 rows로 통째로 바꾼다.
//
// **덧붙이기가 아니라 교체다.** 시트는 파생물이고 SQLite가 원본이라, 태그를 고치거나
// 링크를 지운 것이 시트에도 반영돼야 한다. 덧붙이기만 하면 시트가 원본과 조용히 갈라지고,
// 갈라졌다는 사실이 어디에도 안 보인다.
//
// 그 대가로 **시트에 손으로 적은 것은 지워진다.** 이건 버그가 아니라 계약이다.
func (c *Client) Replace(ctx context.Context, spreadsheetID, tab string, rows [][]any) error {
	// 먼저 비운다. 이전 동기화가 더 많은 행을 썼다면 update만으로는 꼬리가 남는다.
	if err := c.clear(ctx, spreadsheetID, tab); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	payload := map[string]any{"values": rows}
	// A1부터. 범위를 행 수로 지정하지 않는 이유는 values.update가 넘겨준 배열 크기대로
	// 쓰기 때문이고, 범위와 배열이 어긋날 때 조용히 잘리는 것을 피하기 위해서다.
	target := fmt.Sprintf("%s/%s/values/%s!A1?valueInputOption=RAW",
		c.base, url.PathEscape(spreadsheetID), url.PathEscape(tab))
	return c.do(ctx, http.MethodPut, target, payload)
}

func (c *Client) clear(ctx context.Context, spreadsheetID, tab string) error {
	target := fmt.Sprintf("%s/%s/values/%s:clear",
		c.base, url.PathEscape(spreadsheetID), url.PathEscape(tab))
	return c.do(ctx, http.MethodPost, target, map[string]any{})
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
		hint := ""
		if resp.StatusCode == http.StatusForbidden {
			hint = fmt.Sprintf(" — 시트를 %s 에 편집자로 공유했는지 확인하세요", c.acct.ClientEmail)
		}
		return fmt.Errorf("sheets: %s 거부 (%d)%s: %s",
			method, resp.StatusCode, hint, strings.TrimSpace(string(respBody)))
	}
	return nil
}
