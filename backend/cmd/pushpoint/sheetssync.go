package main

// sheets-sync 서브커맨드 — 아카이브를 Google 스프레드시트로 내보낸다.
//
//	pushpoint sheets-sync
//
// **단방향이다. SQLite가 원본이고 시트는 파생물이다.**
//
// 반대 방향(시트를 DB로)은 검토했고 성립하지 않는다: 저장 API의 p99 < 50ms 게이트에
// Sheets 왕복이 안 들어가고, FTS5 전문 검색이 없고, 트랜잭션이 없어 크래시 내구성의
// 근거가 사라지고, 무엇보다 **확장이 비행기 모드에서도 저장을 끝내는 성질**(M4 DoD)이
// 네트워크 전제 위에서는 성립하지 않는다.
//
// 로직은 internal/sheetsync에 있다 — 웹의 동기화 버튼(POST /api/v1/sheets/sync)이
// **같은 코드**를 쓴다. 갈라 두면 둘이 조금씩 다르게 동작하면서 양쪽 다 "성공"이라고 말한다.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/coby/push-point/backend/internal/sheetsync"
)

func runSheetsSync(_ []string) error {
	tr, st, err := sheetsync.Open(dataDir())
	if err != nil {
		return err
	}
	// 탭 이름은 고정이다. 예전에 PUSHPOINT_SHEETS_TAB 노브가 있었지만 문서·justfile·
	// 테스트 어디에도 없이 이 한 줄에만 존재했고, API 경로(POST /api/v1/sheets/sync)는
	// 읽지 않았다 — 즉 그 노브를 쓰는 순간 CLI와 웹 버튼이 **다른 탭에 쓰는** 상태가
	// 됐다. 아무도 안 쓰는 설정을 위해 두 경로를 갈라 둘 이유가 없다.
	tab := sheetsync.DefaultTab

	n, syncErr := sheetsync.Run(context.Background(), tr, dataDir(), tab)
	// 결과는 성공이든 실패든 남긴다 — 웹 화면이 "마지막에 어떻게 됐는지"를 보여줘야
	// 버튼을 누른 사람이 됐는지 안 됐는지 알 수 있다.
	st.LastSyncAt = time.Now().Unix()
	st.LastRows = n
	st.LastError = ""
	if syncErr != nil {
		st.LastError = syncErr.Error()
	}
	_ = sheetsync.Save(dataDir(), st)
	if syncErr != nil {
		return syncErr
	}
	if st.SheetURL != "" {
		fmt.Printf("sheets-sync: %d건을 %q 탭에 썼습니다\n  %s\n", n, tab, st.SheetURL)
	} else {
		fmt.Printf("sheets-sync: %d건을 %q 탭에 썼습니다\n", n, tab)
	}
	return nil
}

// dataDir는 데이터 디렉터리만 읽는다.
//
// config.Load()를 쓰지 않는 이유: 그쪽은 PUSHPOINT_API_KEY를 필수로 요구하는데,
// 시트 동기화는 서버를 띄우지 않으므로 API 키와 아무 상관이 없다. 관계없는 비밀을
// 설정해야 시트를 내보낼 수 있다면 그건 없앨 수 있는 마찰이다.
// 기본값은 config와 같아야 한다 — 다르면 서버와 다른 DB를 보게 된다.
func dataDir() string {
	if d := os.Getenv("PUSHPOINT_DATA_DIR"); d != "" {
		return d
	}
	return "./data"
}
