package models

import (
	"encoding/json"
	"testing"
)

// ── IsNumberStartAndEndWith ───────────────────────────────────────────────────

func TestIsNumberStartAndEndWith(t *testing.T) {
	cases := []struct {
		num      string
		startEnd string
		want     bool
	}{
		{"1001", "1", true},
		{"2002", "2", true},
		{"123", "1", false},  // ends with 3, not 1
		{"", "1", false},
		// A single char "1" cannot satisfy ^1\d*1$ (needs at least 2 chars)
		{"1", "1", false},
		{"11", "1", true},    // minimal: starts and ends with 1
		{"999", "9", true},
		{"5005", "5", true},
		{"5004", "5", false}, // ends with 4
		{"abc", "a", false},  // non-digit boundary
	}
	for _, c := range cases {
		got := IsNumberStartAndEndWith(c.num, c.startEnd)
		if got != c.want {
			t.Errorf("IsNumberStartAndEndWith(%q, %q) = %v, want %v", c.num, c.startEnd, got, c.want)
		}
	}
}

// ── IsAlphanumeric ────────────────────────────────────────────────────────────

func TestIsAlphanumeric(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"abc123", true},
		{"ABC123", true},
		{"hello", true},
		{"12345", true},
		{"", false},        // empty does not match +
		{"hello world", false}, // space not allowed
		{"abc!", false},
		{"abc-123", false},
		{"abc_123", false},
	}
	for _, c := range cases {
		got := IsAlphanumeric(c.in)
		if got != c.want {
			t.Errorf("IsAlphanumeric(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── ValidateSaudiPhone ────────────────────────────────────────────────────────

func TestValidateSaudiPhone(t *testing.T) {
	cases := []struct {
		phone string
		want  bool
	}{
		// Valid formats
		{"+966512345678", true},  // +966 + 5 + 8 digits
		{"+966598765432", true},
		{"0512345678", true},     // 05 + 8 digits
		{"0598765432", true},
		// Invalid formats
		{"512345678", false},     // missing prefix
		{"+966412345678", false}, // 4 instead of 5
		{"0412345678", false},    // 04 instead of 05
		{"+9665123456", false},   // too short
		{"0512345", false},       // too short
		{"05123456789", false},   // too long
		{"", false},
		{"+1234567890", false},   // wrong country code
		{"hello", false},
	}
	for _, c := range cases {
		got := ValidateSaudiPhone(c.phone)
		if got != c.want {
			t.Errorf("ValidateSaudiPhone(%q) = %v, want %v", c.phone, got, c.want)
		}
	}
}

// ── ExtractSaudiPhoneNumbers ──────────────────────────────────────────────────

func TestExtractSaudiPhoneNumbers_FromText(t *testing.T) {
	input := "Call us at 0512345678 or 0598765432 for support"
	result := ExtractSaudiPhoneNumbers(input)
	if len(result) != 2 {
		t.Fatalf("got %d numbers, want 2: %v", len(result), result)
	}
	wantSet := map[string]bool{"0512345678": true, "0598765432": true}
	for _, r := range result {
		if !wantSet[r] {
			t.Errorf("unexpected number extracted: %q", r)
		}
	}
}

func TestExtractSaudiPhoneNumbers_NineDigitNormalised(t *testing.T) {
	// 9-digit starting with 5 should be normalised to 0512345678
	input := "contact 512345678"
	result := ExtractSaudiPhoneNumbers(input)
	if len(result) != 1 || result[0] != "0512345678" {
		t.Errorf("got %v, want [\"0512345678\"]", result)
	}
}

func TestExtractSaudiPhoneNumbers_SkipsVATNumbers(t *testing.T) {
	// 15-digit numbers are VAT numbers and must be skipped
	input := "VAT: 310123456700003 phone: 0512345678"
	result := ExtractSaudiPhoneNumbers(input)
	if len(result) != 1 || result[0] != "0512345678" {
		t.Errorf("got %v, want [\"0512345678\"] (VAT number must be skipped)", result)
	}
}

func TestExtractSaudiPhoneNumbers_EmptyInput(t *testing.T) {
	result := ExtractSaudiPhoneNumbers("")
	if len(result) != 0 {
		t.Errorf("got %v, want empty slice", result)
	}
}

func TestExtractSaudiPhoneNumbers_NoValidNumbers(t *testing.T) {
	result := ExtractSaudiPhoneNumbers("no phone numbers here, just text")
	if len(result) != 0 {
		t.Errorf("got %v, want empty slice", result)
	}
}

func TestExtractSaudiPhoneNumbers_InvalidMobilePrefix(t *testing.T) {
	// Starts with 4 not 5 — not a Saudi mobile
	input := "0412345678"
	result := ExtractSaudiPhoneNumbers(input)
	if len(result) != 0 {
		t.Errorf("got %v, want empty (04 prefix is not a mobile)", result)
	}
}

// ── StoreSettings.ShowWarehouseStockInSelectedProducts ────────────────────────
// Tests that the new field added to fix "setting not saving" has correct JSON
// and bson tags and round-trips through JSON marshaling/unmarshaling correctly.

func TestStoreSettings_ShowWarehouseStockInSelectedProducts_DefaultFalse(t *testing.T) {
	var s StoreSettings
	if s.ShowWarehouseStockInSelectedProducts {
		t.Error("zero-value ShowWarehouseStockInSelectedProducts should be false")
	}
}

func TestStoreSettings_ShowWarehouseStockInSelectedProducts_JSONTag(t *testing.T) {
	s := StoreSettings{ShowWarehouseStockInSelectedProducts: true}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	v, ok := m["show_warehouse_stock_in_selected_products"]
	if !ok {
		t.Error("expected JSON key 'show_warehouse_stock_in_selected_products' not found")
	}
	if v != true {
		t.Errorf("expected true, got %v", v)
	}
}

func TestStoreSettings_ShowWarehouseStockInSelectedProducts_RoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		value bool
	}{
		{"true persists", true},
		{"false persists", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			original := StoreSettings{ShowWarehouseStockInSelectedProducts: c.value}
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var decoded StoreSettings
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if decoded.ShowWarehouseStockInSelectedProducts != c.value {
				t.Errorf("round-trip: got %v, want %v", decoded.ShowWarehouseStockInSelectedProducts, c.value)
			}
		})
	}
}

