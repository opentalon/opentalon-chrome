// Package browser defines the Browser interface and result types for
// headless Chrome operations performed over the Chrome DevTools Protocol.
package browser

import "context"

// Browser abstracts the set of headless browser actions exposed by this plugin.
// The concrete implementation uses chromedp; tests use a stub.
type Browser interface {
	// Navigate loads url and returns the page title.
	Navigate(ctx context.Context, url string) (string, error)

	// GetText returns the text content of the element matching selector on url.
	// An empty selector defaults to "body".
	GetText(ctx context.Context, url, selector string) (string, error)

	// GetHTML returns the outer HTML of the element matching selector on url.
	// An empty selector defaults to ":root".
	GetHTML(ctx context.Context, url, selector string) (string, error)

	// Screenshot navigates to url, captures a PNG screenshot, writes it to
	// outputDir, and returns the absolute file path and the raw PNG bytes.
	// If selector is non-empty, only that element is captured.
	// Callers may embed the bytes as a data URL to send the image inline.
	Screenshot(ctx context.Context, url, selector, outputDir string) (path string, data []byte, err error)

	// Click navigates to url and clicks the first element matching selector.
	Click(ctx context.Context, url, selector string) error

	// TypeText navigates to url and sends keystrokes to the element matching selector.
	TypeText(ctx context.Context, url, selector, text string) error

	// Evaluate navigates to url, runs script, and returns the JSON-encoded result.
	Evaluate(ctx context.Context, url, script string) (string, error)
}
