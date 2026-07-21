// import 서브커맨드 — 브라우저 북마크(Netscape HTML) / YouTube Takeout(HTML·CSV·JSON)에서
// URL을 추출해 POST /api/v1/links로 순차 적재한다 (M2: 실링크 300건+ 워밍, corpus_df 워밍 겸용).
//
//	pushpoint import -type bookmarks -file bookmarks.html -addr http://localhost:8080 -key dev-key
//	pushpoint import -type takeout   -file watch-history.html [-format auto|csv|json|html]
//
// Takeout 시청기록의 기본 export 형식은 HTML(watch-history.html)이며, Takeout 설정에서
// JSON을 선택하면 watch-history.json으로도 받을 수 있다 — 셋 다 지원한다(auto=내용 감지).
// 서버가 url_hash 기준 멱등이라 재실행해도 중복 저장은 200 duplicate로 안전하게 정리된다.
// 네트워크 전송은 초당 ~10건으로 rate limit (서버 부하 배려), 실패는 로그 후 계속.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// importInterval은 전송 간 최소 간격(초당 ~10건). 테스트는 sendLinks에 0을 넘겨 즉시 전송한다.
const importInterval = 100 * time.Millisecond

// runImport는 import 서브커맨드 진입점 (main.go의 switch에서 os.Args[2:]로 호출).
func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	typ := fs.String("type", "", "임포트 유형: bookmarks | takeout")
	file := fs.String("file", "", "입력 파일 경로")
	addr := fs.String("addr", "http://localhost:8080", "서버 주소")
	key := fs.String("key", "dev-key", "API 키")
	format := fs.String("format", "auto", "takeout 형식: auto | csv | json | html (auto=내용으로 감지). Takeout 기본 export는 html")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("-file 필수")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	f, err := os.Open(*file)
	if err != nil {
		return fmt.Errorf("파일 열기 실패: %w", err)
	}
	defer f.Close()

	var urls []string
	switch *typ {
	case "bookmarks":
		urls, err = extractBookmarkURLs(f)
	case "takeout":
		urls, err = extractTakeoutURLs(f, *format)
	case "":
		return fmt.Errorf("-type 필수 (bookmarks | takeout)")
	default:
		return fmt.Errorf("알 수 없는 -type %q (bookmarks | takeout)", *typ)
	}
	if err != nil {
		return fmt.Errorf("URL 추출 실패: %w", err)
	}

	urls = dedupeURLs(urls)
	if len(urls) == 0 {
		logger.Warn("추출된 URL 없음", "file", *file, "type", *typ)
		return nil
	}
	logger.Info("URL 추출 완료", "count", len(urls), "type", *typ)

	client := &http.Client{Timeout: 15 * time.Second}
	saved, dup, failed := sendLinks(context.Background(), client, *addr, *key, urls, importInterval, logger)
	logger.Info("임포트 완료", "saved", saved, "duplicate", dup, "failed", failed)
	// 부분 실패 요약은 항상 출력하되, 실패가 하나라도 있으면 종료 코드로 표면화한다
	// (전량 실패가 exit 0으로 성공처럼 보이던 문제 — main.go가 이 에러로 os.Exit(1)).
	fmt.Printf("저장 %d / 중복 %d / 실패 %d\n", saved, dup, failed)
	if failed > 0 {
		return fmt.Errorf("임포트 %d건 실패 (저장 %d / 중복 %d)", failed, saved, dup)
	}
	return nil
}

// extractBookmarkURLs는 Netscape 북마크 HTML export(<A HREF="...">)에서 모든 http(s) URL을 추출한다.
func extractBookmarkURLs(r io.Reader) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	var urls []string
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		if isHTTPURL(href) {
			urls = append(urls, strings.TrimSpace(href))
		}
	})
	return urls, nil
}

// extractTakeoutURLs는 YouTube Takeout 시청기록/좋아요에서 watch URL을 추출한다.
// Takeout 기본 export는 HTML(watch-history.html)이고, JSON 형식을 선택하면 titleUrl 배열,
// CSV는 셀 스캔이다 — 셋 다 지원한다. format이 auto면 첫 비공백 바이트로 감지한다
// (`[`·`{` → JSON, `<` → HTML, 그 외 → CSV).
func extractTakeoutURLs(r io.Reader, format string) ([]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	switch format {
	case "json":
		return parseTakeoutJSON(data)
	case "csv":
		return parseTakeoutCSV(data)
	case "html":
		return parseTakeoutHTML(data)
	case "auto", "":
		switch {
		case detectJSON(data):
			return parseTakeoutJSON(data)
		case detectHTML(data):
			return parseTakeoutHTML(data)
		default:
			return parseTakeoutCSV(data)
		}
	default:
		return nil, fmt.Errorf("알 수 없는 -format %q (auto | csv | json | html)", format)
	}
}

