package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// tagID는 시드 사전에서 이름으로 tag_id를 찾는다.
func tagID(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	var id int64
	if err := db.Reader.QueryRow(`SELECT id FROM tags WHERE name = ?`, name).Scan(&id); err != nil {
		t.Fatalf("tagID(%q) 실패: %v", name, err)
	}
	return id
}

func TestGetLinkContent(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()

	id, _, _, err := s.SaveLink(ctx, SaveInput{URL: "https://example.com/a", Note: "내 메모"})
	if err != nil {
		t.Fatalf("SaveLink: %v", err)
	}
	// 스크랩 전 — title/description 빈 값, note는 저장값.
	c, err := s.GetLinkContent(ctx, id)
	if err != nil {
		t.Fatalf("GetLinkContent: %v", err)
	}
	if c.Note != "내 메모" || c.Title != "" {
		t.Errorf("스크랩 전 콘텐츠 = %+v", c)
	}
	// 스크랩 후 — title/description/body_text 반영.
	if err := s.ApplyScrape(ctx, id, ScrapeResult{Title: "쿠버네티스 입문", Description: "k8s 튜토리얼", ContentType: "article", BodyText: "본문 내용 추출됨"}); err != nil {
		t.Fatalf("ApplyScrape: %v", err)
	}
	c, _ = s.GetLinkContent(ctx, id)
	if c.Title != "쿠버네티스 입문" || c.Description != "k8s 튜토리얼" || c.Body != "본문 내용 추출됨" {
		t.Errorf("스크랩 후 콘텐츠 = %+v", c)
	}

	// 삭제/부재 → ErrNotFound.
	if _, err := s.GetLinkContent(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("부재 링크 = %v, want ErrNotFound", err)
	}
	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetLinkContent(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("삭제 링크 = %v, want ErrNotFound", err)
	}
}

func TestLoadTagDict(t *testing.T) {
	s, _, _ := newTestStore(t)
	entries, err := s.LoadTagDict(context.Background())
	if err != nil {
		t.Fatalf("LoadTagDict: %v", err)
	}
	// 개수 자체보다 "시드가 실제로 들어왔나"가 요점이다. 사전은 늘어난다 —
	// tags.json과의 정확한 일치는 just dict-lint이 지키므로 여기서는 하한만 본다.
	if len(entries) < 30 {
		t.Fatalf("시드 태그가 비었거나 모자란다: %d개", len(entries))
	}
	// aliases가 디코드돼야 (kubernetes → 쿠버네티스 포함).
	for _, e := range entries {
		if e.Name == "kubernetes" {
			found := false
			for _, a := range e.Aliases {
				if a == "쿠버네티스" {
					found = true
				}
			}
			if !found {
				t.Errorf("kubernetes aliases 디코드 실패: %v", e.Aliases)
			}
		}
	}
}

