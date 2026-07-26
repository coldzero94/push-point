package main

// sheets-sync 서브커맨드 — 아카이브를 Google 스프레드시트로 내보낸다.
//
//	pushpoint sheets-sync
//
// **단방향이다. SQLite가 원본이고 시트는 파생물이다.**
//
// 반대 방향(시트를 DB로)은 검토했고 성립하지 않는다: 저장 API의 p99 < 50ms 게이트에
// Sheets 왕복(수백 ms + 분당 write 한도)이 안 들어가고, FTS5 전문 검색이 없고, 무엇보다
// **확장이 비행기 모드에서도 저장을 끝내는 성질**(M4 DoD)이 네트워크 전제 위에서는 성립하지
// 않는다. 그래서 저장 경로는 건드리지 않고, 다 끝난 데이터를 나중에 밀어 넣기만 한다.
//
// 그래서 이 명령이 실패해도 아카이브는 멀쩡하다. 그게 잡 큐에 태우지 않고 별도 명령으로
// 둔 이유이기도 하다 — 저장할 때마다 시트에 쓰면 외부 서비스가 저장 경로에 들어온다.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coby/push-point/backend/internal/config"
	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/sheets"
	"github.com/coby/push-point/backend/internal/store"
)

// syncPageSize는 한 번에 읽어 오는 링크 수. 커서 목록을 그대로 쓴다.
const syncPageSize = 500

// sheetHeader는 시트 첫 행. 열 순서는 계약이다 — 시트에서 만든 필터·수식이 열 위치에
// 의존하므로, 열을 **중간에 끼워 넣지 말고 뒤에 붙인다**.
var sheetHeader = []any{
	"id", "저장일", "URL", "도메인", "제목", "설명", "태그", "메모", "상태",
}

func runSheetsSync(_ []string) error {
	keyPath := os.Getenv("PUSHPOINT_SHEETS_KEY")
	sheetID := os.Getenv("PUSHPOINT_SHEETS_ID")
	tab := os.Getenv("PUSHPOINT_SHEETS_TAB")
	if tab == "" {
		tab = "links"
	}
	if keyPath == "" || sheetID == "" {
		return fmt.Errorf(`sheets-sync: 설정이 필요합니다.

  PUSHPOINT_SHEETS_KEY   서비스 계정 JSON 키 파일 경로
  PUSHPOINT_SHEETS_ID    스프레드시트 ID (URL의 /d/ 와 /edit 사이)
  PUSHPOINT_SHEETS_TAB   탭 이름 (선택, 기본 "links")

준비 절차:
  1. Google Cloud 콘솔에서 서비스 계정을 만들고 JSON 키를 내려받는다
  2. 그 프로젝트에서 Google Sheets API를 켠다
  3. 시트를 서비스 계정 이메일에 **편집자**로 공유한다 (이걸 빼먹으면 403)`)
	}

	keyJSON, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("sheets-sync: 키 파일 읽기 실패: %w", err)
	}
	client, err := sheets.New(keyJSON)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("sheets-sync: DB 열기 실패: %w", err)
	}
	defer db.Close()
	st := store.New(db, queue.NewSQLite(db.Writer))

	ctx := context.Background()
	rows := [][]any{sheetHeader}
	cursor := ""
	for {
		links, next, err := st.ListLinks(ctx, cursor, syncPageSize, "", "")
		if err != nil {
			return fmt.Errorf("sheets-sync: 링크 조회 실패: %w", err)
		}
		for _, l := range links {
			rows = append(rows, linkRow(l))
		}
		if next == "" {
			break
		}
		cursor = next
	}

	// 헤더만 남는 경우도 정상이다 — 빈 아카이브를 동기화하면 시트가 비워지는 것이 맞다.
	if err := client.Replace(ctx, sheetID, tab, rows); err != nil {
		return err
	}
	fmt.Printf("sheets-sync: %d건을 %q 탭에 썼습니다 (서비스 계정 %s)\n",
		len(rows)-1, tab, client.Email())
	return nil
}

// linkRow는 링크 하나를 시트 한 행으로 만든다.
//
// **body_text와 summary는 넣지 않는다.** 본문은 링크당 최대 32KB인데 시트 셀 상한은
// 50,000자이고 시트 전체가 1,000만 셀이다 — 본문을 넣으면 몇백 건에서 한도에 부딪힌다.
// 그리고 시트에서 본문을 읽을 일이 없다(읽으려면 원문을 연다). 시트는 훑고 거르는
// 자리이므로 훑을 수 있는 것만 넣는다.
func linkRow(l store.Link) []any {
	names := make([]string, 0, len(l.Tags))
	for _, t := range l.Tags {
		names = append(names, t.Name)
	}
	return []any{
		l.ID,
		// 시트가 날짜로 인식하도록 사람이 읽는 형식으로 넣는다. epoch 정수로 넣으면
		// 정렬은 되지만 필터의 날짜 조건이 안 걸린다.
		formatSheetTime(l.CreatedAt),
		l.URL,
		l.Domain,
		l.Title,
		l.Description,
		strings.Join(names, ", "),
		l.Note,
		l.Status,
	}
}

// formatSheetTime은 epoch 초를 시트가 날짜로 알아보는 형식으로 만든다.
//
// epoch 정수로 넣으면 정렬은 되지만 시트의 날짜 필터("지난 7일" 등)가 안 걸린다 —
// 시트에서 거르려고 내보내는 것이므로 그게 걸리지 않으면 내보내는 의미가 절반 준다.
// 로컬 시각으로 넣는 이유는 통계 화면(by_day)이 이미 서버 로컬 기준이라, 두 곳이
// 다른 날짜를 말하면 대조가 안 되기 때문이다.
func formatSheetTime(epoch int64) string {
	return time.Unix(epoch, 0).Format("2006-01-02 15:04:05")
}
