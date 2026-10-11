// Package deploysync checks that deploy_v2.sh runs the same gates as
// .github/workflows/deploy_v2.yml, so a deploy never depends on GitHub
// Actions having runners, and that a failing gate stops the deploy.
package deploysync

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type step struct{ name, mode, fn string }

func steps(script string) []step {
	var out []step
	for _, m := range regexp.MustCompile(`(?m)^run_step\s+(\S+)\s+(gate|report)\s+(\S+)`).FindAllStringSubmatch(script, -1) {
		out = append(out, step{m[1], m[2], m[3]})
	}
	return out
}

func index(ss []step, name string) int {
	for i, s := range ss {
		if s.name == name {
			return i
		}
	}
	return -1
}

// body of a shell function in the script
func fn(script, name string) string {
	m := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(name) + `\(\) \{(.*?)^\}`).FindStringSubmatch(script)
	if m == nil {
		return ""
	}
	return m[1]
}

// a job's block in the workflow: from "  name:" to the next two-space key
func job(wf, name string) string {
	locs := regexp.MustCompile(`(?m)^  [\w-]+:$`).FindAllStringIndex(wf, -1)
	for i, l := range locs {
		if wf[l[0]:l[1]] != "  "+name+":" {
			continue
		}
		end := len(wf)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return wf[l[0]:end]
	}
	return ""
}

func TestDeployScriptIsValidBash(t *testing.T) {
	if out, err := exec.Command("bash", "-n", "../../deploy_v2.sh").CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}

func TestDeployScriptGatesOnEveryJobTheDeployNeeds(t *testing.T) {
	script, wf := read(t, "deploy_v2.sh"), read(t, ".github/workflows/deploy_v2.yml")
	m := regexp.MustCompile(`build-and-deploy:\s*\n\s*needs:\s*\[([^\]]+)\]`).FindStringSubmatch(wf)
	if m == nil {
		t.Fatal("no needs on build-and-deploy")
	}
	ss := steps(script)
	for _, j := range strings.Split(m[1], ",") {
		j = strings.TrimSpace(j)
		i := index(ss, j)
		if i < 0 {
			t.Errorf("deploy_v2.sh has no step for CI job %s", j)
			continue
		}
		if ss[i].mode != "gate" {
			t.Errorf("%s must gate the deploy", j)
		}
		if i > index(ss, "deploy") {
			t.Errorf("%s runs after the deploy", j)
		}
	}
}

func TestReportOnlyJobsStayReportOnly(t *testing.T) {
	script, wf := read(t, "deploy_v2.sh"), read(t, ".github/workflows/deploy_v2.yml")
	ss := steps(script)
	for ci, local := range map[string]string{"zatca-sandbox": "zatca"} {
		if !strings.Contains(job(wf, ci), "continue-on-error: true") {
			t.Errorf("CI job %s is no longer report-only; update deploy_v2.sh", ci)
		}
		if i := index(ss, local); i < 0 || ss[i].mode != "report" {
			t.Errorf("step %s must exist and be report-only", local)
		}
	}
	if index(ss, "cleanup") < index(ss, "deploy") {
		t.Error("branch cleanup must come after the deploy")
	}
}

