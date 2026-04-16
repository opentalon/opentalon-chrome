package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Client implements Browser by communicating with a remote Chrome instance
// over the Chrome DevTools Protocol.
type Client struct {
	cdpURL  string
	timeout time.Duration
}

// NewClient returns a Client targeting the given CDP base URL (e.g. http://localhost:9222).
func NewClient(cdpURL string, timeout time.Duration) *Client {
	return &Client{cdpURL: cdpURL, timeout: timeout}
}

// versionResponse is the relevant subset of Chrome's /json/version response.
type versionResponse struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// wsURL fetches the browser-level WebSocket debugger URL from /json/version and
// rewrites the host to match cdpURL so Docker-network addresses resolve correctly.
func (c *Client) wsURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cdpURL+"/json/version", nil)
	if err != nil {
		return "", fmt.Errorf("build /json/version request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("reach Chrome at %s: %w", c.cdpURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read /json/version response: %w", err)
	}

	var v versionResponse
	if err := json.Unmarshal(body, &v); err != nil {
		return "", fmt.Errorf("parse /json/version response: %w", err)
	}
	if v.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("chrome returned empty webSocketDebuggerUrl")
	}

	// Chrome may advertise "localhost" even when accessed via a Docker hostname.
	// Replace the host portion with the one from cdpURL so the connection works.
	ws, err := rewriteHost(v.WebSocketDebuggerURL, c.cdpURL)
	if err != nil {
		return "", fmt.Errorf("rewrite ws host: %w", err)
	}
	return ws, nil
}

// rewriteHost replaces the host:port of wsRaw with the host:port from cdpRaw.
func rewriteHost(wsRaw, cdpRaw string) (string, error) {
	cdpU, err := url.Parse(cdpRaw)
	if err != nil {
		return "", fmt.Errorf("parse cdp URL: %w", err)
	}
	wsU, err := url.Parse(wsRaw)
	if err != nil {
		return "", fmt.Errorf("parse ws URL: %w", err)
	}
	wsU.Host = cdpU.Host
	return wsU.String(), nil
}

// newTabCtx creates a short-lived remote-allocator + tab context pair.
// The returned cancel must always be called.
func (c *Client) newTabCtx(ctx context.Context) (context.Context, context.CancelFunc, error) {
	wsURL, err := c.wsURL(ctx)
	if err != nil {
		return nil, nil, err
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, wsURL)
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)

	cancel := func() {
		tabCancel()
		allocCancel()
	}
	return tabCtx, cancel, nil
}

// withTimeout wraps ctx with the client timeout, falling back on ctx if it
// already carries a shorter deadline.
func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

// Navigate implements Browser.
func (c *Client) Navigate(ctx context.Context, rawURL string) (string, error) {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()

	var title string
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.Title(&title),
	); err != nil {
		return "", fmt.Errorf("navigate %s: %w", rawURL, err)
	}
	return title, nil
}

// GetText implements Browser.
func (c *Client) GetText(ctx context.Context, rawURL, selector string) (string, error) {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()

	if selector == "" {
		selector = "body"
	}

	var text string
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.Text(selector, &text, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("get text from %s (selector %q): %w", rawURL, selector, err)
	}
	return text, nil
}

// GetHTML implements Browser.
func (c *Client) GetHTML(ctx context.Context, rawURL, selector string) (string, error) {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()

	if selector == "" {
		selector = ":root"
	}

	var html string
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.OuterHTML(selector, &html, chromedp.ByQuery),
	); err != nil {
		return "", fmt.Errorf("get HTML from %s (selector %q): %w", rawURL, selector, err)
	}
	return html, nil
}

// Screenshot implements Browser.
func (c *Client) Screenshot(ctx context.Context, rawURL, selector, outputDir string) (string, []byte, error) {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return "", nil, err
	}
	defer cancel()

	var buf []byte
	actions := []chromedp.Action{chromedp.Navigate(rawURL)}
	if selector != "" {
		actions = append(actions, chromedp.Screenshot(selector, &buf, chromedp.ByQuery))
	} else {
		actions = append(actions, chromedp.CaptureScreenshot(&buf))
	}

	if err := chromedp.Run(tabCtx, actions...); err != nil {
		return "", nil, fmt.Errorf("screenshot %s: %w", rawURL, err)
	}

	// Derive a safe filename from the URL.
	fname := screenshotFilename(rawURL)
	path := filepath.Join(outputDir, fname)
	if err := os.WriteFile(path, buf, 0644); err != nil {
		return "", nil, fmt.Errorf("write screenshot to %s: %w", path, err)
	}
	return path, buf, nil
}

