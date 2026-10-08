package main

import "testing"

func TestSafeDBName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"pos_e2e", true},
		{"POS_E2E", true},
		{"startpos_integration_test", true},
		{"pos_test", true},
		{"e2e", true},
		{"pos", false},
		{"pos_prod", false},
		{"startpos", false},
		{"", false},
		{"  ", false},
	}
	for _, c := range cases {
		if got := safeDBName(c.name); got != c.want {
			t.Errorf("safeDBName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
