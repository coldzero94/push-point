package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/markusmobius/go-trafilatura"

	"github.com/coby/push-point/backend/internal/textutil"
)

// 스크랩 요청 공통 상수 (스펙 docs/v2/ko/05 §4·§6).
const (
	// defaultUserAgent은 대상 서버가 봇 차단·빈 응답을 주지 않도록 식별 가능한 UA를 보낸다.
	defaultUserAgent = "Push-PointBot/2.0 (+https://github.com/coby/push-point)"
	// requestTimeout은 요청당 context timeout — 느린 대상이 워커를 붙잡지 않게 한다.
	requestTimeout = 10 * time.Second
	// maxBodyBytes는 응답 본문 상한(5MB) — 거대한 페이지가 메모리를 삼키지 않도록 LimitReader로 자른다.
	maxBodyBytes = 5 << 20
	// maxBodyText는 추출 본문 저장 상한. 저장 API의 클라이언트 캡처와 **같은 값**을 써야
	// "어디로 들어왔느냐"에 따라 저장 형태가 달라지지 않는다 — 그래서 textutil이 원본이다.
	maxBodyText = textutil.MaxBodyText
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

// Fetch는 HTML을 내려받아 파싱한 메타데이터 + 추출 본문을 반환한다.
func (p *DefaultParser) Fetch(ctx context.Context, u *url.URL) (Metadata, error) {
	// 원본 바이트는 여기서 필요 없다 — 어댑터(youtube shortDescription)만 쓴다.
	doc, _, finalURL, err := p.fetchHTML(ctx, u)
	if err != nil {
		return Metadata{}, err
	}
	m := parseMetadata(doc, finalURL)
	m.ContentType = contentTypeFor(u.Host)
	m.BodyText = extractBodyText(doc, finalURL)
	// 200을 받았어도 내용이 벽이면 성공이 아니다 — 벽의 문구를 저장하면 그게 태그가 된다
	// (blocked.go의 ErrBlockedPage 주석 참조). 메타데이터는 버린다: 벽의 제목을 남기면
	// 목록에 `Reddit - Please wait for verification`이 뜬다.
	if isBlockedPage(m.Title, m.Description, m.BodyText) {
		return Metadata{}, ErrBlockedPage
	}
	return m, nil
}

// fetchHTML은 u를 GET해 goquery 문서, 원본 HTML 바이트, 리다이렉트 후 최종 URL을 반환한다.
// 본문을 한 번 버퍼링해 goquery(메타)와 go-trafilatura(본문) 양쪽에 재사용한다 — 재fetch 없음.
// 요청당 10s 타임아웃, User-Agent 설정, 본문 5MB 상한. 2xx가 아니면 에러(재시도 대상).
func (p *DefaultParser) fetchHTML(ctx context.Context, u *url.URL) (*goquery.Document, []byte, *url.URL, error) {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scraper: 요청 생성 실패 %s: %w", u, err)
	}
	req.Header.Set("User-Agent", p.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "ko,en;q=0.8")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scraper: GET 실패 %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, nil, fmt.Errorf("scraper: 예상 밖 상태 코드 %d (%s)", resp.StatusCode, u)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scraper: 본문 읽기 실패 %s: %w", u, err)
	}
	doc, err := ParseHTMLCharset(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("scraper: HTML 파싱 실패 %s: %w", u, err)
	}
	// 리다이렉트를 따라간 최종 URL — 상대 og:image 해석 기준.
	finalURL := u
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL
	}
	return doc, body, finalURL, nil
}

// extractBodyText는 go-trafilatura로 본문(보일러플레이트 제거)을 뽑아 상한(maxBodyText)으로
// 자른다. 추출 실패는 치명적이지 않다 — 본문 없이 진행하고 태거가 title/description으로
// graceful degrade한다(비-아티클·SPA·추출 실패 시 빈 문자열). EnableFallback으로 trafilatura가
// 실패하면 readability·domdistiller로 폴백해 정밀도를 높인다.
//
// **이미 파싱한 goquery 문서를 넘긴다** — 바이트를 다시 읽히면 trafilatura가 자체 문자셋
// 변환 리더를 태우는데, 정상 UTF-8 한국 기술블로그(카카오뱅크·토스·우아한형제들 등)에서
// `transform: short internal buffer`로 실패해 본문이 통째로 비었다(실측: 0B → 4~25KB로 복구).
// html.Parse는 그 페이지들을 문제없이 처리하므로 파싱 결과를 재사용하는 편이 옳고, 재파싱도
// 없어진다. ExtractDocument는 내부에서 dom.Clone으로 원본을 보존하므로 doc은 변형되지 않는다.
func extractBodyText(doc *goquery.Document, u *url.URL) string {
	if doc == nil || len(doc.Nodes) == 0 {
		return ""
	}
	res, err := trafilatura.ExtractDocument(doc.Nodes[0], trafilatura.Options{
		OriginalURL:     u,
		EnableFallback:  true,
		ExcludeComments: true,
	})
	if err != nil || res == nil {
		return ""
	}
	// 정제까지 textutil에 위임한다 — 스크랩 결과에도 제어문자·불완전 UTF-8이 섞일 수 있고,
	// 클라이언트 캡처와 같은 규칙을 적용해야 저장 형태가 출처에 따라 갈라지지 않는다.
	body := textutil.Clean(res.ContentText, maxBodyText, true)
	// 추출이 본문 대신 **사이트 하단 고지**를 물어온 경우가 있다 — 그러면 거기서 나온 태그는
	// 링크가 아니라 회사의 법적 고지를 설명한다(boilerplate.go 주석 참조). 빈 본문이 낫다.
	if isFooterOnly(body) {
		return ""
	}
	return body
}

