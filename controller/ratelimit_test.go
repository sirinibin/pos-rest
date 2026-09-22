package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRateLimiter_Allow verifies the per-IP counting and window reset logic.
func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, time.Second)

	// First 3 requests from the same IP should be allowed.
	for i := 1; i <= 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("request %d: expected Allow=true", i)
		}
	}

	// 4th request should be denied.
	if rl.Allow("1.2.3.4") {
		t.Error("4th request: expected Allow=false")
	}

	// A different IP is unaffected.
	if !rl.Allow("5.6.7.8") {
		t.Error("different IP: expected Allow=true")
	}
}

// TestRateLimiter_WindowReset verifies that the counter resets after the window expires.
func TestRateLimiter_WindowReset(t *testing.T) {
	rl := NewRateLimiter(1, 50*time.Millisecond)

	if !rl.Allow("1.2.3.4") {
		t.Fatal("first request should be allowed")
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("second request within window should be denied")
	}

	time.Sleep(60 * time.Millisecond) // let the window expire

	if !rl.Allow("1.2.3.4") {
		t.Error("request after window reset should be allowed")
	}
}

// TestRateLimiter_Middleware verifies the HTTP middleware returns 429 once the
// IP exhausts its budget and 200 (from a stub handler) while within budget.
func TestRateLimiter_Middleware(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	stub := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	call := func() int {
		r := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader("{}"))
		r.RemoteAddr = "10.0.0.1:9999" // no X-Real-IP header → uses RemoteAddr
		w := httptest.NewRecorder()
		stub(w, r)
		return w.Code
	}

	if got := call(); got != http.StatusOK {
		t.Errorf("request 1: want 200, got %d", got)
	}
	if got := call(); got != http.StatusOK {
		t.Errorf("request 2: want 200, got %d", got)
	}
	// 3rd request exceeds limit of 2.
	w3 := httptest.NewRecorder()
	r3 := httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader("{}"))
	r3.RemoteAddr = "10.0.0.1:9999"
	stub(w3, r3)

	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("request 3: want 429, got %d", w3.Code)
	}
	if ct := w3.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("want JSON Content-Type on 429, got %q", ct)
	}
	if ra := w3.Header().Get("Retry-After"); ra == "" {
		t.Error("want Retry-After header on 429")
	}
	var resp struct {
		Status bool              `json:"status"`
		Errors map[string]string `json:"errors"`
	}
	if err := json.NewDecoder(w3.Body).Decode(&resp); err != nil {
		t.Fatalf("decode 429 body: %v", err)
	}
	if resp.Status {
		t.Error("want status=false in 429 response")
	}
	if _, ok := resp.Errors["rate_limit"]; !ok {
		t.Errorf("want errors.rate_limit, got %v", resp.Errors)
	}
}

// TestRateLimiter_XRealIP verifies that the X-Real-IP header is preferred over RemoteAddr.
func TestRateLimiter_XRealIP(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)

	newReq := func(remoteAddr, xRealIP string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = remoteAddr
		if xRealIP != "" {
			r.Header.Set("X-Real-IP", xRealIP)
		}
		return r
	}

	// Two requests from RemoteAddr 127.0.0.1 but different X-Real-IP values
	// should be counted separately.
	stub := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w1 := httptest.NewRecorder()
	stub(w1, newReq("127.0.0.1:80", "203.0.113.1"))
	if w1.Code != http.StatusOK {
		t.Errorf("client A first request: want 200, got %d", w1.Code)
	}

	w2 := httptest.NewRecorder()
	stub(w2, newReq("127.0.0.1:80", "203.0.113.2"))
	if w2.Code != http.StatusOK {
		t.Errorf("client B first request: want 200, got %d", w2.Code)
	}

	// Second request from client A should be blocked.
	w3 := httptest.NewRecorder()
	stub(w3, newReq("127.0.0.1:80", "203.0.113.1"))
	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("client A second request: want 429, got %d", w3.Code)
	}
}

// TestRealClientIP checks the IP extraction helper for all three header scenarios.
func TestRealClientIP(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		xRealIP    string
		xff        string
		wantIP     string
	}{
		{"X-Real-IP wins", "127.0.0.1:80", "1.2.3.4", "", "1.2.3.4"},
		{"X-Forwarded-For fallback", "127.0.0.1:80", "", "9.9.9.9, 10.0.0.1", "9.9.9.9"},
		{"RemoteAddr fallback", "5.5.5.5:1234", "", "", "5.5.5.5"},
		{"X-Real-IP trumps X-Forwarded-For", "127.0.0.1:80", "1.1.1.1", "2.2.2.2", "1.1.1.1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.xRealIP != "" {
				r.Header.Set("X-Real-IP", tc.xRealIP)
			}
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			got := realClientIP(r)
			if got != tc.wantIP {
				t.Errorf("want %q, got %q", tc.wantIP, got)
			}
		})
	}
}
