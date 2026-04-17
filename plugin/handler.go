// Package plugin implements the OpenTalon plugin.Handler for headless Chrome.
package plugin

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/opentalon/opentalon-chrome/browser"
	"github.com/opentalon/opentalon-chrome/config"
	"github.com/opentalon/opentalon-chrome/store"
	pluginpkg "github.com/opentalon/opentalon/pkg/plugin"
)

// maxInlineBytes is the maximum PNG size we will base64-encode inline in the
// response.  Screenshots larger than this are returned as a file path only so
// the response stays well within the 64 KB guard limit.
const maxInlineBytes = 40 * 1024 // 40 KB raw → ~54 KB base64

const pluginName = "chrome"

// Handler implements pluginpkg.Handler using a headless Chrome browser.
type Handler struct {
	b                  browser.Browser
	loginBrowser       browser.Browser // VNC Chrome for interactive login sessions; may be nil
	screenshotDir      string
	timeout            time.Duration
	loginURL           string
	loginPassword      string
	store              *store.Store
	allowClientActorID bool // see config.AllowClientActorID
}

// NewHandler returns a Handler with default configuration. The host will call
// Configure (via the Capabilities RPC) before any Execute calls.
func NewHandler() *Handler {
	return &Handler{timeout: config.DefaultTimeout, screenshotDir: ""}
}

// Configure implements pluginpkg.Configurable. It parses the JSON config
// delivered by the OpenTalon host and initialises the browser client.
func (h *Handler) Configure(configJSON string) error {
	cfg, err := config.Load(configJSON)
	if err != nil {
		return err
	}
	h.b = browser.NewClient(cfg.CDPURL, cfg.ParseTimeout())
	h.screenshotDir = cfg.ScreenshotDir
	h.timeout = cfg.ParseTimeout()
	h.loginURL = cfg.LoginURL
	h.loginPassword = cfg.LoginPassword
	h.allowClientActorID = cfg.AllowClientActorID

	// Set up the VNC Chrome client when a separate CDP URL is configured.
	if cfg.LoginCDPURL != "" {
		h.loginBrowser = browser.NewClient(cfg.LoginCDPURL, cfg.ParseTimeout())
	}

	// Open the credential store — Postgres when a URL is provided, SQLite otherwise.
	var storeDB *store.DB
	if cfg.DatabaseURL != "" {
		storeDB, err = store.OpenPostgres(cfg.DatabaseURL)
	} else {
		storeDB, err = store.OpenSQLite(cfg.DataDir)
	}
	if err != nil {
		return fmt.Errorf("open credential store: %w", err)
	}
	h.store = store.New(storeDB)
	return nil
}

// Capabilities declares all actions this plugin exposes.
func (h *Handler) Capabilities() pluginpkg.CapabilitiesMsg {
	return pluginpkg.CapabilitiesMsg{
		Name:        pluginName,
		Description: "Headless Chrome browser: navigate pages, extract content, take screenshots, run JavaScript, and manage login session cookies",
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
			// ── Login session / cookie actions ────────────────────────────────────
			{
				Name:        "get_login_url",
				Description: "Returns the URL and password for the interactive Chrome session. The user opens this URL to log in to a service manually.",
				// UserOnly: true — re-enable once opentalon pkg/plugin.ActionMsg includes UserOnly.
			},
			{
				Name:        "get_cookies",
				Description: "Navigate to url in the interactive VNC Chrome (login browser) and return all cookies as a JSON array, optionally filtered by domain suffix. Call this after the user has logged in to capture the session cookies.",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "URL to navigate to before collecting cookies", Type: "string", Required: true},
					{Name: "domain", Description: "Domain suffix to filter cookies (e.g. linkedin.com). Leave empty to return all cookies.", Type: "string", Required: false},
				},
			},
			{
				Name:        "navigate_with_cookies",
				Description: "Inject the provided cookies into headless Chrome and navigate to url. Returns the page title. Use the JSON cookies returned by get_credentials.",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "url", Description: "URL to navigate to", Type: "string", Required: true},
					{Name: "cookies", Description: "JSON array of cookie objects (from get_credentials)", Type: "string", Required: true},
				},
			},
			{
				// InjectContextArgs: []string{"actor_id"} — re-enable once opentalon pkg/plugin.ActionMsg includes InjectContextArgs.
				// Until then actor_id is injected by the host when the opentalon core is updated.
				Name:        "save_credentials",
				Description: "Save browser login cookies under a user-chosen name for the current entity. Use a descriptive name like 'linkedin-work' or 'github-personal' to distinguish multiple accounts.",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "name", Description: "Unique credential name, e.g. 'linkedin-work'", Type: "string", Required: true},
					{Name: "cookies", Description: "JSON array of cookie objects (from get_cookies)", Type: "string", Required: true},
				},
			},
			{
				Name:        "get_credentials",
				Description: "Retrieve saved browser cookies by name for the current entity. Returns the cookie JSON for use with navigate_with_cookies.",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "name", Description: "Credential name to retrieve", Type: "string", Required: true},
				},
			},
			{
				Name:        "list_credentials",
				Description: "List the names of all saved browser credentials for the current entity.",
			},
			{
				Name:        "delete_credentials",
				Description: "Delete a saved browser credential by name for the current entity.",
				Parameters: []pluginpkg.ParameterMsg{
					{Name: "name", Description: "Credential name to delete", Type: "string", Required: true},
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
	case "get_login_url":
		return h.getLoginURL(req)
	case "get_cookies":
		return h.getCookies(ctx, req)
	case "navigate_with_cookies":
		return h.navigateWithCookies(ctx, req)
	case "save_credentials":
		return h.saveCredentials(req)
	case "get_credentials":
		return h.getCredentials(req)
	case "list_credentials":
		return h.listCredentials(req)
	case "delete_credentials":
		return h.deleteCredentials(req)
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

// ── Login session / cookie handlers ──────────────────────────────────────────

func (h *Handler) getLoginURL(req pluginpkg.Request) pluginpkg.Response {
	if h.loginURL == "" {
		return errResp(req.ID, "get_login_url: no login URL configured (CHROME_LOGIN_URL is not set)")
	}
	content := fmt.Sprintf("Open the following URL in your browser to access the interactive Chrome session:\n\nURL: %s\nUsername: opentalon\nPassword: %s\n\nLog in to the desired service, then tell me when you are done.",
		h.loginURL, h.loginPassword)
	return pluginpkg.Response{CallID: req.ID, Content: content}
}

func (h *Handler) getCookies(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	rawURL, ok := req.Args["url"]
	if !ok || rawURL == "" {
		return errResp(req.ID, "get_cookies: url is required")
	}
	domain := req.Args["domain"]

	// Use the VNC login browser if configured; fall back to headless Chrome.
	b := h.b
	if h.loginBrowser != nil {
		b = h.loginBrowser
	}

	cookiesJSON, err := b.GetCookies(ctx, rawURL, domain)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: cookiesJSON}
}

func (h *Handler) navigateWithCookies(ctx context.Context, req pluginpkg.Request) pluginpkg.Response {
	rawURL, ok := req.Args["url"]
	if !ok || rawURL == "" {
		return errResp(req.ID, "navigate_with_cookies: url is required")
	}
	cookiesJSON, ok := req.Args["cookies"]
	if !ok || cookiesJSON == "" {
		return errResp(req.ID, "navigate_with_cookies: cookies is required")
	}
	title, err := h.b.NavigateWithCookies(ctx, rawURL, cookiesJSON)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: fmt.Sprintf("Navigated to %q (title: %q)", rawURL, title)}
}

