package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/opentalon/opentalon-chrome/browser"
	"github.com/opentalon/opentalon-chrome/store"
	pluginpkg "github.com/opentalon/opentalon/pkg/plugin"
)

// stubBrowser is a test double for browser.Browser.
type stubBrowser struct {
	navigateFunc             func(ctx context.Context, url string) (string, error)
	getTextFunc              func(ctx context.Context, url, selector string) (string, error)
	getHTMLFunc              func(ctx context.Context, url, selector string) (string, error)
	screenshotFunc           func(ctx context.Context, url, selector, outputDir string) (string, []byte, error)
	clickFunc                func(ctx context.Context, url, selector string) error
	typeTextFunc             func(ctx context.Context, url, selector, text string) error
	evaluateFunc             func(ctx context.Context, url, script string) (string, error)
	getCookiesFunc           func(url string) (string, error)
	navigateWithCookiesFunc  func(url string) (string, error)
}

var _ browser.Browser = (*stubBrowser)(nil)

func (s *stubBrowser) Navigate(ctx context.Context, url string) (string, error) {
	return s.navigateFunc(ctx, url)
}
func (s *stubBrowser) GetText(ctx context.Context, url, selector string) (string, error) {
	return s.getTextFunc(ctx, url, selector)
}
func (s *stubBrowser) GetHTML(ctx context.Context, url, selector string) (string, error) {
	return s.getHTMLFunc(ctx, url, selector)
}
func (s *stubBrowser) Screenshot(ctx context.Context, url, selector, outputDir string) (string, []byte, error) {
	return s.screenshotFunc(ctx, url, selector, outputDir)
}
func (s *stubBrowser) Click(ctx context.Context, url, selector string) error {
	return s.clickFunc(ctx, url, selector)
}
func (s *stubBrowser) TypeText(ctx context.Context, url, selector, text string) error {
	return s.typeTextFunc(ctx, url, selector, text)
}
func (s *stubBrowser) Evaluate(ctx context.Context, url, script string) (string, error) {
	return s.evaluateFunc(ctx, url, script)
}
func (s *stubBrowser) GetCookies(_ context.Context, url, _ string) (string, error) {
	if s.getCookiesFunc != nil {
		return s.getCookiesFunc(url)
	}
	return "[]", nil
}
func (s *stubBrowser) NavigateWithCookies(_ context.Context, url, _ string) (string, error) {
	if s.navigateWithCookiesFunc != nil {
		return s.navigateWithCookiesFunc(url)
	}
	return url + " title", nil
}

func newTestHandler(b browser.Browser) *Handler {
	return &Handler{b: b, screenshotDir: "/tmp", timeout: 30 * time.Second}
}

func newTestHandlerWithStore(t *testing.T, b browser.Browser) *Handler {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Handler{
		b:             b,
		screenshotDir: "/tmp",
		timeout:       30 * time.Second,
		store:         store.New(db),
	}
}

// --- Capabilities ---

func TestCapabilities(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	caps := h.Capabilities()

	if caps.Name != pluginName {
		t.Errorf("Name = %q, want %q", caps.Name, pluginName)
	}
	if caps.Description == "" {
		t.Error("Description should not be empty")
	}

	actionNames := make(map[string]bool, len(caps.Actions))
	for _, a := range caps.Actions {
		actionNames[a.Name] = true
	}
	expected := []string{
		"navigate", "get_text", "get_html", "screenshot", "click", "type_text", "evaluate",
		"start_login_session", "get_cookies", "navigate_with_cookies",
		"save_credentials", "get_credentials", "list_credentials", "delete_credentials",
	}
	for _, name := range expected {
		if !actionNames[name] {
			t.Errorf("missing action %q in capabilities", name)
		}
	}
}

// --- Unknown action ---

func TestExecute_unknownAction(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{ID: "r1", Action: "does-not-exist"})

	if resp.Error == "" {
		t.Error("expected error for unknown action")
	}
	if resp.CallID != "r1" {
		t.Errorf("CallID = %q, want r1", resp.CallID)
	}
}

// --- navigate ---

func TestExecute_navigate_missingURL(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{ID: "r2", Action: "navigate", Args: map[string]string{}})
	if resp.Error == "" {
		t.Error("expected error when url is missing")
	}
}

