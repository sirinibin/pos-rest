package main

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
)

// fakeStatus replaces the real checks; set() changes what the next check sees.
type fakeStatus struct {
	mu     sync.Mutex
	api    string
	checks int32
}

func (f *fakeStatus) set(api string) { f.mu.Lock(); f.api = api; f.mu.Unlock() }

func (f *fakeStatus) compute() map[string]ServerStatus {
	atomic.AddInt32(&f.checks, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return map[string]ServerStatus{
		"production": {Name: "Production", Overall: "running", API: ComponentStatus{true, f.api}, UpdatedAt: time.Now()},
		"test":       {Name: "Test", Overall: "running", UpdatedAt: time.Now()},
	}
}

func useFakeStatus(t *testing.T) *fakeStatus {
	t.Helper()
	f := &fakeStatus{api: "Running"}
	oldCompute, oldEvery, oldPing, oldHub := computeStatus, statusCheckEvery, statusStreamPing, statusHubInstance
	computeStatus = f.compute
	statusCheckEvery = 20 * time.Millisecond
	statusStreamPing = time.Hour
	statusHubInstance = newStatusHub()
	t.Cleanup(func() {
		computeStatus, statusCheckEvery, statusStreamPing, statusHubInstance = oldCompute, oldEvery, oldPing, oldHub
	})
	return f
}

func signedToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	claims["type"] = "access_token"
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// openStream starts the handler and returns a channel of the "data:" lines it writes.
func openStream(t *testing.T, srv *httptest.Server, token string) (<-chan string, *http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/health-monitor/status/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	events := make(chan string, 16)
	go func() {
		defer close(events)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data: ") {
				events <- strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	t.Cleanup(func() { cancel(); resp.Body.Close() })
	return events, resp, cancel
}

func nextEvent(t *testing.T, events <-chan string) string {
	t.Helper()
	select {
	case e, ok := <-events:
		if !ok {
			t.Fatal("stream closed")
		}
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("no status pushed")
	}
	return ""
}

func noEvent(t *testing.T, events <-chan string, wait time.Duration) {
	t.Helper()
	select {
	case e := <-events:
		t.Fatalf("unexpected push: %s", e)
	case <-time.After(wait):
	}
}

// streamServer closes after the streams opened later in the test (cleanups run
// last-registered first); closing it first would wait on those open streams.
func streamServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health-monitor/status/stream", statusStreamHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestStatusStream_PushesOnOpenAndOnlyOnChange(t *testing.T) {
	f := useFakeStatus(t)
	srv := streamServer(t)

	events, resp, _ := openStream(t, srv, signedToken(t, jwt.MapClaims{"admin": true, "exp": time.Now().Add(time.Hour).Unix()}))
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal("nginx buffering not turned off")
	}
	if first := nextEvent(t, events); !strings.Contains(first, `"Running"`) {
		t.Fatalf("first status %s", first)
	}

	// Many checks with nothing changed (only the check time moves): no pushes.
	noEvent(t, events, 150*time.Millisecond)
	if atomic.LoadInt32(&f.checks) < 3 {
		t.Fatalf("checker ran only %d times", f.checks)
	}

	f.set("Service failed")
	if e := nextEvent(t, events); !strings.Contains(e, `"Service failed"`) {
		t.Fatalf("change not pushed: %s", e)
	}
	noEvent(t, events, 100*time.Millisecond)
}

func TestStatusStream_SecondWindowGetsCurrentStatusAndCheckerIsShared(t *testing.T) {
	f := useFakeStatus(t)
	srv := streamServer(t)
	tok := signedToken(t, jwt.MapClaims{"admin": true})

	a, _, _ := openStream(t, srv, tok)
	nextEvent(t, a)
	b, _, _ := openStream(t, srv, tok)
	if e := nextEvent(t, b); !strings.Contains(e, `"Running"`) {
		t.Fatalf("second window got %s", e)
	}

	// Two windows, one checker: checks per tick stay at one.
	before := atomic.LoadInt32(&f.checks)
	time.Sleep(200 * time.Millisecond)
	if n := atomic.LoadInt32(&f.checks) - before; n > 13 {
		t.Fatalf("%d checks in 200ms at 20ms: more than one checker is running", n)
	}

	f.set("Starting up")
	if !strings.Contains(nextEvent(t, a), "Starting up") || !strings.Contains(nextEvent(t, b), "Starting up") {
		t.Fatal("both windows must get the change")
	}
}

