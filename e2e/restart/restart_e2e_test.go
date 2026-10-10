//go:build e2e

// Package restart checks, against the real built server, that a deploy-style
// restart under socket activation drops no requests. The test plays systemd:
// it holds the HTTP and HTTPS sockets, starts pos-rest with them (LISTEN_FDS),
// sends SIGTERM, starts a fresh pos-rest, and keeps clients busy throughout.
//
// Needs the binary built at the repo root (go build -o pos-rest .) and the
// same MongoDB/Redis environment as the other e2e tests.
package restart

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func binary(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("POS_REST_BIN")
	if bin == "" {
		bin = filepath.Join(repoRoot(t), "pos-rest")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("server binary not found at %s (go build -o pos-rest . first): %v", bin, err)
	}
	return bin
}

var held = map[int][]io.Closer{}

// releasePorts closes every copy of the held sockets.
func releasePorts(t *testing.T, port int) {
	for _, c := range held[port] {
		c.Close()
	}
	delete(held, port)
}

// heldPorts binds two consecutive ports (HTTP and HTTPS = HTTP+1), as the
// systemd socket unit does.
func heldPorts(t *testing.T) (int, []*os.File) {
	t.Helper()
	for attempt := 0; attempt < 50; attempt++ {
		a, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		port := a.Addr().(*net.TCPAddr).Port
		b, err := net.Listen("tcp", ":"+strconv.Itoa(port+1))
		if err != nil {
			a.Close()
			continue
		}
		fa, _ := a.(*net.TCPListener).File()
		fb, _ := b.(*net.TCPListener).File()
		held[port] = []io.Closer{fa, fb, a, b}
		t.Cleanup(func() { releasePorts(t, port) })
		return port, []*os.File{fa, fb}
	}
	t.Fatal("no two consecutive free ports")
	return 0, nil
}

type server struct {
	cmd *exec.Cmd
	log string
}

// start launches pos-rest the way systemd does under socket activation.
// sh's $$ is the pid that exec hands to pos-rest, so LISTEN_PID matches.
func start(t *testing.T, port int, socks []*os.File, logName string) *server {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), logName)
	logf, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary(t))
	if len(socks) > 0 {
		cmd = exec.Command("sh", "-c", `LISTEN_PID=$$ LISTEN_FDS=`+strconv.Itoa(len(socks))+` exec "$0"`, binary(t))
	}
	cmd.Dir = repoRoot(t) // TLS cert and static files are read relative to it
	cmd.Env = append(os.Environ(), "API_PORT="+strconv.Itoa(port))
	cmd.ExtraFiles = socks
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s := &server{cmd: cmd, log: logPath}
	t.Cleanup(func() {
		cmd.Process.Kill()
		logf.Close()
		if t.Failed() {
			b, _ := os.ReadFile(logPath)
			t.Logf("--- %s ---\n%s", logName, tail(string(b), 4000))
		}
	})
	return s
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func client() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second, // covers a slow startup: requests queue, not fail
		Transport: &http.Transport{
			DisableKeepAlives: true,
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // localhost self-signed cert
		},
	}
}

// probe expects a real HTTP answer. /v1/me without a token answers 401
// without touching the database.
func probe(c *http.Client, url string) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func waitUp(t *testing.T, url string) {
	t.Helper()
	c := client()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if probe(c, url) == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server never answered %s", url)
}

func stop(t *testing.T, s *server) {
	t.Helper()
	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("pos-rest exited uncleanly after SIGTERM: %v", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("pos-rest did not exit within 60s of SIGTERM (systemd would SIGKILL it at 90s)")
	}
	b, _ := os.ReadFile(s.log)
	if !strings.Contains(string(b), "API stopped cleanly") {
		t.Fatalf("no clean-stop log line:\n%s", tail(string(b), 2000))
	}
}

func TestBinaryAdvertisesSocketActivation(t *testing.T) {
	// deploy/enable_socket_activation.sh greps for this before switching a
	// service over; a binary without it would crash on the held port.
	b, err := os.ReadFile(binary(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "systemd-activated socket") {
		t.Fatal(`binary lacks the "systemd-activated socket" marker the setup script checks for`)
	}
}

func TestSocketActivatedRestartDropsNoRequests(t *testing.T) {
	port, socks := heldPorts(t)
	httpURL := fmt.Sprintf("http://127.0.0.1:%d/v1/me", port)
	httpsURL := fmt.Sprintf("https://127.0.0.1:%d/v1/me", port+1)

	old := start(t, port, socks, "old.log")
	waitUp(t, httpURL)
	waitUp(t, httpsURL)

	var ok, failed atomic.Int64
	var firstErr atomic.Value
	stopLoad := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		url := httpURL
		if i%3 == 2 {
			url = httpsURL
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := client()
			for {
				select {
				case <-stopLoad:
					return
				default:
				}
				if err := probe(c, url); err != nil {
					failed.Add(1)
					firstErr.CompareAndSwap(nil, err.Error())
				} else {
					ok.Add(1)
				}
			}
		}()
	}
	time.Sleep(500 * time.Millisecond)

	// systemctl restart: SIGTERM, wait for exit, start the new build.
	stop(t, old)
	next := start(t, port, socks, "new.log")
	waitUp(t, httpURL)
	time.Sleep(time.Second)
	before := ok.Load()
	time.Sleep(500 * time.Millisecond)
	close(stopLoad)
	wg.Wait()

	if failed.Load() != 0 {
		t.Fatalf("%d of %d requests failed across the restart (first: %v)", failed.Load(), failed.Load()+ok.Load(), firstErr.Load())
	}
	if ok.Load() == before {
		t.Fatal("the new process is not answering")
	}
	t.Logf("%d requests across the restart, none failed", ok.Load())

	if os.Getenv("E2E_RESTART_NO_DB") == "" {
		resp, err := client().Get(fmt.Sprintf("http://127.0.0.1:%d/v1/health", port))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("health after restart: %d", resp.StatusCode)
		}
	}
	stop(t, next)
}

func TestSIGTERMWithoutSocketActivationExitsCleanly(t *testing.T) {
	// The fallback path (plain bind, no systemd socket) must also drain and
	// exit 0, so a restart never needs SIGKILL.
	port, socks := heldPorts(t)
	for _, f := range socks {
		f.Close()
	}
	releasePorts(t, port)
	srv := start(t, port, nil, "plain.log")
	waitUp(t, fmt.Sprintf("http://127.0.0.1:%d/v1/me", port))
	stop(t, srv)
}
