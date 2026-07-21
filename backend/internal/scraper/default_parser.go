package scraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// 스크랩 요청 공통 상수 (스펙 docs/v2/05 §4·§6).
const (
	// defaultUserAgent은 대상 서버가 봇 차단·빈 응답을 주지 않도록 식별 가능한 UA를 보낸다.
	defaultUserAgent = "Push-PointBot/2.0 (+https://github.com/coby/push-point)"
	// requestTimeout은 요청당 context timeout — 느린 대상이 워커를 붙잡지 않게 한다.
	requestTimeout = 10 * time.Second
	// maxBodyBytes는 응답 본문 상한(5MB) — 거대한 페이지가 메모리를 삼키지 않도록 LimitReader로 자른다.
	maxBodyBytes = 5 << 20
)

// DefaultParser는 사이트 어댑터가 매치되지 않을 때 쓰이는 범용 og 파서다.
// net/http GET → goquery 파싱으로 og:*, meta, <title>, html[lang]을 추출한다.
// Registry의 fallback으로 등록되므로 Match는 항상 true(모든 URL 처리).
type DefaultParser struct {
	client    *http.Client
	userAgent string
}

// 컴파일 타임 인터페이스 검증.
var _ Adapter = (*DefaultParser)(nil)

// NewDefaultParser는 HTTP 클라이언트와 User-Agent를 주입받아 파서를 만든다.
// client가 nil이면 http.DefaultClient를, userAgent가 비면 defaultUserAgent를 쓴다.
func NewDefaultParser(client *http.Client, userAgent string) *DefaultParser {
	if client == nil {
		client = http.DefaultClient
	}
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	return &DefaultParser{client: client, userAgent: userAgent}
}

// Match는 fallback 파서라 모든 URL을 처리한다 (Registry가 fallback으로 부를 때만 사용).
func (p *DefaultParser) Match(*url.URL) bool { return true }

// Fetch는 HTML을 내려받아 파싱한 메타데이터를 반환한다.
func (p *DefaultParser) Fetch(ctx context.Context, u *url.URL) (Metadata, error) {
	doc, finalURL, err := p.fetchHTML(ctx, u)
	if err != nil {
		return Metadata{}, err
	}
	m := parseMetadata(doc, finalURL)
	m.ContentType = contentTypeFor(u.Host)
	return m, nil
}

// fetchHTML은 u를 GET해 goquery 문서와 리다이렉트 후 최종 URL을 반환한다.
// 요청당 10s 타임아웃, User-Agent 설정, 본문 5MB 상한. 2xx가 아니면 에러(재시도 대상).
func (p *DefaultParser) fetchHTML(ctx context.Context, u *url.URL) (*goquery.Document, *url.URL, error) {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel() // goquery가 본문을 함수 반환 전에 모두 읽으므로 defer로 안전.

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("scraper: 요청 생성 실패 %s: %w", u, err)
	}
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "ko,en;q=0.8")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("scraper: GET 실패 %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("scraper: 예상 밖 상태 코드 %d (%s)", resp.StatusCode, u)
	}

	doc, err := ParseHTML(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("scraper: HTML 파싱 실패 %s: %w", u, err)
	}
	// 리다이렉트를 따라간 최종 URL — 상대 og:image 해석 기준.
	finalURL := u
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL
	}
	return doc, finalURL, nil
}

// parseMetadata는 goquery 문서에서 og:*, meta, <title>, html[lang]을 뽑는다.
// base는 상대 og:image URL을 절대 URL로 해석하는 기준이다. ContentType은 호출자가 채운다.
func parseMetadata(doc *goquery.Document, base *url.URL) Metadata {
	var m Metadata

	// 제목: og:title 우선, 없으면 <title>.
	m.Title = metaContent(doc, "og:title")
	if m.Title == "" {
		m.Title = strings.TrimSpace(doc.Find("title").First().Text())
	}
	m.Description = metaContent(doc, "og:description", "description")
	m.Author = metaContent(doc, "author", "article:author")
	m.SiteName = metaContent(doc, "og:site_name")

	// lang: html[lang]. "en-US" 등은 그대로 둔다 (정규화는 태거 몫).
	if lang, ok := doc.Find("html").First().Attr("lang"); ok {
		m.Lang = strings.TrimSpace(lang)
	}

	// article:published_time → unix epoch 초.
	if ts := parseTime(metaContent(doc, "article:published_time", "og:article:published_time")); ts != nil {
		m.PublishedAt = ts
	}

	// og:image → 상대 URL이면 base 기준으로 절대화.
	if img := metaContent(doc, "og:image", "og:image:url"); img != "" {
		m.ImageURL = resolveURL(base, img)
	}
	return m
}

// metaContent는 keys를 순서대로 훑어 property/name 어느 쪽이든 첫 비어 있지 않은
// content 속성을 반환한다. og:*는 property, 표준 meta는 name을 쓰는 사이트가 섞여 있어 둘 다 본다.
func metaContent(doc *goquery.Document, keys ...string) string {
	for _, k := range keys {
		for _, attr := range []string{"property", "name"} {
			sel := fmt.Sprintf(`meta[%s=%q]`, attr, k)
			if v, ok := doc.Find(sel).First().Attr("content"); ok {
				if v = strings.TrimSpace(v); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// resolveURL은 ref가 상대 URL이면 base 기준으로 절대화한다. 파싱 실패 시 원문을 그대로 돌려준다.
func resolveURL(base *url.URL, ref string) string {
	r, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ref
	}
	if base == nil {
		return r.String()
	}
	return base.ResolveReference(r).String()
}

// parseTime은 흔한 몇 가지 시각 표기를 unix epoch 초로 파싱한다. 실패하면 nil.
func parseTime(s string) *int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04:05", // 타임존 없음 → UTC 가정
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			sec := t.Unix()
			return &sec
		}
	}
	return nil
}

// contentTypeFor는 호스트 휴리스틱으로 content_type을 판정한다 (스펙 §5):
// youtube/vimeo → video, twitter/x/instagram → post, 그 외 → article.
// 반환값은 links.content_type CHECK 제약('video'|'article'|'post'|'other')을 만족한다.
func contentTypeFor(host string) string {
	switch {
	case hostMatches(host, "youtube.com"), hostMatches(host, "youtu.be"), hostMatches(host, "vimeo.com"):
		return "video"
	case hostMatches(host, "twitter.com"), hostMatches(host, "x.com"), hostMatches(host, "instagram.com"):
		return "post"
	default:
		return "article"
	}
}

// hostMatches는 host가 domain이거나 그 하위 도메인이면 true (대소문자·후행 점 무시).
func hostMatches(host, domain string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	// 포트가 붙어 있으면 제거 (테스트의 httptest 주소 등).
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}