func TestApplyTags(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://example.com/k", Note: ""})
	kube := tagID(t, db, "kubernetes")
	dev := tagID(t, db, "dev")

	// rules 태그 부착 — source='rules' + confidence 저장.
	if err := s.ApplyTags(ctx, id, []ScoredTag{{TagID: kube, Confidence: 0.75}}); err != nil {
		t.Fatalf("ApplyTags: %v", err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM link_tags WHERE link_id=? AND source='rules'`, id); n != 1 {
		t.Fatalf("rules 태그 1개여야, got %d", n)
	}
	var conf float64
	if err := db.Reader.QueryRow(`SELECT confidence FROM link_tags WHERE link_id=? AND tag_id=?`, id, kube).Scan(&conf); err != nil || conf != 0.75 {
		t.Errorf("confidence = %v (%v), want 0.75", conf, err)
	}
	// FTS 'tags' 컬럼에 태그명 반영(재색인).
	var ftsTags string
	if err := db.Reader.QueryRow(`SELECT tags FROM links_fts WHERE rowid=?`, id).Scan(&ftsTags); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ftsTags, "kubernetes") {
		t.Errorf("FTS tags에 kubernetes 없음: %q", ftsTags)
	}

	// 사용자(manual) 태그는 보존 — 재태깅이 rules만 지운다.
	if _, err := db.Writer.Exec(`INSERT INTO link_tags (link_id, tag_id, source) VALUES (?, ?, 'manual')`, id, dev); err != nil {
		t.Fatal(err)
	}
	// 다른 rules 세트로 재적용(kube 빠지고 없음) → manual(dev)은 남고, 기존 rules(kube)는 사라짐.
	if err := s.ApplyTags(ctx, id, []ScoredTag{}); err != nil {
		t.Fatalf("재ApplyTags: %v", err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM link_tags WHERE link_id=? AND source='manual'`, id); n != 1 {
		t.Errorf("manual 태그 보존돼야, got %d", n)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM link_tags WHERE link_id=? AND source='rules'`, id); n != 0 {
		t.Errorf("rules 태그 교체돼야(0), got %d", n)
	}

	// 같은 태그의 manual이 있으면 rules INSERT는 ON CONFLICT로 스킵 → manual 우선 보존.
	if err := s.ApplyTags(ctx, id, []ScoredTag{{TagID: dev, Confidence: 0.5}}); err != nil {
		t.Fatalf("ApplyTags(dev): %v", err)
	}
	var src string
	if err := db.Reader.QueryRow(`SELECT source FROM link_tags WHERE link_id=? AND tag_id=?`, id, dev).Scan(&src); err != nil {
		t.Fatal(err)
	}
	if src != "manual" {
		t.Errorf("같은 태그 manual 우선 보존돼야, got source=%q", src)
	}
}

func TestApplyScrapeEnqueuesTag(t *testing.T) {
	s, db, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://example.com/t", Note: ""})

	// og:image 없어도(HasImage=false) tag 잡은 무조건 enqueue.
	if err := s.ApplyScrape(ctx, id, ScrapeResult{Title: "t", ContentType: "article", HasImage: false}); err != nil {
		t.Fatalf("ApplyScrape: %v", err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM jobs WHERE kind='tag' AND link_id=?`, id); n != 1 {
		t.Errorf("tag 잡 1개 enqueue돼야, got %d", n)
	}
}

// SetSummary 왕복 — GetLink가 쓴 값을 그대로 돌려주는지 본다. GetLink의 SELECT는 18컬럼
// 위치 기반 Scan이라 컬럼을 하나 덧붙일 때 정렬이 어긋나기 쉬운데, 그걸 잡는 테스트다.
func TestSetSummaryRoundTrip(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://example.com/s", Note: "메모"})

	// 저장 직후엔 빈 문자열(기본값).
	d, err := s.GetLink(ctx, id)
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if d.Summary != "" {
		t.Errorf("초기 summary = %q, want 빈 문자열", d.Summary)
	}

	const want = "첫 문장이다.\n둘째 문장이다."
	if err := s.SetSummary(ctx, id, want); err != nil {
		t.Fatalf("SetSummary: %v", err)
	}
	d, _ = s.GetLink(ctx, id)
	if d.Summary != want {
		t.Errorf("summary = %q, want %q", d.Summary, want)
	}
	// 다른 필드가 밀리지 않았는지(위치 Scan 어긋남 감지) 함께 확인한다.
	if d.URL != "https://example.com/s" || d.Note != "메모" {
		t.Errorf("다른 컬럼이 어긋남: url=%q note=%q", d.URL, d.Note)
	}

	// 빈 문자열로 덮어쓰기 — 재태깅에서 가드에 걸리면 이전 요약이 남으면 안 된다.
	if err := s.SetSummary(ctx, id, ""); err != nil {
		t.Fatalf("SetSummary(빈값): %v", err)
	}
	d, _ = s.GetLink(ctx, id)
	if d.Summary != "" {
		t.Errorf("빈값 덮어쓰기 실패: %q", d.Summary)
	}

	// 삭제된 링크는 ErrNotFound.
	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSummary(ctx, id, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("삭제된 링크 = %v, want ErrNotFound", err)
	}
}
