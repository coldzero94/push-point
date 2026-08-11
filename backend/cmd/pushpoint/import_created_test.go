package main

import (
	"strings"
	"testing"
)

// 북마크의 `ADD_DATE`가 살아 오는가.
//
// **이게 없으면 임포트한 아카이브 전체가 임포트한 날 저장된 것이 된다** — 되살림의 7일
// 규칙도, 알아봄이 말하는 날짜도, 통계의 30일 창도 같은 하루를 본다. 12-BACKLOG가
// "지금 남은 빚"이라고 부른 것이 이것이고, 여섯 건짜리 아카이브를 몇 년치로 만드는
// 유일한 경로다.
func TestExtractBookmarkURLs_CarriesAddDate(t *testing.T) {
	const html = `<!DOCTYPE NETSCAPE-Bookmark-file-1><DL><p>
	<DT><A HREF="https://a.example.com/1" ADD_DATE="1718409600">초</A>
	<DT><A HREF="https://b.example.com/2" ADD_DATE="1718409600000000">마이크로초</A>
	<DT><A HREF="https://c.example.com/3">시각 없음</A>
	<DT><A HREF="https://d.example.com/4" ADD_DATE="0">0</A>
	<DT><A HREF="https://e.example.com/5" ADD_DATE="쓰레기">비수치</A>
	</DL><p>`

	got, err := extractBookmarkURLs(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("5건이어야 한다: %d", len(got))
	}
	byHost := map[string]int64{}
	for _, l := range got {
		byHost[l.URL] = l.CreatedAt
	}

	if byHost["https://a.example.com/1"] != 1718409600 {
		t.Errorf("초 단위가 안 왔다: %d", byHost["https://a.example.com/1"])
	}
	// **마이크로초를 자릿수로 가른다.** 크롬의 일부 export가 그 단위를 쓰는데, 그대로
	// 받으면 서기 56000년에 저장한 링크가 생기고 서버가 400으로 거부한다 — 즉 그
	// 북마크들만 조용히 임포트에서 사라진다.
	if byHost["https://b.example.com/2"] != 1718409600 {
		t.Errorf("마이크로초가 초로 안 바뀌었다: %d", byHost["https://b.example.com/2"])
	}
	// **모르는 것은 0이다.** 사파리 export가 ADD_DATE를 안 쓰는 경우가 있고, 그때
	// 지어낸 시각을 넣느니 서버가 지금으로 채우는 편이 정직하다.
	for _, u := range []string{"https://c.example.com/3", "https://d.example.com/4", "https://e.example.com/5"} {
		if byHost[u] != 0 {
			t.Errorf("%s: 모르는 시각인데 값이 왔다: %d", u, byHost[u])
		}
	}
}
