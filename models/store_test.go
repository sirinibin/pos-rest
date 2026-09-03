package models

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
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

// ── Zatca.ZatcaReconnectRequired ─────────────────────────────────────────────

// TestZatcaReconnectRequired_ZeroValueFalse verifies that the zero value of
// Zatca has ZatcaReconnectRequired == false.
func TestZatcaReconnectRequired_ZeroValueFalse(t *testing.T) {
	var z Zatca
	if z.ZatcaReconnectRequired {
		t.Error("zero-value ZatcaReconnectRequired should be false")
	}
}

// TestZatcaReconnectRequired_JSONKey checks that the field marshals to the
// expected JSON key "zatca_reconnect_required".
func TestZatcaReconnectRequired_JSONKey(t *testing.T) {
	z := Zatca{ZatcaReconnectRequired: true}
	data, err := json.Marshal(z)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	v, ok := m["zatca_reconnect_required"]
	if !ok {
		t.Error("expected JSON key 'zatca_reconnect_required' not found")
	}
	if v != true {
		t.Errorf("expected true, got %v", v)
	}
}

// TestZatcaReconnectRequired_OmitEmptyWhenFalse verifies that the field is
// absent from JSON output when it is false (omitempty tag).
func TestZatcaReconnectRequired_OmitEmptyWhenFalse(t *testing.T) {
	z := Zatca{ZatcaReconnectRequired: false}
	data, err := json.Marshal(z)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	if _, ok := m["zatca_reconnect_required"]; ok {
		t.Error("zatca_reconnect_required must be absent from JSON when false (omitempty)")
	}
}

// TestZatcaReconnectRequired_RoundTrip verifies that the field survives a
// JSON marshal/unmarshal round-trip for both true and false.
func TestZatcaReconnectRequired_RoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		value bool
	}{
		{"true persists", true},
		{"false persists", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			original := Zatca{ZatcaReconnectRequired: c.value}
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var decoded Zatca
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if decoded.ZatcaReconnectRequired != c.value {
				t.Errorf("round-trip: got %v, want %v", decoded.ZatcaReconnectRequired, c.value)
			}
		})
	}
}

// TestZatcaReconnectRequired_NestedInStore verifies that the field appears in
// the right place when Store is marshaled.
func TestZatcaReconnectRequired_NestedInStore(t *testing.T) {
	s := Store{Zatca: Zatca{ZatcaReconnectRequired: true}}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal Store failed: %v", err)
	}
	// Decode to a two-level map to inspect the nested key.
	var top map[string]interface{}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	zatcaRaw, ok := top["zatca"]
	if !ok {
		t.Fatal("expected 'zatca' key in marshaled Store")
	}
	zatcaMap, ok := zatcaRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("zatca value is %T, expected map", zatcaRaw)
	}
	v, ok := zatcaMap["zatca_reconnect_required"]
	if !ok {
		t.Error("expected 'zatca_reconnect_required' inside zatca object")
	}
	if v != true {
		t.Errorf("expected true, got %v", v)
	}
}

// TestZatcaReconnectRequired_IndependentOfOtherZatcaFields ensures that setting
// ZatcaReconnectRequired does not affect Phase or Connected, and vice-versa.
func TestZatcaReconnectRequired_IndependentOfOtherZatcaFields(t *testing.T) {
	z := Zatca{Phase: "2", Connected: true, ZatcaReconnectRequired: true}
	data, _ := json.Marshal(z)
	var decoded Zatca
	json.Unmarshal(data, &decoded)
	if decoded.Phase != "2" {
		t.Errorf("Phase: got %q, want \"2\"", decoded.Phase)
	}
	if !decoded.Connected {
		t.Error("Connected should be true")
	}
	if !decoded.ZatcaReconnectRequired {
		t.Error("ZatcaReconnectRequired should be true")
	}
}

// ── RemoveInvoiceBackground field ─────────────────────────────────────────────

