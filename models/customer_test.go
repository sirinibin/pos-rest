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
		{"123456789", "10", false}, // one short
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

func TestTrimName(t *testing.T) {
	cases := map[string]string{
		"":                                 "",
		" ":                                "",
		"   ":                              "",
		"\t\n\r":                           "",
		"\u00a0":                           "",
		"\u200b":                           "",
		"\ufeff":                           "",
		" \u200b\u200c\u200d\u2060\ufeff ": "",
		"Ali":                              "Ali",
		"  Ali  ":                          "Ali",
		"\u200bAli\u200b":                  "Ali",
		"Ali Hassan":                       "Ali Hassan",
		"  محمد  ":                         "محمد",
		"می\u200cخواهم":                    "می\u200cخواهم", // a joiner inside a name stays
	}
	for in, want := range cases {
		if got := TrimName(in); got != want {
			t.Errorf("TrimName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCustomerTrimSpaceFromFields_BlankNameBecomesEmpty(t *testing.T) {
	c := Customer{Name: " \u200b ", NameInArabic: "\u00a0"}
	c.TrimSpaceFromFields()
	if c.Name != "" || c.NameInArabic != "" {
		t.Fatalf("got Name=%q NameInArabic=%q, want both empty", c.Name, c.NameInArabic)
	}
}

func TestCustomerInsert_RefusesBlankName(t *testing.T) {
	for _, name := range []string{"", " ", "\t", "\u200b", " \ufeff "} {
		c := Customer{Name: name}
		if err := c.Insert(); err != ErrCustomerNameRequired {
			t.Errorf("Insert with name %q: err = %v, want ErrCustomerNameRequired", name, err)
		}
	}
}
