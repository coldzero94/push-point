package scraper

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/coby/push-point/backend/internal/safedial"
)

// 기본 도메인 rate limit — 도메인당 1 req/s (스펙 §4·§6). 테스트는 WithRateInterval로 낮춘다.
const defaultRateInterval = time.Second

// Pool은 Registry(어댑터 라우팅)를 감싸 두 가지 횡단 관심사를 더한 Scraper 구현이다:
//   - singleflight: 같은 URL 동시 스크랩을 1회 HTTP로 합친다 (중복 저장·재시도 대비).
//   - 도메인별 rate limit: 한 도메인에 1 req/s로 요청 간격을 벌린다 (차단·부하 회피).
//
// 워커 동시성 자체는 dispatcher(워커 goroutine 수)가 제어하므로 여기서 semaphore는 두지 않는다.
type Pool struct {
	inner   Scraper
	sf      singleflight.Group
	limiter *domainLimiter
}

// 컴파일 타임 인터페이스 검증.
var _ Scraper = (*Pool)(nil)

// options는 New의 주입 가능 설정. 테스트가 oEmbed 엔드포인트·클라이언트·rate를 바꾼다.
type options struct {
	client        *http.Client
	userAgent     string
	youtubeOEmbed string
	twitterOEmbed string
	rateInterval  time.Duration
}

// Option은 New의 가변 설정 함수.
type Option func(*options)

// WithHTTPClient는 모든 fetch가 쓸 HTTP 클라이언트를 주입한다 (테스트의 rewrite transport 등).
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.client = c } }

// WithUserAgent는 요청 User-Agent를 바꾼다.
func WithUserAgent(ua string) Option { return func(o *options) { o.userAgent = ua } }

// WithYouTubeOEmbedBase는 youtube oEmbed 엔드포인트를 교체한다 (테스트 fixture 주입용).
func WithYouTubeOEmbedBase(base string) Option { return func(o *options) { o.youtubeOEmbed = base } }

// WithTwitterOEmbedBase는 twitter oEmbed 엔드포인트를 교체한다 (테스트 fixture 주입용).
func WithTwitterOEmbedBase(base string) Option { return func(o *options) { o.twitterOEmbed = base } }

// WithRateInterval은 도메인당 최소 요청 간격을 바꾼다 (기본 1s, 테스트에서 축소).
func WithRateInterval(d time.Duration) Option { return func(o *options) { o.rateInterval = d } }

// New는 기본 og 파서를 fallback으로, 사이트 어댑터(youtube/x/naver/instagram)를 순서대로
// 등록한 Registry를 singleflight·rate limit으로 감싼 Pool을 만든다.
// scrape 잡 핸들러가 이 Scraper 하나로 모든 URL을 처리한다.
func New(opts ...Option) *Pool {
	o := options{
		// 기본값은 SSRF 가드 dial 클라이언트 — 사용자 제공 URL이 내부/사설/루프백으로
		// 못 나가게 막는다. 리다이렉트 각 홉도 dial 단계라 자동 커버. 요청당 타임아웃은
		// fetchHTML/getJSON의 context가 건다(그래서 client Timeout은 0). 테스트는
		// WithHTTPClient로 가드 없는 rewrite 클라이언트를 주입해 httptest(127.0.0.1)로 간다.
		client:       safedial.Client(0),
		userAgent:    defaultUserAgent,
		rateInterval: defaultRateInterval,
	}
	for _, fn := range opts {
		fn(&o)
	}

	parser := NewDefaultParser(o.client, o.userAgent)
	reg := NewRegistry(parser)
	// 등록 순서 = Match 우선순위. 호스트 집합이 서로 겹치지 않아 순서 민감도는 없다.
	reg.Register(newYouTubeAdapter(parser, o.youtubeOEmbed))
	reg.Register(newTwitterAdapter(o.client, o.userAgent, o.twitterOEmbed))
	reg.Register(newNaverAdapter(parser))
	reg.Register(newInstagramAdapter())

	return &Pool{
		inner:   reg,
		limiter: newDomainLimiter(o.rateInterval),
	}
}

// NewWithScraper는 임의의 inner Scraper를 감싼 Pool을 만든다 (테스트·특수 조립용).
func NewWithScraper(inner Scraper, rateInterval time.Duration) *Pool {
	if rateInterval <= 0 {
		rateInterval = defaultRateInterval
	}
	return &Pool{inner: inner, limiter: newDomainLimiter(rateInterval)}
}

