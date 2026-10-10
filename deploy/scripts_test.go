package deploy

// Functional tests for the server-side deploy scripts. They run the real bash
// scripts against a fake systemctl and a local health endpoint, so every
// branch (swap, health check, rollback, one-time socket setup) is exercised
// without a server.

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const fakeSystemctl = `#!/usr/bin/env bash
# Minimal systemctl stand-in. Every call is logged; "start"/"restart" make the
# service "run" whatever $FAKE_BIN currently contains.
echo "$*" >> "$FAKE_STATE/calls"
args=("$@")
[ "${args[0]}" = "is-active" ] && [ "${args[1]}" = "--quiet" ] && unset 'args[1]' && args=("${args[@]}")
case "${args[0]}" in
  cat)       [ "$FAKE_SERVICE_EXISTS" = 1 ] ;;
  show)      echo "{ path=$FAKE_BIN ; argv[]=$FAKE_BIN ; ignore_errors=no }" ;;
  is-active) case "${args[1]}" in
               *.socket) [ -e "$FAKE_STATE/socket_active" ] ;;
               *)        [ -e "$FAKE_STATE/running" ] ;;
             esac ;;
  enable)    [ "$FAKE_SOCKET_FAIL" = 1 ] && exit 1; touch "$FAKE_STATE/socket_active" ;;
  disable)   rm -f "$FAKE_STATE/socket_active" ;;
  stop)      rm -f "$FAKE_STATE/running" ;;
  start|restart) cp "$FAKE_BIN" "$FAKE_STATE/running" ;;
  *) ;;
esac
`

type env struct {
	t      *testing.T
	dir    string // deploy destination
	state  string
	health string
	vars   map[string]string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not installed")
	}
	e := &env{t: t, dir: t.TempDir(), state: t.TempDir()}
	sysctl := filepath.Join(t.TempDir(), "systemctl")
	if err := os.WriteFile(sysctl, []byte(fakeSystemctl), 0o755); err != nil {
		t.Fatal(err)
	}

	// Health endpoint: 200 only when the "running" binary is a good build.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(filepath.Join(e.state, "running"))
		if err != nil || !strings.Contains(string(b), "good") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	e.health = "http://" + ln.Addr().String() + "/v1/health"

	e.vars = map[string]string{
		"SYSTEMCTL":           sysctl,
		"SUDO":                "",
		"FAKE_STATE":          e.state,
		"FAKE_BIN":            filepath.Join(e.dir, "pos-rest"),
		"FAKE_SERVICE_EXISTS": "1",
		"HEALTH_URL":          e.health,
		"HEALTH_TIMEOUT":      "2",
		"HEALTH_INTERVAL":     "0.1",
		"UNIT_DIR":            t.TempDir(),
	}
	return e
}

func (e *env) write(name, content string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.dir, name), []byte(content), 0o755); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) read(name string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(e.dir, name))
	return string(b), err == nil
}

func (e *env) calls() string {
	b, _ := os.ReadFile(filepath.Join(e.state, "calls"))
	return string(b)
}

func (e *env) run(script string, args ...string) (int, string) {
	e.t.Helper()
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = os.Environ()
	for k, v := range e.vars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), string(out)
	} else if err != nil {
		e.t.Fatal(err)
	}
	return 0, string(out)
}

func (e *env) restart() (int, string) {
	return e.run("remote_restart.sh", e.dir, "start-api-test", "2002")
}

// ── remote_restart.sh ────────────────────────────────────────────────────────

func TestRemoteRestart_GoodBuild(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", "old good build")
	e.write("pos-rest.new", "new good build")

	code, out := e.restart()
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if got, _ := e.read("pos-rest"); got != "new good build" {
		t.Fatalf("binary = %q, want the new build", got)
	}
	if _, ok := e.read("pos-rest.new"); ok {
		t.Fatal("pos-rest.new left behind")
	}
	if _, ok := e.read("pos-rest.prev"); ok {
		t.Fatal("pos-rest.prev kept after a healthy deploy (wastes disk)")
	}
	calls := e.calls()
	if strings.Count(calls, "restart start-api-test") != 1 {
		t.Fatalf("want exactly one restart, calls:\n%s", calls)
	}
	if strings.Contains(calls, "stop") || strings.Contains(calls, "kill") {
		t.Fatalf("a graceful restart must not stop or kill the service separately, calls:\n%s", calls)
	}
	if !strings.Contains(out, "Healthy on") {
		t.Fatalf("missing health confirmation:\n%s", out)
	}
	if st, _ := os.Stat(filepath.Join(e.dir, "pos-rest")); st.Mode()&0o111 == 0 {
		t.Fatal("deployed binary is not executable")
	}
}

