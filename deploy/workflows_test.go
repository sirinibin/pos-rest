package deploy

// GitHub Actions is not used (2026-10-11): deploy.sh is the only deploy path.
// It tests once, builds once and deploys that build to the test API and,
// only after it is healthy, to production.

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestNoGitHubActionsWorkflows(t *testing.T) {
	if _, err := os.Stat("../.github/workflows"); err == nil {
		t.Error(".github/workflows exists; GitHub Actions is not used, deploy.sh is the only deploy path")
	}
}

func TestDeploySh_TestsOnceThenTestAPIBeforeProduction(t *testing.T) {
	b, err := os.ReadFile("../deploy.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	tests := strings.Index(s, "bash ci/run_all_tests.sh")
	testAPI := strings.Index(s, `deploy_to "start-api-test"`)
	prod := strings.Index(s, `deploy_to "start-api" `)
	if tests < 0 || testAPI < 0 || prod < 0 {
		t.Fatalf("deploy.sh must run ci/run_all_tests.sh and deploy to start-api-test and start-api (tests=%d test=%d prod=%d)", tests, testAPI, prod)
	}
	if !(tests < testAPI && testAPI < prod) {
		t.Errorf("deploy.sh order must be tests, test API, production (tests=%d test=%d prod=%d)", tests, testAPI, prod)
	}
	if n := strings.Count(s, `go build -o "$BINARY" .`); n != 1 {
		t.Errorf("deploy.sh must build the API binary once, found %d builds", n)
	}
	if strings.Contains(s, ".github/workflows") {
		t.Error("deploy.sh still refers to .github/workflows")
	}
}

func TestQuickDeploy_ReachesTestFromMaster(t *testing.T) {
	b, err := os.ReadFile("../deploy_quick.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "origin/test") {
		t.Error("deploy_quick.sh still refers to the test branch")
	}
	if !strings.Contains(s, `if [ "$BRANCH" = master ] && $TO_TEST; then TARGET=test; fi`) {
		t.Error("deploy_quick.sh must reach the test server from master with --test")
	}
}

// Deploys happen only after every suite passes (user rule 2026-10-11): the
// slow suites run by default, and QUICK=1 is refused by deploy.sh.
func TestRunAllTests_EverySuiteByDefault(t *testing.T) {
	b, err := os.ReadFile("../ci/run_all_tests.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `if [ "${QUICK:-}" = 1 ]; then FULL=0; else FULL=1; fi`) {
		t.Error("ci/run_all_tests.sh must run every suite unless QUICK=1")
	}
	if strings.Contains(s, `"${FULL:-}"`) {
		t.Error("ci/run_all_tests.sh still reads FULL from the environment (opt-in)")
	}
	for _, stage := range []string{"Race detector", "ZATCA sandbox", `UI_PROJECTS="" UI_LABEL="every screen size"`} {
		if !strings.Contains(s, stage) {
			t.Errorf("ci/run_all_tests.sh is missing %q", stage)
		}
	}
}

func TestDeploySh_RefusesQuick(t *testing.T) {
	cmd := exec.Command("bash", "../deploy.sh")
	cmd.Env = append(os.Environ(), "QUICK=1", "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("deploy.sh ran with QUICK=1:\n%s", out)
	}
	if !strings.Contains(string(out), "always runs every test suite") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	if strings.Contains(string(out), "Running the test suites") || strings.Contains(string(out), "Checking for uncommitted") {
		t.Fatalf("deploy.sh went past the QUICK check:\n%s", out)
	}
}
