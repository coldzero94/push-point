package sheets

// Apps Script 웹앱 전송 — **Cloud Console도, JSON 키도, API 켜기도 없다.**
//
// 서비스 계정 경로(sheets.go)는 동작하지만 준비 단계가 사용자에게 너무 비쌌다: 클라우드
// 프로젝트를 만들고, API 두 개를 켜고, 서비스 계정을 만들고, JSON 키를 내려받아 파일로
// 둔다. 단일 사용자가 자기 시트에 자기 데이터를 넣으려고 치르기에는 과하다.
//
// 대신 사용자가 자기 시트에 **한 화면짜리 스크립트를 붙여넣고 배포**한다. 그러면 URL이
// 하나 나오고, 그 URL로 POST하면 끝이다. 우리 쪽에는 OAuth도 토큰 갱신도 없다.
//
// 뒤집힌 신뢰 관계가 이 방식의 진짜 장점이다. 서비스 계정은 **우리가 사용자 드라이브에
// 들어가는** 구조인데, 이쪽은 스크립트가 **사용자 계정 안에서 자기 시트만** 만진다.
// 스크립트가 열 수 있는 것은 `getActiveSpreadsheet()` 하나뿐이고(DriveApp 참조 0,
// doGet 없음) 배포를 지우면 접근이 끊긴다 — 신뢰의 근거는 줄 수가 아니라 그 스코프다.
//
// 대가: "링크를 아는 사람은 누구나" 배포이므로 URL 자체가 비밀이다. 그래서 스크립트에
// 토큰을 박고 요청마다 확인한다 — URL이 로그나 히스토리에 남더라도 토큰 없이는 쓸 수 없다.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// webhookTimeout은 Apps Script가 느릴 수 있어 넉넉히 잡는다 — 콜드 스타트가 몇 초 걸린다.
const webhookTimeout = 60 * time.Second

// Webhook은 Apps Script 웹앱을 통해 시트를 읽고 쓴다.
type Webhook struct {
	http  *http.Client
	url   string
	token string
}

// NewWebhook은 배포 URL과 토큰으로 전송을 만든다.
func NewWebhook(deployURL, token string) (*Webhook, error) {
	if !strings.HasPrefix(deployURL, "https://") {
		return nil, fmt.Errorf("sheets: 배포 URL은 https여야 합니다: %q", deployURL)
	}
	if token == "" {
		return nil, fmt.Errorf("sheets: 토큰이 비어 있습니다")
	}
	return &Webhook{
		// Apps Script는 302로 googleusercontent.com에 넘기고 기본 클라이언트가 따라간다.
		// 리다이렉트에서 POST 본문이 유실되는 것은 문제가 되지 않는다 — 스크립트가
		// **첫 홉에서 이미 doPost를 실행**하고, 뒤따르는 GET은 그 결과를 가져올 뿐이다.
		http:  &http.Client{Timeout: webhookTimeout},
		url:   deployURL,
		token: token,
	}, nil
}

// NewToken은 스크립트에 박아 넣을 무작위 토큰을 만든다.
func NewToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("sheets: 토큰 생성 실패: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// webhookReply는 스크립트가 돌려주는 공통 응답.
type webhookReply struct {
	OK     bool       `json:"ok"`
	Error  string     `json:"error"`
	Title  string     `json:"title"`
	URL    string     `json:"url"`
	Values [][]string `json:"values"`
	Rows   int        `json:"rows"`
}

func (w *Webhook) call(ctx context.Context, action string, extra map[string]any) (webhookReply, error) {
	payload := map[string]any{"token": w.token, "action": action}
	for k, v := range extra {
		payload[k] = v
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return webhookReply{}, fmt.Errorf("sheets: 요청 인코딩 실패: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return webhookReply{}, fmt.Errorf("sheets: 요청 생성 실패: %w", err)
	}
	// text/plain으로 보내는 이유: application/json이면 브라우저 밖 요청에도 Apps Script가
	// CORS 프리플라이트 경로를 타면서 e.postData가 비는 경우가 있다. text/plain은 그
	// 경로를 피하고, 스크립트는 어차피 본문을 직접 JSON.parse한다.
	req.Header.Set("Content-Type", "text/plain;charset=utf-8")

	resp, err := w.http.Do(req)
	if err != nil {
		return webhookReply{}, fmt.Errorf("sheets: 요청 실패: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))

	if resp.StatusCode != http.StatusOK {
		return webhookReply{}, fmt.Errorf("sheets: 스크립트가 %d를 돌려줬습니다: %s",
			resp.StatusCode, truncate(string(raw), 300))
	}
	var reply webhookReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		// 배포 설정이 틀리면 JSON이 아니라 구글 로그인 HTML이 온다. 그게 가장 흔한
		// 실수라서 파싱 실패를 그대로 올리지 않고 원인을 짚어 준다.
		if strings.Contains(string(raw), "<html") || strings.Contains(string(raw), "accounts.google.com") {
			return webhookReply{}, fmt.Errorf("sheets: 스크립트 대신 구글 로그인 페이지가 왔습니다 — " +
				"배포 설정에서 \"액세스 권한이 있는 사용자\"를 **모든 사용자**로 두었는지 확인하세요")
		}
		return webhookReply{}, fmt.Errorf("sheets: 응답 파싱 실패: %w (%s)", err, truncate(string(raw), 200))
	}
	if !reply.OK {
		if strings.Contains(reply.Error, "token") {
			return reply, fmt.Errorf("sheets: 토큰이 맞지 않습니다 — 스크립트의 TOKEN과 " +
				"저장된 값이 다릅니다. `just sheets-setup`으로 다시 붙여넣으세요")
		}
		return reply, fmt.Errorf("sheets: 스크립트 오류: %s", reply.Error)
	}
	return reply, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Ping은 연결과 토큰을 확인하고 시트 이름·URL을 돌려준다.
// 연결 직후 이걸 부르지 않으면 첫 동기화에서야 설정 오류가 드러난다.
func (w *Webhook) Ping(ctx context.Context) (title, sheetURL string, err error) {
	r, err := w.call(ctx, "ping", nil)
	if err != nil {
		return "", "", err
	}
	return r.Title, r.URL, nil
}

// Read는 탭의 값을 읽는다. 없는 탭이면 빈 결과다.
func (w *Webhook) Read(ctx context.Context, tab string) ([][]string, error) {
	r, err := w.call(ctx, "read", map[string]any{"tab": tab})
	if err != nil {
		return nil, err
	}
	return r.Values, nil
}

// Replace는 탭의 **우리 열만** rows로 바꾼다. width 오른쪽은 건드리지 않는다 —
// 사용자가 시트에 손으로 적은 것이 동기화 때마다 사라지면 안 된다.
func (w *Webhook) Replace(ctx context.Context, tab string, rows [][]any) error {
	width := 0
	if len(rows) > 0 {
		width = len(rows[0])
	}
	_, err := w.call(ctx, "replace", map[string]any{"tab": tab, "values": rows, "width": width})
	return err
}
