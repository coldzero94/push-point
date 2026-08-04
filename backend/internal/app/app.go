// Package app은 push-point 인스턴스 하나의 배선과 수명주기를 담는다:
// DB(+마이그레이션) → store/queue → dispatcher(scrape·thumb·tag 핸들러) → chi HTTP 서버.
//
// 소비자가 둘이다: 서버 바이너리(cmd/pushpoint)와, 폰 안에서 같은 서버를 인프로세스로
// 띄우는 iOS 본체 앱(mobile/ppcore). 배선을 여기 한 곳에 두는 이유는 자립 모드가
// **같은 계약(api/openapi.yaml)을 같은 코드로** 서빙하게 하기 위해서다 — 그래야 iOS
// 클라이언트가 서버 모드와 자립 모드에 코드 한 벌로 대응한다(docs/v2/04 §7.4).
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coby/push-point/backend/internal/api"
	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/scraper"
	"github.com/coby/push-point/backend/internal/store"
	"github.com/coby/push-point/backend/internal/tagger"
	"github.com/coby/push-point/backend/internal/thumbs"
)

// shutdownTimeout은 진행 중 요청 드레인 상한.
const shutdownTimeout = 10 * time.Second

// Config는 인스턴스 하나를 띄우는 데 필요한 전부. config.Load()(환경변수)에 묶이지
// 않게 값으로 받는다 — iOS는 환경변수가 없고 App Group 경로를 런타임에 받는다.
type Config struct {
	DataDir           string
	APIKey            string
	Addr              string // 포트가 0이면 OS가 고른다(예: "127.0.0.1:0") — Addr()로 확인
	ScrapeConcurrency int
	AllowPrivateHosts bool
}

// validate는 Start가 진입점에서 한 번 부른다. store.SaveInput.Normalize()가 SaveLink
// 안에서 불려 어떤 진입점도 검증을 건너뛸 수 없게 만든 것과 같은 이유다 — 이 구조체는
// 환경변수 검증(internal/config)을 거치지 않는 호출자(mobile/ppcore)가 직접 만든다.
//
// 특히 Addr는 **빈 문자열이 "모든 인터페이스"를 뜻한다** — net.Listen("tcp", "")는
// [::]:임의포트에 바인딩된다. 개인 링크 DB를 LAN 전체에 여는 것이므로, 나머지 계층이
// 전부(ppcore의 루프백 고정, SSRF 가드 기본 on) 막고 있는 것을 제로값이 되돌리게 둘 수 없다.
func (c *Config) validate() error {
	if c.DataDir == "" {
		return errors.New("app: DataDir가 비어 있다")
	}
	// 빈 키는 열리는 게 아니라 잠긴다(BearerAuth가 빈 토큰을 먼저 거부한다) — Start는
	// 성공하고 Addr()도 주소를 주는데 모든 요청이 401인 서버가 된다. 조용히 망가지는
	// 대신 시작에서 실패시킨다. iOS에서 Keychain 읽기 실패가 ""로 넘어오는 경로가 있다.
	if c.APIKey == "" {
		return errors.New("app: APIKey가 비어 있다 — 모든 요청이 401이 된다")
	}
	if c.Addr == "" {
		return errors.New(`app: Addr가 비어 있다 — 빈 값은 모든 인터페이스에 바인딩된다(예: "127.0.0.1:0")`)
	}
	// 여기서 보정해야 아래 maxInFlight 합산이 유효한 값을 더한다. 보정 전 값으로 합하면
	// dispatcher가 실제 워커 수보다 많은 잡을 running으로 전이시킬 수 있다(NewDispatcher가
	// 경고하는 바로 그 위험 — 미실행 잡이 attempts를 소진한다).
	if c.ScrapeConcurrency < 1 {
		c.ScrapeConcurrency = 1
	}
	return nil
}

// App은 실행 중인 인스턴스.
//
// dispDone은 **닫히는** 채널이고 결과는 dispErr에 담긴다 — Wait와 cleanup 양쪽이
// dispatcher 종료를 기다려야 하는데, 값을 흘리는 채널이면 먼저 받은 쪽이 값을 소비해
// 나머지 한 쪽이 영원히 블록된다. close/receive가 happens-before를 세워 주므로
// 채널을 받은 뒤 dispErr를 읽는 것은 안전하다.
type App struct {
	logger     *slog.Logger
	st         store.Store
	srv        *http.Server
	ln         net.Listener
	dispCancel context.CancelFunc
	dispDone   chan struct{}
	dispErr    error
	serveErr   chan error
	closeOnce  sync.Once
}

