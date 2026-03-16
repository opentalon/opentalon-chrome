// Package config loads Chrome plugin configuration.
//
// Configuration is received as a JSON string via the OpenTalon plugin protocol
// (passed during the Capabilities handshake), with individual CHROME_* env
// vars as fallbacks for standalone use.
package config

import (
	"encoding/json"
	"os"
	"time"
)

const (
	defaultCDPURL = "http://localhost:9222"
	// DefaultTimeout is the per-action deadline used when no timeout is configured.
	DefaultTimeout = 30 * time.Second
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

// Load parses configuration from configJSON (the JSON-encoded config: block
// delivered by the OpenTalon host during the Capabilities handshake), then
// applies any CHROME_* environment variable overrides for standalone use.
//
// configJSON corresponds to the plugin's config: block in config.yaml:
//
//	plugins:
//	  chrome:
//	    config:
//	      cdp_url: "http://chrome-sidecar:9222"
//	      screenshot_dir: "/data/screenshots"
//	      timeout: "45s"
func Load(configJSON string) (Config, error) {
	cfg := Config{}

	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
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
	return DefaultTimeout
}
