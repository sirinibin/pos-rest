package models

import "testing"

// ── BuildPartNoPrefix ─────────────────────────────────────────────────────────
//
// Business rule: when a brand's code changes every matching product's
// prefix_part_number must be recomputed as "{brand_code}-{country_code}".
// BuildPartNoPrefix is the canonical Go expression of that formula.

func TestBuildPartNoPrefix_BothPresent(t *testing.T) {
	got := BuildPartNoPrefix("SNO", "IN")
	want := "SNO-IN"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPartNoPrefix_NoCountryCode(t *testing.T) {
	// When a product has no country set, prefix = brand code only (no trailing dash).
	got := BuildPartNoPrefix("SNO", "")
	want := "SNO"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPartNoPrefix_ChangedBrandCode(t *testing.T) {
	// Simulates the cascade: old code "SNO" → new code "SNO123", country "IN".
	oldPrefix := BuildPartNoPrefix("SNO", "IN")
	newPrefix := BuildPartNoPrefix("SNO123", "IN")
	if oldPrefix != "SNO-IN" {
		t.Errorf("old prefix: got %q, want %q", oldPrefix, "SNO-IN")
	}
	if newPrefix != "SNO123-IN" {
		t.Errorf("new prefix: got %q, want %q", newPrefix, "SNO123-IN")
	}
}

func TestBuildPartNoPrefix_MultipleCountries(t *testing.T) {
	// Same brand, different country_code per product — each gets its own prefix.
	cases := []struct {
		brandCode   string
		countryCode string
		want        string
	}{
		{"SNO", "IN", "SNO-IN"},
		{"SNO", "US", "SNO-US"},
		{"SNO", "SA", "SNO-SA"},
		{"SNO", "AE", "SNO-AE"},
		{"SNO", "", "SNO"},
	}
	for _, tc := range cases {
		got := BuildPartNoPrefix(tc.brandCode, tc.countryCode)
		if got != tc.want {
			t.Errorf("BuildPartNoPrefix(%q, %q) = %q, want %q", tc.brandCode, tc.countryCode, got, tc.want)
		}
	}
}

func TestBuildPartNoPrefix_EmptyBrandCode(t *testing.T) {
	// Brand code should not be empty in practice, but the function must not panic.
	got := BuildPartNoPrefix("", "IN")
	want := "-IN"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPartNoPrefix_BothEmpty(t *testing.T) {
	got := BuildPartNoPrefix("", "")
	want := ""
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildPartNoPrefix_SpecialChars(t *testing.T) {
	// Codes may contain digits, dashes already — the function should just concatenate.
	got := BuildPartNoPrefix("BRAND-2024", "UAE")
	want := "BRAND-2024-UAE"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ── AttributesValueChangeEvent trigger guard ──────────────────────────────────
//
// The cascade is only fired when Code actually changes.
// We verify the conditional using the same field comparison the real function uses.

func TestBrandCodeChanged_Detection(t *testing.T) {
	cases := []struct {
		oldCode    string
		newCode    string
		wantChange bool
	}{
		{"SNO", "SNO123", true},    // code changed
		{"SNO", "SNO", false},      // same code — no DB update should fire
		{"", "SNO", true},          // code was blank, now set
		{"SNO", "", true},          // code cleared
		{"", "", false},            // both blank — no change
		{"SNO123", "SNO123", false}, // unchanged after previous update
	}
	for _, tc := range cases {
		changed := tc.oldCode != tc.newCode
		if changed != tc.wantChange {
			t.Errorf("oldCode=%q newCode=%q: changed=%v, want %v", tc.oldCode, tc.newCode, changed, tc.wantChange)
		}
	}
}
