package deploy

// Structure tests for the GitHub Actions deploy pipeline: master and test
// carry the same code, so one workflow runs the suites once, builds once and
// deploys that build to the test API and then to production.

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type workflow struct {
	On struct {
		Push struct {
			Branches []string `yaml:"branches"`
			Paths    []string `yaml:"paths"`
		} `yaml:"push"`
		WorkflowDispatch *struct {
			Inputs map[string]struct {
				Default string   `yaml:"default"`
				Options []string `yaml:"options"`
			} `yaml:"inputs"`
		} `yaml:"workflow_dispatch"`
	} `yaml:"on"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]struct {
		Uses  string            `yaml:"uses"`
		With  map[string]string `yaml:"with"`
		Needs yaml.Node         `yaml:"needs"`
		Steps []struct {
			Name string            `yaml:"name"`
			Uses string            `yaml:"uses"`
			Run  string            `yaml:"run"`
			With map[string]string `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func loadWorkflow(t *testing.T, path string) workflow {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return w
}

func needsOf(t *testing.T, n yaml.Node) []string {
	t.Helper()
	switch n.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		var out []string
		for _, c := range n.Content {
			out = append(out, c.Value)
		}
		return out
	}
	t.Fatalf("unexpected needs node kind %v", n.Kind)
	return nil
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]bool{}
	for _, x := range a {
		m[x] = true
	}
	for _, x := range b {
		if !m[x] {
			return false
		}
	}
	return true
}

func TestDeployWorkflow_OnePipelineForBothServers(t *testing.T) {
	w := loadWorkflow(t, "../.github/workflows/deploy.yml")

	if !sameSet(w.On.Push.Branches, []string{"master"}) {
		t.Errorf("deploy.yml must run on pushes to master only (test is a mirror), got %v", w.On.Push.Branches)
	}
	if raw, _ := os.ReadFile("../.github/workflows/deploy.yml"); !strings.Contains(string(raw), "\n  workflow_dispatch:") {
		t.Error("deploy.yml should keep a manual Run workflow button")
	}
	for _, p := range []string{"**.go", "**.mod", "**.sum", "e2e/**", "ZatcaPython/**", "deploy/**", ".github/workflows/deploy.yml", ".github/workflows/tests.yml"} {
		found := false
		for _, have := range w.On.Push.Paths {
			found = found || have == p
		}
		if !found {
			t.Errorf("deploy.yml paths filter is missing %q", p)
		}
	}
	if w.Concurrency.Group == "" || w.Concurrency.CancelInProgress {
		t.Errorf("deploys must queue in one concurrency group, got %+v", w.Concurrency)
	}

	want := map[string][]string{
		"tests":             nil,
		"build":             nil,
		"deploy-test":       {"tests", "build"},
		"deploy-production": {"deploy-test"},
	}
	if len(w.Jobs) != len(want) {
		t.Errorf("jobs = %d, want exactly %v", len(w.Jobs), want)
	}
	for name, needs := range want {
		j, ok := w.Jobs[name]
		if !ok {
			t.Errorf("job %q missing", name)
			continue
		}
		if got := needsOf(t, j.Needs); !sameSet(got, needs) {
			t.Errorf("%s needs %v, want %v", name, got, needs)
		}
	}

	// The suites run exactly once, against the frontend's master.
	if j := w.Jobs["tests"]; j.Uses != "./.github/workflows/tests.yml" || j.With["frontend_ref"] != "master" {
		t.Errorf("tests job = uses %q with %v", j.Uses, j.With)
	}

	// One build: only the build job compiles; both deploys take its artifact.
	for name, j := range w.Jobs {
		for _, s := range j.Steps {
			if strings.Contains(s.Run, "go build") && name != "build" {
				t.Errorf("%s compiles its own binary; it must deploy the tested build", name)
			}
		}
	}
	var uploaded string
	for _, s := range w.Jobs["build"].Steps {
		if strings.HasPrefix(s.Uses, "actions/upload-artifact@") {
			uploaded = s.With["name"]
		}
	}
	if uploaded == "" {
		t.Fatal("build job does not upload its binary")
	}
	for _, name := range []string{"deploy-test", "deploy-production"} {
		var downloaded string
		var restart string
		for _, s := range w.Jobs[name].Steps {
			if strings.HasPrefix(s.Uses, "actions/download-artifact@") {
				downloaded = s.With["name"]
			}
			if strings.Contains(s.Run, "< deploy/remote_restart.sh") {
				restart = s.Run
			}
		}
		if downloaded != uploaded {
			t.Errorf("%s deploys artifact %q, want the built %q", name, downloaded, uploaded)
		}
		if restart == "" {
			t.Errorf("%s does not restart through deploy/remote_restart.sh", name)
		}
	}

	// Each job talks to its own server and never the other's.
	checks := map[string]struct{ want, never string }{
		"deploy-test":       {"start-api-test 2002", "AWS_GO_API_PROD_DEST_PATH"},
		"deploy-production": {"start-api 2000", "AWS_GO_API_TEST_DEST_PATH"},
	}
	for name, c := range checks {
		var all strings.Builder
		for _, s := range w.Jobs[name].Steps {
			all.WriteString(s.Run)
		}
		if !strings.Contains(all.String(), c.want) {
			t.Errorf("%s does not restart %q", name, c.want)
		}
		if strings.Contains(all.String(), c.never) {
			t.Errorf("%s touches %s", name, c.never)
		}
	}
}

func TestDeployWorkflow_OldPerBranchWorkflowsAreGone(t *testing.T) {
	for _, p := range []string{"../.github/workflows/deploy_test.yml", "../.github/workflows/deploy_prod.yml"} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s still exists; it would run the suites a second time", p)
		}
	}
}

func TestQuickDeploy_TargetsTestThenProduction(t *testing.T) {
	w := loadWorkflow(t, "../.github/workflows/deploy_quick.yml")
	if len(w.On.Push.Branches) != 0 {
		t.Fatal("quick deploy must never run on push")
	}
	in, ok := w.On.WorkflowDispatch.Inputs["target"]
	if !ok || in.Default != "both" || !sameSet(in.Options, []string{"both", "test", "production"}) {
		t.Errorf("target input = %+v", in)
	}
	if got := needsOf(t, w.Jobs["deploy"].Needs); !sameSet(got, []string{"build"}) {
		t.Errorf("deploy needs %v", got)
	}
	b, _ := os.ReadFile("../.github/workflows/deploy_quick.yml")
	s := string(b)
	if strings.Index(s, "target: test") > strings.Index(s, "target: production") {
		t.Error("the test server must come before production in the matrix")
	}
	for _, w := range []string{"max-parallel: 1", "fail-fast: true"} {
		if !strings.Contains(s, w) {
			t.Errorf("quick deploy is missing %q", w)
		}
	}
}