func TestStatusStream_CheckerStopsWhenLastWindowCloses(t *testing.T) {
	f := useFakeStatus(t)
	srv := streamServer(t)

	events, _, cancel := openStream(t, srv, signedToken(t, jwt.MapClaims{"admin": true}))
	nextEvent(t, events)
	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for {
		statusHubInstance.mu.Lock()
		running := statusHubInstance.running
		statusHubInstance.mu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("checker still running with no window open")
		}
		time.Sleep(10 * time.Millisecond)
	}
	before := atomic.LoadInt32(&f.checks)
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&f.checks) != before {
		t.Fatal("status checked with no window open")
	}
}

func TestStatusStream_RecheckRunsAtOnce(t *testing.T) {
	f := useFakeStatus(t)
	statusCheckEvery = time.Hour // only a recheck can notice the change
	srv := streamServer(t)

	events, _, _ := openStream(t, srv, signedToken(t, jwt.MapClaims{"admin": true}))
	nextEvent(t, events)
	f.set("Running again")
	statusHubInstance.recheck()
	if e := nextEvent(t, events); !strings.Contains(e, "Running again") {
		t.Fatalf("recheck not pushed: %s", e)
	}
}

func TestStatusStream_EndsWhenTokenExpires(t *testing.T) {
	useFakeStatus(t)
	srv := streamServer(t)

	events, _, _ := openStream(t, srv, signedToken(t, jwt.MapClaims{"admin": true, "exp": time.Now().Add(1500 * time.Millisecond).Unix()}))
	nextEvent(t, events)
	select {
	case _, ok := <-events:
		for ok {
			_, ok = <-events
		}
	case <-time.After(4 * time.Second):
		t.Fatal("stream still open after the token expired")
	}
}

func TestStatusStream_Access(t *testing.T) {
	useFakeStatus(t)
	arMu.Lock()
	oldAllowed := arCfg.AllowedUsers
	arCfg.AllowedUsers = []string{"allowed-user"}
	arMu.Unlock()
	t.Cleanup(func() { arMu.Lock(); arCfg.AllowedUsers = oldAllowed; arMu.Unlock() })

	srv := streamServer(t)

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"bad token", "Bearer nonsense", http.StatusUnauthorized},
		{"refresh token", "Bearer " + func() string {
			s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"type": "refresh_token", "admin": true}).SignedString(jwtSecret())
			return s
		}(), http.StatusUnauthorized},
		{"non-admin not allowed", "Bearer " + signedToken(t, jwt.MapClaims{"admin": false, "user_id": "someone"}), http.StatusForbidden},
		{"non-admin allowed", "Bearer " + signedToken(t, jwt.MapClaims{"admin": false, "user_id": "allowed-user"}), http.StatusOK},
		{"admin", "Bearer " + signedToken(t, jwt.MapClaims{"admin": true}), http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/health-monitor/status/stream", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != c.want {
				t.Fatalf("status %d, want %d", resp.StatusCode, c.want)
			}
		})
	}
}

func TestStatusChangeKey_IgnoresCheckTime(t *testing.T) {
	a := map[string]ServerStatus{"production": {Overall: "running", UpdatedAt: time.Now()}}
	b := map[string]ServerStatus{"production": {Overall: "running", UpdatedAt: time.Now().Add(time.Minute)}}
	c := map[string]ServerStatus{"production": {Overall: "down", UpdatedAt: time.Now()}}
	if string(statusChangeKey(a)) != string(statusChangeKey(b)) {
		t.Fatal("check time alone must not count as a change")
	}
	if string(statusChangeKey(a)) == string(statusChangeKey(c)) {
		t.Fatal("a status change must count")
	}
}
