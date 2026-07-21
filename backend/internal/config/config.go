// Package config는 PUSHPOINT_* 환경 변수를 표준 os.Getenv로 읽는다. (viper 금지)
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

// Config는 프로세스 전역 설정. Load 한 번으로 채워지며 이후 읽기 전용.
type Config struct {
	Addr              string     // PUSHPOINT_ADDR (기본 ":8420")
	DataDir           string     // PUSHPOINT_DATA_DIR (기본 "./data") — DB·썸네일 루트
	APIKey            string     // PUSHPOINT_API_KEY (필수)
	ScrapeConcurrency int        // PUSHPOINT_SCRAPE_CONCURRENCY (기본 8)
	LogLevel          slog.Level // PUSHPOINT_LOG_LEVEL (debug|info|warn|error, 기본 info)
	// AllowPrivateHosts는 PUSHPOINT_ALLOW_PRIVATE_HOSTS (기본 false). true면 스크랩·썸네일
	// 다운로드의 SSRF 가드(사설/루프백/링크로컬 대상 거부)를 끈다 — 로컬 fixture·개발 전용
	// (예: scripts/test_crash.sh가 127.0.0.1 fixture 서버를 스크랩). 운영 기본은 가드 활성.
	AllowPrivateHosts bool
}

// Load는 환경 변수에서 설정을 읽는다. PUSHPOINT_API_KEY가 없으면 에러.
func Load() (Config, error) {
	cfg := Config{
		Addr:              getenv("PUSHPOINT_ADDR", ":8420"),
		DataDir:           getenv("PUSHPOINT_DATA_DIR", "./data"),
		APIKey:            os.Getenv("PUSHPOINT_API_KEY"),
		ScrapeConcurrency: 8,
		LogLevel:          slog.LevelInfo,
	}
	if cfg.APIKey == "" {
		return Config{}, fmt.Errorf("config: PUSHPOINT_API_KEY 미설정 (필수)")
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