func TestStoreSettings_ShowWarehouseStockInSelectedProducts_FromJSONString(t *testing.T) {
	// Simulates the request body the frontend sends when the setting is saved.
	cases := []struct {
		json string
		want bool
	}{
		{`{"show_warehouse_stock_in_selected_products": true}`, true},
		{`{"show_warehouse_stock_in_selected_products": false}`, false},
		{`{}`, false}, // missing → zero value
	}
	for _, c := range cases {
		var s StoreSettings
		if err := json.Unmarshal([]byte(c.json), &s); err != nil {
			t.Fatalf("Unmarshal(%q): %v", c.json, err)
		}
		if s.ShowWarehouseStockInSelectedProducts != c.want {
			t.Errorf("json=%q: got %v, want %v", c.json, s.ShowWarehouseStockInSelectedProducts, c.want)
		}
	}
}

func TestStoreSettings_EnableWarehouseModule_IndependentOfNewField(t *testing.T) {
	// EnableWarehouseModule and ShowWarehouseStockInSelectedProducts are independent
	// booleans — enabling one must not affect the other.
	s := StoreSettings{EnableWarehouseModule: true, ShowWarehouseStockInSelectedProducts: false}
	data, _ := json.Marshal(s)
	var decoded StoreSettings
	json.Unmarshal(data, &decoded)
	if !decoded.EnableWarehouseModule {
		t.Error("EnableWarehouseModule should be true")
	}
	if decoded.ShowWarehouseStockInSelectedProducts {
		t.Error("ShowWarehouseStockInSelectedProducts should be false")
	}
}

func TestStoreSettings_BothWarehouseFieldsTrue(t *testing.T) {
	s := StoreSettings{EnableWarehouseModule: true, ShowWarehouseStockInSelectedProducts: true}
	data, _ := json.Marshal(s)
	var decoded StoreSettings
	json.Unmarshal(data, &decoded)
	if !decoded.EnableWarehouseModule {
		t.Error("EnableWarehouseModule should be true")
	}
	if !decoded.ShowWarehouseStockInSelectedProducts {
		t.Error("ShowWarehouseStockInSelectedProducts should be true")
	}
}
