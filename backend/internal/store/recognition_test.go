package store

import (
	"context"
	"fmt"
	"testing"
)

// 도메인 재조우는 **3회부터** 말한다.
//
// 2는 우연이다 — 같은 블로그를 두 번 저장하는 것은 아무것도 뜻하지 않는다. 이 하한이
// 낮으면 배너가 거의 매번 무언가를 말하게 되고, 그러면 사람이 세 번째 줄을 읽는 것을
// 그만둔다. 이 제품이 가진 표면은 몇 개 안 된다.
func TestDomainEncounter_SilentUntilThird(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newTestStore(t)

	var ids []int64
	for i := 1; i <= 3; i++ {
		id, _, _, err := s.SaveLink(ctx, SaveInput{URL: fmt.Sprintf("https://blog.example.com/p%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		domain, n, err := s.DomainEncounter(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 {
			if n != 0 {
				t.Errorf("%d번째인데 말하려 한다: n=%d", i, n)
			}
			continue
		}
		if n != 3 {
			t.Errorf("세 번째인데 n=%d", n)
		}
		if domain != "blog.example.com" {
			t.Errorf("도메인: %q", domain)
		}
	}

	// **지운 것은 세지 않는다.** 지운 링크가 계속 횟수를 올리면 배너가 있지도 않은
	// 아카이브를 근거로 말하게 된다.
	if err := s.DeleteLink(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, n, err := s.DomainEncounter(ctx, ids[2]); err != nil || n != 0 {
		t.Errorf("삭제 뒤에도 말한다: n=%d err=%v", n, err)
	}
}

// 원장은 **노출과 탭을 따로** 센다. 노출만 남으면 "무시당했다"가 데이터에서 사라진다.
func TestRecognitionLedger_ShownAndTapped(t *testing.T) {
	ctx := context.Background()
	s, _, _ := newTestStore(t)

	id, _, _, err := s.SaveLink(ctx, SaveInput{URL: "https://example.com/one"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.RecordRecognition(ctx, id, RungDomain); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkRecognitionTapped(ctx, id); err != nil {
		t.Fatal(err)
	}

	stats, err := s.RecognitionStats(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("단이 하나여야 한다: %+v", stats)
	}
	if stats[0].Shown != 3 {
		t.Errorf("노출 3이어야 한다: %d", stats[0].Shown)
	}
	// **한 번의 탭은 한 행에만.** 링크 단위로 표시하면서 전부를 눌린 것으로 만들면
	// 탭률이 언제나 100%가 되고 원장이 아무것도 못 말한다.
	if stats[0].Tapped != 1 {
		t.Errorf("탭 1이어야 한다: %d", stats[0].Tapped)
	}
}
