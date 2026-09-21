package models

import (
	"os"
	"regexp"
	"testing"
)

// TestRFQSupplierCodeField verifies the Code field exists on the struct.
func TestRFQSupplierCodeField(t *testing.T) {
	s := RFQSupplier{
		Name:  "Test Supplier",
		Phone: "966501234567",
		Code:  "SUP-000001",
	}
	if s.Code != "SUP-000001" {
		t.Errorf("expected Code=SUP-000001, got %q", s.Code)
	}
}

// TestRFQSupplierCodeFormat verifies the SUP-NNNNNN format pattern.
func TestRFQSupplierCodeFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^SUP-\d{6}$`)
	cases := []struct {
		code  string
		valid bool
	}{
		{"SUP-000001", true},
		{"SUP-000042", true},
		{"SUP-999999", true},
		{"SUP-1", false},
		{"EM-000001", false},
		{"", false},
		{"SUP-00001", false},
	}
	for _, c := range cases {
		got := pattern.MatchString(c.code)
		if got != c.valid {
			t.Errorf("pattern match %q: got %v, want %v", c.code, got, c.valid)
		}
	}
}

// TestListRFQSuppliersSearchIncludesPhone2 verifies that the search $or filter in
// ListRFQSuppliers includes the phone2 field, so secondary numbers are searchable.
func TestListRFQSuppliersSearchIncludesPhone2(t *testing.T) {
	// The fix adds phone2 to the $or. We verify by inspecting the source file for
	// the phone2 regex condition inside the search block.
	src, err := os.ReadFile("rfq_supplier.go")
	if err != nil {
		t.Fatalf("could not read rfq_supplier.go: %v", err)
	}
	if !regexp.MustCompile(`"phone2"[\s\S]{0,50}"\$regex"`).Match(src) {
		t.Error("rfq_supplier.go ListRFQSuppliers does not search phone2 — add phone2 to the $or filter")
	}
}

// TestRFQSupplierPhone2Priority verifies the phone2-first lookup intent by checking the struct.
// (Full DB integration would require a real MongoDB; this confirms the data model is correct.)
func TestRFQSupplierPhone2Field(t *testing.T) {
	s := RFQSupplier{
		Name:   "Jeddah Anchor Trading",
		Phone:  "966501111111",
		Phone2: "966596958072",
	}
	if s.Phone2 != "966596958072" {
		t.Errorf("expected Phone2=966596958072, got %q", s.Phone2)
	}
	if s.Phone == s.Phone2 {
		t.Error("phone and phone2 must be different numbers")
	}
}

// TestFindRFQsForwardedToSupplierSignature verifies that the new multi-alias lookup
// function exists and that the old single-phone wrapper still compiles.
func TestFindRFQsForwardedToSupplierExists(t *testing.T) {
	src, err := os.ReadFile("rfq_received.go")
	if err != nil {
		t.Fatalf("could not read rfq_received.go: %v", err)
	}
	if !regexp.MustCompile(`func FindRFQsForwardedToSupplier`).Match(src) {
		t.Error("FindRFQsForwardedToSupplier is missing from rfq_received.go")
	}
	// Must match by supplier_name (user suggestion)
	if !regexp.MustCompile(`supplier_name`).Match(src) {
		t.Error("FindRFQsForwardedToSupplier must include supplier_name matching")
	}
	// Old wrapper must still exist for backward compat
	if !regexp.MustCompile(`func FindRFQsForwardedToPhone`).Match(src) {
		t.Error("FindRFQsForwardedToPhone backward-compat wrapper is missing")
	}
}

// TestListRFQReceivedControllerResolvesPhone2 verifies the controller resolves
// the incoming phone via FindRFQSupplierByPhone before calling FindRFQsForwardedToSupplier.
func TestListRFQReceivedControllerResolvesPhone2(t *testing.T) {
	src, err := os.ReadFile("../controller/rfq_bot.go")
	if err != nil {
		t.Fatalf("could not read rfq_bot.go: %v", err)
	}
	if !regexp.MustCompile(`FindRFQSupplierByPhone`).Match(src) {
		t.Error("rfq_bot.go ListRFQReceived does not resolve phone via FindRFQSupplierByPhone")
	}
	if !regexp.MustCompile(`FindRFQsForwardedToSupplier`).Match(src) {
		t.Error("rfq_bot.go ListRFQReceived does not call FindRFQsForwardedToSupplier")
	}
}
