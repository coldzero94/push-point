// pushpoint — Push-Point v2 단일 바이너리 진입점.
//
//	pushpoint              # API 서버 + 워커 (한 프로세스)
//	pushpoint seed -n N    # 벤치용 한영 혼합 시드 DB 생성 (고정 난수, 잡 없음)
//	pushpoint loadgen ...  # HTTP 저장 경로 p50/p95/p99 측정 (scripts/bench_http.sh)
//	pushpoint readgen ...  # 목록·검색 읽기 경로 p50/p95/p99 측정 (scripts/bench_read.sh)
//	pushpoint eval         # M3에서 구현
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	neturl "net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"

	"github.com/coby/push-point/backend/internal/app"
	"github.com/coby/push-point/backend/internal/config"
	"github.com/coby/push-point/backend/internal/store"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "seed":
			if err := runSeed(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "seed 실패:", err)
				os.Exit(1)
			}
			return
		case "pipegen":
			if err := runPipegen(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "pipegen 실패:", err)
				os.Exit(1)
			}
			return
		case "readgen":
			if err := runReadgen(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "readgen 실패:", err)
				os.Exit(1)
			}
			return
		case "loadgen":
			if err := runLoadgen(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "loadgen 실패:", err)
				os.Exit(1)
			}
			return
		case "import":
			if err := runImport(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "import 실패:", err)
				os.Exit(1)
			}
			return
		case "eval":
			if err := runEval(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "eval 실패:", err)
				os.Exit(1)
			}
			return
		case "summary-eval":
			if err := runSummaryEval(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "summary-eval 실패:", err)
				os.Exit(1)
			}
			return
		case "golden-capture":
			if err := runGoldenCapture(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "golden-capture 실패:", err)
				os.Exit(1)
			}
			return
		case "sheets-setup":
			if err := runSheetsSetup(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "sheets-setup 실패:", err)
				os.Exit(1)
			}
			return
		case "sheets-sync":
			if err := runSheetsSync(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "sheets-sync 실패:", err)
				os.Exit(1)
			}
			return
		case "golden-refill":
			if err := runGoldenRefill(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "golden-refill 실패:", err)
				os.Exit(1)
			}
			return
		case "eval-search":
			if err := runSearchEval(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "eval-search 실패:", err)
				os.Exit(1)
			}
			return
		case "golden-from-db":
			if err := runGoldenFromDB(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "golden-from-db 실패:", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "알 수 없는 서브커맨드 %q (사용: pushpoint [seed|loadgen|readgen|pipegen|import|eval|summary-eval|golden-capture|golden-refill|golden-from-db|eval-search|sheets-setup|sheets-sync])\n", os.Args[1])
			os.Exit(2)
		}
	}
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, "pushpoint:", err)
		os.Exit(1)
	}
}

