package lifecycle

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestShutdownTimeout(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want time.Duration
	}{
		{"unset uses default", "", DefaultShutdownTimeout},
		{"duration string", "45s", 45 * time.Second},
		{"milliseconds", "1500ms", 1500 * time.Millisecond},
		{"whole seconds", "30", 30 * time.Second},
		{"zero falls back", "0", DefaultShutdownTimeout},
		{"zero duration falls back", "0s", DefaultShutdownTimeout},
		{"negative falls back", "-5s", DefaultShutdownTimeout},
		{"negative int falls back", "-5", DefaultShutdownTimeout},
		{"garbage falls back", "soon", DefaultShutdownTimeout},
		{"float seconds without unit falls back", "1.5", DefaultShutdownTimeout},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ShutdownTimeout(envMap(map[string]string{"SHUTDOWN_TIMEOUT": c.val}))
			if got != c.want {
				t.Fatalf("ShutdownTimeout(%q) = %s, want %s", c.val, got, c.want)
			}
		})
	}
}

func TestDefaultShutdownTimeoutBelowSystemdStopTimeout(t *testing.T) {
	if DefaultShutdownTimeout >= 90*time.Second {
		t.Fatalf("default %s must be below systemd's 90s TimeoutStopSec or systemd will SIGKILL mid-drain", DefaultShutdownTimeout)
	}
}

