package main

// reader-eval — 추출의 **모양**을 잰다 (`just eval-reader`).
//
// **왜 새 하네스인가.** 기존 골든(dev/test/wild.jsonl)에는 원본 HTML이 없다 —
// `snapshot{title, description, body_text}`뿐이다. 그래서 `just eval`과 `just eval-search`는
// **이미 추출된** body_text를 읽고, 추출을 어떻게 바꿔도 그 숫자들은 움직이지 않는다.
// 추출의 변화를 볼 수 있는 자가 이 저장소에 하나도 없었다.
//
// **무엇을 재는가.** 품질이 아니라 모양이다. 주요 추출기들의 기사 본문 F1은 0.92~0.93로
// 비슷하고, 우리가 없는 것은 정확도가 아니라 **경계**다. 그래서 중심 지표가 "벽 점수" —
// 블록 경계 없이 이어지는 가장 긴 구간이다. 사람이 읽을 수 없게 만드는 것이 정확히 그것이고,
// 요약기가 문장을 못 나누는 이유도 그것이다.
//
// **두 경로를 같은 HTML로 잰다.** 서버(`scraper.BodyTextForEval` — 런타임과 같은 코드)와
// 클라이언트(`extension/src/extract.js`를 node로 실제 실행). 규칙을 Go로 옮겨 적으면
// 재는 것이 출하품이 아니라 사본이 된다.
//
// **품질 심판이 아니라 회귀 탐지기다.** `-dump`로 눈으로 볼 수 있게 하고, 판정은 사람이 한다.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/coby/push-point/backend/internal/scraper"
)

// wallThreshold는 "벽"으로 볼 경계 없는 구간의 길이(룬).
//
// 2000은 임의의 반올림이 아니다 — `tagger/classify.go`가 본문 길이 2000룬마다 태그 후보를
// 하나 더 요구하고(`need = 1 + runes/2000`), 요약기가 문장을 고르는 단위도 그 아래다.
// 경계 없이 2000룬이 이어지면 두 소비자 모두 한 덩어리를 한 문장으로 본다.
const wallThreshold = 2000

type pageResult struct {
	File   string
	Host   string
	Server wallStat
	Client wallStat
}

type wallStat struct {
	Runes    int
	Blocks   int // 줄바꿈으로 나뉜 비어 있지 않은 조각 수
	Longest  int // 경계 없는 최장 구간(룬)
	IsWall   bool
	Empty    bool
	FirstRow string
}

func measure(text string) wallStat {
	st := wallStat{Runes: utf8.RuneCountInString(text)}
	if strings.TrimSpace(text) == "" {
		st.Empty = true
		return st
	}
	for _, seg := range strings.Split(text, "\n") {
		if strings.TrimSpace(seg) == "" {
			continue
		}
		st.Blocks++
		if n := utf8.RuneCountInString(seg); n > st.Longest {
			st.Longest = n
		}
	}
	st.IsWall = st.Longest >= wallThreshold
	first := strings.SplitN(strings.TrimSpace(text), "\n", 2)[0]
	st.FirstRow = capRunesLocal(first, 72)
	return st
}

func capRunesLocal(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	out := []rune(s)[:n]
	return string(out) + "…"
}

