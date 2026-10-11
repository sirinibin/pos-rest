package main

// Server status pushed to the browser instead of fetched on a timer.
//
// GET /health-monitor/status/stream keeps one response open and writes the
// status of both environments as a server-sent event ("event: status") when
// the window opens and again only when something changes. The browser reads it
// with fetch (so the token stays in the Authorization header, never the URL).
//
// Nothing pushes systemd, Redis or MongoDB state to us, so one shared checker
// looks at them every statusCheckEvery while at least one window is open, and
// not at all otherwise; before this every open window fetched the full status
// every 5 seconds. A restart from this service re-checks at once.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
)

var (
	statusCheckEvery    = 3 * time.Second
	statusStreamPing    = 20 * time.Second // keeps nginx (proxy_read_timeout 60s) from closing an idle stream
	computeStatus       = currentStatus
	statusHubInstance   = newStatusHub()
	statusStreamMaxLife = 12 * time.Hour // when the token has no expiry
)

func currentStatus() map[string]ServerStatus {
	return map[string]ServerStatus{
		"production": buildStatus("Production", "start-api.service", 2000, frontendBuildDir("production")),
		"test":       buildStatus("Test", "start-api-test.service", 2002, frontendBuildDir("test")),
	}
}

type statusHub struct {
	mu      sync.Mutex
	subs    map[chan []byte]struct{}
	last    []byte // last status sent, as JSON
	lastKey []byte // the same without the check times, to tell a real change
	running bool
	kick    chan struct{}
}

func newStatusHub() *statusHub {
	return &statusHub{subs: map[chan []byte]struct{}{}, kick: make(chan struct{}, 1)}
}

// subscribe returns a channel that gets the current status (at once when one
// is known) and every change after it. The checker runs while anyone listens.
func (h *statusHub) subscribe() chan []byte {
	ch := make(chan []byte, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	if h.last != nil {
		ch <- h.last
	}
	if !h.running {
		h.running = true
		go h.run()
	}
	h.mu.Unlock()
	return ch
}

func (h *statusHub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

// recheck asks the checker to look now instead of at its next tick.
func (h *statusHub) recheck() {
	select {
	case h.kick <- struct{}{}:
	default:
	}
}

func (h *statusHub) run() {
	ticker := time.NewTicker(statusCheckEvery)
	defer ticker.Stop()
	for {
		h.check()
		select {
		case <-ticker.C:
		case <-h.kick:
		}
		h.mu.Lock()
		if len(h.subs) == 0 {
			// Nobody is watching: stop checking, and forget the last status
			// so the next window gets a fresh one rather than an old copy.
			h.running = false
			h.last, h.lastKey = nil, nil
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()
	}
}

func (h *statusHub) check() {
	st := computeStatus()
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	key := statusChangeKey(st)

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.last != nil && bytes.Equal(key, h.lastKey) {
		return
	}
	h.last, h.lastKey = data, key
	for ch := range h.subs {
		// Each window needs only the newest status: replace one it hasn't
		// read yet rather than block the checker on a slow connection.
		select {
		case <-ch:
		default:
		}
		ch <- data
	}
}

// statusChangeKey is the status without its check times.
func statusChangeKey(st map[string]ServerStatus) []byte {
	stripped := make(map[string]ServerStatus, len(st))
	for k, v := range st {
		v.UpdatedAt = time.Time{}
		stripped[k] = v
	}
	b, _ := json.Marshal(stripped)
	return b
}

// statusStreamHandler — GET /health-monitor/status/stream
func statusStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		cors(w)
		return
	}
	cors(w)
	claims := viewAccessClaims(w, r)
	if claims == nil {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming unsupported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: pass each event on as it is written
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// The stream ends when the token does; the browser reconnects with its
	// current token, which is checked (and AllowedUsers with it) again.
	life := statusStreamMaxLife
	if exp, ok := claims["exp"].(float64); ok {
		life = time.Until(time.Unix(int64(exp), 0))
	}
	expired := time.NewTimer(life)
	defer expired.Stop()
	ping := time.NewTicker(statusStreamPing)
	defer ping.Stop()

	ch := statusHubInstance.subscribe()
	defer statusHubInstance.unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-expired.C:
			return
		case data := <-ch:
			if _, err := fmt.Fprintf(w, "event: status\ndata: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// viewAccessClaims is requireViewAccess returning the token's claims (nil
// after writing the error response).
func viewAccessClaims(w http.ResponseWriter, r *http.Request) jwt.MapClaims {
	claims := parseJWTClaims(w, r)
	if claims == nil {
		return nil
	}
	if !claimsMayView(claims) {
		http.Error(w, `{"error":"access denied"}`, http.StatusForbidden)
		return nil
	}
	return claims
}