func TestExecute_navigate_success(t *testing.T) {
	b := &stubBrowser{
		navigateFunc: func(_ context.Context, url string) (string, error) {
			return "Example Domain", nil
		},
	}
	h := newTestHandler(b)
	resp := h.Execute(pluginpkg.Request{
		ID:     "r3",
		Action: "navigate",
		Args:   map[string]string{"url": "https://example.com"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if resp.Content == "" {
		t.Error("Content should not be empty on success")
	}
}

func TestExecute_navigate_browserError(t *testing.T) {
	b := &stubBrowser{
		navigateFunc: func(_ context.Context, url string) (string, error) {
			return "", errors.New("connection refused")
		},
	}
	h := newTestHandler(b)
	resp := h.Execute(pluginpkg.Request{
		ID:     "r4",
		Action: "navigate",
		Args:   map[string]string{"url": "https://example.com"},
	})
	if resp.Error == "" {
		t.Error("expected error from browser")
	}
}

// --- get_text ---

func TestExecute_getText_missingURL(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{ID: "r5", Action: "get_text", Args: map[string]string{}})
	if resp.Error == "" {
		t.Error("expected error when url is missing")
	}
}

func TestExecute_getText_success(t *testing.T) {
	b := &stubBrowser{
		getTextFunc: func(_ context.Context, url, selector string) (string, error) {
			return "Hello, world!", nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r6",
		Action: "get_text",
		Args:   map[string]string{"url": "https://example.com"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if resp.Content != "Hello, world!" {
		t.Errorf("Content = %q, want Hello, world!", resp.Content)
	}
}

// --- get_html ---

func TestExecute_getHTML_success(t *testing.T) {
	b := &stubBrowser{
		getHTMLFunc: func(_ context.Context, url, selector string) (string, error) {
			return "<html></html>", nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r7",
		Action: "get_html",
		Args:   map[string]string{"url": "https://example.com", "selector": "html"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if resp.Content != "<html></html>" {
		t.Errorf("Content = %q, want <html></html>", resp.Content)
	}
}

// --- screenshot ---

func TestExecute_screenshot_missingURL(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{ID: "r8", Action: "screenshot", Args: map[string]string{}})
	if resp.Error == "" {
		t.Error("expected error when url is missing")
	}
}

func TestExecute_screenshot_success(t *testing.T) {
	b := &stubBrowser{
		screenshotFunc: func(_ context.Context, url, selector, outputDir string) (string, []byte, error) {
			return "/tmp/example_com.png", []byte("fakepng"), nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r9",
		Action: "screenshot",
		Args:   map[string]string{"url": "https://example.com"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if resp.Content == "" {
		t.Error("Content should not be empty on success")
	}
}

func TestExecute_screenshot_inlineBase64(t *testing.T) {
	pngData := []byte("PNG_CONTENT")
	b := &stubBrowser{
		screenshotFunc: func(_ context.Context, url, selector, outputDir string) (string, []byte, error) {
			return "/tmp/page.png", pngData, nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r9b",
		Action: "screenshot",
		Args:   map[string]string{"url": "https://example.com"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if !contains(resp.Content, "data:image/png;base64,") {
		t.Errorf("expected inline base64 in Content, got: %s", resp.Content)
	}
	if !contains(resp.Content, "/tmp/page.png") {
		t.Errorf("expected file path in Content, got: %s", resp.Content)
	}
}

func TestExecute_screenshot_largeImage(t *testing.T) {
	// Larger than maxInlineBytes — should NOT include base64.
	largeData := make([]byte, maxInlineBytes+1)
	b := &stubBrowser{
		screenshotFunc: func(_ context.Context, url, selector, outputDir string) (string, []byte, error) {
			return "/tmp/big.png", largeData, nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r9c",
		Action: "screenshot",
		Args:   map[string]string{"url": "https://example.com"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if contains(resp.Content, "data:image/png;base64,") {
		t.Error("expected no inline base64 for large image")
	}
	if !contains(resp.Content, "too large") {
		t.Errorf("expected size note in Content, got: %s", resp.Content)
	}
}

// --- start_login_session ---

func TestExecute_startLoginSession_noURL(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	// loginURL is empty — should return an error.
	resp := h.Execute(pluginpkg.Request{ID: "s1", Action: "start_login_session"})
	if resp.Error == "" {
		t.Error("expected error when loginURL is not configured")
	}
}

func TestExecute_startLoginSession_success(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	h.loginURL = "https://chrome-login.example.com"
	h.loginPassword = "testpass"
	resp := h.Execute(pluginpkg.Request{ID: "s2", Action: "start_login_session"})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Content, "https://chrome-login.example.com") {
		t.Errorf("Content should contain the login URL, got: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "testpass") {
		t.Errorf("Content should contain the password, got: %s", resp.Content)
	}
}

// --- get_cookies ---

func TestExecute_getCookies_missingURL(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{ID: "gc1", Action: "get_cookies", Args: map[string]string{}})
	if resp.Error == "" {
		t.Error("expected error when url is missing")
	}
}

func TestExecute_getCookies_success(t *testing.T) {
	b := &stubBrowser{
		getCookiesFunc: func(_ string) (string, error) {
			return `[{"name":"session","value":"xyz"}]`, nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "gc2",
		Action: "get_cookies",
		Args:   map[string]string{"url": "https://app.example.com"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Content, "session") {
		t.Errorf("Content should contain cookie data, got: %s", resp.Content)
	}
}

func TestExecute_getCookies_usesLoginBrowserWhenSet(t *testing.T) {
	headless := &stubBrowser{
		getCookiesFunc: func(_ string) (string, error) { return `["headless"]`, nil },
	}
	login := &stubBrowser{
		getCookiesFunc: func(_ string) (string, error) { return `["login"]`, nil },
	}
	h := newTestHandler(headless)
	h.loginBrowser = login

	resp := h.Execute(pluginpkg.Request{
		ID:     "gc3",
		Action: "get_cookies",
		Args:   map[string]string{"url": "https://app.example.com"},
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Content, "login") {
		t.Errorf("expected login browser cookies, got: %s", resp.Content)
	}
}

func TestExecute_getCookies_browserError(t *testing.T) {
	b := &stubBrowser{
		getCookiesFunc: func(_ string) (string, error) {
			return "", errors.New("CDP unavailable")
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "gc4",
		Action: "get_cookies",
		Args:   map[string]string{"url": "https://app.example.com"},
	})
	if resp.Error == "" {
		t.Error("expected error from browser")
	}
}

// --- navigate_with_cookies ---

func TestExecute_navigateWithCookies_missingURL(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "nwc1",
		Action: "navigate_with_cookies",
		Args:   map[string]string{"cookies": "[]"},
	})
	if resp.Error == "" {
		t.Error("expected error when url is missing")
	}
}

func TestExecute_navigateWithCookies_missingCookies(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "nwc2",
		Action: "navigate_with_cookies",
		Args:   map[string]string{"url": "https://app.example.com"},
	})
	if resp.Error == "" {
		t.Error("expected error when cookies is missing")
	}
}

func TestExecute_navigateWithCookies_success(t *testing.T) {
	b := &stubBrowser{
		navigateWithCookiesFunc: func(url string) (string, error) {
			return "Dashboard — example.com", nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "nwc3",
		Action: "navigate_with_cookies",
		Args:   map[string]string{"url": "https://app.example.com/dash", "cookies": `[]`},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Content, "https://app.example.com/dash") {
		t.Errorf("Content should contain URL, got: %s", resp.Content)
	}
}

// --- save_credentials ---

func TestExecute_saveCredentials_noStore(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "sc1",
		Action: "save_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "myapp", "cookies": "[]"},
	})
	if resp.Error == "" {
		t.Error("expected error when store is nil")
	}
}

func TestExecute_saveCredentials_noActorID(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "sc2",
		Action: "save_credentials",
		Args:   map[string]string{"name": "myapp", "cookies": "[]"},
	})
	if resp.Error == "" {
		t.Error("expected error when actor_id is missing")
	}
}

func TestExecute_saveCredentials_noName(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "sc3",
		Action: "save_credentials",
		Args:   map[string]string{"actor_id": "alice", "cookies": "[]"},
	})
	if resp.Error == "" {
		t.Error("expected error when name is missing")
	}
}

func TestExecute_saveCredentials_success(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "sc4",
		Action: "save_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "myapp-work", "cookies": `[{"name":"sid","value":"abc"}]`},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Content, "myapp-work") {
		t.Errorf("Content should mention the saved name, got: %s", resp.Content)
	}
}

// --- get_credentials ---

func TestExecute_getCredentials_noStore(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "gc_s1",
		Action: "get_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "myapp"},
	})
	if resp.Error == "" {
		t.Error("expected error when store is nil")
	}
}

func TestExecute_getCredentials_notFound(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "gc_s2",
		Action: "get_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "missing"},
	})
	if resp.Error == "" {
		t.Error("expected error for missing credentials")
	}
}

func TestExecute_getCredentials_success(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	// Save first, then retrieve.
	h.Execute(pluginpkg.Request{
		ID:     "gc_setup",
		Action: "save_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "myapp-work", "cookies": `["cookie1"]`},
	})
	resp := h.Execute(pluginpkg.Request{
		ID:     "gc_s3",
		Action: "get_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "myapp-work"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if resp.Content != `["cookie1"]` {
		t.Errorf("Content = %q, want [\"cookie1\"]", resp.Content)
	}
}

// --- list_credentials ---

func TestExecute_listCredentials_empty(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "lc1",
		Action: "list_credentials",
		Args:   map[string]string{"actor_id": "alice"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Content, "No saved") {
		t.Errorf("expected empty message, got: %s", resp.Content)
	}
}

func TestExecute_listCredentials_multiple(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	for _, name := range []string{"myapp-personal", "myapp-work"} {
		h.Execute(pluginpkg.Request{
			ID:     "lc_setup_" + name,
			Action: "save_credentials",
			Args:   map[string]string{"actor_id": "alice", "name": name, "cookies": "[]"},
		})
	}
	resp := h.Execute(pluginpkg.Request{
		ID:     "lc2",
		Action: "list_credentials",
		Args:   map[string]string{"actor_id": "alice"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if !strings.Contains(resp.Content, "myapp-personal") || !strings.Contains(resp.Content, "myapp-work") {
		t.Errorf("Content should list both credentials, got: %s", resp.Content)
	}
}

// --- delete_credentials ---

func TestExecute_deleteCredentials_success(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	h.Execute(pluginpkg.Request{
		ID:     "dc_setup",
		Action: "save_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "myapp-work", "cookies": "[]"},
	})
	resp := h.Execute(pluginpkg.Request{
		ID:     "dc1",
		Action: "delete_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "myapp-work"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	// Confirm it's gone.
	getResp := h.Execute(pluginpkg.Request{
		ID:     "dc_verify",
		Action: "get_credentials",
		Args:   map[string]string{"actor_id": "alice", "name": "myapp-work"},
	})
	if getResp.Error == "" {
		t.Error("expected error after deletion, got nil")
	}
}

func TestExecute_deleteCredentials_noName(t *testing.T) {
	h := newTestHandlerWithStore(t, &stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "dc2",
		Action: "delete_credentials",
		Args:   map[string]string{"actor_id": "alice"},
	})
	if resp.Error == "" {
		t.Error("expected error when name is missing")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// --- click ---

func TestExecute_click_missingURL(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{ID: "r10", Action: "click", Args: map[string]string{"selector": "button"}})
	if resp.Error == "" {
		t.Error("expected error when url is missing")
	}
}

func TestExecute_click_missingSelector(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{ID: "r11", Action: "click", Args: map[string]string{"url": "https://example.com"}})
	if resp.Error == "" {
		t.Error("expected error when selector is missing")
	}
}

func TestExecute_click_success(t *testing.T) {
	b := &stubBrowser{
		clickFunc: func(_ context.Context, url, selector string) error {
			return nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r12",
		Action: "click",
		Args:   map[string]string{"url": "https://example.com", "selector": "#submit"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

// --- type_text ---

func TestExecute_typeText_missingSelector(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "r13",
		Action: "type_text",
		Args:   map[string]string{"url": "https://example.com", "text": "hello"},
	})
	if resp.Error == "" {
		t.Error("expected error when selector is missing")
	}
}

func TestExecute_typeText_success(t *testing.T) {
	b := &stubBrowser{
		typeTextFunc: func(_ context.Context, url, selector, text string) error {
			return nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r14",
		Action: "type_text",
		Args:   map[string]string{"url": "https://example.com", "selector": "input", "text": "hello"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

// --- evaluate ---

func TestExecute_evaluate_missingScript(t *testing.T) {
	h := newTestHandler(&stubBrowser{})
	resp := h.Execute(pluginpkg.Request{
		ID:     "r15",
		Action: "evaluate",
		Args:   map[string]string{"url": "https://example.com"},
	})
	if resp.Error == "" {
		t.Error("expected error when script is missing")
	}
}

func TestExecute_evaluate_success(t *testing.T) {
	b := &stubBrowser{
		evaluateFunc: func(_ context.Context, url, script string) (string, error) {
			return `"hello"`, nil
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r16",
		Action: "evaluate",
		Args:   map[string]string{"url": "https://example.com", "script": "document.title"},
	})
	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
	if resp.Content != `"hello"` {
		t.Errorf("Content = %q, want %q", resp.Content, `"hello"`)
	}
}

func TestExecute_evaluate_browserError(t *testing.T) {
	b := &stubBrowser{
		evaluateFunc: func(_ context.Context, url, script string) (string, error) {
			return "", errors.New("evaluation failed")
		},
	}
	resp := newTestHandler(b).Execute(pluginpkg.Request{
		ID:     "r17",
		Action: "evaluate",
		Args:   map[string]string{"url": "https://example.com", "script": "bad script"},
	})
	if resp.Error == "" {
		t.Error("expected error from evaluate")
	}
	if resp.CallID != "r17" {
		t.Errorf("CallID = %q, want r17", resp.CallID)
	}
}
