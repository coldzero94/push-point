package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// 사이트별 oEmbed 엔드포인트 기본값 (API 키 불필요). 테스트는 Option으로 주입 교체.
const (
	defaultYouTubeOEmbed = "https://www.youtube.com/oembed"
	defaultTwitterOEmbed = "https://publish.twitter.com/oembed"
)

// ---- YouTube 어댑터 ----
//
// oEmbed로 title/author(채널명)/thumbnail을 얻고, watch 페이지에서 og:description과
// 전체 영상 설명(body_text)을 병합한다. content_type=video. (스펙 §5 / 04-DATA-FLOW §1)

type youtubeAdapter struct {
	parser     *DefaultParser // watch 페이지 og:description 병합용 (client·UA 공유)
	oembedBase string
}

var _ Adapter = (*youtubeAdapter)(nil)

func newYouTubeAdapter(parser *DefaultParser, oembedBase string) *youtubeAdapter {
	if oembedBase == "" {
		oembedBase = defaultYouTubeOEmbed
	}
	return &youtubeAdapter{parser: parser, oembedBase: oembedBase}
}

func (a *youtubeAdapter) Match(u *url.URL) bool {
	return hostMatches(u.Host, "youtube.com") || hostMatches(u.Host, "youtu.be")
}

// oembedResponse는 youtube/twitter oEmbed 응답의 공통 관심 필드.
type oembedResponse struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	ThumbnailURL string `json:"thumbnail_url"`
	HTML         string `json:"html"`
}

func (a *youtubeAdapter) Fetch(ctx context.Context, u *url.URL) (Metadata, error) {
	endpoint := a.oembedBase + "?format=json&url=" + url.QueryEscape(u.String())
	var oe oembedResponse
	if err := getJSON(ctx, a.parser.client, a.parser.userAgent, endpoint, &oe); err != nil {
		return Metadata{}, fmt.Errorf("scraper: youtube oEmbed 실패 %s: %w", u, err)
	}
	m := Metadata{
		Title:       strings.TrimSpace(oe.Title),
		Author:      strings.TrimSpace(oe.AuthorName),
		SiteName:    "YouTube",
		ImageURL:    strings.TrimSpace(oe.ThumbnailURL),
		ContentType: "video",
	}
	// watch 페이지 og:description 병합 — best-effort. 실패해도 oEmbed 결과는 유지한다.
	if doc, page, finalURL, err := a.parser.fetchHTML(ctx, u); err == nil {
		meta := parseMetadata(doc, finalURL)
		if meta.Description != "" {
			m.Description = meta.Description
		}
		if m.Title == "" {
			m.Title = meta.Title
		}
		if meta.PublishedAt != nil {
			m.PublishedAt = meta.PublishedAt
		}
		// 영상엔 아티클 본문이 없으므로 trafilatura 대신 **전체 영상 설명**을 body_text로 쓴다.
		// og:description은 유튜브가 160자로 자르지만 페이지 JSON의 shortDescription은 전문이라
		// 실측 6~21배 길다(932~3416자). 이미 받은 페이지에서 뽑으므로 추가 요청은 없다.
		// 자막(timedtext)이 진짜 발화 내용이지만 유튜브가 차단한다(실측: HTTP 200 + 0바이트) —
		// 그래서 "크리에이터가 쓴 설명"이 얻을 수 있는 최선의 내용 신호다.
		m.BodyText = youtubeDescription(page)
	}
	return m, nil
}

// youtubeDescription은 watch 페이지 JSON에서 shortDescription(전체 영상 설명)을 뽑는다.
// 값은 JSON 이스케이프 문자열이라 백슬래시를 존중하며 닫는 따옴표를 찾은 뒤 json으로 언이스케이프한다
// (정규식 `(.*?)","`는 설명에 `","`가 들어가면 깨진다). 못 찾으면 빈 문자열 — best-effort.
func youtubeDescription(page []byte) string {
	const key = `"shortDescription":"`
	i := bytes.Index(page, []byte(key))
	if i < 0 {
		return ""
	}
	start := i + len(key) - 1 // 여는 따옴표부터
	for j := start + 1; j < len(page); j++ {
		switch page[j] {
		case '\\':
			j++ // 이스케이프된 다음 바이트는 건너뛴다
		case '"':
			var s string
			if err := json.Unmarshal(page[start:j+1], &s); err != nil {
				return ""
			}
			return capRunes(strings.TrimSpace(s), maxBodyText)
		}
	}
	return ""
}

// ---- X / Twitter 어댑터 ----
//
// publish.twitter.com/oembed로 author_name과 본문(html)을 얻는다. content_type=post.

type twitterAdapter struct {
	client     *http.Client
	userAgent  string
	oembedBase string
}

var _ Adapter = (*twitterAdapter)(nil)

