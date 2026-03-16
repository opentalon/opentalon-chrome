package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opentalon/opentalon-chrome/browser"
	pluginpkg "github.com/opentalon/opentalon/pkg/plugin"
)

// stubBrowser is a test double for browser.Browser.
type stubBrowser struct {
	navigateFunc    func(ctx context.Context, url string) (string, error)
	getTextFunc     func(ctx context.Context, url, selector string) (string, error)
	getHTMLFunc     func(ctx context.Context, url, selector string) (string, error)
	screenshotFunc  func(ctx context.Context, url, selector, outputDir string) (string, []byte, error)
	clickFunc       func(ctx context.Context, url, selector string) error
	typeTextFunc    func(ctx context.Context, url, selector, text string) error
	evaluateFunc    func(ctx context.Context, url, script string) (string, error)
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

func newTestHandler(b browser.Browser) *Handler {
	return NewHandler(b, "/tmp", 30*time.Second)
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
	expected := []string{"navigate", "get_text", "get_html", "screenshot", "click", "type_text", "evaluate"}
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
