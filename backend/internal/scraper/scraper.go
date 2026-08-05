// Package scraper는 URL에서 링크 메타데이터를 추출하는 계약과 어댑터 레지스트리를 정의한다.
// (스펙 docs/v2/ko/05 §5, 04-DATA-FLOW §1 참조)
//
// [경계]
//   - Metadata는 스크랩 산출물의 도메인 표현이다. links 테이블 컬럼으로의 매핑(store.ScrapeResult)과
//     status 전이는 scrape 잡 핸들러(internal/api 또는 worker 계층)의 책임이다 — 이 패키지는 파싱만 한다.
//   - Adapter는 사이트별 분기(youtube oEmbed, naver blog 재작성 등)를, Registry는 순서 있는
//     첫 Match 우선 라우팅 + 기본 og 파서 fallback을 담당한다.
//
// 외부 네트워크 의존이므로 테스트는 fixture HTTP 서버로 결정적으로 구성한다.
package scraper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"
)

// ParseHTML은 이미 UTF-8인 HTML을 goquery 문서로 파싱한다 — 테스트 fixture처럼 인코딩이
// 확실한 입력용이다. **네트워크 응답에는 ParseHTMLCharset을 쓴다.**
func ParseHTML(r io.Reader) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(r)
}

// ParseHTMLCharset은 UTF-8이 아닐 수 있는 HTML을 변환해서 파싱한다.
//
// **한국 웹에는 아직 EUC-KR/CP949 페이지가 있다.** `news.naver.com`의 레거시 랭킹 페이지가
// 그렇고, UTF-8로 읽으면 제목이 `���̹� ����`가 된다(2026-07-26 wild 세트 16행에 그대로
// 박제돼 있다). 깨진 글자는 사전의 어떤 표면과도 안 맞으므로 그 링크는 **제목·본문 전체가
// 태깅에서 사라진다.** 그런데 상태는 `done`이고 오류도 로그도 없다.
//
// charset.NewReader의 판정 순서는 BOM → Content-Type의 charset 파라미터 → 앞부분
// `<meta charset>` / `<meta http-equiv>` → 통계적 추정이다. 아무것도 못 찾으면 입력을
// 그대로 흘린다(정상 UTF-8이 손상되지 않는다는 뜻이다).
//
// **trafilatura의 자체 변환과 혼동하지 말 것.** 그쪽은 정상 UTF-8 한국 기술블로그에서
// `transform: short internal buffer`로 실패해 본문을 통째로 날렸고(default_parser.go의
// extractBodyText 주석), 그래서 파싱된 문서를 넘기는 것으로 우회했다. 여기 변환은 x/net의
// 다른 구현이고 파싱 **직전** 한 번만 태운다. 그 회귀가 재발하지 않는지는 실제 URL 재수집으로
// 확인했다 — 근거는 이 변경의 커밋 메시지.
//
// **io.Reader가 아니라 []byte를 받는 이유**: charset.NewReader는 판정을 위해 앞부분을
// 미리 읽는다. 실패했을 때 같은 Reader를 다시 쓰면 그 앞부분이 이미 소비돼 잘린 HTML을
// 파싱하게 된다. 바이트를 들고 있으면 어떤 경로로 가든 온전한 원본에서 다시 시작한다.
//
// 아래 에러 분기는 **입력이 bytes.Reader인 한 도달하지 않는다** — charset.NewReader가
// 에러를 내는 경우는 프리뷰 읽기가 실패할 때뿐이고 메모리 리더는 실패하지 않는다.
// 알 수 없는 charset 이름은 에러가 아니라 sniffing으로 흘러간다. 그래서 이 분기에는
// **테스트가 없다**(뮤테이션을 넣어도 아무 테스트가 죽지 않는 것을 확인했다). 지우지 않는
// 이유는 시그니처가 error를 반환하는 이상 nil cr을 goquery에 넘기는 쪽이 더 나쁘기 때문이다.
func ParseHTMLCharset(b []byte, contentType string) (*goquery.Document, error) {
	cr, err := charset.NewReader(bytes.NewReader(b), contentType)
	if err != nil {
		return goquery.NewDocumentFromReader(bytes.NewReader(b))
	}
	return goquery.NewDocumentFromReader(cr)
}

