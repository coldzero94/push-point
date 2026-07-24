package config

import "testing"

// Load는 PUSHPOINT_API_KEY가 있어야 하므로 모든 케이스에서 세팅한다.
func withAPIKey(t *testing.T) {
	t.Helper()
	t.Setenv("PUSHPOINT_API_KEY", "test-key")
}

func TestLoad_LogFormatDefaultAuto(t *testing.T) {
	withAPIKey(t)
	// PUSHPOINT_LOG_FORMAT 미설정 → 기본 auto.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogFormat != "auto" {
		t.Errorf("LogFormat 기본값 = %q, want auto", cfg.LogFormat)
	}
}

func TestLoad_LogFormatValid(t *testing.T) {
	withAPIKey(t)
	for _, v := range []string{"json", "text", "auto"} {
		t.Setenv("PUSHPOINT_LOG_FORMAT", v)
		cfg, err := Load()
		if err != nil {
			t.Errorf("Load(LOG_FORMAT=%q): 예상치 못한 에러 %v", v, err)
			continue
		}
		if cfg.LogFormat != v {
			t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, v)
		}
	}
}

func TestLoad_LogFormatInvalid(t *testing.T) {
	withAPIKey(t)
	t.Setenv("PUSHPOINT_LOG_FORMAT", "pretty")
	if _, err := Load(); err == nil {
		t.Error("LOG_FORMAT=pretty 는 에러여야 하는데 통과함")
	}
}
