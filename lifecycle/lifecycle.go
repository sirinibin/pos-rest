// Package lifecycle keeps API restarts short and lossless.
//
// Three things make a deploy cost the least downtime:
//
//  1. Listen binds the port at the very start of main, before MongoDB, Redis
//     and the rest of startup. Requests that arrive while the process is still
//     initialising wait in the kernel's accept queue instead of being refused,
//     so nginx sees a slow response rather than a 502.
//  2. Listen reuses a socket handed over by systemd socket activation when one
//     is present. systemd then keeps the port open across the whole restart and
//     no connection is ever refused.
//  3. Serve shuts down gracefully on SIGTERM/SIGINT: it stops accepting, lets
//     in-flight requests (invoice saves, ZATCA submissions, reports) finish,
//     and only then exits.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// DefaultShutdownTimeout is how long in-flight requests get to finish after a
// stop signal. It must stay below systemd's TimeoutStopSec (90 s by default).
const DefaultShutdownTimeout = 20 * time.Second

// sdListenFDsStart is the first file descriptor systemd passes (SD_LISTEN_FDS_START).
const sdListenFDsStart = 3

// ShutdownTimeout reads SHUTDOWN_TIMEOUT (a Go duration such as "20s", or a
// whole number of seconds). Invalid, zero or negative values fall back to the
// default.
func ShutdownTimeout(getenv func(string) string) time.Duration {
	v := getenv("SHUTDOWN_TIMEOUT")
	if v == "" {
		return DefaultShutdownTimeout
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	log.Printf("[lifecycle] invalid SHUTDOWN_TIMEOUT %q, using %s", v, DefaultShutdownTimeout)
	return DefaultShutdownTimeout
}

// activatedFDs returns how many sockets systemd passed to this process, or 0
// when the process was not socket-activated (or the variables belong to a
// parent process).
func activatedFDs(getenv func(string) string, pid int) int {
	if getenv("LISTEN_PID") != strconv.Itoa(pid) {
		return 0
	}
	n, err := strconv.Atoi(getenv("LISTEN_FDS"))
	if err != nil || n < 1 {
		return 0
	}
	return n
}

// portOf returns the TCP port a listener is bound to, or "" if unknown.
func portOf(ln net.Listener) string {
	if a, ok := ln.Addr().(*net.TCPAddr); ok {
		return strconv.Itoa(a.Port)
	}
	return ""
}

var (
	inheritOnce sync.Once
	inheritMu   sync.Mutex
	inheritedLn map[string]net.Listener
)

// Listen returns a listener for port. When systemd passed in a TCP socket for
// the same port it is reused; otherwise a fresh socket is bound.
func Listen(port string) (net.Listener, error) {
	inheritOnce.Do(func() {
		inheritedLn = inheritedListeners(os.Getenv, os.Getpid(), sdListenFDsStart)
	})
	inheritMu.Lock()
	defer inheritMu.Unlock()
	return take(inheritedLn, port)
}

// take hands out the inherited listener for port (each one only once) or
// binds a fresh socket.
func take(inherited map[string]net.Listener, port string) (net.Listener, error) {
	if ln, ok := inherited[port]; ok {
		delete(inherited, port)
		log.Printf("[lifecycle] using systemd-activated socket on :%s", port)
		return ln, nil
	}
	return net.Listen("tcp", ":"+port)
}

// inheritedListeners wraps every socket systemd passed in, keyed by port. It
// must run once: the descriptors are closed after wrapping (the listeners hold
// duplicates, and systemd keeps its own copy open across restarts).
func inheritedListeners(getenv func(string) string, pid int, firstFD int) map[string]net.Listener {
	out := map[string]net.Listener{}
	n := activatedFDs(getenv, pid)
	for i := 0; i < n; i++ {
		f := os.NewFile(uintptr(firstFD+i), fmt.Sprintf("systemd-socket-%d", i))
		if f == nil {
			continue
		}
		ln, err := net.FileListener(f)
		f.Close()
		if err != nil {
			log.Printf("[lifecycle] ignoring inherited fd %d: %v", firstFD+i, err)
			continue
		}
		if p := portOf(ln); p != "" {
			if _, dup := out[p]; !dup {
				out[p] = ln
				continue
			}
		}
		ln.Close()
	}
	return out
}

// maxSettle bounds how long Serve waits for connections that were accepted but
// have not sent their request yet. Shutdown itself gives such connections up
// to 5 s more before treating them as idle.
const maxSettle = 2 * time.Second

// Serve runs srv on ln until ctx is cancelled, then shuts it down gracefully,
// giving in-flight requests up to timeout to finish. It returns nil after a
// clean shutdown and the serve error if the server fails on its own.
//
// net/http's Shutdown closes, without a reply, any connection whose request is
// read after shutdown began. Under socket activation the kernel keeps queueing
// connections for the next process, so a bare Shutdown would drop the few that
// were accepted just before the signal. Serve avoids that by stopping accepts
// first, letting every already-accepted connection deliver its request, and
// only then calling Shutdown.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, timeout time.Duration) error {
	fresh := trackFreshConns(srv)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	deadline := time.Now().Add(timeout)
	shutdownCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	// 1. Stop accepting. Pending connections stay in the kernel queue for the
	//    next process (socket activation) instead of being taken and dropped.
	srv.SetKeepAlivesEnabled(false) // every reply from here on closes its connection
	ln.Close()
	serveErr := <-errc

	// 2. Let accepted connections send their request before Shutdown starts
	//    discarding late ones.
	settle := time.Now().Add(maxSettle)
	if settle.After(deadline) {
		settle = deadline
	}
	for fresh.count() > 0 && time.Now().Before(settle) {
		time.Sleep(5 * time.Millisecond)
	}

	// 3. Wait for in-flight requests to finish.
	err := srv.Shutdown(shutdownCtx)
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) {
		return serveErr
	}
	if err != nil {
		srv.Close() // drop whatever is still running after the timeout
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return nil
}

// freshConns counts connections that are accepted but have not started a
// request yet (http.StateNew).
type freshConns struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

func (f *freshConns) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.conns)
}

func trackFreshConns(srv *http.Server) *freshConns {
	f := &freshConns{conns: map[net.Conn]struct{}{}}
	prev := srv.ConnState
	srv.ConnState = func(c net.Conn, st http.ConnState) {
		f.mu.Lock()
		if st == http.StateNew {
			f.conns[c] = struct{}{}
		} else {
			delete(f.conns, c)
		}
		f.mu.Unlock()
		if prev != nil {
			prev(c, st)
		}
	}
	return f
}

// SignalContext is cancelled on SIGTERM (systemctl stop/restart) or SIGINT.
func SignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
}