// newLogger는 cfg.LogFormat에 따라 stderr 핸들러를 고른다. text는 tint(사람이 읽는
// 컬러), json은 구조화(운영 파싱), auto는 stderr가 터미널이면 text 아니면 json.
// 색은 isatty가 아니라 format에 묶는다 — `just dev`는 air가 stderr를 파이프로 감싸
// isatty=false여도 text를 강제하고, air는 tint의 ANSI를 터미널로 그대로 흘려보낸다.
func newLogger(cfg config.Config) *slog.Logger {
	colorText := cfg.LogFormat == "text" ||
		(cfg.LogFormat == "auto" && isatty.IsTerminal(os.Stderr.Fd()))
	if colorText {
		return slog.New(tint.NewTextHandler(os.Stderr, &tint.Options{
			Level:      cfg.LogLevel,
			TimeFormat: "15:04:05.000",
		}))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
}

// maskKey는 API 키를 로그에 안전하게 노출한다 — 전체 값 대신 상태와 끝 4자리만.
func maskKey(k string) string {
	if k == "" {
		return "unset"
	}
	if len(k) <= 4 {
		return "set(****)"
	}
	return "set(…" + k[len(k)-4:] + ")"
}

// serve는 서버 모드: config → slog → DB(+마이그레이션) → store/queue/dispatcher
// → chi 서버 → graceful shutdown. 콜드 스타트 < 1s를 위해 이 이상의 초기화는 없다.
func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// 배선은 internal/app에 있다 — iOS 본체 앱이 폰 안에서 **같은 서버를 인프로세스로**
	// 띄우기 때문에(mobile/ppcore), 여기서 복제하면 두 실행 모드가 갈라진다.
	a, err := app.Start(app.Config{
		ScrapeRateInterval: cfg.ScrapeRateInterval,
		DataDir:            cfg.DataDir,
		APIKey:             cfg.APIKey,
		Addr:               cfg.Addr,
		ScrapeConcurrency:  cfg.ScrapeConcurrency,
		AllowPrivateHosts:  cfg.AllowPrivateHosts,
	}, logger)
	if err != nil {
		return err
	}

	logger.Info("pushpoint 시작",
		"addr", a.Addr(), // 실제 바인딩된 주소 (cfg.Addr의 포트가 0이면 OS가 고른 값)
		"data_dir", cfg.DataDir,
		"scrape_concurrency", cfg.ScrapeConcurrency,
		"log_level", cfg.LogLevel.String(),
		"log_format", cfg.LogFormat,
		"allow_private_hosts", cfg.AllowPrivateHosts,
		"api_key", maskKey(cfg.APIKey), // 값 전체는 로그에 남기지 않는다 — set(…끝4자리)만
	)

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wait는 서버/dispatcher 중 하나가 비정상 종료할 때만 돌아온다(fail-stop).
	waitErr := make(chan error, 1)
	go func() { waitErr <- a.Wait() }()

	select {
	case err := <-waitErr:
		return err
	case <-sigCtx.Done():
	}

	logger.Info("셧다운 시작")
	err = a.Shutdown()
	logger.Info("셧다운 완료")
	return err
}

// ---- seed: 벤치용 시드 DB 생성 ----

// 한영 혼합 단어 풀 — trigram FTS·LIKE 폴백·태그 매칭 벤치가 모두 의미 있도록 구성.
var (
	seedKo = []string{
		"쿠버네티스", "고루틴", "동시성", "검색", "백엔드", "성능", "튜토리얼", "머신러닝",
		"임베딩", "파이프라인", "최적화", "데이터베이스", "프론트엔드", "배포", "아키텍처",
		"형태소", "분석", "자동", "태깅", "링크", "저장", "워커", "트랜잭션", "색인", "정리",
	}
	seedEn = []string{
		"golang", "kubernetes", "sqlite", "performance", "concurrency", "tutorial",
		"embedding", "search", "backend", "design", "react", "swift", "fts5", "chi",
		"slog", "migrate", "worker", "queue", "index", "benchmark",
	}
	seedDomains = []struct{ host, ctype string }{
		{"youtube.com", "video"},
		{"blog.naver.com", "article"},
		{"velog.io", "article"},
		{"medium.com", "article"},
		{"github.com", "other"},
		{"x.com", "post"},
		{"news.ycombinator.com", "article"},
		{"example.com", "other"},
	}
)

func seedWord(rng *rand.Rand) string {
	if rng.Intn(2) == 0 {
		return seedKo[rng.Intn(len(seedKo))]
	}
	return seedEn[rng.Intn(len(seedEn))]
}

func seedPhrase(rng *rand.Rand, minWords, maxWords int) string {
	n := minWords + rng.Intn(maxWords-minWords+1)
	words := make([]string, n)
	for i := range words {
		words[i] = seedWord(rng)
	}
	return strings.Join(words, " ")
}