// capRunes는 textutil.CapRunes의 패키지 내 별칭 (어댑터들이 쓰는 짧은 이름 유지).
func capRunes(s string, limit int) string { return textutil.CapRunes(s, limit) }

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

	m.Keywords = parseKeywords(doc)
	return m
}

// parseKeywords는 **발행자가 스스로 붙인 분류**를 모은다 — 우리가 본문에서 추론한 것이
// 아니라 사이트가 "이 글은 스포츠다"라고 선언한 값이다.
//
// 도메인 맵과 같은 급의 신호이면서 그보다 일반적이다: 도메인 맵은 사이트를 하나씩
// 등록해야 하지만 이 메타 태그들은 사실상 모든 뉴스·블로그 CMS가 내보낸다. 등록되지 않은
// 사이트에서도 동작한다는 뜻이다.
//
// 네 출처를 다 모으고 합집합을 쓴다(하나만 고르지 않는다) — 사이트마다 쓰는 태그가 다르고,
// 여러 개가 있을 때 어느 하나가 더 정확하다고 볼 근거가 없다. 태거는 매칭 수를 matchCap으로
// 자르므로 중복 나열이 점수를 부풀리지도 않는다.
func parseKeywords(doc *goquery.Document) string {
	var parts []string
	seen := map[string]bool{}
	add := func(v string) {
		// 콤마로 나눠 넣는 이유는 중복 제거를 위해서다. 태거는 어차피 다시 토큰화한다.
		for _, f := range strings.Split(v, ",") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if key := strings.ToLower(f); !seen[key] {
				seen[key] = true
				parts = append(parts, f)
			}
		}
	}

	// news_keywords는 Google News 규격 — keywords보다 정제된 값을 넣는 사이트가 많다.
	add(metaContent(doc, "keywords"))
	add(metaContent(doc, "news_keywords"))
	// article:section은 섹션(정치·스포츠), article:tag는 개별 주제어. 둘 다 여러 개일 수 있어
	// metaContent(첫 값만)가 아니라 전부 훑는다.
	doc.Find(`meta[property="article:section"], meta[property="article:tag"], meta[name="article:section"], meta[name="article:tag"]`).
		Each(func(_ int, s *goquery.Selection) {
			if v, ok := s.Attr("content"); ok {
				add(v)
			}
		})
	// JSON-LD — og를 안 쓰고 schema.org만 쓰는 사이트가 있다. 스키마 모양이
	// 사이트마다 제각각(배열·@graph·중첩)이라 구조를 가정하지 않고 키만 찾아 훑는다.
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, s *goquery.Selection) {
		var v any
		if json.Unmarshal([]byte(s.Text()), &v) != nil {
			return // 깨진 JSON-LD는 흔하다 — 조용히 넘긴다. 이건 보조 신호다.
		}
		// 정렬한다. collectJSONLDKeywords가 맵을 순회해 순서가 실행마다 달라지는데,
		// 512바이트에서 잘릴 때 **어느 분류가 살아남는지**가 그 순서에 좌우된다.
		// 지금 golden에서는 최대 315바이트라 초과가 없지만, 같은 페이지가 실행마다
		// 다른 태그를 얻는 상태를 남겨 둘 이유가 없다.
		kws := collectJSONLDKeywords(v, 0)
		slices.Sort(kws)
		for _, kw := range kws {
			add(kw)
		}
	})

	return textutil.CleanMeta(strings.Join(parts, ", "), textutil.MaxKeywords)
}

// collectJSONLDKeywords는 임의 모양의 JSON-LD 값을 훑어 articleSection·keywords·genre의
// 문자열 값을 모은다. depth는 순환 참조가 아니라 **깊이 폭주**를 막는 안전장치다
// (json.Unmarshal 결과는 순환할 수 없지만 중첩은 얼마든지 깊을 수 있다).
func collectJSONLDKeywords(v any, depth int) []string {
	if depth > 8 {
		return nil
	}
	var out []string
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			switch strings.ToLower(k) {
			case "articlesection", "keywords", "genre":
				out = append(out, jsonLDStrings(val)...)
			default:
				out = append(out, collectJSONLDKeywords(val, depth+1)...)
			}
		}
	case []any:
		for _, e := range t {
			out = append(out, collectJSONLDKeywords(e, depth+1)...)
		}
	}
	return out
}

// jsonLDStrings는 문자열 하나 또는 문자열 배열 — 두 모양 다 허용하는 스펙을 평탄화한다.
func jsonLDStrings(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
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
