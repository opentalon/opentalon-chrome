package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_defaults(t *testing.T) {
	t.Setenv("OPENTALON_CHROME_CONFIG", "")
	t.Setenv("CHROME_CDP_URL", "")
	t.Setenv("CHROME_SCREENSHOT_DIR", "")
	t.Setenv("CHROME_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.CDPURL != defaultCDPURL {
		t.Errorf("CDPURL = %q, want %q", cfg.CDPURL, defaultCDPURL)
	}
	if cfg.ScreenshotDir != os.TempDir() {
		t.Errorf("ScreenshotDir = %q, want %q", cfg.ScreenshotDir, os.TempDir())
	}
	if cfg.ParseTimeout() != defaultTimeout {
		t.Errorf("Timeout = %v, want %v", cfg.ParseTimeout(), defaultTimeout)
	}
}

func TestLoad_fromOpenTalonConfig(t *testing.T) {
	t.Setenv("OPENTALON_CHROME_CONFIG", `{
		"cdp_url": "http://chrome-sidecar:9222",
		"screenshot_dir": "/data/screenshots",
		"timeout": "60s"
	}`)
	t.Setenv("CHROME_CDP_URL", "")
	t.Setenv("CHROME_SCREENSHOT_DIR", "")
	t.Setenv("CHROME_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.CDPURL != "http://chrome-sidecar:9222" {
		t.Errorf("CDPURL = %q, want http://chrome-sidecar:9222", cfg.CDPURL)
	}
	if cfg.ScreenshotDir != "/data/screenshots" {
		t.Errorf("ScreenshotDir = %q, want /data/screenshots", cfg.ScreenshotDir)
	}
	if cfg.ParseTimeout() != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.ParseTimeout())
	}
}

func TestLoad_envOverridesJSON(t *testing.T) {
	t.Setenv("OPENTALON_CHROME_CONFIG", `{"cdp_url": "http://from-json:9222"}`)
	t.Setenv("CHROME_CDP_URL", "http://from-env:9222")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.CDPURL != "http://from-env:9222" {
		t.Errorf("CDPURL = %q, want http://from-env:9222 (env should override JSON)", cfg.CDPURL)
	}
}

func TestLoad_invalidJSON(t *testing.T) {
	t.Setenv("OPENTALON_CHROME_CONFIG", "not-valid-json")

	_, err := Load()
	if err == nil {
		t.Error("Load() expected error for invalid JSON, got nil")
	}
}

func TestParseTimeout_invalid(t *testing.T) {
	cfg := Config{Timeout: "not-a-duration"}
	if cfg.ParseTimeout() != defaultTimeout {
		t.Errorf("ParseTimeout = %v, want default %v for invalid input", cfg.ParseTimeout(), defaultTimeout)
	}
}

func TestParseTimeout_zero(t *testing.T) {
	cfg := Config{Timeout: "0s"}
	if cfg.ParseTimeout() != defaultTimeout {
		t.Errorf("ParseTimeout = %v, want default %v for zero duration", cfg.ParseTimeout(), defaultTimeout)
	}
}

func TestParseTimeout_empty(t *testing.T) {
	cfg := Config{}
	if cfg.ParseTimeout() != defaultTimeout {
		t.Errorf("ParseTimeout = %v, want default %v for empty", cfg.ParseTimeout(), defaultTimeout)
	}
}
