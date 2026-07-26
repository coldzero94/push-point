// Package sheetsync는 아카이브를 Google 스프레드시트로 내보내는 로직이다.
//
// CLI(`pushpoint sheets-sync`)와 HTTP API(`POST /api/v1/sheets/sync`)가 **같은 코드**를
// 쓴다. 갈라 두면 웹 버튼과 터미널 명령이 조금씩 다르게 동작하게 되고, 그 차이는
// 둘 다 "성공"이라고 말하는 동안 조용히 자란다.
package sheetsync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/sheets"
	"github.com/coby/push-point/backend/internal/store"
)

// PageSize는 한 번에 읽어 오는 링크 수.
const PageSize = 500

// DefaultTab은 기본 탭 이름.
const DefaultTab = "links"

// LastCol은 **우리가 소유하는 마지막 열**이다. Header가 9열이므로 I다.
//
// 이 경계가 계약이다: A~I는 파생물이라 매 동기화에 재생성되고, **J열부터는 사용자 것이라
// 우리가 영원히 건드리지 않는다.** 그래서 시트에 손으로 적어 둔 것이 동기화 때문에
// 사라지는 일이 구조적으로 불가능하다 — 손실을 감지하려 애쓰는 것보다 싸고 확실하다.
// Header에 열을 더하면 여기도 같이 옮겨야 한다.
const LastCol = "I"

// Header는 시트 첫 행. 열 순서는 계약이다 — 시트에서 만든 필터·수식이 열 위치에
// 의존하므로, 열은 **중간에 끼워 넣지 말고 뒤에 붙인다**.
var Header = []any{
	"id", "저장일", "URL", "도메인", "제목", "설명", "태그", "메모", "상태",
}

// transport는 시트에 읽고 쓰는 방법. 두 구현이 있다 — Apps Script 웹훅(기본)과
// 서비스 계정. 화면과 동기화 로직은 어느 쪽인지 몰라도 된다.
// Transport는 시트에 읽고 쓰는 방법.
type Transport interface {
	Read(ctx context.Context, tab string) ([][]string, error)
	Replace(ctx context.Context, tab string, rows [][]any) error
}

// saTransport는 서비스 계정 클라이언트를 transport에 맞춘다(시트 ID를 고정해 둔다).
type saTransport struct {
	c  *sheets.Client
	id string
}

func (t saTransport) Read(ctx context.Context, tab string) ([][]string, error) {
	return t.c.Read(ctx, t.id, tab)
}

func (t saTransport) Replace(ctx context.Context, tab string, rows [][]any) error {
	return t.c.Replace(ctx, t.id, tab, LastCol, rows)
}

// 웹훅이 먼저다 — 준비가 훨씬 싸서 기본 경로이고, 서비스 계정은 그쪽을 이미 쓰던
// 사람을 위해 남긴다.
// Connected는 지금 동기화가 가능한 상태인지 본다.
//
// **Open과 같은 규칙을 써야 한다.** 화면의 "연결됨" 판정과 실제 동작이 갈라지면,
// 서비스 계정으로 멀쩡히 동기화되는 서버가 웹에서는 "연결 안 됨"으로 보이고 안내가
// `sheets-setup`을 가리킨다 — 그걸 따르면 State가 통째로 교체돼 서비스 계정 경로가
// 다시는 선택되지 않는다. 실제로 그런 상태였다.
func Connected(st State) bool {
	if st.Mode == "webhook" && st.DeployURL != "" {
		return true
	}
	if os.Getenv("PUSHPOINT_SHEETS_KEY") == "" {
		return false
	}
	return os.Getenv("PUSHPOINT_SHEETS_ID") != "" || st.SpreadsheetID != ""
}

// Open은 저장된 연결 정보로 전송을 만든다.
func Open(dataDir string) (Transport, State, error) {
	st := Load(dataDir)
	if st.Mode == "webhook" && st.DeployURL != "" {
		wh, err := sheets.NewWebhook(st.DeployURL, st.Token)
		if err != nil {
			// nil인 *Webhook을 그대로 돌려주면 인터페이스는 non-nil인데 안이 nil인
			// 상태가 된다. 지금 호출자는 err를 먼저 보므로 무해하지만, 언젠가
			// `if tr != nil`로 판단하는 코드가 생기면 첫 호출에서 패닉이다.
			return nil, st, err
		}
		return wh, st, nil
	}
	// 서비스 계정 경로 — 환경변수가 있을 때만.
	keyPath := os.Getenv("PUSHPOINT_SHEETS_KEY")
	if keyPath == "" {
		return nil, st, fmt.Errorf(`sheets: 아직 연결되지 않았습니다.

  just sheets-setup

한 번 실행하면 스크립트를 클립보드에 넣고 브라우저를 열어 줍니다.
붙여넣고 배포한 뒤 URL만 되돌려 주면 끝입니다.`)
	}
	keyJSON, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, st, fmt.Errorf("sheets: 키 파일 읽기 실패: %w", err)
	}
	client, err := sheets.New(keyJSON)
	if err != nil {
		return nil, st, err
	}
	id := os.Getenv("PUSHPOINT_SHEETS_ID")
	if id == "" {
		id = st.SpreadsheetID
	}
	if id == "" {
		return nil, st, fmt.Errorf("sheets: PUSHPOINT_SHEETS_ID가 필요합니다 (또는 just sheets-setup)")
	}
	return saTransport{c: client, id: id}, st, nil
}

