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
	Addr              string     // PUSHPOINT_ADDR (기본 ":8080")
	DataDir           string     // PUSHPOINT_DATA_DIR (기본 "./data") — DB·썸네일 루트
	APIKey            string     // PUSHPOINT_API_KEY (필수)
	ScrapeConcurrency int        // PUSHPOINT_SCRAPE_CONCURRENCY (기본 8)
	LogLevel          slog.Level // PUSHPOINT_LOG_LEVEL (debug|info|warn|error, 기본 info)
}

// Load는 환경 변수에서 설정을 읽는다. PUSHPOINT_API_KEY가 없으면 에러.
func Load() (Config, error) {
	cfg := Config{
		Addr:              getenv("PUSHPOINT_ADDR", ":8080"),
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
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
