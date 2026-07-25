package ppcore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 진단 로그가 실제로 파일에 남아야 한다.
//
// 기기에서 앱의 stderr는 버려진다 — internal/app의 "log and continue" 경로(dispatcher
// 사망, store close 실패)가 전부 읽을 수 없는 곳으로 가면 자립 모드는 진단이 불가능해진다.
// 그래서 "로그가 남는다"는 것 자체가 검증 대상이다.
func TestLoggerWritesToDataDir(t *testing.T) {
	dir := t.TempDir()
	logger := newLogger(dir)
	logger.Info("테스트 기록", "key", "value")

	data, err := os.ReadFile(filepath.Join(dir, logFileName))
	if err != nil {
		t.Fatalf("로그 파일이 없다 — 기기에서 진단 정보가 사라진다: %v", err)
	}
	if !strings.Contains(string(data), "테스트 기록") {
		t.Errorf("기록한 메시지가 없다: %s", data)
	}
}

// 상한을 넘으면 다음 시작 때 비운다 — 개인 기기에서 로그가 무한히 자라면 안 된다.
func TestLoggerTruncatesOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, logFileName)
	if err := os.WriteFile(path, make([]byte, maxLogBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	newLogger(dir).Info("새 실행")

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > maxLogBytes {
		t.Errorf("상한을 넘은 로그가 그대로 남았다: %d바이트", fi.Size())
	}
}

// 파일을 열 수 없어도 서버는 떠야 한다 — 로그는 진단 수단이지 기동 조건이 아니다.
func TestLoggerFallsBackWhenFileUnavailable(t *testing.T) {
	// 디렉터리 자리에 파일을 두면 그 안에 로그 파일을 만들 수 없다.
	dir := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(dir, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	if logger := newLogger(dir); logger == nil {
		t.Fatal("파일을 못 열어도 로거는 반환돼야 한다")
	}
}