// Metadata는 스크랩으로 추출한 링크 메타데이터. 시각은 unix epoch 초, NULL 가능 값은 포인터.
type Metadata struct {
	Title       string // <title> 또는 og:title
	Description string // og:description / meta description
	Author      string // author 메타 / og:site 채널명(youtube 등)
	SiteName    string // og:site_name (links 컬럼 없음 — 태거 입력 피처·domain 보정에 사용)
	ContentType string // 'video' | 'article' | 'post' | 'other' — 도메인·URL 휴리스틱 결과
	Lang        string // html lang 속성
	PublishedAt *int64 // article:published_time. 없으면 nil
	DurationSec *int64 // 영상 길이(초). 없으면 nil
	WordCount   *int64 // 본문 단어 수. 없으면 nil
	ImageURL    string // og:image 썸네일 원본 URL. 없으면 ""
	BodyText    string // go-trafilatura로 추출한 본문(보일러플레이트 제거). 비-아티클/추출실패면 ""
	Keywords    string // 발행자 분류: meta keywords · article:section · JSON-LD articleSection
}

// Scraper는 URL 하나를 메타데이터로 변환한다. 구현은 Registry(어댑터 라우팅)가 제공한다.
type Scraper interface {
	Fetch(ctx context.Context, rawURL string) (Metadata, error)
}

// Adapter는 특정 사이트 집합을 처리하는 스크래퍼다. Match가 true인 첫 어댑터가 선택된다.
type Adapter interface {
	// Match는 u를 이 어댑터가 처리하는지 판정한다 (호스트 등 기준).
	Match(u *url.URL) bool
	// Fetch는 파싱된 URL에서 메타데이터를 추출한다.
	Fetch(ctx context.Context, u *url.URL) (Metadata, error)
}

// Registry는 순서 있는 어댑터 슬라이스 + 기본 og 파서 fallback을 묶어 Scraper를 구현한다.
// 첫 Match가 우선하고, 아무 어댑터도 안 맞으면 fallback(범용 og 파서)이 처리한다.
type Registry struct {
	adapters []Adapter
	fallback Adapter // 기본 og 파서 (nil이면 Fetch가 에러)
}

// 컴파일 타임 인터페이스 검증.
var _ Scraper = (*Registry)(nil)

// NewRegistry는 fallback og 파서를 받아 빈 레지스트리를 만든다. 어댑터는 Register로 순서대로 추가한다.
func NewRegistry(fallback Adapter) *Registry {
	return &Registry{fallback: fallback}
}

// Register는 어댑터를 등록 순서대로 추가한다 (먼저 등록된 것이 우선 Match).
func (r *Registry) Register(a Adapter) { r.adapters = append(r.adapters, a) }

// pick은 u에 맞는 첫 어댑터를 반환한다. 없으면 fallback (nil일 수 있음).
func (r *Registry) pick(u *url.URL) Adapter {
	for _, a := range r.adapters {
		if a.Match(u) {
			return a
		}
	}
	return r.fallback
}

// Fetch는 rawURL을 파싱해 맞는 어댑터(없으면 fallback)로 위임한다.
func (r *Registry) Fetch(ctx context.Context, rawURL string) (Metadata, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Metadata{}, fmt.Errorf("scraper: URL 파싱 실패 %q: %w", rawURL, err)
	}
	a := r.pick(u)
	if a == nil {
		return Metadata{}, fmt.Errorf("scraper: 처리할 어댑터 없음 (fallback 미설정): %s", rawURL)
	}
	return a.Fetch(ctx, u)
}
