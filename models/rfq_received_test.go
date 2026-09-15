package models

import "testing"

func TestPhoneCoreSuffix(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"966501971075", "501971075"}, // international Saudi → 9 core digits
		{"0501971075", "501971075"},   // local Saudi with leading 0 → same
		{"+966501971075", "501971075"}, // with leading + (stripped before calling)
		{"971501234567", "501234567"}, // UAE international
		{"0501234567", "501234567"},   // UAE local
		{"501971075", "501971075"},    // already 9 digits
		{"", ""},                      // empty
	}
	for _, tc := range cases {
		got := phoneCoreSuffix(tc.input)
		if got != tc.want {
			t.Errorf("phoneCoreSuffix(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
