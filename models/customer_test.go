package models

import (
	"testing"
)

// ── IsValidDigitNumber ────────────────────────────────────────────────────────

func TestIsValidDigitNumber(t *testing.T) {
	cases := []struct {
		s           string
		digitsCount string
		want        bool
	}{
		// 4-digit checks
		{"1234", "4", true},
		{"0000", "4", true},
		{"9999", "4", true},
		{"123", "4", false},   // too short
		{"12345", "4", false}, // too long
		{"abcd", "4", false},  // non-digits
		{"12 4", "4", false},  // space
		// 10-digit checks (like VAT numbers in some countries)
		{"1234567890", "10", true},
		{"123456789", "10", false},  // one short
		// 1-digit check
		{"5", "1", true},
		{"55", "1", false},
		// empty
		{"", "4", false},
		// special chars
		{"12-34", "4", false},
	}
	for _, c := range cases {
		got := IsValidDigitNumber(c.s, c.digitsCount)
		if got != c.want {
			t.Errorf("IsValidDigitNumber(%q, %q) = %v, want %v", c.s, c.digitsCount, got, c.want)
		}
	}
}