func newTwitterAdapter(client *http.Client, userAgent, oembedBase string) *twitterAdapter {
	if oembedBase == "" {
		oembedBase = defaultTwitterOEmbed
	}
	return &twitterAdapter{client: client, userAgent: userAgent, oembedBase: oembedBase}
}

func (a *twitterAdapter) Match(u *url.URL) bool {
	return hostMatches(u.Host, "twitter.com") || hostMatches(u.Host, "x.com")
}

func (a *twitterAdapter) Fetch(ctx context.Context, u *url.URL) (Metadata, error) {
	endpoint := a.oembedBase + "?format=json&omit_script=1&url=" + url.QueryEscape(u.String())
	var oe oembedResponse
	if err := getJSON(ctx, a.client, a.userAgent, endpoint, &oe); err != nil {
		return Metadata{}, fmt.Errorf("scraper: twitter oEmbed 실패 %s: %w", u, err)
	}
	author := strings.TrimSpace(oe.AuthorName)
	m := Metadata{
		Author:      author,
		SiteName:    "X",
		ContentType: "post",
	}
	// oEmbed html은 <blockquote><p>본문</p> &mdash; 작성자 (@handle) <a>날짜</a></blockquote>
	// 형태다. blockquote 안의 <p> 텍스트만 뽑아 본문으로 쓴다 — 뒤따르는 작성자·날짜
	// 귀속이 description을 오염시키지 않도록(문서 전체 stripHTML은 귀속을 섞는다).
	body := tweetBody(oe.HTML)
	m.Description = body
	// 제목이 따로 없는 매체라 작성자를 제목으로 둔다 (본문은 description).
	if author != "" {
		m.Title = author
	} else {
		m.Title = body
	}
	return m, nil
}

// tweetBody는 twitter oEmbed html 조각에서 blockquote 안의 <p> 텍스트만 뽑아 공백을
// 정리한다. blockquote 밖에 붙는 작성자·날짜 귀속("&mdash; 홍길동 (@hong) 2026년…")은
// <p> 밖이라 자연히 제외된다 — 문서 전체 텍스트를 벗기던 이전 방식의 오염을 없앤다.
func tweetBody(fragment string) string {
	fragment = strings.TrimSpace(fragment)
	if fragment == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fragment))
	if err != nil {
		return ""
	}
	var parts []string
	doc.Find("blockquote p").Each(func(_ int, s *goquery.Selection) {
		if t := strings.TrimSpace(s.Text()); t != "" {
			parts = append(parts, t)
		}
	})
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

// ---- 네이버 블로그 어댑터 ----
//
// blog.naver.com은 데스크톱 페이지가 iframe이라 og가 비어 있다 → m.blog.naver.com으로
// 재작성 후 기본 파서. content_type=article (기본 파서가 호스트 휴리스틱으로 판정).

type naverAdapter struct {
	parser *DefaultParser
}

var _ Adapter = (*naverAdapter)(nil)

func newNaverAdapter(parser *DefaultParser) *naverAdapter {
	return &naverAdapter{parser: parser}
}

func (a *naverAdapter) Match(u *url.URL) bool {
	return hostMatches(u.Host, "blog.naver.com")
}

func (a *naverAdapter) Fetch(ctx context.Context, u *url.URL) (Metadata, error) {
	rewritten := *u // 얕은 복사 후 호스트만 교체 (경로·쿼리 보존).
	if !strings.HasPrefix(strings.ToLower(u.Host), "m.") {
		rewritten.Host = "m.blog.naver.com"
	}
	return a.parser.Fetch(ctx, &rewritten)
}

// ---- Instagram 어댑터 ----
//
// 로그인·봇 차단으로 og 메타가 없는 경우가 많다 → 메타 부재를 에러가 아니라 정상으로 본다.
// domain+URL만으로 done. HTTP 요청조차 하지 않는다. content_type=post.

type instagramAdapter struct{}

var _ Adapter = (*instagramAdapter)(nil)

func newInstagramAdapter() *instagramAdapter { return &instagramAdapter{} }

func (a *instagramAdapter) Match(u *url.URL) bool {
	return hostMatches(u.Host, "instagram.com")
}

func (a *instagramAdapter) Fetch(context.Context, *url.URL) (Metadata, error) {
	// 빈 메타데이터(도메인만) 반환 — 에러 아님.
	return Metadata{ContentType: "post"}, nil
}

// ---- oEmbed JSON GET 헬퍼 ----

// getJSON은 endpoint를 GET해 out으로 디코드한다. 요청당 10s 타임아웃·5MB 상한·UA 설정.
// 2xx가 아니면 에러(재시도 대상).
func getJSON(ctx context.Context, client *http.Client, userAgent, endpoint string, out any) error {
	if client == nil {
		client = http.DefaultClient
	}
	if userAgent == "" {
		userAgent = defaultUserAgent
	}
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("요청 생성 실패: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("예상 밖 상태 코드 %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("JSON 디코드 실패: %w", err)
	}
	return nil
}
