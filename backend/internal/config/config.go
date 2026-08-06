// Package config는 PUSHPOINT_* 환경 변수를 표준 os.Getenv로 읽는다. (viper 금지)
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config는 프로세스 전역 설정. Load 한 번으로 채워지며 이후 읽기 전용.
type Config struct {
	Addr              string // PUSHPOINT_ADDR (기본 ":8420")
	DataDir           string // PUSHPOINT_DATA_DIR (기본 "./data") — DB·썸네일 루트
	APIKey            string // PUSHPOINT_API_KEY (필수)
	ScrapeConcurrency int    // PUSHPOINT_SCRAPE_CONCURRENCY (기본 8)
	// 도메인당 최소 요청 간격. PUSHPOINT_SCRAPE_RATE_INTERVAL (기본 1s, 0이면 없음).
	//
	// **남의 사이트에 대한 예의이지 파이프라인의 일부가 아니다.** 벤치가 fixture 한
	// 호스트에 몰아넣으면 이 상수가 측정값을 지배한다 — 실측으로 p50 2000ms vs 27ms,
	// 즉 잰 것의 99%가 하네스가 만든 대기였다. 그래서 벤치는 0으로 둔다.
	ScrapeRateInterval time.Duration
	LogLevel           slog.Level // PUSHPOINT_LOG_LEVEL (debug|info|warn|error, 기본 info)
	// LogFormat은 PUSHPOINT_LOG_FORMAT (json|text|auto, 기본 auto). text는 사람이 읽는
	// 컬러 출력(개발), json은 구조화 출력(운영 파싱), auto는 stderr가 터미널이면 text
	// 아니면 json. `just dev`는 text를 강제한다(air가 stderr를 파이프로 감싸 auto만으론
	// json이 되기 때문) — 운영 배포는 미설정→auto→non-TTY→json으로 떨어진다.
	LogFormat string
	// AllowPrivateHosts는 PUSHPOINT_ALLOW_PRIVATE_HOSTS (기본 false). true면 스크랩·썸네일
	// 다운로드의 SSRF 가드(사설/루프백/링크로컬 대상 거부)를 끈다 — 로컬 fixture·개발 전용
	// (예: scripts/test_crash.sh가 127.0.0.1 fixture 서버를 스크랩). 운영 기본은 가드 활성.
	AllowPrivateHosts bool
}

// Load는 환경 변수에서 설정을 읽는다. PUSHPOINT_API_KEY가 없으면 에러.
func Load() (Config, error) {
	cfg := Config{
		Addr:               getenv("PUSHPOINT_ADDR", ":8420"),
		DataDir:            getenv("PUSHPOINT_DATA_DIR", "./data"),
		APIKey:             os.Getenv("PUSHPOINT_API_KEY"),
		ScrapeConcurrency:  8,
		ScrapeRateInterval: time.Second,
		LogLevel:           slog.LevelInfo,
		LogFormat:          getenv("PUSHPOINT_LOG_FORMAT", "auto"),
	}
	if cfg.APIKey == "" {
		return Config{}, fmt.Errorf("config: PUSHPOINT_API_KEY 미설정 (필수)")
	}
	switch cfg.LogFormat {
	case "json", "text", "auto":
	default:
		return Config{}, fmt.Errorf("config: PUSHPOINT_LOG_FORMAT=%q 는 json|text|auto 중 하나여야 함", cfg.LogFormat)
	}
	if v := os.Getenv("PUSHPOINT_SCRAPE_RATE_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: PUSHPOINT_SCRAPE_RATE_INTERVAL=%q 는 duration이어야 함 (예: 1s, -1s = 간격 없음)", v)
		}
		cfg.ScrapeRateInterval = d
	}
	if v := os.Getenv("PUSHPOINT_SCRAPE_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("config: PUSHPOINT_SCRAPE_CONCURRENCY=%q 는 양의 정수여야 함", v)
		}
		cfg.ScrapeConcurrency = n
	}
	if v := os.Getenv("PUSHPOINT_LOG_LEVEL"); v != "" {
		var lvl slog.Level
		if err := lvl.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("config: PUSHPOINT_LOG_LEVEL=%q 파싱 실패: %w", v, err)
		}
		cfg.LogLevel = lvl
	}
	if v := os.Getenv("PUSHPOINT_ALLOW_PRIVATE_HOSTS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("config: PUSHPOINT_ALLOW_PRIVATE_HOSTS=%q 는 불리언이어야 함 (true|false|1|0)", v)
		}
		cfg.AllowPrivateHosts = b
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
