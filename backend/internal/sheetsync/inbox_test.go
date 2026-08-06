package sheetsync

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeTransport는 탭별 내용을 메모리에 들고 있다. 실제로 무엇이 써졌는지 봐야 하므로
// 쓰기를 기록한다 — "인박스에 되쓰지 않는다"가 규칙이라 그걸 검사할 수 있어야 한다.
type fakeTransport struct {
	tabs     map[string][][]string
	replaced []string // Replace가 불린 탭 이름들, 순서대로
	readErr  error
}

func newFake() *fakeTransport {
	return &fakeTransport{tabs: map[string][][]string{}}
}

func (f *fakeTransport) Read(_ context.Context, tab string) ([][]string, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.tabs[tab], nil
}

func (f *fakeTransport) Replace(_ context.Context, tab string, rows [][]any) error {
	f.replaced = append(f.replaced, tab)
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		cells := make([]string, 0, len(r))
		for _, c := range r {
			cells = append(cells, toStr(c))
		}
		out = append(out, cells)
	}
	f.tabs[tab] = out
	return nil
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int64:
		return string(rune('0' + x)) // 테스트에서 id는 한 자리만 쓴다
	default:
		return ""
	}
}

// recordingApplier는 무엇이 적용됐는지만 기록한다. 실패를 흉내 낼 수 있어야
// "실패해도 원장에 안 들어간다"를 볼 수 있다.
type recordingApplier struct {
	applied []Command
	failOn  Action
}

func (r *recordingApplier) Before(context.Context, Command) string { return "이전값" }
func (r *recordingApplier) Apply(_ context.Context, c Command) error {
	if c.Action == r.failOn {
		return errors.New("일부러 실패")
	}
	r.applied = append(r.applied, c)
	return nil
}

func header() []string { return []string{"id", "작업", "값", "확인 URL", "실행"} }

// **규칙 1** — `실행`이 켜진 행만 실행된다. 이 한 칸이 "타자 치는 중에 발화"를 막는다.
func TestOnlyCheckedRowsRun(t *testing.T) {
	f := newFake()
	f.tabs[InboxTab] = [][]string{
		header(),
		{"1", "메모", "고친 메모", "", "TRUE"},
		{"2", "메모", "아직 쓰는 중", "", ""},     // 체크 안 됨
		{"3", "메모", "이것도 아직", "", "FALSE"}, // 명시적 FALSE
	}
	ap := &recordingApplier{}
	applied, _, err := RunInbox(context.Background(), f, t.TempDir(), ap)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 1 || len(ap.applied) != 1 {
		t.Fatalf("체크된 한 행만 돌아야 한다: applied=%d, %v", applied, ap.applied)
	}
	if ap.applied[0].LinkID != 1 {
		t.Errorf("엉뚱한 행이 돌았다: %+v", ap.applied[0])
	}
}

// **규칙 2** — 인박스에 되쓰지 않는다. 결과는 로그 탭으로만 간다.
func TestNeverWritesBackToInbox(t *testing.T) {
	f := newFake()
	f.tabs[InboxTab] = [][]string{header(), {"1", "메모", "값", "", "TRUE"}}
	if _, _, err := RunInbox(context.Background(), f, t.TempDir(), &recordingApplier{}); err != nil {
		t.Fatal(err)
	}
	for _, tab := range f.replaced {
		if tab == InboxTab {
			t.Fatal("인박스에 되썼다 — 사람 입력을 덮을 수 있다")
		}
	}
	if len(f.tabs[LogTab]) != 2 { // 머리글 + 한 줄
		t.Errorf("로그가 한 줄이어야 한다: %v", f.tabs[LogTab])
	}
}

// **규칙 3** — 같은 명령은 두 번 실행되지 않는다. 그리고 그 판정은 **행 번호가 아니라
// 내용**이라, 위에 행을 끼워 넣어 번호가 밀려도 다시 실행되지 않는다.
func TestAppliedOnceEvenWhenRowsMove(t *testing.T) {
	dir := t.TempDir()
	f := newFake()
	f.tabs[InboxTab] = [][]string{header(), {"1", "메모", "값", "", "TRUE"}}
	ap := &recordingApplier{}
	if _, _, err := RunInbox(context.Background(), f, dir, ap); err != nil {
		t.Fatal(err)
	}

	// 위에 새 행을 끼운다 — 원래 명령의 **행 번호가 바뀐다**.
	f.tabs[InboxTab] = [][]string{
		header(),
		{"9", "재시도", "", "", "TRUE"},
		{"1", "메모", "값", "", "TRUE"}, // 아까 것, 이제 3행
	}
	applied, skipped, err := RunInbox(context.Background(), f, dir, ap)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 1 || skipped != 1 {
		t.Fatalf("새 것만 돌고 옛 것은 건너뛰어야 한다: applied=%d skipped=%d", applied, skipped)
	}
}

