// b1gate는 12 §4의 B1 착수 게이트를 잰다.
//
//	"요약에만 있고 title·description·note·tags에는 없는 3-gram을 하나 이상 얻는 링크 비율"
//
// 30% 미만이면 B1도 B2도 접힌다 — 이 문서에서 가장 큰 덩어리가 근거를 갖고 사라진다.
//
// **왜 이 숫자가 게이트인가.** B1은 `links_fts`에 요약을 넣는 제안인데, 요약의 문자열이
// 이미 색인된 네 컬럼에 다 들어 있으면 색인만 커지고 **찾을 수 있는 것은 하나도 안 는다.**
// 새 검색 표면의 크기를 코드 전에 수치화하는 것이 요점이다.
//
// 3-gram인 이유는 검색이 trigram FTS5라서다 — 색인의 토큰 단위와 같은 단위로 세지 않으면
// 재는 대상이 검색이 아니게 된다.
//
//	go run ./cmd/b1gate
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/coby/push-point/backend/internal/summarizer"
)

type row struct {
	URL      string `json:"url"`
	Snapshot struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		BodyText    string `json:"body_text"`
	} `json:"snapshot"`
	ExpectedTags []string `json:"expected_tags"`
}

// trigrams는 FTS5 trigram 토크나이저를 흉내 낸다: 소문자로 접고, 3룬 창을 한 룬씩 민다.
// 공백류는 남긴다 — 색인도 그렇게 하고, 여기서 지우면 실제보다 겹침이 적게 나온다.
func trigrams(s string) map[string]struct{} {
	r := []rune(strings.ToLower(s))
	out := make(map[string]struct{})
	for i := 0; i+3 <= len(r); i++ {
		g := string(r[i : i+3])
		if strings.TrimFunc(g, unicode.IsSpace) == "" {
			continue // 공백만 있는 창은 검색어가 될 수 없다
		}
		out[g] = struct{}{}
	}
	return out
}

func main() {
	root := "../nlu/golden"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	var (
		total, withSummary, gained int
		gainedGrams                []int
	)
	for _, set := range []string{"dev", "test", "wild"} {
		f, err := os.Open(filepath.Join(root, set+".jsonl"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		setTotal, setSummary, setGained := 0, 0, 0
		for sc.Scan() {
			var r row
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				continue
			}
			setTotal++
			total++

			// 런타임과 같은 함수로 요약을 만든다 — golden 스냅샷에는 summary가 없다.
			sum := summarizer.Summarize(r.Snapshot.BodyText, r.Snapshot.Description)
			if sum == "" {
				continue // 요약이 없으면 새 표면도 없다
			}
			setSummary++
			withSummary++

			// 이미 색인되는 네 컬럼. note는 golden에 없으므로 빈 문자열이다 —
			// 실제로는 사용자가 쓴 메모가 더 겹칠 수 있으니 이 수치는 **낙관적 상한**이다.
			indexed := strings.Join([]string{
				r.Snapshot.Title, r.Snapshot.Description, "",
				strings.Join(r.ExpectedTags, " "),
			}, " ")

			have := trigrams(indexed)
			n := 0
			for g := range trigrams(sum) {
				if _, ok := have[g]; !ok {
					n++
				}
			}
			if n > 0 {
				setGained++
				gained++
				gainedGrams = append(gainedGrams, n)
			}
		}
		f.Close()
		fmt.Printf("  %-5s %3d건 · 요약 있음 %3d (%4.1f%%) · 새 3-gram 얻음 %3d (%4.1f%%)\n",
			set, setTotal, setSummary, pct(setSummary, setTotal), setGained, pct(setGained, setTotal))
	}

	fmt.Printf("\n  전체 %d건\n", total)
	fmt.Printf("  요약이 나온 링크        %d (%.1f%%)\n", withSummary, pct(withSummary, total))
	fmt.Printf("  **요약 전용 3-gram 획득** %d (%.1f%%)   ← 게이트 30%%\n", gained, pct(gained, total))
	if len(gainedGrams) > 0 {
		fmt.Printf("  획득한 링크의 3-gram 수: 중앙값 %d, 최소 %d, 최대 %d\n",
			median(gainedGrams), min(gainedGrams), max(gainedGrams))
	}
	fmt.Println()
	if pct(gained, total) < 30 {
		fmt.Println("  판정: **접는다** — 30% 미만.")
		os.Exit(1)
	}
	fmt.Println("  판정: 통과 — 30% 이상.")
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) * 100 / float64(b)
}

func median(v []int) int {
	c := append([]int(nil), v...)
	for i := range c {
		for j := i + 1; j < len(c); j++ {
			if c[j] < c[i] {
				c[i], c[j] = c[j], c[i]
			}
		}
	}
	return c[len(c)/2]
}

func min(v []int) int {
	m := v[0]
	for _, x := range v {
		if x < m {
			m = x
		}
	}
	return m
}

func max(v []int) int {
	m := v[0]
	for _, x := range v {
		if x > m {
			m = x
		}
	}
	return m
}
