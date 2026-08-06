package sheetsync

// HTTPApplier — 인박스 명령을 **도는 서버의 HTTP API로** 실행한다.
//
// 왜 store를 직접 부르지 않는가: 단일 라이터 SQLite에 두 번째 쓰기 프로세스를 만들지
// 않기 위해서다. 그리고 API를 지나면 `Normalize`·FTS 재색인·`tag_feedback`·잡 enqueue가
// 전부 서버 안에서 일어난다 — `import` 서브커맨드가 같은 이유로 같은 형태다.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type HTTPApplier struct {
	Base   string // 예: http://127.0.0.1:8420
	Key    string
	Client *http.Client
}

func (a HTTPApplier) client() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return http.DefaultClient
}

// Before는 되돌릴 재료를 읽는다(규칙 4). **실패해도 적용을 막지 않는다** — 로그가 덜
// 유용해질 뿐이고, 여기서 멈추면 읽기 하나가 쓰기 전체를 인질로 잡는다.
func (a HTTPApplier) Before(ctx context.Context, c Command) string {
	if c.LinkID == 0 {
		return ""
	}
	var d struct {
		Note string `json:"note"`
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}
	if err := a.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/links/%d", c.LinkID), nil, &d); err != nil {
		return ""
	}
	switch c.Action {
	case ActionNote:
		return d.Note
	case ActionTags:
		names := make([]string, 0, len(d.Tags))
		for _, t := range d.Tags {
			names = append(names, t.Name)
		}
		return strings.Join(names, ", ")
	}
	return ""
}

func (a HTTPApplier) Apply(ctx context.Context, c Command) error {
	switch c.Action {
	case ActionNote:
		return a.do(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/links/%d", c.LinkID),
			map[string]any{"note": c.Value}, nil)

	case ActionTags:
		// 쉼표로 나눈다. 빈 값은 "태그를 전부 떼라"는 뜻이고, 그건 유효한 의도다.
		var names []string
		for t := range strings.SplitSeq(c.Value, ",") {
			if t = strings.TrimSpace(t); t != "" {
				names = append(names, t)
			}
		}
		if names == nil {
			names = []string{}
		}
		return a.do(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/links/%d", c.LinkID),
			map[string]any{"tags": names}, nil)

	case ActionSave:
		return a.do(ctx, http.MethodPost, "/api/v1/links", map[string]any{"url": c.Value}, nil)

	case ActionDelete:
		return a.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/links/%d", c.LinkID), nil, nil)

	case ActionRetry:
		return a.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/links/%d/retry", c.LinkID), nil, nil)
	}
	return fmt.Errorf("알 수 없는 작업: %s", c.Action)
}

func (a HTTPApplier) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.Base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.Key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // 응답 본문
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		// 서버가 준 사유를 그대로 옮긴다 — 시트의 «결과» 칸에 그대로 들어가고,
		// 거기가 사용자가 무엇이 잘못됐는지 볼 유일한 자리다.
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			return fmt.Errorf("%s", e.Error.Message)
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