// detectJSON은 첫 비공백 바이트가 [ 또는 { 인지로 JSON 여부를 판정한다.
func detectJSON(data []byte) bool {
	t := bytes.TrimSpace(data)
	return len(t) > 0 && (t[0] == '[' || t[0] == '{')
}

// detectHTML은 첫 비공백 바이트가 < 인지로 HTML 여부를 판정한다 (JSON 감지 뒤에 검사).
func detectHTML(data []byte) bool {
	t := bytes.TrimSpace(data)
	return len(t) > 0 && t[0] == '<'
}

// parseTakeoutJSON은 Takeout watch-history.json(엔트리 배열, titleUrl 필드)에서 watch URL을 추출한다.
func parseTakeoutJSON(data []byte) ([]string, error) {
	var entries []struct {
		TitleURL string `json:"titleUrl"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	var urls []string
	for _, e := range entries {
		u := strings.TrimSpace(e.TitleURL)
		if isYouTubeWatchURL(u) {
			urls = append(urls, u)
		}
	}
	return urls, nil
}

// parseTakeoutCSV는 CSV(watch-history.csv 등)의 모든 셀에서 YouTube watch URL을 추출한다.
// 컬럼명·헤더 유무에 의존하지 않고 셀 값을 직접 검사하므로 형식 변형에 강하다.
func parseTakeoutCSV(data []byte) ([]string, error) {
	rd := csv.NewReader(bytes.NewReader(data))
	rd.FieldsPerRecord = -1 // 행마다 열 수가 달라도 허용
	var urls []string
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		for _, cell := range rec {
			u := strings.TrimSpace(cell)
			if isYouTubeWatchURL(u) {
				urls = append(urls, u)
			}
		}
	}
	return urls, nil
}

// parseTakeoutHTML은 Takeout 기본 export인 시청기록 HTML(watch-history.html)의
// 모든 <a href>에서 YouTube watch URL을 추출한다. Takeout HTML에는 영상 제목 링크와
// 채널 링크·검색어 링크가 섞여 있으므로(무차별 추출 방지) watch URL만 걸러낸다 —
// bookmarks 경로(모든 http(s) 추출)와 달리 JSON·CSV 파서와 같은 필터를 적용한다.
func parseTakeoutHTML(data []byte) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var urls []string
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		if u := strings.TrimSpace(href); isYouTubeWatchURL(u) {
			urls = append(urls, u)
		}
	})
	return urls, nil
}

// isHTTPURL은 절대 http(s) URL인지 판정한다 (place: 북마크에는 javascript:·about: 등이 섞인다).
func isHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// isYouTubeWatchURL은 YouTube 영상 watch URL(youtube.com/watch?v=, youtu.be/ID)인지 판정한다.
func isYouTubeWatchURL(raw string) bool {
	if !isHTTPURL(raw) {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	switch {
	case host == "youtu.be":
		return strings.Trim(u.Path, "/") != "" // youtu.be/<id>
	case host == "youtube.com" || host == "m.youtube.com" || host == "music.youtube.com":
		return u.Path == "/watch" && u.Query().Get("v") != ""
	default:
		return false
	}
}

// dedupeURLs는 순서를 유지하며 중복 URL을 제거한다 (같은 파일 내 재등장 노이즈 감소 — 서버도 멱등).
func dedupeURLs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, u := range in {
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}

// sendLinks는 urls를 POST /api/v1/links로 순차 전송한다.
// interval>0이면 요청 사이에 그만큼 쉬어 rate limit(초당 ~10)한다. 201=저장, 200=중복,
// 그 외/에러=실패(로그 후 계속). 100건마다 진행률을 로그. 반환은 (saved, dup, failed).
func sendLinks(ctx context.Context, client *http.Client, addr, key string, urls []string, interval time.Duration, logger *slog.Logger) (saved, dup, failed int) {
	endpoint := strings.TrimRight(addr, "/") + "/api/v1/links"
	for i, u := range urls {
		if i > 0 && interval > 0 {
			time.Sleep(interval)
		}
		switch code, err := postLink(ctx, client, endpoint, key, u); {
		case err != nil:
			failed++
			logger.Warn("전송 실패", "url", u, "err", err)
		case code == http.StatusCreated:
			saved++
		case code == http.StatusOK:
			dup++
		default:
			failed++
			logger.Warn("예상 밖 상태 코드", "url", u, "code", code)
		}
		if n := i + 1; n%100 == 0 {
			logger.Info("임포트 진행", "sent", n, "total", len(urls),
				"saved", saved, "duplicate", dup, "failed", failed)
		}
	}
	return saved, dup, failed
}

// postLink는 URL 한 건을 저장 API로 보내고 상태 코드를 돌려준다. 본문은 버린다.
func postLink(ctx context.Context, client *http.Client, endpoint, key, rawURL string) (int, error) {
	body, err := json.Marshal(map[string]string{"url": rawURL})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
