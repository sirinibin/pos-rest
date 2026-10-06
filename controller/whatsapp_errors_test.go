package controller

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The UI must never show anything about "Evolution API": error bodies and
// errors that reach the client name only the WhatsApp service.
func TestNoEvolutionTextInClientErrors(t *testing.T) {
	// a string or raw-string literal that contains "Evolution"
	lit := regexp.MustCompile("\"[^\"\\n]*Evolution[^\"\\n]*\"|`[^`]*Evolution[^`]*`")
	for _, dir := range []string{".", "../models", "../erp"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(b), "\n") {
				code := line
				if c := strings.Index(code, "//"); c >= 0 && !strings.Contains(code[:c], "\"") && !strings.Contains(code[:c], "`") {
					code = code[:c]
				}
				if lit.MatchString(code) {
					t.Errorf("%s:%d user-visible Evolution text: %s", f, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}
