// Package config loads Chrome plugin configuration.
//
// Configuration is read from the OPENTALON_CHROME_CONFIG environment variable
// (a JSON object injected by the OpenTalon host from the plugin's config: block
// in config.yaml), with individual env vars as fallbacks for standalone use.
package config

import (
	"encoding/json"
	"os"
	"time"
)

const (
	defaultCDPURL  = "http://localhost:9222"
	defaultTimeout = 30 * time.Second
)

// Config holds runtime configuration for the Chrome plugin.
type Config struct {
	// CDPURL is the Chrome DevTools Protocol HTTP base URL, e.g. http://localhost:9222.
	CDPURL string `json:"cdp_url"`
	// ScreenshotDir is where screenshot files are written.
	ScreenshotDir string `json:"screenshot_dir"`
	// Timeout is the per-action deadline as a Go duration string, e.g. "45s".
	Timeout string `json:"timeout"`
}

// Load reads configuration, preferring OPENTALON_CHROME_CONFIG JSON and
// falling back to individual CHROME_* environment variables.
//
// OpenTalon injects OPENTALON_CHROME_CONFIG from the plugin's config: block:
//
//	plugins:
//	  chrome:
//	    config:
//	      cdp_url: "http://chrome-sidecar:9222"
//	      screenshot_dir: "/data/screenshots"
//	      timeout: "45s"
func Load() (Config, error) {
	cfg := Config{}

	if raw := os.Getenv("OPENTALON_CHROME_CONFIG"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return Config{}, err
		}
	}

	// Individual env vars override the JSON config (useful for ad-hoc overrides).
	if v := os.Getenv("CHROME_CDP_URL"); v != "" {
		cfg.CDPURL = v
	}
	if v := os.Getenv("CHROME_SCREENSHOT_DIR"); v != "" {
		cfg.ScreenshotDir = v
	}
	if v := os.Getenv("CHROME_TIMEOUT"); v != "" {
		cfg.Timeout = v
	}

	// Apply defaults for any unset fields.
	if cfg.CDPURL == "" {
		cfg.CDPURL = defaultCDPURL
	}
	if cfg.ScreenshotDir == "" {
		cfg.ScreenshotDir = os.TempDir()
	}

	return cfg, nil
}

// ParseTimeout returns the configured timeout as a duration,
// falling back to the default if the value is empty or invalid.
func (c Config) ParseTimeout() time.Duration {
	if c.Timeout != "" {
		if d, err := time.ParseDuration(c.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return defaultTimeout
}
