package controller

import (
	"reflect"
	"testing"
)

// The comma fallback trimmed with a raw-string cutset (`\t` = '\' and 't'),
// which ate leading/trailing t, n and r letters ("Paint" became "Pai").
func TestParseCategories_CommaFallbackKeepsLetters(t *testing.T) {
	cases := map[string][]string{
		`Paint, Tires, filter`:   {"Paint", "Tires", "filter"},
		` "Battery" ,"Motor"`:    {"Battery", "Motor"},
		"Rotor\t,\nnut\r":        {"Rotor", "nut"},
		`["Paint","Engine oil"]`: {"Paint", "Engine oil"},
		` , `:                    nil,
	}
	for in, want := range cases {
		if got := parseCategories(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parseCategories(%q) = %q, want %q", in, got, want)
		}
	}
}