// runSeed는 고정 시드 rand로 링크 N건을 DB에 직접 넣는다 (Store 인터페이스는
// title/tags를 못 채우므로 SQL 직행). links_fts 동기화 포함, 잡 enqueue 없음 —
// 벤치가 스크랩 파이프라인 없이 목록/검색 경로만 재도록.
func runSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	n := fs.Int("n", 10000, "생성할 링크 수")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dataDir := os.Getenv("PUSHPOINT_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	db, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	defer db.Close()

	// 태그 사전 로드 (마이그레이션 시드 30개)
	rows, err := db.Reader.Query(`SELECT id, name FROM tags ORDER BY id`)
	if err != nil {
		return fmt.Errorf("태그 사전 조회 실패: %w", err)
	}
	var tagIDs []int64
	var tagNames []string
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return err
		}
		tagIDs = append(tagIDs, id)
		tagNames = append(tagNames, name)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rng := rand.New(rand.NewSource(20260720)) // 고정 시드 — 재현 가능한 벤치 DB
	now := time.Now().Unix()
	const batch = 1000
	start := time.Now()

	for lo := 0; lo < *n; lo += batch {
		hi := lo + batch
		if hi > *n {
			hi = *n
		}
		tx, err := db.Writer.Begin()
		if err != nil {
			return err
		}
		for i := lo; i < hi; i++ {
			d := seedDomains[rng.Intn(len(seedDomains))]
			url := fmt.Sprintf("https://%s/seed/%d", d.host, i+1)
			sum := sha256.Sum256([]byte(url))
			title := seedPhrase(rng, 3, 7)
			desc := seedPhrase(rng, 10, 28)
			note := ""
			if rng.Intn(10) < 3 {
				note = seedPhrase(rng, 2, 6)
			}
			createdAt := now - rng.Int63n(2*365*86400) // 최근 2년에 분산

			res, err := tx.Exec(`
				INSERT INTO links (url, url_hash, domain, title, description, content_type,
				                   note, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 'done', ?, ?)`,
				url, hex.EncodeToString(sum[:]), d.host, title, desc, d.ctype,
				note, createdAt, createdAt)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("링크 %d 삽입 실패 (이미 시드된 DB?): %w", i+1, err)
			}
			linkID, err := res.LastInsertId()
			if err != nil {
				_ = tx.Rollback()
				return err
			}

			// 태그 분포: 0개 15% / 1개 30% / 2개 30% / 3개 20% / 4개 5%
			var k int
			switch p := rng.Intn(100); {
			case p < 15:
				k = 0
			case p < 45:
				k = 1
			case p < 75:
				k = 2
			case p < 95:
				k = 3
			default:
				k = 4
			}
			var names []string
			for _, idx := range rng.Perm(len(tagIDs))[:min(k, len(tagIDs))] {
				conf := 0.5 + rng.Float64()*0.5
				if _, err := tx.Exec(`
					INSERT INTO link_tags (link_id, tag_id, source, confidence)
					VALUES (?, ?, 'rules', ?)`, linkID, tagIDs[idx], conf); err != nil {
					_ = tx.Rollback()
					return err
				}
				names = append(names, tagNames[idx])
			}

			// FTS 동기화 — 링크/태그 쓰기와 같은 트랜잭션 (store 계층과 동일 규약)
			if _, err := tx.Exec(`
				INSERT INTO links_fts (rowid, title, description, note, tags)
				VALUES (?, ?, ?, ?, ?)`,
				linkID, title, desc, note, strings.Join(names, " ")); err != nil {
				_ = tx.Rollback()
				return err
			}

			if (i+1)%10000 == 0 {
				logger.Info("seed 진행", "count", i+1, "total", *n)
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	logger.Info("seed 완료", "count", *n, "elapsed", time.Since(start).String())
	return nil
}

// ---- loadgen: HTTP 저장 경로 지연 측정 ----

// runLoadgen은 POST /api/v1/links를 순차 N회 보내 p50/p95/p99(ms)를 stdout JSON으로
// 출력한다. p99 > 50ms면 exit 1 (M1 성능 게이트 — scripts/bench_http.sh가 판정에 사용).
func runLoadgen(args []string) error {
	fs := flag.NewFlagSet("loadgen", flag.ExitOnError)
	addr := fs.String("addr", "http://localhost:8080", "서버 주소")
	key := fs.String("key", "dev-key", "API 키")
	n := fs.Int("n", 1000, "요청 수")
	// 클라이언트 캡처 경로를 실제로 태우기 위한 페이로드 크기. 기본 0이면 예전과 같은
	// ~50B 요청이라 새 경로가 게이트를 한 번도 통과하지 않은 채 그린이 유지된다 —
	// 그러면 그린이 증거가 아니게 되므로 bench_http.sh가 캡 최대치로도 한 번 더 돈다.
	bodyBytes := fs.Int("body-bytes", 0, "클라이언트 캡처 body_text 바이트 수 (0이면 미전송)")
	metaBytes := fs.Int("meta-bytes", 0, "title+description 합계 바이트 수 (0이면 미전송)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// 한글을 섞는다 — 절단·정제가 룬 경계를 다루므로 ASCII만으로는 실제 비용을 못 잰다.
	filler := func(nBytes int) string {
		if nBytes <= 0 {
			return ""
		}
		unit := "본문 텍스트 sample body text "
		var b strings.Builder
		for b.Len() < nBytes {
			b.WriteString(unit)
		}
		return b.String()[:nBytes]
	}
	capture := ""
	if *bodyBytes > 0 || *metaBytes > 0 {
		half := *metaBytes / 2
		blob, err := json.Marshal(map[string]string{
			"title": filler(half), "description": filler(*metaBytes - half), "body_text": filler(*bodyBytes),
		})
		if err != nil {
			return err
		}
		capture = "," + string(blob[1:len(blob)-1]) // 바깥 중괄호를 벗겨 url 뒤에 이어 붙인다
	}
	client := &http.Client{Timeout: 10 * time.Second}
	nonce := time.Now().UnixNano() // 실행마다 고유 URL — 항상 신규 저장(201) 경로 측정
	lat := make([]float64, 0, *n)

	for i := 0; i < *n; i++ {
		body := fmt.Sprintf(`{"url":"https://loadgen.example.com/%d/%d"%s}`, nonce, i, capture)
		req, err := http.NewRequest(http.MethodPost, *addr+"/api/v1/links", strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+*key)
		req.Header.Set("Content-Type", "application/json")

		t0 := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("요청 %d 실패: %w", i+1, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		elapsed := float64(time.Since(t0).Microseconds()) / 1000.0

		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("요청 %d: 예상 밖 상태 코드 %d", i+1, resp.StatusCode)
		}
		lat = append(lat, elapsed)
	}

	sort.Float64s(lat)
	out := struct {
		N   int     `json:"n"`
		P50 float64 `json:"p50_ms"`
		P95 float64 `json:"p95_ms"`
		P99 float64 `json:"p99_ms"`
	}{
		N:   *n,
		P50: percentile(lat, 0.50),
		P95: percentile(lat, 0.95),
		P99: percentile(lat, 0.99),
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		return err
	}
	if out.P99 > 50 {
		fmt.Fprintf(os.Stderr, "성능 게이트 실패: p99 %.2fms > 50ms\n", out.P99)
		os.Exit(1)
	}
	return nil
}

// ---- pipegen: 저장 → 태깅 완료까지의 파이프라인 지연 측정 ----

// runPipegen은 저장 후 링크가 **종단 상태에 닿기까지**를 잰다(08 §4의 3s 예산).
//
// **요청 하나를 재는 loadgen과 다른 모양이다.** 저장 API는 201을 즉시 돌려주고
// (그게 p99 50ms 게이트의 내용이다) 수집·태깅은 잡 큐에서 비동기로 돈다. 그래서 여기서
// 재는 것은 응답 시간이 아니라 **사용자가 태그를 보게 되기까지**이고, 폴링 말고는 볼
// 방법이 없다.
//
// 종단은 `done` 또는 `failed`다. failed도 종단으로 세는 이유: 예산을 넘겼는지를 재는 것이
// 목적이고, 실패로 끝난 것도 그 시간은 걸렸다. 다만 실패 건수를 따로 보고해 **전부
// 실패해서 빨랐던 경우**를 초록으로 읽지 않게 한다.
func runPipegen(args []string) error {
	fs := flag.NewFlagSet("pipegen", flag.ExitOnError)
	addr := fs.String("addr", "http://localhost:8080", "서버 주소")
	key := fs.String("key", "dev-key", "API 키")
	target := fs.String("target", "", "저장할 URL의 접두사 (fixture 서버)")
	n := fs.Int("n", 20, "저장 건수")
	gate := fs.Float64("gate-ms", 3000, "p99 게이트(ms)")
	timeout := fs.Duration("timeout", 30*time.Second, "한 건이 종단에 닿기를 기다리는 한도")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return fmt.Errorf("-target 이 필요하다 (예: http://127.0.0.1:19090/page)")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	nonce := time.Now().UnixNano()
	lat := make([]float64, 0, *n)
	failed, tagged := 0, 0

	for i := 0; i < *n; i++ {
		// **태깅할 거리를 함께 싣는다.** fixture HTML에는 사전이 아는 낱말이 없어서
		// 수집만으로는 태그가 0건이 되고, 그러면 "저장 → 태깅 완료"라는 이름의 지표가
		// 태깅을 한 번도 안 재게 된다. 공유 확장이 본문을 실어 보내는 실제 경로와도 같다.
		body := fmt.Sprintf(
			`{"url":"%s/%d/%d","title":"쿠버네티스 프로덕션 운영","body_text":"%s"}`,
			*target, nonce, i, pipeTaggableBody)
		req, err := http.NewRequest(http.MethodPost, *addr+"/api/v1/links", strings.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+*key)
		req.Header.Set("Content-Type", "application/json")

		t0 := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("저장 %d 실패: %w", i+1, err)
		}
		var created struct {
			ID int64 `json:"id"`
		}
		dec := json.NewDecoder(resp.Body)
		decErr := dec.Decode(&created)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated || decErr != nil || created.ID == 0 {
			return fmt.Errorf("저장 %d: 상태 %d, id=%d (%v)", i+1, resp.StatusCode, created.ID, decErr)
		}

		// 폴링 간격은 25ms — 3s 예산에서 1% 미만의 관측 오차다. 더 촘촘히 하면 재는 행위가
		// 서버를 붙잡아 측정 대상을 바꾼다.
		deadline := time.Now().Add(*timeout)
		var status string
		var nTags int
		for time.Now().Before(deadline) {
			time.Sleep(25 * time.Millisecond)
			st, tg, err := pipeStatus(client, *addr, *key, created.ID)
			if err != nil {
				return fmt.Errorf("링크 %d 조회 실패: %w", created.ID, err)
			}
			status, nTags = st, tg
			if st == "done" || st == "failed" {
				break
			}
		}
		if status != "done" && status != "failed" {
			return fmt.Errorf("링크 %d가 %s 안에 종단에 닿지 않았다 (마지막 상태 %q)", created.ID, *timeout, status)
		}
		if status == "failed" {
			failed++
		}
		if nTags > 0 {
			tagged++
		}
		lat = append(lat, float64(time.Since(t0).Microseconds())/1000.0)
	}

	sort.Float64s(lat)
	out := struct {
		N      int     `json:"n"`
		Failed int     `json:"failed"`
		Tagged int     `json:"tagged"`
		P50    float64 `json:"p50_ms"`
		P95    float64 `json:"p95_ms"`
		P99    float64 `json:"p99_ms"`
	}{
		N: *n, Failed: failed, Tagged: tagged,
		P50: percentile(lat, 0.50), P95: percentile(lat, 0.95), P99: percentile(lat, 0.99),
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		return err
	}
	// **태그가 하나도 안 붙었으면 실패다.** 이 지표의 이름이 "저장 → 태깅 완료"인데
	// 태깅이 아무 일도 안 하고 끝나면 빠른 것이 아니라 안 한 것이다.
	if tagged == 0 {
		fmt.Fprintln(os.Stderr, "파이프라인 게이트 실패: 태그가 붙은 링크가 0건이다")
		os.Exit(1)
	}
	if out.P99 > *gate {
		fmt.Fprintf(os.Stderr, "파이프라인 게이트 실패: p99 %.0fms > %.0fms\n", out.P99, *gate)
		os.Exit(1)
	}
	return nil
}

// pipeTaggableBody는 사전이 아는 낱말을 여럿 담은 본문이다. 태거가 실제로 일하게 만드는
// 것이 목적이라 문장은 자연스러울 필요가 없지만, 여러 facet에 걸치게 해서 한 낱말이
// 사전에서 빠져도 측정이 통째로 0이 되지 않게 한다.
const pipeTaggableBody = "쿠버네티스 클러스터를 프로덕션에서 운영하는 일은 설치와 다르다. " +
	"도커 이미지와 데이터베이스 백업, golang 으로 쓴 백엔드 서비스의 성능 모니터링까지 함께 본다."

// pipeStatus는 링크의 현재 상태와 태그 수를 읽는다.
func pipeStatus(c *http.Client, addr, key string, id int64) (string, int, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/links/%d", addr, id), nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := c.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("상태 %d", resp.StatusCode)
	}
	var d struct {
		Status string `json:"status"`
		Tags   []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return "", 0, err
	}
	return d.Status, len(d.Tags), nil
}

