package sheetsync

// inbox — 시트에서 명령을 받아 실행한다 (12 §D1).
//
// **`links` 격자를 편집 표면으로 쓰지 않는다.** 그 설계를 여섯 개 검토해 넷을 잘랐고 사인이
// 전부 같았다: **어느 셀이 사람 손인지 알 방법이 없다.** 시트는 셀 수정 시각도 리비전 id도
// 주지 않고, Drive의 `modifiedTime`은 파일 단위인 데다 우리 동기화가 스스로 갱신해 사람
// 편집과 구분되지 않는다. 반대편 `links.updated_at`도 스크랩·썸네일·요약이 전부 올려서
// 사람 편집 시각의 근사치가 못 된다.
//
// 그래서 우리가 **절대 읽지 않는** `links` 격자와 별개로, 사람이 쓰는 `inbox` 탭을 둔다.
//
// 규칙 넷이 이 파일의 전부다:
//
//  1. **`실행` 체크가 마지막 열이다.** 사람은 왼쪽에서 오른쪽으로 채우므로 체크가 자연히
//     마지막 행동이 되고, "이 행이 완성됐는가"가 추측에서 **사람의 선언**으로 바뀐다.
//     이 한 칸이 "타자 치는 중에 실행이 발화" 사고를 통째로 없앤다.
//  2. **인박스에 되쓰지 않는다.** 결과는 `inbox-log` 탭에 쓴다. 읽고 좌표로 되쓰면 사람이
//     열 하나만 끼워도 결과 기록기가 사람 입력을 덮는데, Sheets에는 범위 CAS가 없어
//     안전한 변형이 존재하지 않는다.
//  3. **원장은 행 번호가 아니라 명령 내용의 해시로 둔다.** 행은 지우고 끼우면 움직인다.
//     적용된 행을 그대로 두면 다시 적용되지 않고, 지우면 원장에서도 사라져 같은 명령을
//     다시 쓸 수 있다.
//  4. **충돌 판정을 하지 않는다.** 명령은 상태가 아니라 명령형 의도라 "누가 최신인가"
//     문제가 없다. 되돌릴 재료로 적용 직전 값을 로그에 남긴다.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// InboxTab·LogTab은 사람이 쓰는 탭과 우리가 결과를 남기는 탭.
const (
	InboxTab = "inbox"
	LogTab   = "inbox-log"
)

// inboxHeader는 사람이 채우는 열. **`실행`이 마지막인 것이 규칙 1이다.**
var inboxHeader = []any{"id", "작업", "값", "확인 URL", "실행"}

// logHeader는 결과 탭. 되돌릴 재료(`이전 값`)를 남긴다 — 규칙 4.
var logHeader = []any{"시각", "id", "작업", "값", "결과", "이전 값"}

// Action은 인박스가 받는 다섯. 이름은 시트에 그대로 보이므로 한국어다.
type Action string

const (
	ActionNote   Action = "메모"
	ActionTags   Action = "태그"
	ActionSave   Action = "저장"
	ActionDelete Action = "삭제"
	ActionRetry  Action = "재시도"
)

// Command는 인박스 한 행을 읽은 것.
type Command struct {
	LinkID int64
	Action Action
	Value  string
	// Key는 이 명령의 신원. **행 번호가 아니라 내용의 해시다**(규칙 3).
	Key string
}

// Applier는 명령을 실제로 실행한다. **HTTP API를 쓴다** — 단일 라이터 SQLite에 두 번째
// 쓰기 프로세스를 만들지 않고, `Normalize`·FTS 재색인·`tag_feedback`·잡 enqueue가
// 서버 안에서 일어난다(`import` 서브커맨드와 같은 형태).
type Applier interface {
	// Before는 되돌릴 재료를 읽는다. 못 읽으면 빈 문자열 — 로그가 덜 유용해질 뿐
	// 적용을 막지는 않는다.
	Before(ctx context.Context, c Command) string
	Apply(ctx context.Context, c Command) error
}

