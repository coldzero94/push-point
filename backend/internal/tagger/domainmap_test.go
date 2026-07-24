package tagger

import (
	"slices"
	"testing"
)

func TestDomainTags(t *testing.T) {
	cases := []struct {
		host string
		want []string // nil이면 미등록 기대
	}{
		{"github.com", []string{"opensource", "dev"}},
		{"www.github.com", []string{"opensource", "dev"}}, // www 제거
		{"youtube.com", []string{"video"}},
		{"m.youtube.com", []string{"video"}},    // 서브도메인 폴백
		{"blog.naver.com", []string{"article"}}, // 명시 등록 서브도메인 — 폴백 전 히트
		{"example.unregistered.com", nil},       // 미등록
		{"nope.io", nil},
	}
	for _, tc := range cases {
		got := DomainTags(tc.host)
		if !slices.Equal(got, tc.want) {
			t.Errorf("DomainTags(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// _comment 등 메타 키(_ 접두)는 맵에 실리지 않아야.
func TestDomainMapSkipsMetaKeys(t *testing.T) {
	if got := DomainTags("_comment"); got != nil {
		t.Errorf("_comment는 스킵되어야, got %v", got)
	}
}
