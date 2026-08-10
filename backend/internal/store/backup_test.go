package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// 백업 → 복원 왕복. **이 테스트의 값은 무엇을 비교하느냐에 있다.**
//
// 12-BACKLOG가 `pushpoint export`를 자르며 적어 둔 함정이 정확히 여기다: URL만 왕복시키는
// 왕복 테스트는 **안전망이 아니라 잘못된 안심**이다. 그래서 링크 수가 아니라 링크의 내용,
// 태그 연결, 메모, 열람 시각, 그리고 **FTS로 실제 검색이 되는지**까지 본다 — 복원된 것이
// 원본과 같은 파일인지가 아니라 **같은 앱인지**를 묻는 것이다.
func TestBackupRestore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	src := t.TempDir()

	db, err := Open(src)
	if err != nil {
		t.Fatalf("원본 열기: %v", err)
	}
	s := New(db, &fakeQueue{})

	id, _, _, err := s.SaveLink(ctx, SaveInput{
		URL:   "https://kubernetes.io/docs/concepts/overview/",
		Title: "쿠버네티스 개요",
	})
	if err != nil {
		t.Fatalf("저장: %v", err)
	}
	note := "복원 뒤에도 남아 있어야 하는 메모"
	if _, err := s.UpdateLink(ctx, id, &note, []string{"devops", "backend"}); err != nil {
		t.Fatalf("메모·태그: %v", err)
	}
	if err := s.MarkOpened(ctx, id); err != nil {
		t.Fatalf("열람 표시: %v", err)
	}

	backup := filepath.Join(t.TempDir(), "archive.db")
	if err := db.Backup(ctx, backup); err != nil {
		t.Fatalf("백업: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("원본 닫기: %v", err)
	}

	// **빈 데이터 디렉터리에 복원한다.** 원본 위에 복원하면 아무것도 안 하고도 통과할 수
	// 있다 — 그건 복원이 아니라 그 자리에 있던 파일을 다시 읽은 것이다.
	dst := t.TempDir()
	if err := StageRestore(dst, backup); err != nil {
		t.Fatalf("복원 대기: %v", err)
	}
	restored, err := Open(dst)
	if err != nil {
		t.Fatalf("복원 후 열기: %v", err)
	}
	defer restored.Close()
	rs := New(restored, &fakeQueue{})

	got, err := rs.GetLink(ctx, id)
	if err != nil {
		t.Fatalf("복원된 링크 조회: %v", err)
	}
	if got.Title != "쿠버네티스 개요" {
		t.Errorf("제목: got %q", got.Title)
	}
	if got.Note != "복원 뒤에도 남아 있어야 하는 메모" {
		t.Errorf("메모가 안 왔다: got %q", got.Note)
	}
	if got.OpenedAt == nil {
		t.Error("열람 시각이 안 왔다 — 되살림 후보 판정이 복원 뒤에 달라진다")
	}
	names := map[string]bool{}
	for _, tag := range got.Tags {
		names[tag.Name] = true
	}
	if !names["devops"] || !names["backend"] {
		t.Errorf("태그가 안 왔다: got %v", got.Tags)
	}

	// **FTS까지 왔는지 본다.** 행만 옮기고 색인이 비면 앱은 멀쩡해 보이는데 검색만 조용히
	// 아무것도 못 찾는다 — 백업의 가치가 사라지는 실패인데 화면에서는 "저장한 적 없나 보다"와
	// 구분되지 않는다.
	hits, _, _, err := rs.Search(ctx, "쿠버네티스", "", nil, nil, "", 10)
	if err != nil {
		t.Fatalf("복원 후 검색: %v", err)
	}
	if len(hits) == 0 {
		t.Error("복원 후 검색이 0건 — FTS 색인이 백업에 안 실렸다")
	}
}

// 아무 파일이나 받으면 아카이브가 조용히 빈 앱이 된다. 거절은 **받는 자리에서** 해야
// 원본이 남는다.
func TestStageRestore_RejectsForeignFile(t *testing.T) {
	dir := t.TempDir()
	junk := filepath.Join(dir, "photos.db")
	if err := os.WriteFile(junk, []byte("이건 우리 DB가 아니다"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := StageRestore(t.TempDir(), junk); err == nil {
		t.Fatal("남의 파일을 받아들였다")
	}
}

// 대기 파일이 망가져 있으면 **앱은 그래도 떠야 한다.** 원본으로 뜨고 대기 파일만 치운다 —
// 못 쓰는 파일 하나 때문에 아카이브에 영영 못 들어가는 것이 최악이다.
func TestOpen_IgnoresBrokenStagedRestore(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, &fakeQueue{})
	if _, _, _, err := s.SaveLink(context.Background(), SaveInput{URL: "https://example.com/keep-me"}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, restoreName), []byte("깨진 파일"), 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err == nil {
		defer reopened.Close()
		t.Fatal("깨진 대기 파일인데 오류를 안 냈다 — 사용자가 알 방법이 없다")
	}

	// 대기 파일은 치워졌어야 하고, 그래서 다음 기동은 원본으로 성공해야 한다.
	if _, err := os.Stat(filepath.Join(dir, restoreName)); !os.IsNotExist(err) {
		t.Error("깨진 대기 파일이 안 치워졌다 — 다음 기동도 같은 이유로 실패한다")
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatalf("원본으로 다시 열기: %v", err)
	}
	defer again.Close()
	n, _, err := New(again, &fakeQueue{}).ListLinks(context.Background(), "", 10, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(n) != 1 {
		t.Errorf("원본이 안 남았다: %d건", len(n))
	}
}
