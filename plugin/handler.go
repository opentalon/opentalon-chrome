// Package plugin implements the OpenTalon plugin.Handler for headless Chrome.
package plugin

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/opentalon/opentalon-chrome/browser"
	pluginpkg "github.com/opentalon/opentalon/pkg/plugin"
)

// maxInlineBytes is the maximum PNG size we will base64-encode inline in the
// response.  Screenshots larger than this are returned as a file path only so
// the response stays well within the 64 KB guard limit.
const maxInlineBytes = 40 * 1024 // 40 KB raw → ~54 KB base64

const pluginName = "chrome"

// Handler implements pluginpkg.Handler using a headless Chrome browser.
type Handler struct {
	b             browser.Browser
	screenshotDir string
	timeout       time.Duration
}

// NewHandler returns a Handler backed by the given Browser implementation.
func NewHandler(b browser.Browser, screenshotDir string, timeout time.Duration) *Handler {
	return &Handler{b: b, screenshotDir: screenshotDir, timeout: timeout}
}

// Capabilities declares all actions this plugin exposes.
func (h *Handler) Capabilities() pluginpkg.CapabilitiesMsg {
	return pluginpkg.CapabilitiesMsg{
		Name:        pluginName,
		Description: "Headless Chrome browser: navigate pages, extract content, take screenshots, and run JavaScript via a Chrome sidecar",
		Actions: []pluginpkg.ActionMsg{
			{
				Name:        "navigate",
				Description: "Navigate to a URL and return the page title",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "Full URL to load, e.g. https://example.com", Type: "string", Required: true},
				},
			},
			{
				Name:        "get_text",
				Description: "Return the visible text content of a page or CSS-selected element",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "Full URL to load", Type: "string", Required: true},
					{Name: "selector", Description: "CSS selector of the element to extract (default: body)", Type: "string", Required: false},
				},
			},
			{
				Name:        "get_html",
				Description: "Return the outer HTML of a page or CSS-selected element",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "Full URL to load", Type: "string", Required: true},
					{Name: "selector", Description: "CSS selector of the element (default: :root)", Type: "string", Required: false},
				},
			},
			{
				Name:        "screenshot",
				Description: "Take a PNG screenshot of a page (or element) and return the saved file path",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "Full URL to load", Type: "string", Required: true},
					{Name: "selector", Description: "CSS selector to screenshot a specific element (optional)", Type: "string", Required: false},
				},
			},
			{
				Name:        "click",
				Description: "Navigate to a URL and click the first element matching a CSS selector",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "Full URL to load", Type: "string", Required: true},
					{Name: "selector", Description: "CSS selector of the element to click", Type: "string", Required: true},
				},
			},
			{
				Name:        "type_text",
				Description: "Navigate to a URL and send keystrokes to a CSS-selected input element",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "Full URL to load", Type: "string", Required: true},
					{Name: "selector", Description: "CSS selector of the input element", Type: "string", Required: true},
					{Name: "text", Description: "Text to type", Type: "string", Required: true},
				},
			},
			{
				Name:        "evaluate",
				Description: "Navigate to a URL, execute a JavaScript expression, and return the JSON-encoded result",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "Full URL to load", Type: "string", Required: true},
					{Name: "script", Description: "JavaScript expression to evaluate", Type: "string", Required: true},
				},
			},
		},
	}
}

// Execute dispatches an action request to the underlying Browser.
func (h *Handler) Execute(req pluginpkg.Request) pluginpkg.Response {
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	switch req.Action {
	case "navigate":
		return h.navigate(ctx, req)
	case "get_text":
		return h.getText(ctx, req)
	case "get_html":
		return h.getHTML(ctx, req)
	case "screenshot":
		return h.screenshot(ctx, req)
	case "click":
		return h.click(ctx, req)
	case "type_text":
		return h.typeText(ctx, req)
	case "evaluate":
		return h.evaluate(ctx, req)
	default:
		return pluginpkg.Response{
			CallID: req.ID,
			Error:  fmt.Sprintf("unknown action %q", req.Action),
		}
	}
}

func (h *Handler) navigate(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	url, ok := req.Args["url"]
	if !ok || url == "" {
		return errResp(req.ID, "navigate: url is required")
	}
	title, err := h.b.Navigate(ctx, url)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: fmt.Sprintf("Page loaded: %q (title: %q)", url, title)}
}

func (h *Handler) getText(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	url, ok := req.Args["url"]
	if !ok || url == "" {
		return errResp(req.ID, "get_text: url is required")
	}
	text, err := h.b.GetText(ctx, url, req.Args["selector"])
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: text}
}

func (h *Handler) getHTML(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	url, ok := req.Args["url"]
	if !ok || url == "" {
		return errResp(req.ID, "get_html: url is required")
	}
	html, err := h.b.GetHTML(ctx, url, req.Args["selector"])
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: html}
}

func (h *Handler) screenshot(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	url, ok := req.Args["url"]
	if !ok || url == "" {
		return errResp(req.ID, "screenshot: url is required")
	}
	path, data, err := h.b.Screenshot(ctx, url, req.Args["selector"], h.screenshotDir)
	if err != nil {
		return errResp(req.ID, err.Error())
	}

	// Always include the file path so the user knows where the full-quality
	// image was saved.  When the PNG is small enough, also embed it as a
	// base64 data URL so the channel/LLM can send it inline to the user.
	content := fmt.Sprintf("Screenshot saved to: %s", path)
	if len(data) <= maxInlineBytes {
		b64 := base64.StdEncoding.EncodeToString(data)
		content += fmt.Sprintf("\ndata:image/png;base64,%s", b64)
	} else {
		content += fmt.Sprintf("\n(image too large for inline display: %d KB)", len(data)/1024)
	}
	return pluginpkg.Response{CallID: req.ID, Content: content}
}

func (h *Handler) click(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	url, ok := req.Args["url"]
	if !ok || url == "" {
		return errResp(req.ID, "click: url is required")
	}
	selector, ok := req.Args["selector"]
	if !ok || selector == "" {
		return errResp(req.ID, "click: selector is required")
	}
	if err := h.b.Click(ctx, url, selector); err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: fmt.Sprintf("Clicked %q on %s", selector, url)}
}

func (h *Handler) typeText(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	url, ok := req.Args["url"]
	if !ok || url == "" {
		return errResp(req.ID, "type_text: url is required")
	}
	selector, ok := req.Args["selector"]
	if !ok || selector == "" {
		return errResp(req.ID, "type_text: selector is required")
	}
	text := req.Args["text"]
	if err := h.b.TypeText(ctx, url, selector, text); err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: fmt.Sprintf("Typed into %q on %s", selector, url)}
}

func (h *Handler) evaluate(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	url, ok := req.Args["url"]
	if !ok || url == "" {
		return errResp(req.ID, "evaluate: url is required")
	}
	script, ok := req.Args["script"]
	if !ok || script == "" {
		return errResp(req.ID, "evaluate: script is required")
	}
	result, err := h.b.Evaluate(ctx, url, script)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: result}
}

func errResp(callID, msg string) pluginpkg.Response {
	return pluginpkg.Response{CallID: callID, Error: msg}
}
