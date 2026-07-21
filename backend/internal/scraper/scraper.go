// Package scraper는 URL에서 링크 메타데이터를 추출하는 계약과 어댑터 레지스트리를 정의한다.
// (스펙 docs/v2/05 §5, 04-DATA-FLOW §1 참조)
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
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/PuerkitoBio/goquery"
)

// ParseHTML은 HTML 응답 본문을 goquery 문서로 파싱한다 — fallback og 파서와
// 사이트 어댑터가 공유하는 primitive. (다음 단계 구현이 이 위에서 og:* / meta를 뽑는다.)
func ParseHTML(r io.Reader) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(r)
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