func runReaderEval(args []string) error {
	fs := flag.NewFlagSet("reader-eval", flag.ExitOnError)
	dir := fs.String("dir", "../nlu/golden/reader", "코퍼스 디렉터리")
	dump := fs.String("dump", "", "이 호스트의 첫 줄들을 그대로 찍는다(눈으로 보는 자리)")
	nodeDir := fs.String("node-dir", "../frontend", "extract_via_node.mjs가 있는 디렉터리")
	if err := fs.Parse(args); err != nil {
		return err
	}

	idxRaw, err := os.ReadFile(filepath.Join(*dir, "index.json"))
	if err != nil {
		return fmt.Errorf("reader-eval: 코퍼스 색인을 못 읽었다 (scripts/fetch_reader_corpus.py를 먼저 돌린다): %w", err)
	}
	var idx struct {
		Pages []struct {
			File string `json:"file"`
			URL  string `json:"url"`
			Host string `json:"host"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(idxRaw, &idx); err != nil {
		return err
	}

	var results []pageResult
	for _, pg := range idx.Pages {
		raw, err := readGz(filepath.Join(*dir, pg.File))
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [skip] %s: %v\n", pg.File, err)
			continue
		}
		u, _ := url.Parse(pg.URL)

		doc, err := scraper.ParseHTMLCharset(raw, "text/html")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [skip] %s: 파싱 실패 %v\n", pg.File, err)
			continue
		}
		serverText := scraper.BodyTextForEval(doc, u)

		clientText, err := viaNode(*nodeDir, raw, pg.URL)
		if err != nil {
			return fmt.Errorf("reader-eval: 클라이언트 경로 실행 실패 (%s): %w", pg.File, err)
		}

		results = append(results, pageResult{
			File: pg.File, Host: pg.Host,
			Server: measure(serverText), Client: measure(clientText),
		})
	}
	if len(results) == 0 {
		return fmt.Errorf("reader-eval: 잰 페이지가 0건이다")
	}

	report(results)

	if *dump != "" {
		fmt.Printf("\n--- dump: %s ---\n", *dump)
		for _, r := range results {
			if !strings.Contains(r.Host, *dump) {
				continue
			}
			fmt.Printf("[%s]\n  서버   블록 %3d · 최장 %5d · 첫줄 %q\n  클라이언트 블록 %3d · 최장 %5d · 첫줄 %q\n",
				r.Host, r.Server.Blocks, r.Server.Longest, r.Server.FirstRow,
				r.Client.Blocks, r.Client.Longest, r.Client.FirstRow)
		}
	}

	// **클라이언트 경로가 벽을 내면 회귀다.** extract.js는 경계를 만드는 것이 유일한
	// 일이므로, 거기서 벽이 나오면 그 파일이 깨진 것이다. 서버 경로의 벽은 오늘의
	// 알려진 상태이고 S3가 고칠 대상이라 여기서 실패시키지 않는다 — 대신 숫자로 남긴다.
	clientWalls := 0
	for _, r := range results {
		if r.Client.IsWall {
			clientWalls++
		}
	}
	if clientWalls > 0 {
		fmt.Fprintf(os.Stderr, "\n실패: 클라이언트 경로가 %d건에서 벽을 냈다 — extract.js의 경계 규칙이 깨졌다\n", clientWalls)
		os.Exit(1)
	}
	return nil
}

func report(rs []pageResult) {
	srvWall, cliWall, srvEmpty, cliEmpty := 0, 0, 0, 0
	var srvLongest, cliLongest []int
	for _, r := range rs {
		if r.Server.IsWall {
			srvWall++
		}
		if r.Client.IsWall {
			cliWall++
		}
		if r.Server.Empty {
			srvEmpty++
		}
		if r.Client.Empty {
			cliEmpty++
		}
		srvLongest = append(srvLongest, r.Server.Longest)
		cliLongest = append(cliLongest, r.Client.Longest)
	}
	n := len(rs)
	fmt.Printf("코퍼스 %d건 (원본 HTML, 커밋된 스냅샷)\n\n", n)
	fmt.Printf("%-12s %8s %8s %8s %8s\n", "", "벽", "빈본문", "최장p50", "최장p90")
	fmt.Printf("%-12s %6d/%d %8d %8d %8d\n", "서버",
		srvWall, n, srvEmpty, pct(srvLongest, 0.5), pct(srvLongest, 0.9))
	fmt.Printf("%-12s %6d/%d %8d %8d %8d\n", "클라이언트",
		cliWall, n, cliEmpty, pct(cliLongest, 0.5), pct(cliLongest, 0.9))
	fmt.Printf("\n벽 = 경계 없이 %d룬 이상 이어지는 구간이 있는 페이지\n", wallThreshold)

	fmt.Printf("\n%-30s %14s %14s\n", "호스트", "서버(블록/최장)", "클라(블록/최장)")
	sorted := append([]pageResult(nil), rs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Server.Longest > sorted[j].Server.Longest })
	for _, r := range sorted {
		mark := " "
		if r.Server.IsWall {
			mark = "!"
		}
		fmt.Printf("%s%-29s %6d/%7d %6d/%7d\n", mark, capRunesLocal(r.Host, 29),
			r.Server.Blocks, r.Server.Longest, r.Client.Blocks, r.Client.Longest)
	}
	fmt.Println("\n측정치는 기록용이다 — 추출을 이 숫자에 맞추는 것은 허용, 코퍼스를 추출 결과에 맞추는 것은 금지.")
}

func pct(v []int, q float64) int {
	if len(v) == 0 {
		return 0
	}
	s := append([]int(nil), v...)
	sort.Ints(s)
	i := int(float64(len(s)-1) * q)
	return s[i]
}

func readGz(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// viaNode는 **진짜 extract.js**를 실행한다. 규칙을 Go로 옮겨 적지 않는 이유는 파일 상단 주석.
func viaNode(dir string, html []byte, pageURL string) (string, error) {
	cmd := exec.Command("node", "scripts/extract_via_node.mjs", pageURL)
	cmd.Dir = dir
	cmd.Stdin = bytes.NewReader(html)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, capRunesLocal(errb.String(), 300))
	}
	return out.String(), nil
}