// Start는 배선을 세우고 서빙을 시작한다. 바인딩 실패는 여기서 즉시 error로 나온다
// (리스너를 명시적으로 만들기 때문 — ListenAndServe와 달리 비동기로 새지 않는다).
func Start(cfg Config, logger *slog.Logger) (*App, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	db, err := store.Open(cfg.DataDir) // 마이그레이션 자동 적용 포함
	if err != nil {
		return nil, err
	}
	q := queue.NewSQLite(db.Writer)
	// 검색 질의를 태그 사전으로 넓힌다 — `쿠버네티스`로 물어도 영어 문서에 닿는다.
	// 사전을 못 읽으면 확장 없이 검색한다: 검색이 조금 나빠지는 것과 검색이 없는 것은
	// 다르고, 여기서 실패해 앱이 안 뜨면 후자가 된다.
	var st store.Store
	st = store.New(db, q, store.WithQueryExpander(func(ctx context.Context) store.QueryExpander {
		entries, err := st.LoadTagDict(ctx)
		if err != nil {
			logger.Warn("검색 질의 확장용 사전 로드 실패 — 확장 없이 검색한다", "err", err)
			return nil
		}
		te := make([]tagger.TagEntry, len(entries))
		for i, e := range entries {
			te[i] = tagger.TagEntry{ID: e.ID, Name: e.Name, Aliases: e.Aliases, Facet: e.Facet}
		}
		return tagger.BuildDictionary(te)
	}))

	// 기본은 SSRF 가드 dial(사설/루프백/링크로컬 대상 거부) — 사용자 링크가 내부망으로
	// 못 나가게 막는다. AllowPrivateHosts면 가드 없는 클라이언트를 주입한다
	// (로컬 fixture·개발 전용 — 예: scripts/test_crash.sh의 127.0.0.1 fixture 스크랩).
	var scOpts []scraper.Option
	var tsOpts []thumbs.Option
	if cfg.AllowPrivateHosts {
		logger.Warn("SSRF 가드 비활성 — 사설/루프백 대상 허용 (개발/로컬 fixture 전용)")
		scOpts = append(scOpts, scraper.WithHTTPClient(&http.Client{}))
		tsOpts = append(tsOpts, thumbs.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}))
	}
	sc := scraper.New(scOpts...)
	ts := thumbs.NewDiskStore(cfg.DataDir, tsOpts...)

	// dispatcher: Run 내부에서 running→pending 크래시 복구 후 claim 루프.
	// scrape/thumb/tag 핸들러를 Run 전에 등록한다 — 등록된 kind만 claim 대상이 된다.
	// maxInFlight = 총 워커 수(scrape + thumb + tag) — claim을 실제 처리 용량에 묶어
	// 무제한 running 전이(반복 크래시 시 미실행 잡 attempts 소진)를 막는다.
	disp := queue.NewDispatcher(q, logger, cfg.ScrapeConcurrency+thumbConcurrency+tagConcurrency)
	disp.Register(queue.KindScrape, newScrapeHandler(sc, st, cfg.ScrapeConcurrency, logger))
	disp.Register(queue.KindThumb, newThumbHandler(sc, ts, st, logger))
	disp.Register(queue.KindTag, newTagHandler(st, logger))

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		if cerr := st.Close(); cerr != nil {
			logger.Error("store close 오류", "err", cerr)
		}
		return nil, fmt.Errorf("app: 리스너 바인딩 실패(%s): %w", cfg.Addr, err)
	}

	dispCtx, dispCancel := context.WithCancel(context.Background())
	a := &App{
		logger:     logger,
		st:         st,
		ln:         ln,
		dispCancel: dispCancel,
		dispDone:   make(chan struct{}),
		serveErr:   make(chan error, 1),
		srv: &http.Server{
			Handler:           api.NewRouter(api.NewServer(st, cfg.DataDir, logger), cfg.APIKey, logger),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
	go func() {
		a.dispErr = disp.Run(dispCtx)
		close(a.dispDone)
	}()
	go func() {
		err := a.srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil // 정상 종료
		}
		a.serveErr <- err
	}()
	return a, nil
}