func TestRemoteRestart_NewBinaryMadeExecutable(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", "old good build")
	if err := os.WriteFile(filepath.Join(e.dir, "pos-rest.new"), []byte("new good build"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := e.restart(); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if st, _ := os.Stat(filepath.Join(e.dir, "pos-rest")); st.Mode()&0o111 == 0 {
		t.Fatal("scp'd binary without +x was deployed non-executable")
	}
}

func TestRemoteRestart_BadBuildRollsBack(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", "old good build")
	e.write("pos-rest.new", "new bad build")

	code, out := e.restart()
	if code == 0 {
		t.Fatalf("a deploy that never became healthy must fail\n%s", out)
	}
	if got, _ := e.read("pos-rest"); got != "old good build" {
		t.Fatalf("binary = %q, want rollback to the old build", got)
	}
	if strings.Count(e.calls(), "restart start-api-test") != 2 {
		t.Fatalf("want restart + rollback restart, calls:\n%s", e.calls())
	}
	if !strings.Contains(out, "Rolled back; the previous build is serving again") {
		t.Fatalf("missing rollback confirmation:\n%s", out)
	}
	running, _ := os.ReadFile(filepath.Join(e.state, "running"))
	if string(running) != "old good build" {
		t.Fatalf("service is running %q after rollback", running)
	}
}

func TestRemoteRestart_RollbackAlsoUnhealthy(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", "old bad build") // e.g. MongoDB itself is down
	e.write("pos-rest.new", "new bad build")
	code, out := e.restart()
	if code == 0 || !strings.Contains(out, "Manual intervention required") {
		t.Fatalf("exit %d, want failure asking for manual help\n%s", code, out)
	}
}

func TestRemoteRestart_FirstDeployNoPrevious(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest.new", "new good build")
	if code, out := e.restart(); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}

	e2 := newEnv(t)
	e2.write("pos-rest.new", "new bad build")
	code, out := e2.restart()
	if code == 0 || !strings.Contains(out, "No previous binary") {
		t.Fatalf("exit %d, want failure without rollback\n%s", code, out)
	}
}

func TestRemoteRestart_MissingUpload(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", "old good build")
	code, out := e.restart()
	if code == 0 || !strings.Contains(out, "missing or empty") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if e.calls() != "" {
		t.Fatalf("service touched without an upload: %s", e.calls())
	}
	if got, _ := e.read("pos-rest"); got != "old good build" {
		t.Fatal("live binary changed")
	}
}

func TestRemoteRestart_EmptyUpload(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", "old good build")
	e.write("pos-rest.new", "")
	if code, _ := e.restart(); code == 0 {
		t.Fatal("an empty (truncated) upload must not be deployed")
	}
	if e.calls() != "" {
		t.Fatalf("service touched: %s", e.calls())
	}
}

func TestRemoteRestart_BadArguments(t *testing.T) {
	e := newEnv(t)
	cases := [][]string{
		{},
		{e.dir},
		{e.dir, "start-api-test"},
		{e.dir, "start-api-test", "20o2"},
		{e.dir, "start-api-test", ""},
		{e.dir, "start-api-test", "2002", "extra"},
	}
	for _, args := range cases {
		if code, out := e.run("remote_restart.sh", args...); code != 2 {
			t.Errorf("args %q: exit %d, want 2 (usage)\n%s", args, code, out)
		}
	}
}

// ── enable_socket_activation.sh ──────────────────────────────────────────────

func (e *env) enable() (int, string) {
	return e.run("enable_socket_activation.sh", "start-api-test", "2002")
}

func (e *env) unit() (string, bool) {
	b, err := os.ReadFile(filepath.Join(e.vars["UNIT_DIR"], "start-api-test.socket"))
	return string(b), err == nil
}

const supportedBuild = "good build [lifecycle] using systemd-activated socket on :%s"

func TestEnableSocket_WritesUnitAndHandsOverPorts(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", supportedBuild)
	code, out := e.enable()
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	unit, ok := e.unit()
	if !ok {
		t.Fatal("socket unit not written")
	}
	for _, want := range []string{"ListenStream=2002\n", "ListenStream=2003\n", "WantedBy=sockets.target"} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q:\n%s", want, unit)
		}
	}
	calls := e.calls()
	order := []string{"daemon-reload", "stop start-api-test", "enable --now start-api-test.socket", "start start-api-test"}
	pos := 0
	for _, c := range order {
		i := strings.Index(calls[pos:], c)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", c, calls)
		}
		pos += i + len(c)
	}
}

func TestEnableSocket_Idempotent(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", supportedBuild)
	if code, out := e.enable(); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	os.Remove(filepath.Join(e.state, "calls"))
	code, out := e.enable()
	if code != 0 || !strings.Contains(out, "already enabled") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if strings.Contains(e.calls(), "stop") {
		t.Fatalf("second run restarted the API needlessly: %s", e.calls())
	}
}

func TestEnableSocket_RefusesOldBinary(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", "good build from before this change")
	code, out := e.enable()
	if code == 0 || !strings.Contains(out, "does not support socket activation") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if _, ok := e.unit(); ok {
		t.Fatal("unit written for a binary that would crash with it")
	}
	if strings.Contains(e.calls(), "stop") {
		t.Fatal("service stopped")
	}
}