// **규칙 3의 뒷면** — 행을 지우면 원장에서도 빠져서, 같은 명령을 다시 쓸 수 있다.
func TestDeletingTheRowLetsTheCommandRunAgain(t *testing.T) {
	dir := t.TempDir()
	f := newFake()
	cmd := []string{"1", "메모", "값", "", "TRUE"}
	f.tabs[InboxTab] = [][]string{header(), cmd}
	ap := &recordingApplier{}
	if _, _, err := RunInbox(context.Background(), f, dir, ap); err != nil {
		t.Fatal(err)
	}

	f.tabs[InboxTab] = [][]string{header()} // 사람이 행을 지웠다
	if _, _, err := RunInbox(context.Background(), f, dir, ap); err != nil {
		t.Fatal(err)
	}

	f.tabs[InboxTab] = [][]string{header(), cmd} // 같은 명령을 다시 썼다
	applied, _, err := RunInbox(context.Background(), f, dir, ap)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatal("지웠다 다시 쓴 명령이 실행되지 않았다 — 원장이 영원히 기억하고 있다")
	}
}

// 실패한 명령은 원장에 들어가지 않는다. 들어가면 고쳐서 다시 체크해도 영영 안 돈다.
func TestFailedCommandIsNotRemembered(t *testing.T) {
	dir := t.TempDir()
	f := newFake()
	f.tabs[InboxTab] = [][]string{header(), {"1", "메모", "값", "", "TRUE"}}

	failing := &recordingApplier{failOn: ActionNote}
	if _, _, err := RunInbox(context.Background(), f, dir, failing); err != nil {
		t.Fatal(err)
	}
	if got := f.tabs[LogTab][1][4]; !strings.HasPrefix(got, "실패") {
		t.Errorf("로그에 실패가 남아야 한다: %q", got)
	}

	ok := &recordingApplier{}
	applied, _, err := RunInbox(context.Background(), f, dir, ok)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatal("실패한 명령이 원장에 들어가 다시 시도되지 않는다")
	}
}

// **같은 링크에 대한 서로 다른 명령은 서로 다른 명령이다.** 키가 링크 id로만 만들어지면
// 둘째가 "이미 적용됨"으로 건너뛰어지고, 사용자는 체크했는데 아무 일도 안 일어난 것을 본다.
//
// 이 케이스는 변이 검증이 찾아냈다 — 키를 id 기준으로 바꿔도 위 테스트들이 전부 통과했다.
func TestSameLinkDifferentCommandsBothRun(t *testing.T) {
	f := newFake()
	f.tabs[InboxTab] = [][]string{
		header(),
		{"1", "메모", "첫 번째", "", "TRUE"},
		{"1", "메모", "두 번째", "", "TRUE"}, // 같은 링크, 같은 작업, **다른 값**
		{"1", "재시도", "", "", "TRUE"},    // 같은 링크, 다른 작업
	}
	ap := &recordingApplier{}
	applied, _, err := RunInbox(context.Background(), f, t.TempDir(), ap)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 3 {
		t.Fatalf("셋 다 실행돼야 한다 — 키가 내용이 아니라 id로 만들어졌다: applied=%d, %v", applied, ap.applied)
	}
}

// 빈 인박스는 머리글을 세워 둔다 — 무엇을 채워야 하는지 화면에 있어야 시작할 수 있다.
func TestEmptyInboxGetsHeader(t *testing.T) {
	f := newFake()
	if _, _, err := RunInbox(context.Background(), f, t.TempDir(), &recordingApplier{}); err != nil {
		t.Fatal(err)
	}
	got := f.tabs[InboxTab]
	if len(got) != 1 || got[0][4] != "실행" {
		t.Fatalf("머리글이 세워져야 하고 마지막 열이 실행이어야 한다: %v", got)
	}
}

// 열을 끼워 모양이 달라진 행은 **적용하지 않는다.** 잘못 적용하는 것보다 안 하는 쪽이 안전하다.
func TestMalformedRowIsSkipped(t *testing.T) {
	f := newFake()
	f.tabs[InboxTab] = [][]string{
		header(),
		{"1", "메모", "값", "TRUE"},       // 열이 하나 모자람
		{"1", "없는작업", "값", "", "TRUE"}, // 모르는 작업
		{"abc", "메모", "값", "", "TRUE"}, // id가 숫자가 아님
	}
	ap := &recordingApplier{}
	applied, _, err := RunInbox(context.Background(), f, t.TempDir(), ap)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("모양이 다른 행은 실행되면 안 된다: %v", ap.applied)
	}
}