func TestStepsRunWhatTheJobsRun(t *testing.T) {
	script, wf := read(t, "deploy_v2.sh"), read(t, ".github/workflows/deploy_v2.yml")
	for _, c := range []struct {
		job, fn string
		same    []string
	}{
		{"lint", "step_lint", []string{"go vet -composites=false -unreachable=false ./...", "staticcheck@2026.2.1 -checks 'SA*,-SA1019,-SA4006,-SA9003'"}},
		{"unit", "step_unit", []string{"go test -race ./... -count=1 -json", `"Unit tests (race)" 2000`, "go test -race ./... -count=1"}},
		{"api", "step_api", []string{"ci/s3-test-server.sh", "ERP_ROUTE_COVERAGE_MIN=100", "go test -tags integration ./... -count=1 -json", `"API + integration tests (MongoDB)" 2500`}},
		{"smoke", "step_smoke", []string{"ci/smoke.sh"}},
		{"e2e", "step_e2e", []string{"E2E_RACE", "e2e/run.sh -json -timeout 20m", `"API e2e + functional tests (real server)" 300`}},
		{"race-db", "step_race_db", []string{"go test -race ./erp/ -count=1 -json"}},
		{"zatca-sandbox", "step_zatca", []string{"ci/zatca-sdk.sh", "e2e/run.sh -run 'TestZatcaSandbox' -json -timeout 15m"}},
	} {
		j, f := job(wf, c.job), fn(script, c.fn)
		if j == "" || f == "" {
			t.Errorf("missing job %s or function %s", c.job, c.fn)
			continue
		}
		for _, s := range c.same {
			if !strings.Contains(j, s) {
				t.Errorf("CI job %s no longer contains %q; update this test and deploy_v2.sh", c.job, s)
			}
			if !strings.Contains(f, s) {
				t.Errorf("%s does not run %q like CI job %s", c.fn, s, c.job)
			}
		}
	}
	for _, c := range []struct{ job, fn string }{{"unit", "step_unit"}, {"api", "step_api"}, {"race-db", "step_race_db"}} {
		if !strings.Contains(job(wf, c.job), "TZ: Europe/London") || !strings.Contains(fn(script, c.fn), "TZ=Europe/London") {
			t.Errorf("%s and CI job %s must both run in TZ Europe/London", c.fn, c.job)
		}
	}
	if !strings.Contains(fn(script, "step_deploy"), "cardbridge/build.sh") {
		t.Error("the deploy must ship the Card Bridge setup files like the workflow")
	}
}

// a fake go that fails stops the run at lint and never deploys
func TestFailingGateStopsTheDeploy(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"go": "echo fake go $*; exit 1", "jq": "exit 0"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "../../deploy_v2.sh", "--only", "lint")
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "DEPLOY_LOG_ROOT="+filepath.Join(dir, "logs"), "DEPLOY_NO_TREE_CLEANUP=1")
	out, err := cmd.CombinedOutput()
	s := string(out)
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		t.Fatalf("want exit 1, got %v\n%s", err, s)
	}
	for _, want := range []string{"✖ [lint] failed", "fake go vet", "Not deployed", "lint       FAILED"} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Uploading binary") {
		t.Error("deployed after a failing gate")
	}
}

func TestUnknownOptionIsRejected(t *testing.T) {
	err := exec.Command("bash", "../../deploy_v2.sh", "--nope").Run()
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 2 {
		t.Fatalf("want exit 2, got %v", err)
	}
}

func writeJSON(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSkipGate(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatal("jq is needed by ci/skip-gate.sh")
	}
	dir := t.TempDir()
	unit := writeJSON(t, dir, "unit.json",
		`{"Action":"pass","Package":"p","Test":"TestA"}`,
		`{"Action":"output","Package":"p","Test":"TestDB","Output":"    db_test.go:9: needs ERP_TEST_DB=1\n"}`,
		`{"Action":"skip","Package":"p","Test":"TestDB"}`)
	apiOK := writeJSON(t, dir, "api.json", `{"Action":"pass","Package":"p","Test":"TestDB"}`)
	apiMissing := writeJSON(t, dir, "api2.json", `{"Action":"pass","Package":"p","Test":"TestA"}`)
	run := func(args ...string) (string, int) {
		out, err := exec.Command("bash", append([]string{"../skip-gate.sh"}, args...)...).CombinedOutput()
		if ee, ok := err.(*exec.ExitError); ok {
			return string(out), ee.ExitCode()
		}
		return string(out), 0
	}
	if out, code := run("unit", unit, apiOK); code != 0 {
		t.Errorf("a skip that passed in the api run must be accepted: %s", out)
	}
	if out, code := run("unit", unit, apiMissing); code != 1 || !strings.Contains(out, "needs ERP_TEST_DB=1") {
		t.Errorf("a skip that ran nowhere must fail with its reason (exit %d): %s", code, out)
	}
	if out, code := run("api", unit); code != 1 {
		t.Errorf("any skip without a covering run must fail: %s", out)
	}
	if out, code := run("api", apiOK); code != 0 || !strings.Contains(out, "SKIPS (api): 0") {
		t.Errorf("no skips must pass: %s", out)
	}
}