// TestInvoiceBg_RemoveInvoiceBackground_ZeroValueFalse verifies that the
// zero value of Store has RemoveInvoiceBackground == false.
func TestInvoiceBg_RemoveInvoiceBackground_ZeroValueFalse(t *testing.T) {
	var s Store
	if s.RemoveInvoiceBackground {
		t.Error("zero-value RemoveInvoiceBackground should be false")
	}
}

// TestInvoiceBg_RemoveInvoiceBackground_JSONKey verifies the field marshals
// to the expected JSON key "remove_invoice_background".
func TestInvoiceBg_RemoveInvoiceBackground_JSONKey(t *testing.T) {
	s := Store{RemoveInvoiceBackground: true}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	v, ok := m["remove_invoice_background"]
	if !ok {
		t.Error("expected JSON key 'remove_invoice_background' not found")
	}
	if v != true {
		t.Errorf("expected true, got %v", v)
	}
}

// TestInvoiceBg_RemoveInvoiceBackground_OmitEmptyWhenFalse verifies the field
// is absent from JSON when false (omitempty tag).
func TestInvoiceBg_RemoveInvoiceBackground_OmitEmptyWhenFalse(t *testing.T) {
	s := Store{RemoveInvoiceBackground: false}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	if _, ok := m["remove_invoice_background"]; ok {
		t.Error("remove_invoice_background must be absent from JSON when false (omitempty)")
	}
}

// TestInvoiceBg_RemoveInvoiceBackground_PresentWhenTrue verifies the field
// appears in JSON output when true.
func TestInvoiceBg_RemoveInvoiceBackground_PresentWhenTrue(t *testing.T) {
	s := Store{RemoveInvoiceBackground: true}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	if _, ok := m["remove_invoice_background"]; !ok {
		t.Error("remove_invoice_background must be present in JSON when true")
	}
}

// TestInvoiceBg_RemoveInvoiceBackground_RoundTrip verifies the field survives
// a JSON marshal/unmarshal round-trip for both true and false.
func TestInvoiceBg_RemoveInvoiceBackground_RoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		value bool
	}{
		{"true persists", true},
		{"false persists", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			original := Store{RemoveInvoiceBackground: c.value}
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var decoded Store
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if decoded.RemoveInvoiceBackground != c.value {
				t.Errorf("round-trip: got %v, want %v", decoded.RemoveInvoiceBackground, c.value)
			}
		})
	}
}

// ── InvoiceBackground and InvoiceBackgroundContent field tags ─────────────────

// TestInvoiceBg_InvoiceBackground_OmitEmptyWhenEmpty verifies the field is
// absent from JSON (and by the same omitempty bson tag, from bson) when empty.
func TestInvoiceBg_InvoiceBackground_OmitEmptyWhenEmpty(t *testing.T) {
	s := Store{}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	if _, ok := m["invoice_background"]; ok {
		t.Error("invoice_background must be absent from JSON when empty string (omitempty)")
	}
}