// ── Credential store handlers ─────────────────────────────────────────────────

// actorIDFromRequest returns the actor_id from req.Args when the operator has
// explicitly opted in via AllowClientActorID.  When the flag is false it
// returns an error, refusing to run credential actions with an untrusted
// identity claim.  Remove this helper (and use a trusted context field instead)
// once the opentalon host wires InjectContextArgs into the plugin protocol.
func (h *Handler) actorIDFromRequest(action string, req pluginpkg.Request) (string, error) {
	if !h.allowClientActorID {
		return "", fmt.Errorf("%s: credential actions are disabled — "+
			"set allow_client_actor_id: true (or CHROME_ALLOW_CLIENT_ACTOR_ID=true) only on single-tenant "+
			"deployments where callers cannot forge actor_id; "+
			"this restriction will be lifted once the host wires InjectContextArgs", action)
	}
	id := req.Args["actor_id"]
	if id == "" {
		return "", fmt.Errorf("%s: actor_id not available (context injection failed)", action)
	}
	return id, nil
}

func (h *Handler) saveCredentials(req pluginpkg.Request) pluginpkg.Response {
	if h.store == nil {
		return errResp(req.ID, "save_credentials: credential store not initialised")
	}
	actorID, err := h.actorIDFromRequest("save_credentials", req)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	name := req.Args["name"]
	if name == "" {
		return errResp(req.ID, "save_credentials: name is required")
	}
	cookies := req.Args["cookies"]
	if cookies == "" {
		return errResp(req.ID, "save_credentials: cookies is required")
	}
	if err := h.store.Save(actorID, name, cookies); err != nil {
		return errResp(req.ID, fmt.Sprintf("save_credentials: %v", err))
	}
	return pluginpkg.Response{CallID: req.ID, Content: fmt.Sprintf("Credentials saved as %q.", name)}
}

func (h *Handler) getCredentials(req pluginpkg.Request) pluginpkg.Response {
	if h.store == nil {
		return errResp(req.ID, "get_credentials: credential store not initialised")
	}
	actorID, err := h.actorIDFromRequest("get_credentials", req)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	name := req.Args["name"]
	if name == "" {
		return errResp(req.ID, "get_credentials: name is required")
	}
	cookies, err := h.store.Get(actorID, name)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	return pluginpkg.Response{CallID: req.ID, Content: cookies}
}

func (h *Handler) listCredentials(req pluginpkg.Request) pluginpkg.Response {
	if h.store == nil {
		return errResp(req.ID, "list_credentials: credential store not initialised")
	}
	actorID, err := h.actorIDFromRequest("list_credentials", req)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	names, err := h.store.List(actorID)
	if err != nil {
		return errResp(req.ID, fmt.Sprintf("list_credentials: %v", err))
	}
	if len(names) == 0 {
		return pluginpkg.Response{CallID: req.ID, Content: "No saved credentials."}
	}
	return pluginpkg.Response{CallID: req.ID, Content: strings.Join(names, "\n")}
}

func (h *Handler) deleteCredentials(req pluginpkg.Request) pluginpkg.Response {
	if h.store == nil {
		return errResp(req.ID, "delete_credentials: credential store not initialised")
	}
	actorID, err := h.actorIDFromRequest("delete_credentials", req)
	if err != nil {
		return errResp(req.ID, err.Error())
	}
	name := req.Args["name"]
	if name == "" {
		return errResp(req.ID, "delete_credentials: name is required")
	}
	if err := h.store.Delete(actorID, name); err != nil {
		return errResp(req.ID, fmt.Sprintf("delete_credentials: %v", err))
	}
	return pluginpkg.Response{CallID: req.ID, Content: fmt.Sprintf("Credentials %q deleted.", name)}
}

func errResp(callID, msg string) pluginpkg.Response {
	return pluginpkg.Response{CallID: callID, Error: msg}
}
