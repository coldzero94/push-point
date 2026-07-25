package store

import (
	"context"
	"testing"
)

// 클라이언트 캡처의 핵심 계약: 서버가 못 가져오는 페이지라서 클라이언트가 준 값이므로
// 이후 어떤 서버 경로도 그 값을 덮어쓰지 않는다.
func TestClientCapture_notOverwrittenByScrape(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()

	id, _, dup, err := s.SaveLink(ctx, SaveInput{
		URL: "https://spa.example/a", Note: "메모",
		Title: "클라이언트 제목", Description: "클라이언트 설명", BodyText: "클라이언트가 캡처한 본문이다.",
	})
	if err != nil || dup {
		t.Fatalf("SaveLink: %v dup=%v", err, dup)
	}
	var src string
	if err := db.Reader.QueryRow(`SELECT body_source FROM links WHERE id=?`, id).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if src != "client" {
		t.Fatalf("body_source = %q, want client", src)
	}
	// 클라이언트 본문이 있으면 스크랩을 기다리지 않고 tag 잡이 바로 생겨야 한다 —
	// 그러지 않으면 스크랩이 실패하는 바로 그 페이지에서 태그·요약이 영원히 안 생긴다.
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE kind='tag' AND link_id=?`, id); n != 1 {
		t.Errorf("tag 잡이 저장 시점에 enqueue돼야, got %d", n)
	}

	// 스크랩이 뒤늦게 (더 나쁜) 결과를 들고 와도 3필드를 덮지 않는다.
	if err := s.ApplyScrape(ctx, id, ScrapeResult{
		Title: "서버 제목", Description: "서버 설명", ContentType: "article", BodyText: "서버 본문",
	}); err != nil {
		t.Fatalf("ApplyScrape: %v", err)
	}
	c, _ := s.GetLinkContent(ctx, id)
	if c.Title != "클라이언트 제목" || c.Description != "클라이언트 설명" || c.Body != "클라이언트가 캡처한 본문이다." {
		t.Errorf("스크랩이 클라이언트 값을 덮어씀: %+v", c)
	}
}

// 서버 경로의 독립 버그: 추출 실패(빈 본문)가 멀쩡한 기존 본문을 지우면 안 된다.
func TestApplyScrape_emptyBodyDoesNotErase(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://a.example/x"})

	if err := s.ApplyScrape(ctx, id, ScrapeResult{Title: "t", ContentType: "article", BodyText: "처음 추출된 본문"}); err != nil {
		t.Fatal(err)
	}
	// 재시도에서 추출이 실패해 빈 본문이 왔다 — 기존 본문이 남아야 한다.
	if err := s.ApplyScrape(ctx, id, ScrapeResult{Title: "t2", ContentType: "article", BodyText: ""}); err != nil {
		t.Fatal(err)
	}
	c, _ := s.GetLinkContent(ctx, id)
	if c.Body != "처음 추출된 본문" {
		t.Errorf("빈 추출 결과가 기존 본문을 지움: %q", c.Body)
	}
	if c.Title != "t2" { // 제목은 서버 출처라 정상적으로 갱신된다
		t.Errorf("서버 출처 제목은 갱신돼야: %q", c.Title)
	}
}

// 이미 저장돼 있는(스크랩 실패한) 링크에 나중에 본문을 넣을 수 있어야 한다 — 이게 이 기능의
// 실제 동기다. 단방향: 클라이언트 본문이 이미 있으면 다시 덮지 않는다.
func TestClientCapture_backfillsExistingLink(t *testing.T) {
	s, db, fq := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://blocked.example/a", Note: "메모"})

	id2, _, dup, err := s.SaveLink(ctx, SaveInput{
		URL: "https://blocked.example/a", Title: "보충 제목", BodyText: "나중에 확장이 보낸 본문이다.",
	})
	if err != nil || !dup || id2 != id {
		t.Fatalf("중복 저장 = (%d, dup=%v, %v), want (%d, true, nil)", id2, dup, err, id)
	}
	c, _ := s.GetLinkContent(ctx, id)
	if c.Body != "나중에 확장이 보낸 본문이다." || c.Title != "보충 제목" {
		t.Errorf("보충되지 않음: %+v", c)
	}
	if c.Note != "메모" {
		t.Errorf("기존 메모가 사라짐: %q", c.Note)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE kind='tag' AND link_id=?`, id); n != 1 {
		t.Errorf("보충 후 tag 잡 1개여야, got %d", n)
	}
	if fq.wakes == 0 {
		t.Error("보충했으면 dispatcher를 깨워야 한다")
	}

	// 두 번째 보충 시도는 무동작이어야 한다(단방향 + 멱등 수렴).
	if _, _, _, err := s.SaveLink(ctx, SaveInput{
		URL: "https://blocked.example/a", Title: "다시", BodyText: "다른 본문",
	}); err != nil {
		t.Fatal(err)
	}
	c2, _ := s.GetLinkContent(ctx, id)
	if c2.Body != "나중에 확장이 보낸 본문이다." || c2.Title != "보충 제목" {
		t.Errorf("이미 클라이언트 본문인데 덮어씀: %+v", c2)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE kind='tag' AND link_id=?`, id); n != 1 {
		t.Errorf("두 번째 시도는 tag 잡을 더 만들면 안 됨, got %d", n)
	}
}

// undelete는 body_source를 무조건 리셋해야 한다 — 안 하면 옛 'client' 플래그가 남아
// 새 스크랩이 영원히 3필드를 못 쓴다.
func TestClientCapture_undeleteResetsSource(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://u.example/a", BodyText: "클라이언트 본문"})
	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatal(err)
	}
	// 본문 없이 재저장 → body_source가 ''로 돌아가야 한다.
	if _, _, _, err := s.SaveLink(ctx, SaveInput{URL: "https://u.example/a", Note: "새 메모"}); err != nil {
		t.Fatal(err)
	}
	var src string
	if err := db.Reader.QueryRow(`SELECT body_source FROM links WHERE id=?`, id).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if src != "" {
		t.Errorf("undelete 후 body_source = %q, want 빈 문자열", src)
	}
	// 이제 스크랩이 정상적으로 3필드를 쓸 수 있어야 한다.
	if err := s.ApplyScrape(ctx, id, ScrapeResult{Title: "서버 제목", ContentType: "article", BodyText: "서버 본문"}); err != nil {
		t.Fatal(err)
	}
	c, _ := s.GetLinkContent(ctx, id)
	if c.Title != "서버 제목" || c.Body != "서버 본문" {
		t.Errorf("undelete 후 스크랩이 막힘: %+v", c)
	}
}