func TestActivatedFDs(t *testing.T) {
	const pid = 4242
	cases := []struct {
		name string
		env  map[string]string
		want int
	}{
		{"not activated", map[string]string{}, 0},
		{"one socket for us", map[string]string{"LISTEN_PID": "4242", "LISTEN_FDS": "1"}, 1},
		{"two sockets for us", map[string]string{"LISTEN_PID": "4242", "LISTEN_FDS": "2"}, 2},
		{"sockets meant for another pid", map[string]string{"LISTEN_PID": "1", "LISTEN_FDS": "1"}, 0},
		{"pid without fds", map[string]string{"LISTEN_PID": "4242"}, 0},
		{"zero fds", map[string]string{"LISTEN_PID": "4242", "LISTEN_FDS": "0"}, 0},
		{"negative fds", map[string]string{"LISTEN_PID": "4242", "LISTEN_FDS": "-1"}, 0},
		{"garbage fds", map[string]string{"LISTEN_PID": "4242", "LISTEN_FDS": "x"}, 0},
		{"fds without pid", map[string]string{"LISTEN_FDS": "1"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := activatedFDs(envMap(c.env), pid); got != c.want {
				t.Fatalf("activatedFDs = %d, want %d", got, c.want)
			}
		})
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

func TestListen_BindsFreshSocketWhenNotActivated(t *testing.T) {
	port := freePort(t)
	ln, err := take(inheritedListeners(envMap(nil), 1, 3), port)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if portOf(ln) != port {
		t.Fatalf("bound %s, want %s", portOf(ln), port)
	}
}

func TestListen_PublicEntryPointBindsWhenNotActivated(t *testing.T) {
	port := freePort(t)
	ln, err := Listen(port)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
}

func TestListen_PortInUseIsAnError(t *testing.T) {
	held, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if ln, err := take(nil, portOf(held)); err == nil {
		ln.Close()
		t.Fatal("expected an error binding a port already in use")
	}
}

// heldSocket is the "systemd" side: a listening socket that stays open for
// the whole test, like the one PID 1 holds across service restarts.
func heldSocket(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

// passFD gives a new process-style copy of the held socket's descriptor, as
// systemd does at each service start. inheritedListeners closes it.
func passFD(t *testing.T, ln net.Listener) int {
	t.Helper()
	f, err := ln.(*net.TCPListener).File()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return dup(t, f)
}

// dup returns a raw descriptor nobody else owns, so inheritedListeners can
// close it without racing an *os.File finalizer.
func dup(t *testing.T, f *os.File) int {
	t.Helper()
	fd, err := syscall.Dup(int(f.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	return fd
}

func activated(pid string) func(string) string {
	return envMap(map[string]string{"LISTEN_PID": pid, "LISTEN_FDS": "1"})
}

func TestListen_ReusesActivatedSocketAcrossRestarts(t *testing.T) {
	held := heldSocket(t)
	port := portOf(held)

	// First "process" takes the socket, then exits.
	first, err := take(inheritedListeners(activated("7"), 7, passFD(t, held)), port)
	if err != nil {
		t.Fatalf("listen: %v (a fresh bind would fail because the port is held)", err)
	}
	first.Close()

	// The next "process" still gets a working socket on the same port.
	second, err := take(inheritedListeners(activated("9"), 9, passFD(t, held)), port)
	if err != nil {
		t.Fatalf("socket was closed by the previous process: %v", err)
	}
	defer second.Close()
	go http.Serve(second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	resp, err := http.Get("http://127.0.0.1:" + port + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestListen_EachInheritedSocketHandedOutOnce(t *testing.T) {
	held := heldSocket(t)
	port := portOf(held)
	inherited := inheritedListeners(activated("7"), 7, passFD(t, held))
	ln, err := take(inherited, port)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if again, err := take(inherited, port); err == nil {
		again.Close()
		t.Fatal("second take should fall back to a fresh bind and fail on the held port")
	}
}

func TestListen_IgnoresActivatedSocketForOtherPort(t *testing.T) {
	held := heldSocket(t)
	want := freePort(t)
	inherited := inheritedListeners(activated("7"), 7, passFD(t, held))
	ln, err := take(inherited, want)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if portOf(ln) != want {
		t.Fatalf("got port %s, want fresh bind on %s", portOf(ln), want)
	}
	// The unrelated inherited socket is still there for its own port.
	if _, ok := inherited[portOf(held)]; !ok {
		t.Fatal("inherited socket for the other port was lost")
	}
}

func TestListen_IgnoresActivationMeantForAnotherProcess(t *testing.T) {
	held := heldSocket(t)
	inherited := inheritedListeners(activated("8"), 7, -1)
	if len(inherited) != 0 {
		t.Fatalf("took sockets meant for another pid: %v", inherited)
	}
	if ln, err := take(inherited, portOf(held)); err == nil {
		ln.Close()
		t.Fatal("expected a fresh bind (and address-in-use) when LISTEN_PID is not ours")
	}
}

func TestInheritedListeners_SkipsBadDescriptors(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-socket")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got := inheritedListeners(activated("7"), 7, dup(t, f))
	if len(got) != 0 {
		t.Fatalf("a regular file was accepted as a socket: %v", got)
	}
}

// A request that arrives after the port is bound but before Serve starts must
// wait and succeed, not be refused. This is what keeps startup off the
// downtime clock.
func TestEarlyBind_RequestsQueueDuringStartup(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			done <- result{err: err}
			return
		}
		resp.Body.Close()
		done <- result{code: resp.StatusCode}
	}()

	time.Sleep(300 * time.Millisecond) // "startup": Mongo, Redis, routes...
	select {
	case r := <-done:
		t.Fatalf("request finished before the server started: %+v", r)
	default:
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})}
	go Serve(ctx, srv, ln, time.Second)

	select {
	case r := <-done:
		if r.err != nil || r.code != 200 {
			t.Fatalf("queued request failed: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued request never completed")
	}
}

func TestServe_DrainsInFlightRequestsOnShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var finished atomic.Bool
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(400 * time.Millisecond) // e.g. saving an invoice
		finished.Store(true)
		io.WriteString(w, "saved")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, srv, ln, 5*time.Second) }()

	type result struct {
		body string
		err  error
	}
	resc := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			resc <- result{err: err}
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		resc <- result{body: string(b)}
	}()

	<-started
	cancel() // SIGTERM arrives mid-request

	r := <-resc
	if r.err != nil || r.body != "saved" {
		t.Fatalf("in-flight request was cut off: %+v", r)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve returned %v after a clean drain", err)
	}
	if !finished.Load() {
		t.Fatal("handler did not finish")
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second); err == nil {
		t.Fatal("listener still accepting after shutdown")
	}
}

func TestServe_GivesUpAfterTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	})}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, srv, ln, 150*time.Millisecond) }()
	go http.Get("http://" + ln.Addr().String() + "/")
	<-started

	begin := time.Now()
	cancel()
	select {
	case err := <-served:
		if err == nil {
			t.Fatal("expected a timeout error when a request outlives the drain window")
		}
		if d := time.Since(begin); d > 2*time.Second {
			t.Fatalf("shutdown took %s, should stop near the 150ms timeout", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve hung past its shutdown timeout")
	}
}

func TestServe_ReturnsServeErrorWithoutSignal(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // the listener dies under the server
	err = Serve(context.Background(), &http.Server{}, ln, time.Second)
	if err == nil {
		t.Fatal("expected the serve error to be returned")
	}
}

func TestServe_IdleServerStopsImmediately(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, &http.Server{}, ln, 10*time.Second) }()
	time.Sleep(50 * time.Millisecond)
	begin := time.Now()
	cancel()
	if err := <-served; err != nil {
		t.Fatal(err)
	}
	if d := time.Since(begin); d > time.Second {
		t.Fatalf("idle shutdown took %s; an idle server should not wait for the timeout", d)
	}
}

// A connection that is accepted just before the stop signal but whose request
// bytes arrive just after it must still be answered.
func TestServe_AnswersConnectionAcceptedJustBeforeStop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan struct{}, 1)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }),
		ConnState: func(c net.Conn, st http.ConnState) {
			if st == http.StateNew {
				accepted <- struct{}{}
			}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, srv, ln, 5*time.Second) }()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	<-accepted
	cancel()                          // stop signal lands...
	time.Sleep(50 * time.Millisecond) // ...before the request bytes do
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: x\r\n\r\n")
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	b, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasPrefix(string(b), "HTTP/1.1 200") || !strings.HasSuffix(string(b), "ok") {
		t.Fatalf("connection accepted before the stop was dropped; got %q", b)
	}
	if !strings.Contains(string(b), "Connection: close") {
		t.Fatalf("reply during shutdown should close the connection; got %q", b)
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
}

// A user-supplied ConnState hook keeps working alongside the tracker.
func TestServe_PreservesConnStateHook(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var seen atomic.Int64
	srv := &http.Server{
		Handler:   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		ConnState: func(net.Conn, http.ConnState) { seen.Add(1) },
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, srv, ln, time.Second) }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cancel()
	<-served
	if seen.Load() == 0 {
		t.Fatal("original ConnState hook was not called")
	}
}
