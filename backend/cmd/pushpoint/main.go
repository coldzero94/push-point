// pushpoint — Push-Point v2 단일 바이너리 진입점.
//
//	pushpoint              # API 서버 + 워커 (한 프로세스)
//	pushpoint seed -n N    # 벤치용 한영 혼합 시드 DB 생성 (고정 난수, 잡 없음)
//	pushpoint loadgen ...  # HTTP 저장 경로 p50/p95/p99 측정 (scripts/bench_http.sh)
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
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/coby/push-point/backend/internal/api"
	"github.com/coby/push-point/backend/internal/config"
	"github.com/coby/push-point/backend/internal/queue"
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
		case "loadgen":
			if err := runLoadgen(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "loadgen 실패:", err)
				os.Exit(1)
			}
			return
		case "eval":
			fmt.Println("eval: M3에서 구현 예정 — golden set(nlu/golden/) 태깅 정확도 측정")
			return
		default:
			fmt.Fprintf(os.Stderr, "알 수 없는 서브커맨드 %q (사용: pushpoint [seed|loadgen|eval])\n", os.Args[1])
			os.Exit(2)
		}
	}
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, "pushpoint:", err)
		os.Exit(1)
	}
}

// serve는 서버 모드: config → slog → DB(+마이그레이션) → store/queue/dispatcher
// → chi 서버 → graceful shutdown. 콜드 스타트 < 1s를 위해 이 이상의 초기화는 없다.
func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	db, err := store.Open(cfg.DataDir) // 마이그레이션 자동 적용 포함
	if err != nil {
		return err
	}
	q := queue.NewSQLite(db.Writer)
	st := store.New(db, q)

	// dispatcher: Run 내부에서 running→pending 크래시 복구 후 claim 루프.
	// M1은 등록된 잡 핸들러가 없다 — pending 잡은 그대로 남고 M2에서
	// scraper/tagger/thumb 핸들러가 등록되면 소비된다.
	disp := queue.NewDispatcher(q, logger)
	dispCtx, dispCancel := context.WithCancel(context.Background())
	defer dispCancel()
	dispDone := make(chan error, 1)
	go func() { dispDone <- disp.Run(dispCtx) }()

	router := api.NewRouter(api.NewServer(st, cfg.DataDir, logger), cfg.APIKey, logger)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()
	logger.Info("pushpoint 시작", "addr", cfg.Addr, "data_dir", cfg.DataDir)

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr: // 포트 바인딩 실패 등 — 즉시 정리 후 에러 반환
		dispCancel()
		<-dispDone
		if cerr := st.Close(); cerr != nil {
			logger.Error("store close 오류", "err", cerr)
		}
		return fmt.Errorf("서버 실행 실패: %w", err)
	case err := <-dispDone:
		// 서빙 중 dispatcher 사망(결과 기록 재시도 소진 등) — 워커가 죽은 채 서빙하는
		// 상태를 없앤다. HTTP 서버를 graceful 종료한 뒤 non-zero exit(fail-stop)하고,
		// 재시작의 RecoverStale이 running 잡을 복구한다.
		logger.Error("dispatcher 비정상 종료 — fail-stop", "err", err)
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if serr := srv.Shutdown(shCtx); serr != nil {
			logger.Error("서버 셧다운 오류 — 강제 종료", "err", serr)
			if cerr := srv.Close(); cerr != nil {
				logger.Error("서버 강제 종료 오류", "err", cerr)
			}
		}
		if cerr := st.Close(); cerr != nil {
			logger.Error("store close 오류", "err", cerr)
		}
		return fmt.Errorf("dispatcher 비정상 종료: %w", err)
	case <-sigCtx.Done():
	}

	// graceful shutdown 순서: 서버 먼저(새 요청 차단 + 진행 중 요청 드레인),
	// dispatcher 다음(마지막 요청이 enqueue한 잡의 알림·기록까지 처리).
	logger.Info("셧다운 시작")
	shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shCtx); err != nil {
		// 드레인 타임아웃 — 잔여 커넥션을 강제 종료한 뒤에 dispatcher·store를 닫는다
		// (진행 중 요청이 닫힌 DB를 만나 500이 되는 것을 막는다).
		logger.Error("서버 셧다운 오류 — 강제 종료", "err", err)
		if cerr := srv.Close(); cerr != nil {
			logger.Error("서버 강제 종료 오류", "err", cerr)
		}
	}
	dispCancel()
	if err := <-dispDone; err != nil {
		logger.Error("dispatcher 종료 오류", "err", err)
	}
	if err := st.Close(); err != nil {
		logger.Error("store close 오류", "err", err)
	}
	logger.Info("셧다운 완료")
	return nil
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	nonce := time.Now().UnixNano() // 실행마다 고유 URL — 항상 신규 저장(201) 경로 측정
	lat := make([]float64, 0, *n)

	for i := 0; i < *n; i++ {
		body := fmt.Sprintf(`{"url":"https://loadgen.example.com/%d/%d"}`, nonce, i)
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
