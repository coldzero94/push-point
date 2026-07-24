package main

import "testing"

func TestMaskKey(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "unset"},
		{"abc", "set(****)"},      // 4자 이하는 끝자리도 가리기 부족 → 전량 마스킹
		{"abcd", "set(****)"},     // 경계: 길이 4
		{"dev-key", "set(…-key)"}, // 정상: 끝 4자리만
		{"pushpoint-secret-6a1f", "set(…6a1f)"},
	}
	for _, c := range cases {
		if got := maskKey(c.in); got != c.want {
			t.Errorf("maskKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
