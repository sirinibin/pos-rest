package erp

import (
	"strings"
	"testing"
)

func TestValidSerialPrefix(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"", true},
		{"DEL-NOTE-UMLJ-DATE-", true}, // reported value: 19 chars, used to fail the old 16 cap
		{"DEL-NOTE-UMLJ-DATE", true},
		{"S-INV-", true},
		{"INV/2026/", true},
		{"PO_2026.", true},
		{strings.Repeat("A", 50), true},
		{strings.Repeat("A", 51), false},
		{"bad prefix", false},
		{"INV#", false},
		{"INV?", false},
		{"فاتورة", false},
	}
	for _, c := range cases {
		if got := validSerialPrefix(c.in); got != c.ok {
			t.Errorf("validSerialPrefix(%q) = %v, want %v", c.in, got, c.ok)
		}
	}
}

func TestStoreValidate_SerialPrefix(t *testing.T) {
	base := func(prefix string) M {
		return M{"serials": M{"deliveryNote": M{"prefix": prefix, "start": 1}}}
	}
	if errs := storeValidate(nil, base("DEL-NOTE-UMLJ-DATE-"), nil); errs["serials.deliveryNote.prefix"] != "" {
		t.Fatalf("long legacy prefix must be accepted: %v", errs)
	}
	errs := storeValidate(nil, base("DEL NOTE"), nil)
	if errs["serials.deliveryNote.prefix"] != serialPrefixError {
		t.Fatalf("prefix with a space must be rejected with the clear message: %v", errs)
	}
}
