package safedial

import (
	"context"
	"net"
	"strings"
	"testing"
)

// isBlockedIP 정책을 실제 연결 시도 없이 직접 검증한다 — 루프백/사설/링크로컬/미지정은
// 차단(true), 공용 주소는 허용(false).
func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback v4 대역", "127.10.20.30", true},
		{"loopback v6", "::1", true},
		{"사설 10.x", "10.1.2.3", true},
		{"사설 192.168.x", "192.168.1.1", true},
		{"사설 172.16.x", "172.16.5.9", true},
		{"ULA v6 (fc00::/7)", "fd00::1", true},
		{"링크로컬 유니캐스트 v4", "169.254.1.1", true},
		{"링크로컬 유니캐스트 v6", "fe80::1", true},
		{"링크로컬 멀티캐스트 v4", "224.0.0.1", true},
		{"링크로컬 멀티캐스트 v6", "ff02::1", true},
		{"미지정 v4", "0.0.0.0", true},
		{"미지정 v6", "::", true},

		{"공용 8.8.8.8", "8.8.8.8", false},
		{"공용 1.1.1.1", "1.1.1.1", false},
		{"공용 172.15.x (사설 아님)", "172.15.0.1", false},
		{"공용 172.32.x (사설 아님)", "172.32.0.1", false},
		{"공용 v6", "2001:4860:4860::8888", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("IP 파싱 실패: %q", tc.ip)
			}
			if got := isBlockedIP(ip); got != tc.want {
				t.Errorf("isBlockedIP(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

// DialContext는 차단 대역으로 해석되는 대상을 소켓 연결 이전에 거부한다. IP 리터럴은
// resolver가 DNS 조회 없이 그대로 반환하므로 이 테스트는 네트워크에 접촉하지 않는다 —
// 루프백은 dial 전에 차단되어 blockedError가 나온다.
func TestDialContextRejectsBlockedLiteralIP(t *testing.T) {
	dc := DialContext(nil, nil)
	conn, err := dc(context.Background(), "tcp", "127.0.0.1:80")
	if conn != nil {
		_ = conn.Close()
		t.Fatal("루프백 대상인데 연결이 성립됨 (차단 실패)")
	}
	if err == nil {
		t.Fatal("루프백 대상인데 에러가 nil (차단 실패)")
	}
	if !strings.Contains(err.Error(), "차단된 주소") {
		t.Fatalf("차단 에러 메시지 예상 밖: %v", err)
	}
}

// host:port 형식이 아닌 addr는 형식 에러로 거부된다.
func TestDialContextRejectsMalformedAddr(t *testing.T) {
	dc := DialContext(nil, nil)
	if _, err := dc(context.Background(), "tcp", "no-port-here"); err == nil {
		t.Fatal("host:port 형식이 아닌 addr인데 에러가 nil")
	}
}