func TestEnableSocket_MissingService(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", supportedBuild)
	e.vars["FAKE_SERVICE_EXISTS"] = "0"
	if code, out := e.enable(); code == 0 || !strings.Contains(out, "does not exist") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestEnableSocket_UnhealthyRestoresOldSetup(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", "bad build [lifecycle] using systemd-activated socket on :%s")
	code, out := e.enable()
	if code == 0 || !strings.Contains(out, "restoring the old setup") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if _, ok := e.unit(); ok {
		t.Fatal("socket unit left in place after a failed setup")
	}
	calls := e.calls()
	if !strings.Contains(calls, "disable --now start-api-test.socket") || !strings.HasSuffix(strings.TrimSpace(calls), "start start-api-test") {
		t.Fatalf("service not restarted without the socket, calls:\n%s", calls)
	}
}

func TestEnableSocket_SocketStartFailureRestores(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", supportedBuild)
	e.vars["FAKE_SOCKET_FAIL"] = "1"
	code, out := e.enable()
	if code == 0 {
		t.Fatalf("exit 0\n%s", out)
	}
	if _, ok := e.unit(); ok {
		t.Fatal("socket unit left behind")
	}
	if !strings.HasSuffix(strings.TrimSpace(e.calls()), "start start-api-test") {
		t.Fatalf("service left stopped, calls:\n%s", e.calls())
	}
}

func TestEnableSocket_BadArguments(t *testing.T) {
	e := newEnv(t)
	for _, args := range [][]string{{}, {"start-api"}, {"start-api", "x"}, {"start-api", "2000", "x"}} {
		if code, _ := e.run("enable_socket_activation.sh", args...); code != 2 {
			t.Errorf("args %q: exit %d, want 2", args, code)
		}
	}
}

func TestEnableSocket_HTTPSPortIsNextPort(t *testing.T) {
	e := newEnv(t)
	e.write("pos-rest", supportedBuild)
	if code, out := e.run("enable_socket_activation.sh", "start-api", "2000"); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	b, _ := os.ReadFile(filepath.Join(e.vars["UNIT_DIR"], "start-api.socket"))
	if !strings.Contains(string(b), "ListenStream=2000\n") || !strings.Contains(string(b), "ListenStream="+strconv.Itoa(2001)+"\n") {
		t.Fatalf("unit:\n%s", b)
	}
}

// The workflows run the script as `ssh host "bash -s -- ..." < remote_restart.sh`.
// A command that reads stdin (sudo, systemctl) must not eat the rest of it.
func TestRemoteRestart_SafeWhenPipedToBashS(t *testing.T) {
	e := newEnv(t)
	greedy := filepath.Join(t.TempDir(), "systemctl")
	os.WriteFile(greedy, []byte("#!/usr/bin/env bash\ncat >/dev/null\nexec "+e.vars["SYSTEMCTL"]+" \"$@\"\n"), 0o755)
	e.vars["SYSTEMCTL"] = greedy
	e.write("pos-rest", "old good build")
	e.write("pos-rest.new", "new good build")

	script, err := os.Open("remote_restart.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer script.Close()
	cmd := exec.Command("bash", "-s", "--", e.dir, "start-api-test", "2002")
	cmd.Stdin = script
	cmd.Env = os.Environ()
	for k, v := range e.vars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Healthy on") {
		t.Fatalf("script was cut short:\n%s", out)
	}
	if got, _ := e.read("pos-rest"); got != "new good build" {
		t.Fatalf("binary = %q", got)
	}
}

// ── every deploy path uses the graceful restart ──────────────────────────────

func TestDeployPathsUseGracefulRestart(t *testing.T) {
	paths := map[string][]string{
		"../.github/workflows/deploy_test.yml":  {"start-api-test 2002"},
		"../.github/workflows/deploy_prod.yml":  {"start-api 2000"},
		"../.github/workflows/deploy_quick.yml": {"service=start-api\"", "port=2000", "service=start-api-test\"", "port=2002"},
		"../deploy.sh":                          {`"TEST" 2002`, `"PRODUCTION" 2000`},
		"../deploy_quick.sh":                    {"PORT=2000", "PORT=2002"},
	}
	for path, wants := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, "< deploy/remote_restart.sh") {
			t.Errorf("%s does not deploy through deploy/remote_restart.sh", path)
		}
		// fuser -k on the live binary SIGKILLs the running API mid-request,
		// and stop+start leaves the port closed for the whole startup.
		for _, banned := range []string{"fuser -k", "systemctl stop $SERVICE", "systemctl stop start-api", "systemctl stop $service"} {
			if strings.Contains(s, banned) {
				t.Errorf("%s still contains %q", path, banned)
			}
		}
		for _, w := range wants {
			if !strings.Contains(s, w) {
				t.Errorf("%s: missing %q (service/port mapping)", path, w)
			}
		}
	}
}

func TestDeployWorkflowsRedeployOnScriptChange(t *testing.T) {
	for _, path := range []string{"../.github/workflows/deploy_test.yml", "../.github/workflows/deploy_prod.yml"} {
		b, _ := os.ReadFile(path)
		if !strings.Contains(string(b), `- "deploy/**"`) {
			t.Errorf("%s does not trigger on deploy/** changes", path)
		}
	}
}
