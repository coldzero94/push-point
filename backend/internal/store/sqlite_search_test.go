package store

import (
	"context"
	"testing"
)

// ftsMatchQueryAny는 같은 토큰을 OR로 잇는다 — AND가 빈손일 때의 재시도용.
func TestFtsMatchQueryAny(t *testing.T) {
	if got := ftsMatchQueryAny("쿠버네티스 하드웨이"); got != `"쿠버네티스" OR "하드웨이"` {
		t.Errorf("OR로 이어야 한다: %q", got)
	}
	// 토큰이 하나면 AND와 **글자까지 같아야** 한다 — 호출부가 그걸 보고 재시도를 건너뛴다.
	if a, o := ftsMatchQuery("제네릭"), ftsMatchQueryAny("제네릭"); a != o {
		t.Errorf("단일 토큰이면 AND와 OR가 같아야 재시도를 건너뛴다: %q vs %q", a, o)
	}
	// 3자 미만은 AND와 같은 규칙으로 버린다.
	if got := ftsMatchQueryAny("웹 취약점 top 10"); got != `"취약점" OR "top"` {
		t.Errorf("3자 미만 토큰은 빠져야 한다: %q", got)
	}
	// 주입 차단은 **두 겹**이다: 따옴표 이스케이프 + 3자 미만 제거.
	// `abc" OR "x`에서 `OR`와 `"x`는 2자라 먼저 버려지고, 남은 토큰은 따옴표가 이중화된다.
	if got := ftsMatchQueryAny(`abc" OR "x`); got != `"abc"""` {
		t.Errorf("주입 시도가 무력화돼야 한다: %q", got)
	}
}

// LIKE 폴백은 **낱말별로** 찾아야 한다. 예전에는 질의 전체가 한 덩어리 패턴이라
// `%직방 다방 차이%`처럼 그 문자열이 통째로 있어야 했다 — 사람은 기억나는 낱말을
// 순서대로 치지 문서의 어구를 그대로 치지 않는다.
//
// LIKE 경로를 타려면 **모든 토큰이 3자 미만**이어야 한다(그 이상이면 FTS로 간다).
// 한국어 2음절 낱말이 정확히 그 구간이고, 그래서 이 경로가 한국어에서 자주 쓰인다.
func TestSearchLikeMatchesWordsSeparately(t *testing.T) {
	st, db, _ := newTestStore(t)
	ctx := context.Background()

	// 제목에 `직방`·`다방`, 설명에 `비교`가 흩어져 있다.
	if _, err := db.Writer.ExecContext(ctx, `
		INSERT INTO links (url, url_hash, domain, title, description, status)
		VALUES ('https://a.example/1', 'h1', 'a.example',
		        '직방, 다방, 네이버 부동산', '두 서비스를 비교 분석', 'done')`); err != nil {
		t.Fatal(err)
	}

	// 낱말이 서로 다른 필드에 흩어져 있어도 잡혀야 한다(낱말끼리 AND, 낱말 하나는 필드 OR).
	got, _, mode, err := st.Search(ctx, "직방 비교", "", nil, nil, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if mode != SearchModeLike {
		t.Fatalf("2자 토큰뿐이면 LIKE 폴백이어야 한다: %v", mode)
	}
	if len(got) != 1 {
		t.Errorf("제목의 `직방` + 설명의 `비교`로 찾아야 한다: %d건", len(got))
	}

	// 문서에 없는 낱말이 섞이면 AND는 빈손이 되지만 **OR 재시도**가 건져야 한다.
	// 실제 사례가 이것이다 — 문서는 `뭐가 다를까?`라고 쓰는데 사람은 `차이`를 친다.
	got, _, _, err = st.Search(ctx, "직방 다방 차이", "", nil, nil, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Error("`차이`가 문서에 없어도 나머지 낱말로 건져야 한다 — OR 재시도가 안 돈다")
	}

	// 낱말이 하나뿐이고 그게 없으면 재시도해도 없다 — 없는 것을 만들어내면 안 된다.
	got, _, _, err = st.Search(ctx, "핫도그", "", nil, nil, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("없는 낱말은 없어야 한다: %d건", len(got))
	}

	// **낱말끼리는 AND가 기본이어야 한다.** 두 번째 문서에는 `직방`만 있고 `비교`는 없다.
	// AND면 첫 문서만, 항상 OR이면 둘 다 나온다 — 흔한 한 낱말이 전부를 끌어오는 것이
	// 정확히 피하려던 상태다. 이 문서가 있어야 두 규칙이 구분된다.
	if _, err := db.Writer.ExecContext(ctx, `
		INSERT INTO links (url, url_hash, domain, title, description, status)
		VALUES ('https://a.example/2', 'h2', 'a.example',
		        '직방 앱 사용 후기', '원룸을 구하며', 'done')`); err != nil {
		t.Fatal(err)
	}
	got, _, _, err = st.Search(ctx, "직방 비교", "", nil, nil, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("AND가 기본이어야 한다 — `비교`가 없는 문서까지 나왔다: %d건", len(got))
	}
}

// 질의 확장이 실제로 FTS 문자열에 얹히는지. **조용히 안 붙어도 검색은 그냥 조금 나빠질
// 뿐 아무것도 실패하지 않으므로**, 붙는다는 사실 자체를 고정한다.
func TestFtsMatchExpanded(t *testing.T) {
	s := &sqliteStore{}
	// `쿠버네티스`도 `하드웨이`도 3룬을 넘으므로 둘 다 남는다 — 처음에 이 테스트를
	// `"하드웨이"`만 기대하게 썼다가 틀렸다. 3룬 하한이 버리는 것은 2음절이다.
	if got := s.ftsMatchExpanded(context.Background(), "쿠버네티스 하드웨이"); got != `"쿠버네티스" "하드웨이"` {
		t.Fatalf("확장기 없을 때는 원래 동작이어야 한다: %q", got)
	}

	s.newExpander = func(context.Context) QueryExpander { return fakeExpander{"kubernetes"} }
	got := s.ftsMatchExpanded(context.Background(), "쿠버네티스 하드웨이")
	if got != `("쿠버네티스" "하드웨이") OR ("kubernetes")` {
		t.Fatalf("확장이 OR로 얹혀야 한다: %q", got)
	}

	// 3룬 하한이 토큰을 전부 버린 경우 — 확장만으로 FTS를 탄다
	s.newExpander = func(context.Context) QueryExpander { return fakeExpander{"devops"} }
	if got := s.ftsMatchExpanded(context.Background(), "도커"); got != `"devops"` {
		t.Fatalf("원래 토큰이 없으면 확장만 남아야 한다: %q", got)
	}
}

type fakeExpander []string

func (f fakeExpander) TagsInQuery(string) []string { return f }