// Run은 링크 전량을 읽어 시트를 교체하고 쓴 링크 수를 돌려준다.
// API 핸들러도 이걸 부르므로 CLI 출력과 섞지 않는다.
func Run(ctx context.Context, tr Transport, dataDir, tab string) (int, error) {
	db, err := store.Open(dataDir)
	if err != nil {
		return 0, fmt.Errorf("sheets: DB 열기 실패: %w", err)
	}
	defer db.Close()
	st := store.New(db, queue.NewSQLite(db.Writer))

	rows := [][]any{Header}
	cursor := ""
	for {
		links, next, err := st.ListLinks(ctx, cursor, PageSize, "", "")
		if err != nil {
			return 0, fmt.Errorf("sheets: 링크 조회 실패: %w", err)
		}
		for _, l := range links {
			rows = append(rows, linkRow(l))
		}
		if next == "" {
			break
		}
		cursor = next
	}
	// 시트에 넣기 직전에 무해화한다. 긁어 온 제목이 수식으로 실행되면 아카이브가
	// 통째로 유출된다(sheets/escape.go). 여기 한 자리에 두면 전송이 늘어도 빠지지 않는다.
	if err := tr.Replace(ctx, tab, sheets.EscapeRows(rows)); err != nil {
		return 0, err
	}
	return len(rows) - 1, nil
}

// State는 연결 정보. 서비스 계정 경로와 웹훅 경로를 한 파일에서 구분한다.
// State는 연결 정보와 마지막 동기화 결과.
type State struct {
	// Mode는 "webhook"(Apps Script) 또는 ""(서비스 계정 — 옛 경로).
	Mode string `json:"mode,omitempty"`
	// DeployURL·Token은 webhook 모드용.
	DeployURL string `json:"deploy_url,omitempty"`
	Token     string `json:"token,omitempty"`
	SheetURL  string `json:"sheet_url,omitempty"`
	// SpreadsheetID는 서비스 계정 모드에서 우리가 만든 시트.
	SpreadsheetID string `json:"spreadsheet_id,omitempty"`
	CreatedAt     int64  `json:"created_at,omitempty"`
	// LastSyncAt·LastRows는 마지막 동기화 결과. 웹 화면이 "언제 무엇을 보냈는지"를
	// 보여주려면 필요하다 — 버튼만 있고 결과가 안 보이면 눌러도 됐는지 알 수 없다.
	LastSyncAt int64  `json:"last_sync_at,omitempty"`
	LastRows   int    `json:"last_rows,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

func statePath(dataDir string) string { return filepath.Join(dataDir, "sheets.json") }

// Load는 저장된 연결 정보를 읽는다.
func Load(dataDir string) State {
	var st State
	b, err := os.ReadFile(statePath(dataDir))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st) // 깨졌으면 없는 것으로 본다 — 다시 연결하면 복구된다
	return st
}

// Save는 연결 정보를 저장한다.
func Save(dataDir string, st State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	// 토큰이 들어 있으므로 0600이다.
	return os.WriteFile(statePath(dataDir), b, 0o600)
}

// linkRow는 링크 하나를 시트 한 행으로 만든다.
//
// **body_text와 summary는 넣지 않는다.** 본문은 링크당 최대 32KB인데 시트 셀 상한이
// 50,000자이고 시트 전체가 1,000만 셀이라, 본문을 넣으면 몇백 건에서 한도에 부딪힌다.
// 그리고 시트에서 본문을 읽을 일이 없다 — 읽으려면 원문을 연다.
func linkRow(l store.Link) []any {
	names := make([]string, 0, len(l.Tags))
	for _, t := range l.Tags {
		names = append(names, t.Name)
	}
	return []any{
		l.ID,
		formatTime(l.CreatedAt),
		l.URL,
		l.Domain,
		l.Title,
		l.Description,
		strings.Join(names, ", "),
		l.Note,
		l.Status,
	}
}

// formatTime은 epoch 초를 시트가 날짜로 알아보는 형식으로 만든다.
//
// epoch 정수로 넣으면 정렬은 되지만 시트의 날짜 필터("지난 7일" 등)가 안 걸린다 —
// 시트에서 거르려고 내보내는 것이므로 그게 안 걸리면 내보내는 의미가 절반 준다.
func formatTime(epoch int64) string {
	return time.Unix(epoch, 0).Format("2006-01-02 15:04:05")
}
