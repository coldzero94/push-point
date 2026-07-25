package ppcore

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// logFileName은 dataDir 안에 남기는 진단 로그. App Group 컨테이너라 앱과 확장이 모두
// 볼 수 있고, 파일 공유로 꺼내 볼 수도 있다.
const logFileName = "pushpoint.log"

// maxLogBytes를 넘으면 다음 시작 때 파일을 비운다. 회전 대신 절단인 이유: 개인 기기의
// 진단 로그이고, 최근 실행 것만 있으면 충분하며, 회전 로직 자체가 새 실패 지점이 된다.
const maxLogBytes = 1 << 20 // 1MB

// newLogger는 dataDir/pushpoint.log에 쓰는 로거를 만든다.
//
// **stderr에 쓰면 안 된다.** Xcode에 연결된 동안에는 콘솔에 보이지만, 기기에서 평소처럼
// 실행하면 앱의 stderr는 버려지고 os_log로도 가지 않는다. internal/app이 남기는
// dispatcher 사망·store close 실패 같은 신호가 전부 읽을 수 없는 fd로 사라진다는 뜻이라,
// 자립 모드에서는 "로그로만 남긴다"가 곧 "아무 데도 안 남긴다"가 된다.
//
// 파일도 열지 못하면 그때는 stderr로 폴백한다 — 시뮬레이터·Xcode 실행에서는 그래도 보인다.
func newLogger(dataDir string) *slog.Logger {
	path := filepath.Join(dataDir, logFileName)
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogBytes {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		l.Warn("진단 로그 파일을 열 수 없어 stderr로 폴백한다 (기기에서는 보이지 않는다)",
			"path", path, "err", err)
		return l
	}
	// 파일 핸들은 프로세스 수명 동안 유지한다 — 앱이 죽으면 OS가 닫는다. 로거를 닫는
	// 시점을 따로 만들면 Stop 이후에 오는 지각 로그가 닫힌 fd에 쓰게 된다.
	var w io.Writer = f
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