// RunInbox는 인박스를 한 번 처리한다. 돌려주는 것은 (적용 수, 건너뛴 수, 오류).
//
// **오류가 나도 export를 막지 않는다.** 검토한 설계 여섯 중 다섯이 "충돌 시 전면 중단"으로
// 멀쩡히 돌던 순방향까지 인질로 잡았다. 호출부는 이 오류를 로그로만 남기면 된다.
func RunInbox(ctx context.Context, tr Transport, dataDir string, ap Applier) (applied, skipped int, err error) {
	rows, err := tr.Read(ctx, InboxTab)
	if err != nil {
		return 0, 0, fmt.Errorf("sheetsync: inbox 읽기 실패: %w", err)
	}
	// 탭이 없거나 비었으면 머리글만 세워 두고 끝낸다 — 사람이 무엇을 채워야 하는지
	// 화면에 있어야 시작할 수 있다.
	if len(rows) == 0 {
		if err := tr.Replace(ctx, InboxTab, [][]any{inboxHeader}); err != nil {
			return 0, 0, fmt.Errorf("sheetsync: inbox 머리글 생성 실패: %w", err)
		}
		return 0, 0, nil
	}

	ledger, err := loadLedger(dataDir)
	if err != nil {
		return 0, 0, err
	}

	var logRows [][]any
	seen := map[string]bool{}

	for i, r := range rows {
		if i == 0 {
			continue // 머리글
		}
		c, ok := parseCommand(r)
		if !ok {
			continue // 체크 안 됨 · 빈 행 · 알 수 없는 작업 — 조용히 넘어간다(아직 쓰는 중일 수 있다)
		}
		seen[c.Key] = true
		if ledger[c.Key] {
			skipped++
			continue
		}
		before := ap.Before(ctx, c)
		result := "적용됨"
		if err := ap.Apply(ctx, c); err != nil {
			result = "실패: " + err.Error()
		} else {
			applied++
			ledger[c.Key] = true
		}
		logRows = append(logRows, []any{
			time.Now().Format("2006-01-02 15:04"), c.LinkID, string(c.Action), c.Value, result, before,
		})
	}

	// **지워진 행은 원장에서도 지운다**(규칙 3). 그래야 같은 명령을 다시 쓸 수 있다.
	for k := range ledger {
		if !seen[k] {
			delete(ledger, k)
		}
	}
	if err := saveLedger(dataDir, ledger); err != nil {
		return applied, skipped, err
	}

	if len(logRows) > 0 {
		if err := appendLog(ctx, tr, logRows); err != nil {
			// 로그를 못 써도 적용은 이미 됐다. 그 사실을 오류로 돌려주되 적용 수는 유지한다.
			return applied, skipped, fmt.Errorf("sheetsync: inbox-log 쓰기 실패: %w", err)
		}
	}
	return applied, skipped, nil
}

// parseCommand는 한 행을 읽는다. **열 이름이 아니라 위치로 읽지 않는다** — 라고 쓰고 싶지만
// Sheets에는 열 이름이 없다. 대신 머리글을 고정하고 위치로 읽되, `실행`이 켜진 행만 본다.
// 사람이 열을 끼우면 그 행은 파싱에 실패하고 조용히 넘어간다(적용되지 않는 쪽이 안전하다).
func parseCommand(r []string) (Command, bool) {
	if len(r) < 5 {
		return Command{}, false
	}
	if !isChecked(r[4]) {
		return Command{}, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(r[0]), 10, 64)
	action := Action(strings.TrimSpace(r[1]))
	value := strings.TrimSpace(r[2])

	switch action {
	case ActionNote, ActionTags, ActionDelete, ActionRetry:
		if err != nil || id <= 0 {
			return Command{}, false
		}
	case ActionSave:
		// 저장은 id가 없다 — 값이 URL이다.
		if value == "" {
			return Command{}, false
		}
		id = 0
	default:
		return Command{}, false
	}

	c := Command{LinkID: id, Action: action, Value: value}
	c.Key = commandKey(c)
	return c, true
}

// isChecked는 시트의 체크박스를 읽는다. `getDisplayValues`가 체크박스를 "TRUE"/"FALSE"로
// 주지만, 사람이 손으로 적을 수도 있어 흔한 표기를 함께 받는다.
func isChecked(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "y", "yes", "예", "o", "ok", "1", "v", "✓", "x":
		return true
	}
	return false
}

// commandKey는 명령 내용의 해시. 행 번호가 아니다 — 행은 지우고 끼우면 움직인다.
func commandKey(c Command) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%d\x00%s\x00%s", c.LinkID, c.Action, c.Value))
	return hex.EncodeToString(h[:12])
}

// appendLog는 로그 탭에 줄을 덧붙인다. 스크립트에 append 액션이 없으므로 읽고 합쳐 다시 쓴다.
//
// **로그가 길어지면 앞을 자른다.** 시트 한 탭의 셀 수는 유한하고, 이 탭이 커져서 실패하기
// 시작하면 인박스 전체가 조용히 멈춘다 — 되돌릴 재료를 무한히 들고 있는 값어치보다
// 계속 도는 쪽이 크다.
const logKeep = 500

func appendLog(ctx context.Context, tr Transport, rows [][]any) error {
	old, err := tr.Read(ctx, LogTab)
	if err != nil {
		return err
	}
	out := [][]any{logHeader}
	for i, r := range old {
		if i == 0 {
			continue
		}
		cells := make([]any, len(r))
		for j, c := range r {
			cells[j] = c
		}
		out = append(out, cells)
	}
	out = append(out, rows...)
	if n := len(out) - 1; n > logKeep {
		out = append([][]any{logHeader}, out[1+n-logKeep:]...)
	}
	return tr.Replace(ctx, LogTab, out)
}

// ---- 원장 ----

func ledgerPath(dataDir string) string { return filepath.Join(dataDir, "sheets-inbox.json") }

func loadLedger(dataDir string) (map[string]bool, error) {
	b, err := os.ReadFile(ledgerPath(dataDir))
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sheetsync: 인박스 원장 읽기 실패: %w", err)
	}
	var keys []string
	if err := json.Unmarshal(b, &keys); err != nil {
		// 원장이 깨졌으면 **비운 채로 간다.** 그러면 이미 적용된 명령이 한 번 더 실행될
		// 수 있는데, 다섯 작업 모두 같은 값으로 다시 적용해도 결과가 같다(멱등).
		return map[string]bool{}, nil
	}
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m, nil
}

func saveLedger(dataDir string, m map[string]bool) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	b, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	return os.WriteFile(ledgerPath(dataDir), b, 0o600)
}