// Fetch는 도메인 rate limit을 통과한 뒤 singleflight로 중복을 합쳐 inner에 위임한다.
// singleflight 안에서 rate·fetch를 수행하므로 동시 같은 URL은 1회 HTTP·1회 토큰만 쓴다.
//
// [F3 교차 취소 방어] sf.Do로 합쳐진 실행은 첫 호출자(리더)의 ctx로 돌기 때문에,
// 리더의 ctx가 취소되면 같은 URL을 기다리던 무관한 호출자까지 context 에러를 받는다.
// 이를 막는 가장 단순한 안전책: sf.Do가 context 취소/만료 에러를 냈지만 이 호출자의
// ctx는 아직 살아 있으면, 그 취소는 이 호출자 탓이 아니므로 자신의 ctx로 한 번 더
// 시도한다(이때는 이전 공유 실행이 끝나 이 호출자가 새 리더가 되거나, 살아 있는 다른
// 리더에 합류한다). 자기 ctx가 이미 취소된 경우엔 정상적으로 그 에러를 그대로 돌려준다.
func (p *Pool) Fetch(ctx context.Context, rawURL string) (Metadata, error) {
	host := ""
	if u, err := url.Parse(rawURL); err == nil {
		host = u.Hostname()
	}
	m, err := p.fetchShared(ctx, rawURL, host)
	if err != nil && ctx.Err() == nil && isContextErr(err) {
		// 내 ctx는 멀쩡한데 공유 실행이 남의 취소로 죽었다 — 내 ctx로 한 번만 재시도.
		m, err = p.fetchShared(ctx, rawURL, host)
	}
	return m, err
}

// fetchShared는 rate limit + inner.Fetch를 singleflight로 합쳐 실행한다.
func (p *Pool) fetchShared(ctx context.Context, rawURL, host string) (Metadata, error) {
	v, err, _ := p.sf.Do(rawURL, func() (any, error) {
		if err := p.limiter.wait(ctx, host); err != nil {
			return Metadata{}, err
		}
		return p.inner.Fetch(ctx, rawURL)
	})
	return v.(Metadata), err
}

// isContextErr는 err가 context 취소/만료(또는 그것을 감싼) 에러인지 판정한다.
func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// ---- 도메인별 rate limiter (자체 구현, 외부 의존 없음) ----
//
// x/time/rate 대신 도메인마다 "다음 허용 시각"을 예약하는 방식이다. 예약이 뮤텍스 아래
// 즉시 이뤄지므로 같은 도메인 동시 요청도 interval 간격으로 직렬화된다. 대기는 ctx로 취소 가능.
const (
	// limiterTTL은 이 시간 이상 미접근한 호스트 엔트리를 정리 대상으로 본다.
	limiterTTL = 10 * time.Minute
	// limiterSweep은 정리 스캔 최소 간격 — 매 wait마다 O(n) 스캔을 돌지 않게 한다.
	limiterSweep = time.Minute
)

// limiterEntry는 호스트별 예약 상태. next는 다음 요청 허용 시각, seen은 마지막 접근 시각(정리 기준).
type limiterEntry struct {
	next time.Time
	seen time.Time
}

type domainLimiter struct {
	mu        sync.Mutex
	entries   map[string]*limiterEntry // 도메인 → 예약 상태
	interval  time.Duration
	lastSweep time.Time
}

func newDomainLimiter(interval time.Duration) *domainLimiter {
	return &domainLimiter{
		entries:   make(map[string]*limiterEntry),
		interval:  interval,
		lastSweep: time.Now(),
	}
}

// wait는 host에 대한 슬롯을 예약하고 그 시각까지 (ctx 취소 가능하게) 대기한다.
// ctx 취소로 대기가 중단되면 예약 전진분을 롤백한다 (같은 호스트 다음 요청이 불필요하게
// 한 interval 더 대기하지 않도록).
func (l *domainLimiter) wait(ctx context.Context, host string) error {
	if l.interval <= 0 {
		return nil
	}
	l.mu.Lock()
	now := time.Now()
	l.sweepLocked(now)
	e := l.entries[host]
	if e == nil {
		e = &limiterEntry{}
		l.entries[host] = e
	}
	e.seen = now
	slot := e.next
	if slot.Before(now) {
		slot = now
	}
	e.next = slot.Add(l.interval)
	l.mu.Unlock()

	d := time.Until(slot)
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		l.rollback(host, slot)
		return ctx.Err()
	}
}

// rollback은 ctx 취소로 실제 요청이 나가지 않았을 때 wait가 더한 interval 전진분을
// 되돌린다. 우리가 마지막 예약자일 때만(next가 우리가 세팅한 값 그대로일 때) 되돌려
// 이후 다른 요청의 예약을 훼손하지 않는다.
func (l *domainLimiter) rollback(host string, slot time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[host]
	if e == nil {
		return
	}
	if e.next.Equal(slot.Add(l.interval)) {
		e.next = slot
	}
}

// sweepLocked는 마지막 스캔 후 limiterSweep이 지났을 때만 오래된(TTL 초과 미접근) 호스트
// 엔트리를 제거한다 — 맵 무한 증가 방지. 호출자는 l.mu를 잡고 있어야 한다. 단일 사용자
// 스케일이라 O(n) 스캔이 드물게(분 단위) 일어나도 무해하다.
func (l *domainLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < limiterSweep {
		return
	}
	l.lastSweep = now
	for host, e := range l.entries {
		if now.Sub(e.seen) > limiterTTL {
			delete(l.entries, host)
		}
	}
}