// TestInvoiceBg_InvoiceBackground_PresentWhenNonEmpty verifies the field
// appears in JSON with its value when set to a non-empty string.
func TestInvoiceBg_InvoiceBackground_PresentWhenNonEmpty(t *testing.T) {
	s := Store{InvoiceBackground: "invoice_background_abc123.png"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	v, ok := m["invoice_background"]
	if !ok {
		t.Error("expected JSON key 'invoice_background' not found")
	}
	if v != "invoice_background_abc123.png" {
		t.Errorf("invoice_background = %v, want \"invoice_background_abc123.png\"", v)
	}
}

// TestInvoiceBg_InvoiceBackgroundContent_OmitEmptyWhenEmpty verifies the
// json-only field is absent from JSON output when empty (omitempty tag).
func TestInvoiceBg_InvoiceBackgroundContent_OmitEmptyWhenEmpty(t *testing.T) {
	s := Store{}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	if _, ok := m["invoice_background_content"]; ok {
		t.Error("invoice_background_content must be absent from JSON when empty (omitempty)")
	}
}

// TestInvoiceBg_InvoiceBackgroundContent_PresentWhenNonEmpty verifies the
// field appears in JSON when set to a non-empty string.
func TestInvoiceBg_InvoiceBackgroundContent_PresentWhenNonEmpty(t *testing.T) {
	s := Store{InvoiceBackgroundContent: "data123"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}
	v, ok := m["invoice_background_content"]
	if !ok {
		t.Error("expected JSON key 'invoice_background_content' not found")
	}
	if v != "data123" {
		t.Errorf("invoice_background_content = %v, want \"data123\"", v)
	}
}

// ── SaveInvoiceBackgroundFile ─────────────────────────────────────────────────

// TestInvoiceBg_SaveInvoiceBackgroundFile exercises SaveInvoiceBackgroundFile
// for valid image content (success paths) and various invalid inputs (error
// paths).  Success tests chdir into t.TempDir() so the relative
// "images/<id>/store/..." path lands in a temporary location that is cleaned
// up automatically.
func TestInvoiceBg_SaveInvoiceBackgroundFile(t *testing.T) {
	// chdir switches the process working directory to dir and restores the
	// original directory at the end of the sub-test.
	chdir := func(t *testing.T, dir string) {
		t.Helper()
		orig, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd: %v", err)
		}
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("Chdir(%q): %v", dir, err)
		}
		t.Cleanup(func() { os.Chdir(orig) }) //nolint:errcheck
	}

	// minimalJPEGBase64 returns a base64-encoded JFIF header — enough bytes
	// for http.DetectContentType to return "image/jpeg".
	minimalJPEGBase64 := base64.StdEncoding.EncodeToString([]byte{
		0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10,
		0x4a, 0x46, 0x49, 0x46, 0x00, 0x01,
	})

	// 1×1 pixel PNG (valid, fully decodable).
	const minimalPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAAAAAA6fptVAAAACklEQVQI12NgAAAAAgAB4iG8MwAAAABJRU5ErkJggg=="

	t.Run("valid PNG saves file and clears InvoiceBackgroundContent", func(t *testing.T) {
		chdir(t, t.TempDir())
		id := primitive.NewObjectID()
		store := &Store{ID: id, InvoiceBackgroundContent: minimalPNGBase64}
		if err := store.SaveInvoiceBackgroundFile(); err != nil {
			t.Fatalf("SaveInvoiceBackgroundFile: %v", err)
		}
		if !strings.HasSuffix(store.InvoiceBackground, ".png") {
			t.Errorf("InvoiceBackground = %q, want suffix .png", store.InvoiceBackground)
		}
		if !strings.Contains(store.InvoiceBackground, "invoice_background_") {
			t.Errorf("InvoiceBackground = %q, missing prefix invoice_background_", store.InvoiceBackground)
		}
		if store.InvoiceBackgroundContent != "" {
			t.Errorf("InvoiceBackgroundContent = %q, want empty after save", store.InvoiceBackgroundContent)
		}
	})

	t.Run("data URI prefix is stripped before decode (PNG)", func(t *testing.T) {
		chdir(t, t.TempDir())
		id := primitive.NewObjectID()
		store := &Store{ID: id, InvoiceBackgroundContent: "data:image/png;base64," + minimalPNGBase64}
		if err := store.SaveInvoiceBackgroundFile(); err != nil {
			t.Fatalf("SaveInvoiceBackgroundFile with data URI: %v", err)
		}
		if !strings.HasSuffix(store.InvoiceBackground, ".png") {
			t.Errorf("InvoiceBackground = %q, want suffix .png", store.InvoiceBackground)
		}
		if store.InvoiceBackgroundContent != "" {
			t.Errorf("InvoiceBackgroundContent = %q, want empty after save", store.InvoiceBackgroundContent)
		}
	})

	t.Run("valid JPEG saves file with .jpg extension", func(t *testing.T) {
		chdir(t, t.TempDir())
		id := primitive.NewObjectID()
		store := &Store{ID: id, InvoiceBackgroundContent: minimalJPEGBase64}
		if err := store.SaveInvoiceBackgroundFile(); err != nil {
			t.Fatalf("SaveInvoiceBackgroundFile: %v", err)
		}
		if !strings.HasSuffix(store.InvoiceBackground, ".jpg") {
			t.Errorf("InvoiceBackground = %q, want suffix .jpg", store.InvoiceBackground)
		}
	})

	t.Run("filename is invoice_background_<hex>.png for PNG", func(t *testing.T) {
		chdir(t, t.TempDir())
		id := primitive.NewObjectID()
		store := &Store{ID: id, InvoiceBackgroundContent: minimalPNGBase64}
		if err := store.SaveInvoiceBackgroundFile(); err != nil {
			t.Fatalf("SaveInvoiceBackgroundFile: %v", err)
		}
		want := "invoice_background_" + id.Hex() + ".png"
		if store.InvoiceBackground != want {
			t.Errorf("InvoiceBackground = %q, want %q", store.InvoiceBackground, want)
		}
	})

	// Base64 decode error cases — the function returns before touching the
	// filesystem so no chdir is needed.
	decodeErrorCases := []struct {
		name    string
		content string
	}{
		{
			name:    "invalid base64 string returns decode error",
			content: "!!!not-base64!!!",
		},
	}
	for _, c := range decodeErrorCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			id := primitive.NewObjectID()
			store := &Store{ID: id, InvoiceBackgroundContent: c.content}
			if err := store.SaveInvoiceBackgroundFile(); err == nil {
				t.Fatalf("expected error, got nil (InvoiceBackground=%q)", store.InvoiceBackground)
			}
		})
	}

	// GetFileExtensionFromBase64 falls back to mime.ExtensionsByType for any
	// MIME type not in its explicit switch, so plain-text content (including
	// empty bytes, which DetectContentType reports as text/plain) succeeds and
	// produces a .txt file rather than an error.
	t.Run("plain text base64 saves as .txt extension", func(t *testing.T) {
		chdir(t, t.TempDir())
		id := primitive.NewObjectID()
		store := &Store{
			ID:                       id,
			InvoiceBackgroundContent: base64.StdEncoding.EncodeToString([]byte("hello world")),
		}
		if err := store.SaveInvoiceBackgroundFile(); err != nil {
			t.Fatalf("SaveInvoiceBackgroundFile: %v", err)
		}
		if !strings.HasSuffix(store.InvoiceBackground, ".txt") {
			t.Errorf("InvoiceBackground = %q, want suffix .txt for plain-text content", store.InvoiceBackground)
		}
	})

	t.Run("empty InvoiceBackgroundContent saves as .txt extension", func(t *testing.T) {
		chdir(t, t.TempDir())
		id := primitive.NewObjectID()
		// Empty string decodes to zero bytes; DetectContentType returns
		// "text/plain; charset=utf-8", which mime.ExtensionsByType resolves to .txt.
		store := &Store{ID: id, InvoiceBackgroundContent: ""}
		if err := store.SaveInvoiceBackgroundFile(); err != nil {
			t.Fatalf("SaveInvoiceBackgroundFile: %v", err)
		}
		if !strings.HasSuffix(store.InvoiceBackground, ".txt") {
			t.Errorf("InvoiceBackground = %q, want suffix .txt for empty content", store.InvoiceBackground)
		}
	})
}