// Addr는 실제로 바인딩된 주소를 반환한다. Config.Addr의 포트가 0이었으면
// OS가 고른 포트가 여기 담긴다 — iOS 앱이 이 값으로 인프로세스 서버에 접속한다.
func (a *App) Addr() string { return a.ln.Addr().String() }

// Wait는 서버나 dispatcher가 끝날 때까지 블록한다. **정상 종료면 nil**, 비정상이면 원인을
// 반환한다 — Shutdown이 불린 경우도 여기로 깨어나므로 "비정상일 때만 돌아온다"가 아니다.
// 서빙 중 dispatcher가 죽는 것은 "워커 없이 서빙하는" 상태이므로 fail-stop으로 취급한다:
// 서버를 graceful 종료한 뒤 error를 올리고, 재시작의 RecoverStale이 running 잡을 복구한다.
func (a *App) Wait() error {
	select {
	case err := <-a.serveErr:
		if err == nil {
			return nil // ErrServerClosed — Shutdown으로 인한 정상 종료
		}
		a.cleanup()
		return fmt.Errorf("서버 실행 실패: %w", err)
	case <-a.dispDone:
		// **취소로 끝난 것은 죽은 것이 아니다.** Shutdown이 dispatcher를 취소하면 이쪽도
		// 깨어나는데, 그때 dispErr는 nil이거나(Run은 취소 시 nil) 시작 직후라면
		// "stale 잡 복구 실패: context canceled"다 — Run이 제일 먼저 RecoverStale(ctx)을
		// 부르기 때문이다. 이 판별이 없으면 정상 종료가 fail-stop으로 오보된다(실측 29/30).
		//
		// "종료 중" 플래그로 무조건 nil을 돌려주는 방법도 있지만 그러면 같은 창에서 난
		// **진짜** 결함(SQLITE_CORRUPT·I/O 오류)까지 삼킨다. 원인을 직접 보는 편이
		// 정확하고, cleanup()도 같은 판별을 쓴다.
		if a.dispErr == nil || errors.Is(a.dispErr, context.Canceled) {
			return nil
		}
		a.logger.Error("dispatcher 비정상 종료 — fail-stop", "err", a.dispErr)
		a.stopServer()
		a.cleanup()
		return fmt.Errorf("dispatcher 비정상 종료: %w", a.dispErr)
	}
}

// Shutdown은 graceful 종료다. 순서가 중요하다: 서버 먼저(새 요청 차단 + 진행 중 요청
// 드레인), dispatcher 다음(마지막 요청이 enqueue한 잡의 알림·기록까지 처리), store 마지막.
//
// **항상 nil을 반환한다** — 드레인 타임아웃·강제 종료·store close 실패는 전부 로그로만
// 남는다. 셧다운 중에 할 수 있는 복구가 없어서인데, 시그니처가 error인 것은 호출자가
// defer로 다루기 편하고 나중에 의미를 얹을 여지를 남기기 위해서다. 지금은 이 값으로
// "깨끗하게 닫혔는지"를 판단할 수 없다는 점을 호출자가 알아야 한다.
func (a *App) Shutdown() error {
	a.stopServer()
	a.cleanup()
	return nil
}

// stopServer는 HTTP 서버만 멈춘다. 드레인 타임아웃이면 잔여 커넥션을 강제 종료한다 —
// 진행 중 요청이 닫힌 DB를 만나 500이 되는 것을 막기 위해 store보다 먼저 끝낸다.
func (a *App) stopServer() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := a.srv.Shutdown(ctx); err != nil {
		a.logger.Error("서버 셧다운 오류 — 강제 종료", "err", err)
		if cerr := a.srv.Close(); cerr != nil {
			a.logger.Error("서버 강제 종료 오류", "err", cerr)
		}
	}
}

// cleanup은 dispatcher와 store를 닫는다. Wait와 Shutdown 양쪽에서 불릴 수 있어 1회만 실행된다.
func (a *App) cleanup() {
	a.closeOnce.Do(func() {
		a.dispCancel()
		<-a.dispDone
		if a.dispErr != nil && !errors.Is(a.dispErr, context.Canceled) {
			a.logger.Error("dispatcher 종료 오류", "err", a.dispErr)
		}
		if err := a.st.Close(); err != nil {
			a.logger.Error("store close 오류", "err", err)
		}
	})
}