// ---- readgen: 읽기 경로(목록·검색) 지연 측정 ----

// runReadgen은 목록 스크롤과 검색의 p50/p95/p99(ms)를 잰다.
//
// **이 명령이 없어서 5지표 중 셋을 한 번도 안 쟀다**(08 §4). 저장 p99와 콜드 스타트만
// 게이트가 있었고, 목록 100k·검색 10k는 목표만 문서에 적혀 있었다. 목표는 재지 않으면
// 목표가 아니라 희망이다 — 12 §4.6도 "재는 명령이 리포에 없다"고 적어 두고 있었다.
//
// 저장과 달리 **커서를 태운다.** 1장만 반복해 받으면 keyset 커서가 깊은 페이지에서
// 어떻게 되는지 못 본다 — 목록 게이트가 존재하는 이유가 정확히 거기다(OFFSET 금지).
func runReadgen(args []string) error {
	fs := flag.NewFlagSet("readgen", flag.ExitOnError)
	addr := fs.String("addr", "http://localhost:8080", "서버 주소")
	key := fs.String("key", "dev-key", "API 키")
	mode := fs.String("mode", "list", "list | search")
	n := fs.Int("n", 500, "요청 수")
	limit := fs.Int("limit", 50, "목록 한 장 크기")
	gate := fs.Float64("gate-ms", 0, "p99 게이트(ms). 0이면 모드 기본값")
	queries := fs.String("queries", "", "검색어 쉼표 구분 (search 모드)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *gate == 0 {
		// 08 §4의 목표 그대로. 검색이 더 빡빡한 것은 사람이 기다리는 자리라서다.
		*gate = map[string]float64{"list": 50, "search": 30}[*mode]
		if *gate == 0 {
			return fmt.Errorf("알 수 없는 모드 %q (list|search)", *mode)
		}
	}
	terms := strings.Split(*queries, ",")
	if *mode == "search" && *queries == "" {
		// 한영을 섞는다 — FTS5 trigram은 3룬 하한이 있어 한국어 2음절 낱말이 다른 경로를
		// 타고, 한쪽만 재면 느린 쪽을 통째로 놓친다.
		terms = []string{"쿠버네티스", "데이터베이스", "kubernetes", "database", "성능", "golang"}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	lat := make([]float64, 0, *n)
	cursor := ""
	pages := 0

	for i := 0; i < *n; i++ {
		var url string
		switch *mode {
		case "list":
			url = fmt.Sprintf("%s/api/v1/links?limit=%d", *addr, *limit)
			if cursor != "" {
				url += "&cursor=" + cursor
			}
		case "search":
			url = fmt.Sprintf("%s/api/v1/search?q=%s&limit=%d", *addr,
				neturl.QueryEscape(terms[i%len(terms)]), *limit)
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+*key)

		t0 := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("요청 %d 실패: %w", i+1, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		elapsed := float64(time.Since(t0).Microseconds()) / 1000.0
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("요청 %d: 예상 밖 상태 코드 %d", i+1, resp.StatusCode)
		}
		lat = append(lat, elapsed)

		if *mode == "list" {
			// **커서를 이어 간다.** 끝에 닿으면 처음부터 다시 — 그래야 얕은 페이지와 깊은
			// 페이지가 표본에 함께 들어간다.
			var page struct {
				NextCursor *string `json:"next_cursor"`
			}
			if err := json.Unmarshal(raw, &page); err != nil {
				return fmt.Errorf("요청 %d: 목록 응답 파싱 실패: %w", i+1, err)
			}
			if page.NextCursor == nil || *page.NextCursor == "" {
				cursor = ""
			} else {
				cursor = *page.NextCursor
				pages++
			}
		}
	}

	sort.Float64s(lat)
	out := struct {
		Mode  string  `json:"mode"`
		N     int     `json:"n"`
		Pages int     `json:"pages_walked"`
		P50   float64 `json:"p50_ms"`
		P95   float64 `json:"p95_ms"`
		P99   float64 `json:"p99_ms"`
	}{
		Mode: *mode, N: *n, Pages: pages,
		P50: percentile(lat, 0.50), P95: percentile(lat, 0.95), P99: percentile(lat, 0.99),
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		return err
	}
	if out.P99 > *gate {
		fmt.Fprintf(os.Stderr, "성능 게이트 실패: %s p99 %.2fms > %.0fms\n", *mode, out.P99, *gate)
		os.Exit(1)
	}
	return nil
}

// percentile은 정렬된 표본에서 nearest-rank 백분위수를 구한다.
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(q*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
