package main

// sheets-inbox 서브커맨드 — 시트의 `inbox` 탭에 적힌 명령을 실행한다 (12 §D1).
//
//	pushpoint sheets-inbox
//
// **`sheets-sync`의 반대 방향이지만 대칭이 아니다.** 내보내기는 아카이브 전량을 시트에
// 다시 쓰고, 이쪽은 사람이 **체크한 행만** 실행한다. `links` 격자는 여전히 읽지 않는다 —
// 어느 셀이 사람 손인지 알 방법이 없기 때문이고, 그 논증은 `internal/sheetsync/inbox.go`
// 머리에 있다.
//
// **도는 서버가 필요하다.** 명령을 HTTP API로 실행하기 때문이다(단일 라이터 SQLite에
// 두 번째 쓰기 프로세스를 만들지 않는다). 서버가 없으면 그 사실을 말하고 멈춘다.

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/coby/push-point/backend/internal/sheetsync"
)

func runSheetsInbox(args []string) error {
	fs := flag.NewFlagSet("sheets-inbox", flag.ExitOnError)
	addr := fs.String("addr", "http://127.0.0.1:8420", "도는 서버 주소")
	key := fs.String("key", os.Getenv("PUSHPOINT_API_KEY"), "API 키")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *key == "" {
		return fmt.Errorf("sheets-inbox: API 키가 필요합니다 (PUSHPOINT_API_KEY 또는 -key)")
	}

	tr, _, err := sheetsync.Open(dataDir())
	if err != nil {
		return err
	}

	// 서버가 살아 있는지 먼저 본다. 없으면 인박스를 읽어 놓고 전부 실패로 기록하게 되는데,
	// 그 로그는 사용자에게 "명령이 잘못됐다"로 읽힌다 — 서버가 없다는 사실과 아주 다르다.
	client := &http.Client{Timeout: 30 * time.Second}
	if resp, err := client.Get(*addr + "/healthz"); err != nil {
		return fmt.Errorf("sheets-inbox: 서버에 붙지 못했습니다 (%s) — `just dev`로 띄운 뒤 다시 실행하세요: %w", *addr, err)
	} else {
		resp.Body.Close() //nolint:errcheck // healthz 본문은 읽지 않는다
	}

	ap := sheetsync.HTTPApplier{Base: *addr, Key: *key, Client: client}
	applied, skipped, err := sheetsync.RunInbox(context.Background(), tr, dataDir(), ap)
	fmt.Printf("sheets-inbox: %d건 적용, %d건 건너뜀\n", applied, skipped)
	if err != nil {
		return err
	}
	if applied == 0 && skipped == 0 {
		fmt.Println("  실행할 명령이 없습니다 — `inbox` 탭에 적고 «실행»을 체크하세요.")
	}
	return nil
}
