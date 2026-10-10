package controller

import (
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

func TestZatcaReportingOn(t *testing.T) {
	cases := []struct {
		name      string
		phase     string
		connected bool
		want      bool
	}{
		{"phase 2 connected", "2", true, true},
		{"phase 2 not connected", "2", false, false},
		{"phase 1", "1", true, false},
		{"no phase", "", false, false},
	}
	for _, c := range cases {
		s := &models.Store{}
		s.Zatca.Phase, s.Zatca.Connected = c.phase, c.connected
		if got := zatcaReportingOn(s); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if zatcaReportingOn(nil) {
		t.Errorf("nil store reports to ZATCA")
	}
}

func TestLastLines(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"one", "one"},
		{"a\nb\nc", "b\nc"},
		{"a\nb\n", "a\nb"},
	}
	for _, c := range cases {
		if got := lastLines(c.in, 2); got != c.want {
			t.Errorf("lastLines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
