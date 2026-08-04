package store

import (
	"context"
	"errors"
	"testing"
)

const day = int64(86400)

func seedResurface(t *testing.T, s Store, now int64) {
	t.Helper()
	ctx := context.Background()
	for i, c := range []struct {
		url    string
		ageDay int64
		opened bool
	}{
		{"https://a.example/1", 30, false},
		{"https://b.example/2", 20, false},
		{"https://c.example/3", 10, false},
		{"https://d.example/4", 40, true}, // 열었음 — 후보 아님
		{"https://e.example/5", 2, false}, // 너무 최근 — 후보 아님
	} {
		id, _, _, err := s.SaveLink(ctx, SaveInput{URL: c.url})
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		st := s.(*sqliteStore)
		if _, err := st.db.Writer.ExecContext(ctx,
			`UPDATE links SET created_at = ? WHERE id = ?`, now-c.ageDay*day, id); err != nil {
			t.Fatal(err)
		}
		if c.opened {
			if err := s.MarkOpened(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// **하루 동안 같은 하나여야 한다.** 새로고침마다 바뀌면 추천이 아니라 슬롯머신이고,
// 오늘 넘긴 것이 내일 다시 오지 않으면 되살릴 이유가 없다.
func TestResurfaced_stableWithinADay(t *testing.T) {
	s, _, _ := newTestStore(t)
	now := int64(1_800_000_000)
	seedResurface(t, s, now)

	first, err := s.Resurfaced(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	// 같은 날 안에서 시각이 흘러도 같은 답
	for _, at := range []int64{now, now + 3600, now + 12*3600, now + day - 1} {
		got, err := s.Resurfaced(context.Background(), at)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != first.ID {
			t.Fatalf("같은 날인데 답이 바뀌었다: %d → %d (t=%d)", first.ID, got.ID, at)
		}
	}
}

// 날이 바뀌면 언젠가는 다른 것이 나와야 한다 — 늘 같은 하나면 되살리기가 아니라 고정 배너다.
func TestResurfaced_rotatesAcrossDays(t *testing.T) {
	s, _, _ := newTestStore(t)
	now := int64(1_800_000_000)
	seedResurface(t, s, now)

	seen := map[int64]bool{}
	for d := int64(0); d < 30; d++ {
		l, err := s.Resurfaced(context.Background(), now+d*day)
		if err != nil {
			t.Fatal(err)
		}
		seen[l.ID] = true
	}
	if len(seen) < 2 {
		t.Fatalf("30일 동안 %d개만 나왔다 — 돌지 않는다", len(seen))
	}
}

// 후보 조건: 연 적 없고, 일주일 지났고, 안 지워진 것.
func TestResurfaced_candidateRules(t *testing.T) {
	s, _, _ := newTestStore(t)
	now := int64(1_800_000_000)
	seedResurface(t, s, now)

	// **오늘 기준으로** 본다. 날짜를 멀리 밀면서 확인하면 안 된다 — 이틀 전 저장도
	// 아흐레 뒤에는 정당하게 후보가 되고(그게 이 기능이다), 처음에 그렇게 썼다가
	// 테스트가 통과할 수 없는 단정이 됐다.
	for d := int64(0); d < 5; d++ {
		l, err := s.Resurfaced(context.Background(), now+d*day)
		if err != nil {
			t.Fatal(err)
		}
		if l.URL == "https://d.example/4" {
			t.Fatal("연 적 있는 링크가 되살아났다 — 그건 잊은 것이 아니라 본 것이다")
		}
		if l.URL == "https://e.example/5" {
			t.Fatalf("%d일째: 이틀 전 저장이 되살아났다 — 아직 목록 위쪽에 있어서 스스로 보인다", d)
		}
	}

	// 그리고 시간이 지나면 그 링크도 후보가 되어야 한다 — 나이 기준이 절대값이 아니라
	// 그때그때의 now 기준이라는 것을 고정한다.
	late := now + 20*day
	found := false
	for d := int64(0); d < 30 && !found; d++ {
		l, err := s.Resurfaced(context.Background(), late+d*day)
		if err != nil {
			t.Fatal(err)
		}
		found = l.URL == "https://e.example/5"
	}
	if !found {
		t.Fatal("20일이 지나도 그 링크가 후보가 되지 않는다 — 나이 기준이 now를 안 따라간다")
	}
}

// 후보가 없으면 오류가 아니라 상태다 — 호출부가 204로 옮긴다.
func TestResurfaced_emptyIsNotAnError(t *testing.T) {
	s, _, _ := newTestStore(t)
	_, err := s.Resurfaced(context.Background(), 1_800_000_000)
	if !errors.Is(err, ErrNoResurface) {
		t.Fatalf("빈손은 ErrNoResurface여야 한다: %v", err)
	}
}
