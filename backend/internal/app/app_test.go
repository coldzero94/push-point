package app

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{DataDir: t.TempDir(), APIKey: "test-key", Addr: "127.0.0.1:0", ScrapeConcurrency: 1}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func start(t *testing.T) *App {
	t.Helper()
	a, err := Start(testConfig(t), discardLogger())
	if err != nil {
		t.Fatalf("Start 실패: %v", err)
	}
	return a
}

// 정상 종료가 "dispatcher 비정상 종료"로 오보되면 안 된다.
//
// 회귀 방지 테스트다: Shutdown은 서버를 닫고(→serveErr) dispatcher를 취소하는데
// (→dispDone), 그 순간 Wait의 select에서 두 case가 모두 ready가 된다. select는 그중
// 하나를 무작위로 고르므로, "우리가 시킨 종료"라는 표시가 없으면 정상 셧다운이 fail-stop
// 에러로 보고된다 — 운영 로그의 가짜 경보가 되고, iOS에서는 Stop()마다 발생했다.
func TestShutdownDoesNotReportDispatcherDeath(t *testing.T) {
	for i := range 30 {
		a := start(t)
		waitErr := make(chan error, 1)
		go func() { waitErr <- a.Wait() }()
		if err := a.Shutdown(); err != nil {
			t.Fatalf("Shutdown 실패: %v", err)
		}
		if err := <-waitErr; err != nil {
			t.Fatalf("%d회차: 정상 종료인데 Wait가 에러를 반환했다: %v", i, err)
		}
	}
}

// 포트 0으로 띄우면 OS가 고른 실제 포트를 알 수 있어야 한다 —
// iOS 앱이 인프로세스 서버에 접속하는 유일한 방법이다.
func TestAddrReportsBoundPort(t *testing.T) {
	a := start(t)
	defer a.Shutdown() //nolint:errcheck // 테스트 정리

	addr := a.Addr()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("루프백에 바인딩돼야 한다: %q", addr)
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "0" {
		t.Fatalf("실제 포트가 나와야 한다 (%v): %q", err, addr)
	}
	// 실제로 응답하는지 — 주소만 맞고 안 뜬 경우를 잡는다.
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("healthz 요청 실패: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // 테스트
	if resp.StatusCode != http.StatusOK {
		t.Errorf("healthz가 200이어야 한다: %d", resp.StatusCode)
	}
}

// 바인딩 실패는 Start에서 즉시 드러나야 한다(리스너를 명시적으로 만드는 이유).
// 그리고 실패 경로에서 store를 닫지 않으면 DB 핸들이 샌다.
func TestStartFailsOnBadAddr(t *testing.T) {
	cfg := testConfig(t)
	cfg.Addr = "127.0.0.1:1" // 특권 포트 — 바인딩 불가
	a, err := Start(cfg, discardLogger())
	if err == nil {
		_ = a.Shutdown()
		t.Fatal("바인딩 실패는 Start에서 에러여야 한다")
	}
	if a != nil {
		t.Error("실패 시 App은 nil이어야 한다")
	}
	// 같은 디렉터리를 다시 열 수 있어야 한다 — 실패 경로가 DB를 닫았다는 뜻.
	cfg.Addr = "127.0.0.1:0"
	b, err := Start(cfg, discardLogger())
	if err != nil {
		t.Fatalf("실패 후 재시작이 돼야 한다 (store 누수 의심): %v", err)
	}
	if err := b.Shutdown(); err != nil {
		t.Errorf("Shutdown 실패: %v", err)
	}
}

// Config의 제로값이 조용히 통과하면 안 된다.
//
// 특히 Addr가 비면 net.Listen("tcp", "")이 **모든 인터페이스**에 바인딩된다 — 개인 링크
// DB가 LAN 전체에 열린다. ppcore가 루프백을 고정하고 SSRF 가드가 기본 on인 것을
// 제로값 하나가 되돌리게 둘 수 없다. APIKey가 비면 서버는 뜨지만 모든 요청이 401인
// "조용히 망가진" 상태가 된다.
func TestStartRejectsInvalidConfig(t *testing.T) {
	for _, c := range []struct {
		name string
		mut  func(*Config)
	}{
		{"DataDir 없음", func(c *Config) { c.DataDir = "" }},
		{"APIKey 없음", func(c *Config) { c.APIKey = "" }},
		{"Addr 없음 (모든 인터페이스에 바인딩됨)", func(c *Config) { c.Addr = "" }},
	} {
		cfg := testConfig(t)
		c.mut(&cfg)
		a, err := Start(cfg, discardLogger())
		if err == nil {
			_ = a.Shutdown()
			t.Errorf("%s: Start가 실패해야 한다", c.name)
		}
	}
}

// ScrapeConcurrency는 보정되어야 한다. 보정 전 값으로 maxInFlight를 합산하면 dispatcher가
// 실제 워커 수보다 많은 잡을 running으로 전이시킬 수 있다(미실행 잡이 attempts를 소진).
func TestStartClampsScrapeConcurrency(t *testing.T) {
	cfg := testConfig(t)
	cfg.ScrapeConcurrency = -10
	if err := cfg.validate(); err != nil {
		t.Fatalf("validate 실패: %v", err)
	}
	if cfg.ScrapeConcurrency < 1 {
		t.Errorf("1 이상으로 보정돼야 한다: %d", cfg.ScrapeConcurrency)
	}
	a, err := Start(testConfigWith(t, -10), discardLogger())
	if err != nil {
		t.Fatalf("보정된 값으로 시작돼야 한다: %v", err)
	}
	if err := a.Shutdown(); err != nil {
		t.Errorf("Shutdown 실패: %v", err)
	}
}

func testConfigWith(t *testing.T, scrapeConcurrency int) Config {
	t.Helper()
	cfg := testConfig(t)
	cfg.ScrapeConcurrency = scrapeConcurrency
	return cfg
}

// 앱 생명주기상 Shutdown이 두 번 불릴 수 있다(예: ppcore.Stop 직후 Wait 경로).
// closeOnce가 없으면 두 번째 호출이 닫힌 채널·닫힌 DB에 부딪힌다.
func TestShutdownIsIdempotent(t *testing.T) {
	a := start(t)
	if err := a.Shutdown(); err != nil {
		t.Fatalf("첫 Shutdown 실패: %v", err)
	}
	if err := a.Shutdown(); err != nil {
		t.Errorf("두 번째 Shutdown도 에러 없이 끝나야 한다: %v", err)
	}
}
