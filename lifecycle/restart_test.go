package lifecycle

// Functional test: real processes, real SIGTERM, real kernel sockets. It plays
// systemd's part (hold the socket, stop the old process, start the new one) and
// checks that a client hammering the API through a restart never sees an error.

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const helperEnv = "LIFECYCLE_TEST_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "1" {
		runHelperServer()
		return
	}
	os.Exit(m.Run())
}

// runHelperServer is a stand-in for main(): bind early, do slow startup work,
// serve, drain on SIGTERM.
func runHelperServer() {
	if os.Getenv("LIFECYCLE_ACTIVATE") == "1" {
		// systemd sets LISTEN_PID to the service's pid; emulate that here.
		os.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))
		os.Setenv("LISTEN_FDS", "1")
	}
	ln, err := Listen(os.Getenv("LIFECYCLE_PORT"))
	if err != nil {
		fmt.Println("listen error:", err)
		os.Exit(2)
	}
	delay, _ := time.ParseDuration(os.Getenv("LIFECYCLE_STARTUP_DELAY"))
	time.Sleep(delay) // MongoDB, Redis, indexes...

	pid := strconv.Itoa(os.Getpid())
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, pid) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(700 * time.Millisecond)
		io.WriteString(w, "done")
	})
	ctx, stop := SignalContext()
	defer stop()
	fmt.Println("ready")
	if err := Serve(ctx, &http.Server{Handler: mux}, ln, 10*time.Second); err != nil {
		fmt.Println("serve error:", err)
		os.Exit(1)
	}
}

func startHelper(t *testing.T, port string, sock *os.File, startupDelay time.Duration) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		helperEnv+"=1",
		"LIFECYCLE_PORT="+port,
		"LIFECYCLE_STARTUP_DELAY="+startupDelay.String(),
	)
	if sock != nil {
		cmd.ExtraFiles = []*os.File{sock} // becomes fd 3, like systemd
		cmd.Env = append(cmd.Env, "LIFECYCLE_ACTIVATE=1")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go io.Copy(io.Discard, bufio.NewReader(out))
	return cmd
}

func get(url string) (string, error) {
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := c.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	return string(b), nil
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out")
}

func TestRestart_SocketActivatedRestartDropsNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes")
	}
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	sock, err := held.(*net.TCPListener).File()
	if err != nil {
		t.Fatal(err)
	}
	defer sock.Close()
	port := portOf(held)
	base := "http://127.0.0.1:" + port

	old := startHelper(t, port, sock, 0)
	defer old.Process.Kill()
	var oldPID string
	waitFor(t, func() bool { oldPID, err = get(base + "/"); return err == nil })

	// Steady client traffic for the whole restart.
	var ok, failed atomic.Int64
	var firstErr atomic.Value
	stopLoad := make(chan struct{})
	var wg sync.WaitGroup
	var lastPID atomic.Value
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopLoad:
					return
				default:
				}
				body, err := get(base + "/")
				if err != nil {
					failed.Add(1)
					firstErr.CompareAndSwap(nil, err.Error())
					continue
				}
				ok.Add(1)
				lastPID.Store(body)
			}
		}()
	}

	// A long request is in flight when the stop signal lands.
	slow := make(chan error, 1)
	go func() {
		body, err := get(base + "/slow")
		if err == nil && body != "done" {
			err = fmt.Errorf("body %q", body)
		}
		slow <- err
	}()
	time.Sleep(200 * time.Millisecond)

	// systemctl restart: SIGTERM, wait for exit, start the new binary.
	if err := old.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := old.Wait(); err != nil {
		t.Fatalf("old process did not exit cleanly: %v", err)
	}
	next := startHelper(t, port, sock, 500*time.Millisecond) // slow startup on purpose
	defer func() { next.Process.Signal(syscall.SIGTERM); next.Wait() }()

	waitFor(t, func() bool { p, _ := lastPID.Load().(string); return p != "" && p != oldPID })
	time.Sleep(200 * time.Millisecond)
	close(stopLoad)
	wg.Wait()

	if err := <-slow; err != nil {
		t.Fatalf("in-flight request was dropped by the restart: %v", err)
	}
	if failed.Load() != 0 {
		t.Fatalf("%d of %d requests failed during the restart (first: %v)", failed.Load(), failed.Load()+ok.Load(), firstErr.Load())
	}
	t.Logf("%d requests across the restart, 0 failed", ok.Load())
}

func TestRestart_SIGTERMDrainsAndExitsZero(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns processes")
	}
	port := freePort(t)
	base := "http://127.0.0.1:" + port
	cmd := startHelper(t, port, nil, 0)
	defer cmd.Process.Kill()
	waitFor(t, func() bool { _, err := get(base + "/"); return err == nil })

	slow := make(chan error, 1)
	go func() { _, err := get(base + "/slow"); slow <- err }()
	time.Sleep(200 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGTERM)

	if err := <-slow; err != nil {
		t.Fatalf("in-flight request cut off by SIGTERM: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("exit after SIGTERM: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("process did not exit after draining")
	}
}
