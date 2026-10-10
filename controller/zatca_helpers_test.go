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

func TestZatcaDeviceSerialNumber(t *testing.T) {
	const dev = "-4bd41220-f619-47bc-830b-7fedd3b33032"
	cases := []struct {
		name, prefix string
		padding      int64
		want         string
	}{
		{"prefix and padding", "GUOJ", 5, "1-GUOJ|2-00001|3" + dev},
		{"prefix with a date", "INV-DATE", 3, "1-INV|2-20261010|3-001|4" + dev},
		{"no prefix", "", 5, "1-INV|2-00001|3" + dev},
		{"no prefix, no padding", "", 0, "1-INV|2-1|3" + dev},
		{"prefix with a trailing dash", "S-", 2, "1-S|2-01|3" + dev},
		{"blank prefix", "  ", 4, "1-INV|2-0001|3" + dev},
	}
	for _, c := range cases {
		if got := zatcaDeviceSerialNumber(c.prefix, c.padding, "20261010"); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
