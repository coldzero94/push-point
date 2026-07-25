// Package safedial provides an SSRF-hardened net dialer and http.Client: it
// resolves the target host and refuses to connect to loopback, private
// (RFC1918/ULA), link-local (unicast+multicast), or unspecified addresses.
//
// A user-supplied URL (the whole point of a link scraper) must never be able to
// reach the host's own internal network — metadata services, admin panels,
// databases bound to localhost, etc. Each redirect hop re-enters DialContext, so
// redirects to an internal address are blocked automatically (no separate
// redirect check needed).
//
// Standard library only (net / net/http) — no external dependency.
package safedial

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// isBlockedIP reports whether ip falls in a range we must never connect to from
// a user-supplied URL. Kept unexported and unit-tested directly (safedial_test.go)
// so the IP-classification policy can be verified without any real network dial.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// blockedError describes a refused connection (a resolved IP in a blocked range).
type blockedError struct {
	host string
	ip   net.IP
}

func (e *blockedError) Error() string {
	return fmt.Sprintf("safedial: 차단된 주소 host=%s ip=%s (루프백/사설/링크로컬/미지정 대상 거부)", e.host, e.ip)
}

// DialContext returns a DialContext that resolves addr, rejects the host if ANY
// resolved IP is in a blocked range (defends DNS rebinding — a name that mixes
// public and private answers is refused wholesale), then dials a verified IP
// directly so the connection cannot be repointed by a second resolution
// (TOCTOU-safe). dialer/resolver default to sensible values when nil.
func DialContext(dialer *net.Dialer, resolver *net.Resolver) func(context.Context, string, string) (net.Conn, error) {
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("safedial: %s 해석 결과 없음", host)
		}
		// 하나라도 차단 대역이면 호스트 전체를 거부한다.
		for _, ipa := range ips {
			if isBlockedIP(ipa.IP) {
				return nil, &blockedError{host: host, ip: ipa.IP}
			}
		}
		// 검증된 IP로 직접 다이얼 (재해석에 의한 우회 차단).
		var lastErr error
		for _, ipa := range ips {
			conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ipa.IP.String(), port))
			if derr == nil {
				return conn, nil
			}
			lastErr = derr
		}
		return nil, lastErr
	}
}

// Transport returns an *http.Transport whose DialContext is the SSRF guard.
// A fresh transport is returned each call (callers own its lifecycle/pooling).
//
// HTTP/2 negotiation is deliberately left off. Measured: medium.com answers Go's
// HTTP/2 client with 403 and the same request over HTTP/1.1 with 200 (same
// User-Agent, same headers) — its bot detection keys on the h2 client profile.
// We fetch one page per link behind a 1 req/s per-domain limit, so multiplexing
// buys nothing here and HTTP/1.1 costs nothing. Note this is protocol negotiation,
// not fingerprint spoofing: we still identify ourselves honestly in User-Agent and
// we do not attempt to imitate a browser's TLS handshake.
func Transport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           DialContext(nil, nil),
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// Client returns an *http.Client using the SSRF-guarded transport. timeout is
// the whole-request timeout; pass 0 to rely on the caller's per-request context
// deadline instead (the scraper does this).
func Client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: Transport(),
	}
}
