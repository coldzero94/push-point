package sheetsync

import (
	"os"
	"strings"
	"testing"
)

// 연결 정보는 토큰을 담으므로 **소유자만 읽을 수 있어야** 한다.
// 배포 URL과 토큰이 함께 새면 누구나 그 시트를 덮어쓸 수 있다.
func TestSave_isOwnerOnly(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, State{Mode: "webhook", DeployURL: "https://x", Token: "s3cret"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("연결 정보 권한이 %o — 토큰이 들어 있으므로 0600이어야 한다", perm)
	}
}

func TestSaveLoad_roundTrip(t *testing.T) {
	dir := t.TempDir()
	want := State{
		Mode: "webhook", DeployURL: "https://script.google.com/x", Token: "tok",
		SheetURL: "https://docs.google.com/y", LastRows: 42, LastError: "지난번 실패",
	}
	if err := Save(dir, want); err != nil {
		t.Fatal(err)
	}
	got := Load(dir)
	if got != want {
		t.Errorf("왕복이 어긋남:\n got %+v\nwant %+v", got, want)
	}
}

// 상태 파일이 깨졌을 때 죽지 않아야 한다 — 죽으면 다시 연결할 방법까지 막힌다.
// 없는 것으로 보고 넘어가면 `just sheets-setup`이 복구한다.
func TestLoad_brokenFileIsTreatedAsAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(statePath(dir), []byte("{이건 JSON이 아니다"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Load(dir); got.Mode != "" || got.DeployURL != "" {
		t.Errorf("깨진 파일에서 값을 읽었다: %+v", got)
	}
}

// 연결 전에는 **에러 메시지가 무엇을 하라고 알려줘야** 한다. 여기가 사용자가
// 이 기능을 처음 만나는 자리이고, "설정이 없습니다"로 끝나면 다음 행동을 모른다.
func TestOpen_unconnectedTellsWhatToDo(t *testing.T) {
	t.Setenv("PUSHPOINT_SHEETS_KEY", "")
	_, _, err := Open(t.TempDir())
	if err == nil {
		t.Fatal("연결 안 된 상태인데 성공했다")
	}
	if !strings.Contains(err.Error(), "sheets-setup") {
		t.Errorf("다음에 뭘 할지 안 알려준다: %v", err)
	}
}

// 웹훅 정보가 있으면 서비스 계정 환경변수가 있어도 웹훅이 이긴다 —
// 웹훅이 기본 경로이고, 옛 환경변수가 남아 있다고 그쪽으로 끌려가면 안 된다.
func TestOpen_webhookWinsOverServiceAccount(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, State{Mode: "webhook", DeployURL: "https://script.google.com/x", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PUSHPOINT_SHEETS_KEY", "/존재하지/않는/키.json")

	tr, _, err := Open(dir)
	if err != nil {
		t.Fatalf("웹훅으로 열려야 한다: %v", err)
	}
	if tr == nil {
		t.Fatal("전송이 nil이다")
	}
}

// 실패했을 때 인터페이스가 non-nil인데 안이 nil이면, 나중에 `tr != nil`로 판단하는
// 코드가 첫 호출에서 패닉한다. 지금 호출자는 err를 먼저 보지만 함정은 남는다.
func TestOpen_returnsNilTransportOnError(t *testing.T) {
	dir := t.TempDir()
	// https가 아닌 URL이면 NewWebhook이 거부한다.
	if err := Save(dir, State{Mode: "webhook", DeployURL: "http://insecure", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	tr, _, err := Open(dir)
	if err == nil {
		t.Fatal("http URL을 받아들였다")
	}
	if tr != nil {
		t.Error("실패했는데 전송이 non-nil이다 — typed-nil 함정")
	}
}

// 우리가 소유한 마지막 열은 헤더 폭과 일치해야 한다. 어긋나면 clear 범위가
// 헤더보다 좁아 옛 데이터가 남거나, 넓어 사용자 열을 지운다.
func TestLastCol_matchesHeaderWidth(t *testing.T) {
	// A=1 … I=9
	want := len(Header)
	got := int(LastCol[0]-'A') + 1
	if got != want {
		t.Errorf("Header가 %d열인데 LastCol이 %s(%d열)다 — 열을 더했으면 LastCol도 옮겨야 한다",
			want, LastCol, got)
	}
}

// 화면의 "연결됨" 판정이 실제 동작과 같아야 한다.
//
// 서비스 계정으로 멀쩡히 동기화되는 서버가 웹에서 "연결 안 됨"으로 보이면, 화면은
// `sheets-setup`을 안내하고 그걸 따르면 State가 통째로 교체돼 서비스 계정 경로가
// 다시는 선택되지 않는다. 실제로 그런 상태였다.
func TestConnected_matchesWhatOpenAccepts(t *testing.T) {
	cases := []struct {
		name  string
		state State
		key   string
		id    string
		want  bool
	}{
		{"아무것도 없음", State{}, "", "", false},
		{"웹훅", State{Mode: "webhook", DeployURL: "https://x"}, "", "", true},
		{"서비스 계정 + 환경변수 ID", State{}, "/k.json", "SHEET", true},
		{"서비스 계정 + 기억해 둔 ID", State{SpreadsheetID: "SHEET"}, "/k.json", "", true},
		{"키만 있고 시트를 모름", State{}, "/k.json", "", false},
		{"시트만 있고 키가 없음", State{SpreadsheetID: "SHEET"}, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PUSHPOINT_SHEETS_KEY", tc.key)
			t.Setenv("PUSHPOINT_SHEETS_ID", tc.id)
			if got := Connected(tc.state); got != tc.want {
				t.Errorf("Connected=%v, want %v", got, tc.want)
			}
		})
	}
}
