package controller

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

// statusTracker remembers whether a handler has already written a status, so
// ensureStatus can set one without a second WriteHeader. Validation helpers
// set the status on some paths and not others, and many handlers then
// answered validation errors with HTTP 200 and "status": false.
type statusTracker struct {
	http.ResponseWriter
	wrote bool
}

func (t *statusTracker) WriteHeader(code int) {
	t.wrote = true
	t.ResponseWriter.WriteHeader(code)
}

func (t *statusTracker) Write(b []byte) (int, error) {
	t.wrote = true
	return t.ResponseWriter.Write(b)
}

func (t *statusTracker) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok {
		t.wrote = true
		f.Flush()
	}
}

func (t *statusTracker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := t.ResponseWriter.(http.Hijacker); ok {
		t.wrote = true
		return h.Hijack()
	}
	return nil, nil, errors.New("hijacking not supported")
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (t *statusTracker) Unwrap() http.ResponseWriter { return t.ResponseWriter }

// StatusTrackerMiddleware wraps every response in a statusTracker.
func StatusTrackerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&statusTracker{ResponseWriter: w}, r)
	})
}

// ensureStatus writes code unless the handler already chose a status.
func ensureStatus(w http.ResponseWriter, code int) {
	if t, ok := w.(*statusTracker); ok && t.wrote {
		return
	}
	w.WriteHeader(code)
}
