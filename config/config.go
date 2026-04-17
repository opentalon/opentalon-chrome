// Package config loads Chrome plugin configuration.
//
// Configuration is received as a JSON string via the OpenTalon plugin protocol
// (passed during the Capabilities handshake), with individual CHROME_* env
// vars as fallbacks for standalone use.
package config

import (
	"encoding/json"
	"fmt"
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
	// CDPURL is the Chrome DevTools Protocol HTTP base URL for headless Chrome, e.g. http://localhost:9222.
	CDPURL string `json:"cdp_url"`
	// ScreenshotDir is where screenshot files are written.
	ScreenshotDir string `json:"screenshot_dir"`
	// Timeout is the per-action deadline as a Go duration string, e.g. "45s".
	Timeout string `json:"timeout"`
	// DataDir is the directory where the plugin stores its SQLite database (browser credentials).
	// Mutually exclusive with DatabaseURL; one of the two must be set.
	DataDir string `json:"data_dir"`
	// DatabaseURL is a PostgreSQL connection string, e.g.
	// "postgres://user:pass@host/db?sslmode=disable".
	// When set, DataDir is ignored and Postgres is used instead of SQLite.
	DatabaseURL string `json:"database_url"`
	// LoginCDPURL is the CDP base URL for the interactive VNC Chrome instance.
	// When set, get_cookies uses this endpoint instead of the headless Chrome.
	LoginCDPURL string `json:"login_cdp_url"`
	// LoginURL is the public URL where the user can access the VNC Chrome session.
	// Returned by the get_login_url action.
	LoginURL string `json:"login_url"`
	// LoginPassword is the VNC session password shared with the user.
	// Returned by the get_login_url action.
	LoginPassword string `json:"login_password"`
	// AllowClientActorID enables the credential-store actions (save_credentials,
	// get_credentials, list_credentials, delete_credentials) when set to true.
	//
	// WARNING: with the current plugin protocol the actor_id used for per-entity
	// isolation comes from client-supplied Args, not from a trusted host context.
	// Any caller can forge actor_id and read another entity's cookies.  Only
	// enable this flag when the deployment is single-tenant, or when you have an
	// out-of-band guarantee that callers cannot supply arbitrary actor_id values.
	// This flag will be removed once the opentalon host wires InjectContextArgs.
	AllowClientActorID bool `json:"allow_client_actor_id"`
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
	if v := os.Getenv("CHROME_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("CHROME_DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("CHROME_LOGIN_CDP_URL"); v != "" {
		cfg.LoginCDPURL = v
	}
	if v := os.Getenv("CHROME_LOGIN_URL"); v != "" {
		cfg.LoginURL = v
	}
	if v := os.Getenv("CHROME_LOGIN_PASSWORD"); v != "" {
		cfg.LoginPassword = v
	}
	if os.Getenv("CHROME_ALLOW_CLIENT_ACTOR_ID") == "true" {
		cfg.AllowClientActorID = true
	}

	// Apply defaults for any unset fields.
	if cfg.CDPURL == "" {
		cfg.CDPURL = defaultCDPURL
	}
	if cfg.ScreenshotDir == "" {
		cfg.ScreenshotDir = os.TempDir()
	}
	if cfg.DataDir == "" && cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("either data_dir (SQLite) or database_url (PostgreSQL) must be set; " +
			"refusing to default to a world-readable temp directory for credential storage")
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
