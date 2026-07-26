package main

// sheets-setup — Google 스프레드시트 연결을 한 번에 끝낸다.
//
//	pushpoint sheets-setup
//
// **왜 안내형 명령인가.** 구글은 사용자가 무언가를 직접 승인하는 단계를 없앨 수 없게 해
// 뒀다. 그 한 단계는 남지만, 나머지 — 클라우드 프로젝트, API 켜기, 서비스 계정, JSON 키,
// 시트 ID 복사 — 는 전부 없앨 수 있다. 그래서 이 명령이 스크립트를 만들어 클립보드에 넣고,
// 브라우저를 열고, 사용자는 붙여넣고 배포한 뒤 URL 하나만 되돌려 준다.
//
// 사용자가 하는 일: **붙여넣기 한 번, 배포 클릭, URL 붙여넣기 한 번.**

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/coby/push-point/backend/internal/sheets"
	"github.com/coby/push-point/backend/internal/sheetsync"
)

func runSheetsSetup(_ []string) error {
	if err := os.MkdirAll(dataDir(), 0o755); err != nil {
		return fmt.Errorf("sheets-setup: 데이터 디렉터리 준비 실패: %w", err)
	}

	token, err := sheets.NewToken()
	if err != nil {
		return err
	}
	script := sheets.AppsScript(token)

	// 스크립트를 파일로도 남긴다. 클립보드가 안 되는 환경이 있고, 붙여넣기를 실수했을 때
	// 다시 열 자리가 필요하다.
	scriptPath := dataDir() + "/sheets-script.gs"
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return fmt.Errorf("sheets-setup: 스크립트 저장 실패: %w", err)
	}
	copied := copyToClipboard(script)

	fmt.Println()
	fmt.Println("Google 스프레드시트 연결 — 1분이면 끝납니다.")
	fmt.Println()
	if copied {
		fmt.Println("  스크립트를 클립보드에 넣었습니다. 아래 순서로 붙여넣기만 하면 됩니다.")
	} else {
		fmt.Printf("  스크립트: %s  (열어서 전체 복사하세요)\n", scriptPath)
	}
	fmt.Println()
	fmt.Println("  1. 브라우저에 새 시트가 열립니다 (안 열리면 sheets.new 로 직접)")
	fmt.Println("  2. 메뉴에서  확장 프로그램 → Apps Script")
	fmt.Println("  3. 편집기의 내용을 전부 지우고 붙여넣기 (Cmd+A → Cmd+V)")
	fmt.Println("  4. 저장(Cmd+S) 후  배포 → 새 배포 → 유형: 웹 앱")
	fmt.Println("       - 실행 계정: 나")
	fmt.Println("       - 액세스 권한: 모든 사용자   ← 이걸 빼먹으면 로그인 페이지가 옵니다")
	fmt.Println("  5. 배포를 누르고 권한을 승인한 뒤, 나온 **웹 앱 URL**을 복사")
	fmt.Println()
	openBrowser("https://sheets.new")

	fmt.Print("웹 앱 URL을 여기에 붙여넣고 Enter: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return fmt.Errorf("sheets-setup: URL을 받지 못했습니다")
	}
	deployURL := strings.TrimSpace(line)
	if deployURL == "" {
		return fmt.Errorf("sheets-setup: URL이 비어 있습니다")
	}

	wh, err := sheets.NewWebhook(deployURL, token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	fmt.Println("\n연결을 확인하는 중…")
	title, sheetURL, err := wh.Ping(ctx)
	if err != nil {
		return fmt.Errorf("%w\n\n스크립트는 %s 에 남아 있습니다. 배포 설정을 고친 뒤 다시 실행하세요", err, scriptPath)
	}

	st := sheetsync.State{
		Mode:      "webhook",
		DeployURL: deployURL,
		Token:     token,
		SheetURL:  sheetURL,
		CreatedAt: time.Now().Unix(),
	}
	if err := sheetsync.Save(dataDir(), st); err != nil {
		return fmt.Errorf("sheets-setup: 연결 정보 저장 실패: %w", err)
	}

	fmt.Printf("연결됨 — %q\n  %s\n\n", title, sheetURL)
	fmt.Println("이제 `just sheets-sync` 로 언제든 내보낼 수 있습니다.")
	fmt.Println("웹 앱의 설정 화면에도 동기화 버튼이 있습니다.")

	// 바로 한 번 돌린다. "연결됐다"보다 "시트에 실제로 들어갔다"가 확실한 확인이다.
	fmt.Println("\n첫 동기화를 실행합니다…")
	return runSheetsSync(nil)
}

// copyToClipboard는 되면 하고 안 되면 조용히 넘어간다 — 클립보드는 편의이지 전제가 아니다.
func copyToClipboard(s string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		}
	}
	if cmd == nil {
		return false
	}
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run() == nil
}

// openBrowser도 마찬가지로 best-effort다. 안내문에 URL이 있으므로 실패해도 막히지 않는다.
func openBrowser(rawURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	default:
		return
	}
	_ = cmd.Start()
}
