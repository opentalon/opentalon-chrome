package browser

import (
	"testing"
)

func TestRewriteHost(t *testing.T) {
	cases := []struct {
		name    string
		wsRaw   string
		cdpRaw  string
		want    string
		wantErr bool
	}{
		{
			name:   "localhost to docker hostname",
			wsRaw:  "ws://localhost:9222/devtools/browser/abc123",
			cdpRaw: "http://chrome-sidecar:9222",
			want:   "ws://chrome-sidecar:9222/devtools/browser/abc123",
		},
		{
			name:   "same host no-op",
			wsRaw:  "ws://chrome:9222/devtools/browser/xyz",
			cdpRaw: "http://chrome:9222",
			want:   "ws://chrome:9222/devtools/browser/xyz",
		},
		{
			name:   "different port",
			wsRaw:  "ws://localhost:9222/devtools/browser/abc",
			cdpRaw: "http://remote-host:9333",
			want:   "ws://remote-host:9333/devtools/browser/abc",
		},
		{
			name:    "invalid cdp url",
			wsRaw:   "ws://localhost:9222/devtools/browser/abc",
			cdpRaw:  "://bad",
			wantErr: true,
		},
		{
			name:    "invalid ws url",
			wsRaw:   "://bad",
			cdpRaw:  "http://chrome:9222",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := rewriteHost(c.wsRaw, c.cdpRaw)
			if c.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("rewriteHost = %q, want %q", got, c.want)
			}
		})
	}
}

func TestCookieDomainMatches(t *testing.T) {
	cases := []struct {
		cookieDomain string
		target       string
		want         bool
	}{
		// exact match
		{"example.com", "example.com", true},
		// leading-dot variant (Chrome style)
		{".example.com", "example.com", true},
		// subdomain match
		{"sub.example.com", "example.com", true},
		{".sub.example.com", "example.com", true},
		// lookalike domain must NOT match
		{"evil-example.com", "example.com", false},
		{"evilexample.com", "example.com", false},
		// unrelated domain
		{"other.com", "example.com", false},
		// deeper subdomain
		{"a.b.example.com", "example.com", true},
		// target is itself a subdomain
		{"sub.example.com", "sub.example.com", true},
		{"other.example.com", "sub.example.com", false},
	}
	for _, c := range cases {
		got := cookieDomainMatches(c.cookieDomain, c.target)
		if got != c.want {
			t.Errorf("cookieDomainMatches(%q, %q) = %v, want %v",
				c.cookieDomain, c.target, got, c.want)
		}
	}
}

func TestScreenshotFilename(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{
			url:  "https://example.com/page",
			want: "https_example_com_page.png",
		},
		{
			url:  "http://localhost:8080",
			want: "http_localhost_8080.png",
		},
		{
			url:  "https://example.com/search?q=hello&lang=en",
			want: "https_example_com_search_q_hello_lang_en.png",
		},
	}
	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			got := screenshotFilename(c.url)
			if got != c.want {
				t.Errorf("screenshotFilename(%q) = %q, want %q", c.url, got, c.want)
			}
		})
	}
}

func TestScreenshotFilename_truncated(t *testing.T) {
	long := "https://example.com/" + string(make([]byte, 200))
	name := screenshotFilename(long)
	// should be at most maxLen + len(".png") characters
	if len(name) > 80+len(".png") {
		t.Errorf("filename length %d exceeds maximum, got: %q", len(name), name)
	}
}
