package store

import (
	"context"
	"errors"
	"testing"
)

// 열람 기록은 **사실 하나**만 남긴다. 횟수도, updated_at 변경도 없다.
//
// updated_at을 건드리지 않는 것이 중요하다 — 목록 정렬과 인스펙터의 "수정됨"이
// 그 값에 걸려 있어서, 링크를 열기만 해도 목록에서 위로 튀어 오르면 시간 축이 거짓말이 된다.
func TestMarkOpened_recordsWithoutTouchingUpdatedAt(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://open.example/a"})

	before, err := s.GetLink(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if before.OpenedAt != nil {
		t.Fatalf("저장 직후에 이미 열람 기록이 있다: %v", *before.OpenedAt)
	}

	if err := s.MarkOpened(ctx, id); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetLink(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.OpenedAt == nil {
		t.Fatal("열람이 기록되지 않았다")
	}
	if after.UpdatedAt != before.UpdatedAt {
		t.Errorf("열람이 updated_at을 올렸다 — 목록 정렬과 \"수정됨\"이 흔들린다: %d → %d",
			before.UpdatedAt, after.UpdatedAt)
	}
}

// 삭제된 링크에는 기록하지 않는다 — 상세도 목록도 못 여는 링크를 "열었다"고 적을 수 없다.
func TestMarkOpened_refusesDeleted(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	id, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://open.example/gone"})
	if err := s.DeleteLink(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkOpened(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("삭제된 링크에 열람이 기록됐다: %v", err)
	}
}

// "안 연 것" 필터가 실제로 좁혀야 한다. 이게 이 컬럼을 만든 이유이고,
// 필터가 조용히 전체를 반환하면 화면은 멀쩡해 보이면서 아무 일도 안 한다.
func TestListLinks_unopenedNarrows(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	opened, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://open.example/1"})
	unread, _, _, _ := s.SaveLink(ctx, SaveInput{URL: "https://open.example/2"})
	if err := s.MarkOpened(ctx, opened); err != nil {
		t.Fatal(err)
	}

	all, _, err := s.ListLinks(ctx, "", 50, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("전체가 2건이어야 한다: %d", len(all))
	}

	only, _, err := s.ListLinks(ctx, "", 50, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(only) != 1 || only[0].ID != unread {
		t.Errorf("안 연 것만 나와야 한다: %d건", len(only))
	}
}