// ── RemoveLogo field ──────────────────────────────────────────────────────────

func TestRemoveLogo_ZeroValueFalse(t *testing.T) {
	var s Store
	if s.RemoveLogo {
		t.Error("zero-value RemoveLogo must be false")
	}
}

func TestRemoveLogo_JSONKey(t *testing.T) {
	s := Store{RemoveLogo: true}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"remove_logo"`) {
		t.Errorf("expected JSON key \"remove_logo\" in %s", data)
	}
}

func TestRemoveLogo_OmitEmptyWhenFalse(t *testing.T) {
	s := Store{RemoveLogo: false}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(data), `"remove_logo"`) {
		t.Errorf("remove_logo must be omitted when false, got %s", data)
	}
}

func TestRemoveLogo_PresentWhenTrue(t *testing.T) {
	s := Store{RemoveLogo: true}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	v, ok := out["remove_logo"]
	if !ok {
		t.Fatal("remove_logo key missing from JSON")
	}
	if v != true {
		t.Errorf("remove_logo = %v, want true", v)
	}
}

func TestRemoveLogo_RoundTrip(t *testing.T) {
	for _, want := range []bool{true, false} {
		s := Store{RemoveLogo: want}
		data, _ := json.Marshal(s)
		var s2 Store
		if err := json.Unmarshal(data, &s2); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if s2.RemoveLogo != want {
			t.Errorf("round-trip RemoveLogo: got %v, want %v", s2.RemoveLogo, want)
		}
	}
}

func TestRemoveLogo_NoBSONTag(t *testing.T) {
	// RemoveLogo is a transient JSON-only flag that must never be persisted
	// to MongoDB via $set. Verify by inspecting the struct field tag.
	ft, ok := reflect.TypeOf(Store{}).FieldByName("RemoveLogo")
	if !ok {
		t.Fatal("Store has no field RemoveLogo")
	}
	if tag := string(ft.Tag); strings.Contains(tag, "bson:") {
		t.Errorf("RemoveLogo must have no bson tag, got %q", tag)
	}
}

func TestRemoveLogo_IndependentOfRemoveInvoiceBackground(t *testing.T) {
	// The two remove flags are independent — setting one must not affect the other.
	s := Store{RemoveLogo: true, RemoveInvoiceBackground: false}
	data, _ := json.Marshal(s)
	var out map[string]interface{}
	json.Unmarshal(data, &out)
	if out["remove_logo"] != true {
		t.Errorf("remove_logo = %v, want true", out["remove_logo"])
	}
	if _, ok := out["remove_invoice_background"]; ok {
		t.Error("remove_invoice_background must be absent when false")
	}
}

func TestRemoveLogo_BothFlagsTrue(t *testing.T) {
	s := Store{RemoveLogo: true, RemoveInvoiceBackground: true}
	data, _ := json.Marshal(s)
	var out map[string]interface{}
	json.Unmarshal(data, &out)
	if out["remove_logo"] != true {
		t.Errorf("remove_logo = %v, want true", out["remove_logo"])
	}
	if out["remove_invoice_background"] != true {
		t.Errorf("remove_invoice_background = %v, want true", out["remove_invoice_background"])
	}
}

// ── TrimSpaceFromFields ───────────────────────────────────────────────────────

func TestTrimSpaceFromFields(t *testing.T) {
	cases := []struct {
		name  string
		input Store
		check func(t *testing.T, got Store)
	}{
		{
			name: "BusinessCategory leading/trailing spaces trimmed",
			input: Store{BusinessCategory: "  Retail  "},
			check: func(t *testing.T, got Store) {
				if got.BusinessCategory != "Retail" {
					t.Errorf("BusinessCategory = %q, want %q", got.BusinessCategory, "Retail")
				}
			},
		},
		{
			name: "Name with mixed whitespace trimmed",
			input: Store{Name: "\t My Store \n"},
			check: func(t *testing.T, got Store) {
				if got.Name != "My Store" {
					t.Errorf("Name = %q, want %q", got.Name, "My Store")
				}
			},
		},
		{
			name: "NameInArabic trimmed",
			input: Store{NameInArabic: "  مخزن  "},
			check: func(t *testing.T, got Store) {
				if got.NameInArabic != "مخزن" {
					t.Errorf("NameInArabic = %q, want %q", got.NameInArabic, "مخزن")
				}
			},
		},
		{
			name: "VATNo trimmed",
			input: Store{VATNo: " 310000000000003 "},
			check: func(t *testing.T, got Store) {
				if got.VATNo != "310000000000003" {
					t.Errorf("VATNo = %q, want %q", got.VATNo, "310000000000003")
				}
			},
		},
		{
			name: "NationalAddress.ShortCode trimmed",
			input: Store{NationalAddress: NationalAddress{ShortCode: "  SC1 "}},
			check: func(t *testing.T, got Store) {
				if got.NationalAddress.ShortCode != "SC1" {
					t.Errorf("ShortCode = %q, want %q", got.NationalAddress.ShortCode, "SC1")
				}
			},
		},
		{
			name: "NationalAddress.BuildingNo trimmed",
			input: Store{NationalAddress: NationalAddress{BuildingNo: " 1234 "}},
			check: func(t *testing.T, got Store) {
				if got.NationalAddress.BuildingNo != "1234" {
					t.Errorf("BuildingNo = %q, want %q", got.NationalAddress.BuildingNo, "1234")
				}
			},
		},
		{
			name: "RegistrationNumber trimmed",
			input: Store{RegistrationNumber: "  CR123  "},
			check: func(t *testing.T, got Store) {
				if got.RegistrationNumber != "CR123" {
					t.Errorf("RegistrationNumber = %q, want %q", got.RegistrationNumber, "CR123")
				}
			},
		},
		{
			name: "SalesSerialNumber.Prefix trimmed",
			input: Store{SalesSerialNumber: SerialNumber{Prefix: " INV "}},
			check: func(t *testing.T, got Store) {
				if got.SalesSerialNumber.Prefix != "INV" {
					t.Errorf("SalesSerialNumber.Prefix = %q, want %q", got.SalesSerialNumber.Prefix, "INV")
				}
			},
		},
		{
			name: "empty string stays empty",
			input: Store{Name: ""},
			check: func(t *testing.T, got Store) {
				if got.Name != "" {
					t.Errorf("Name = %q, want empty", got.Name)
				}
			},
		},
		{
			name: "fields with no whitespace unchanged",
			input: Store{
				Name:               "Store",
				BusinessCategory:   "Tech",
				RegistrationNumber: "REG1",
			},
			check: func(t *testing.T, got Store) {
				if got.Name != "Store" || got.BusinessCategory != "Tech" || got.RegistrationNumber != "REG1" {
					t.Errorf("fields should be unchanged: Name=%q BusinessCategory=%q Reg=%q",
						got.Name, got.BusinessCategory, got.RegistrationNumber)
				}
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			s := c.input
			s.TrimSpaceFromFields()
			c.check(t, s)
		})
	}
}

// TestTrimSpaceFromFields_WhitespaceFalsePositivePrevention verifies the root
// cause of the Business Category false-positive reconnect bug:
// if both stores have the same value but one has trailing whitespace (e.g. from
// legacy DB data), TrimSpaceFromFields normalizes them to equal strings.
func TestTrimSpaceFromFields_WhitespaceFalsePositivePrevention(t *testing.T) {
	// Simulate legacy DB value with trailing space vs. frontend-trimmed value.
	oldStore := Store{
		Name:             "Store",
		BusinessCategory: "Retail ",  // legacy DB value with trailing space
		VATNo:            "123456789",
	}
	newStore := Store{
		Name:             "Store",
		BusinessCategory: "Retail",   // frontend trimmed
		VATNo:            "123456789",
	}

	// Without trimming: the fields appear different
	if oldStore.BusinessCategory == newStore.BusinessCategory {
		t.Error("pre-condition failed: expected them to appear different before trimming")
	}

	// After trimming: they should be equal
	oldStore.TrimSpaceFromFields()
	newStore.TrimSpaceFromFields()

	if oldStore.BusinessCategory != newStore.BusinessCategory {
		t.Errorf("after TrimSpaceFromFields, BusinessCategory should match: %q != %q",
			oldStore.BusinessCategory, newStore.BusinessCategory)
	}
}