// Click implements Browser.
func (c *Client) Click(ctx context.Context, rawURL, selector string) error {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.Click(selector, chromedp.ByQuery),
	); err != nil {
		return fmt.Errorf("click %q on %s: %w", selector, rawURL, err)
	}
	return nil
}

// TypeText implements Browser.
func (c *Client) TypeText(ctx context.Context, rawURL, selector, text string) error {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return err
	}
	defer cancel()

	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	); err != nil {
		return fmt.Errorf("type text into %q on %s: %w", selector, rawURL, err)
	}
	return nil
}

// Evaluate implements Browser.
func (c *Client) Evaluate(ctx context.Context, rawURL, script string) (string, error) {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()

	var result interface{}
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.Evaluate(script, &result),
	); err != nil {
		return "", fmt.Errorf("evaluate script on %s: %w", rawURL, err)
	}

	out, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal evaluate result: %w", err)
	}
	return string(out), nil
}

// GetCookies implements Browser.
// It navigates to url in a fresh tab, collects all cookies via CDP, optionally
// filters them by domain suffix, and returns a JSON array of cookie objects.
func (c *Client) GetCookies(ctx context.Context, rawURL, domain string) (string, error) {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()

	var cookies []*network.Cookie
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var e error
			cookies, e = network.GetCookies().Do(ctx)
			return e
		}),
	); err != nil {
		return "", fmt.Errorf("get cookies from %s: %w", rawURL, err)
	}

	// Filter by domain suffix when requested.
	if domain != "" {
		filtered := cookies[:0]
		for _, c := range cookies {
			if cookieDomainMatches(c.Domain, domain) {
				filtered = append(filtered, c)
			}
		}
		cookies = filtered
	}

	out, err := json.Marshal(cookies)
	if err != nil {
		return "", fmt.Errorf("marshal cookies: %w", err)
	}
	return string(out), nil
}

// NavigateWithCookies implements Browser.
// It deserialises cookiesJSON (a JSON array of network.Cookie objects), injects
// them via SetCookies before navigation, then navigates to url and returns the
// page title. This lets headless Chrome replay a login session captured via
// GetCookies on the interactive VNC Chrome instance.
func (c *Client) NavigateWithCookies(ctx context.Context, rawURL, cookiesJSON string) (string, error) {
	ctx, tCancel := c.withTimeout(ctx)
	defer tCancel()

	tabCtx, cancel, err := c.newTabCtx(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()

	// Deserialise stored cookies into CDP CookieParam objects.
	var stored []*network.Cookie
	if err := json.Unmarshal([]byte(cookiesJSON), &stored); err != nil {
		return "", fmt.Errorf("parse cookies JSON: %w", err)
	}
	params := make([]*network.CookieParam, len(stored))
	for i, c := range stored {
		p := &network.CookieParam{
			Name:         c.Name,
			Value:        c.Value,
			Domain:       c.Domain,
			Path:         c.Path,
			Secure:       c.Secure,
			HTTPOnly:     c.HTTPOnly,
			SameSite:     c.SameSite,
			Priority:     c.Priority,
			SourceScheme: c.SourceScheme,
			SourcePort:   c.SourcePort,
			PartitionKey: c.PartitionKey,
		}
		// Preserve expiry for persistent cookies; session cookies have Expires == -1.
		if c.Expires >= 0 {
			t := cdp.TimeSinceEpoch(time.Unix(int64(c.Expires), 0))
			p.Expires = &t
		}
		params[i] = p
	}

	var title string
	if err := chromedp.Run(tabCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookies(params).Do(ctx)
		}),
		chromedp.Navigate(rawURL),
		chromedp.Title(&title),
	); err != nil {
		return "", fmt.Errorf("navigate with cookies to %s: %w", rawURL, err)
	}
	return title, nil
}

// cookieDomainMatches reports whether the cookie domain matches target using
// label-boundary semantics. A leading dot in cookieDomain (the RFC 6265 host-
// only flag indicator used by Chrome) is stripped before comparison.
// This prevents "evil-example.com" from matching a target of "example.com".
func cookieDomainMatches(cookieDomain, target string) bool {
	d := strings.TrimPrefix(cookieDomain, ".")
	return d == target || strings.HasSuffix(d, "."+target)
}

// screenshotFilename turns a URL into a safe .png filename.
func screenshotFilename(rawURL string) string {
	r := strings.NewReplacer(
		"://", "_",
		"/", "_",
		"?", "_",
		"&", "_",
		"=", "_",
		":", "_",
		".", "_",
	)
	safe := r.Replace(rawURL)
	// Trim to a reasonable length.
	const maxLen = 80
	if len(safe) > maxLen {
		safe = safe[:maxLen]
	}
	return safe + ".png"
}
