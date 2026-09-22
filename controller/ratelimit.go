package controller

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

type ipEntry struct {
	count       int
	windowStart time.Time
}

// RateLimiter counts requests per IP within a sliding window.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*ipEntry
	limit   int
	window  time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*ipEntry),
		limit:   limit,
		window:  window,
	}
	go rl.cleanup()
	return rl
}

// cleanup removes expired entries every 5 minutes to prevent unbounded growth.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, e := range rl.entries {
			if now.Sub(e.windowStart) > rl.window {
				delete(rl.entries, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow returns true if the IP is within the request budget.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	e, ok := rl.entries[ip]
	if !ok || now.Sub(e.windowStart) > rl.window {
		rl.entries[ip] = &ipEntry{count: 1, windowStart: now}
		return true
	}
	if e.count >= rl.limit {
		return false
	}
	e.count++
	return true
}

// Middleware wraps a handler and rejects over-limit IPs with HTTP 429.
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := realClientIP(r)
		if !rl.Allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "900")
			w.WriteHeader(http.StatusTooManyRequests)
			var resp models.Response
			resp.Status = false
			resp.Errors = map[string]string{
				"rate_limit": "Too many requests. Please try again later.",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		next(w, r)
	}
}

// realClientIP reads the IP nginx set in X-Real-IP (trusted proxy header).
// Falls back to RemoteAddr if the header is absent.
func realClientIP(r *http.Request) string {
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// X-Forwarded-For: take the leftmost entry (original client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Package-level rate limiters shared across requests.
var (
	// AuthRateLimiter: 10 login attempts per IP per 15 minutes.
	AuthRateLimiter = NewRateLimiter(10, 15*time.Minute)

	// RegisterRateLimiter: 5 registration attempts per IP per hour.
	RegisterRateLimiter = NewRateLimiter(5, 60*time.Minute)
)
