package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// ── Shared llmHTTPClient ──────────────────────────────────────────────────────

func TestLLMHTTPClient_NotNil(t *testing.T) {
	if llmHTTPClient == nil {
		t.Fatal("llmHTTPClient is nil — expected a non-nil package-level client")
	}
}

func TestLLMHTTPClient_HasCustomTransport(t *testing.T) {
	tr, ok := llmHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("llmHTTPClient.Transport is %T, want *http.Transport", llmHTTPClient.Transport)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("llmHTTPClient.Transport.ForceAttemptHTTP2 is false, want true")
	}
}

func TestLLMHTTPClient_ReasonableTimeout(t *testing.T) {
	if llmHTTPClient.Timeout == 0 {
		t.Error("llmHTTPClient.Timeout is 0 — expected a positive timeout")
	}
}

// ── Connection: keep-alive removal ───────────────────────────────────────────

func TestSSEHandlers_NoConnectionKeepAlive(t *testing.T) {
	files := []string{
		"rfq_sse.go",
		"s3_storage.go",
		"s3_cleanup.go",
	}
	for _, fname := range files {
		src, err := os.ReadFile(fname)
		if err != nil {
			t.Skipf("cannot read %s: %v", fname, err)
		}
		if strings.Contains(string(src), `"Connection", "keep-alive"`) {
			t.Errorf("%s still sets 'Connection: keep-alive' — forbidden hop-by-hop header in HTTP/2", fname)
		}
	}
}

// ── crawlWebsiteForEmail — parallel contact page fetch ────────────────────────

// mockCrawlServer builds a test HTTP server whose home page links to a
// /contact path.  The /contact page contains an email address.
func mockCrawlServer(t *testing.T, homeEmail, contactEmail string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "":
			if homeEmail != "" {
				fmt.Fprintf(w, `<html><body>%s</body></html>`, homeEmail)
			} else {
				fmt.Fprintf(w, `<html><body><a href="/contact">contact</a></body></html>`)
			}
		case "/contact":
			fmt.Fprintf(w, `<html><body>Email: %s</body></html>`, contactEmail)
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

func TestCrawlWebsiteForEmail_HomePageEmail(t *testing.T) {
	// example.com is in skipEmailDomain, use a real-looking domain instead
	srv := mockCrawlServer(t, "hello@acme-supplier.com", "")
	defer srv.Close()
	got := crawlWebsiteForEmail(srv.URL)
	if got != "hello@acme-supplier.com" {
		t.Errorf("got %q, want %q", got, "hello@acme-supplier.com")
	}
}

func TestCrawlWebsiteForEmail_ContactPageEmail(t *testing.T) {
	srv := mockCrawlServer(t, "", "info@acme-supplier.com")
	defer srv.Close()
	got := crawlWebsiteForEmail(srv.URL)
	if got != "info@acme-supplier.com" {
		t.Errorf("got %q, want %q", got, "info@acme-supplier.com")
	}
}

func TestCrawlWebsiteForEmail_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<html><body>No contact info here.</body></html>")
	}))
	defer srv.Close()
	got := crawlWebsiteForEmail(srv.URL)
	if got != "" {
		t.Errorf("got %q, want empty string for page with no email", got)
	}
}

func TestCrawlWebsiteForEmail_EmptyInput(t *testing.T) {
	got := crawlWebsiteForEmail("")
	if got != "" {
		t.Errorf("got %q, want empty for empty input", got)
	}
}

func TestCrawlWebsiteForEmail_UnreachableURL(t *testing.T) {
	// Should not panic; just return empty
	got := crawlWebsiteForEmail("http://127.0.0.1:1") // port 1 is always refused
	if got != "" {
		t.Errorf("got %q, want empty for unreachable server", got)
	}
}

func TestCrawlWebsiteForEmail_SchemePrefixed(t *testing.T) {
	srv := mockCrawlServer(t, "noreply@acme-supplier.io", "")
	defer srv.Close()
	// Verify the function works with a URL that already has http:// prefix
	got := crawlWebsiteForEmail(srv.URL + "/")
	if got != "noreply@acme-supplier.io" {
		t.Errorf("got %q, want noreply@acme-supplier.io", got)
	}
}
