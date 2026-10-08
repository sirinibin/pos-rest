package models

import "testing"

func TestGSTINChecksum(t *testing.T) {
	tests := []struct {
		gstin string
		ok    bool
	}{
		{"27AAPFU0939F1ZV", true},   // published example (Maharashtra)
		{"27aapfu0939f1zv", true},   // case-insensitive
		{" 27AAPFU0939F1ZV ", true}, // trimmed
		{"27AAPFU0939F1ZA", false},  // wrong check character
		{"27AAPFU0939F1Z", false},   // 14 characters
		{"99AAPFU0939F1ZV", false},  // unknown state
		{"27AAPFU0939F0ZV", false},  // entity number 0
		{"27AAPFU0939F1XV", false},  // 14th must be Z
		{"", false},
	}
	for _, tc := range tests {
		if got := ValidGSTIN(tc.gstin); got != tc.ok {
			t.Errorf("ValidGSTIN(%q) = %v, want %v", tc.gstin, got, tc.ok)
		}
	}
	if c := GSTINCheckChar("27AAPFU0939F1Z"); c != "V" {
		t.Errorf("check char %q, want V", c)
	}
	// every state code makes a valid GSTIN with its computed check character
	for _, s := range IndiaStates {
		g := s.Code + "ABCDE1234F1Z"
		if !ValidGSTIN(g + GSTINCheckChar(g)) {
			t.Errorf("%s: computed GSTIN rejected", s.Code)
		}
	}
}

func TestPANAndGSTINState(t *testing.T) {
	for _, tc := range []struct {
		pan string
		ok  bool
	}{{"ABCDE1234F", true}, {"abcde1234f", true}, {"ABCD1234F", false}, {"ABCDE12345", false}, {"", false}} {
		if ValidPAN(tc.pan) != tc.ok {
			t.Errorf("ValidPAN(%q) != %v", tc.pan, tc.ok)
		}
	}
	if GSTINState("32AAPFU0939F1ZV") != "32" || GSTINState("bad") != "" {
		t.Error("GSTINState")
	}
}

func TestSplitGST(t *testing.T) {
	tests := []struct {
		name             string
		tax              float64
		seller, buyer    string
		inter            bool
		cgst, sgst, igst float64
	}{
		{"same state", 180, "27", "27", false, 90, 90, 0},
		{"odd paise go to SGST", 18.01, "27", "27", false, 9.01, 9, 0},
		{"walk-in (no place of supply)", 36, "32", "", false, 18, 18, 0},
		{"other state", 180, "27", "29", true, 0, 0, 180},
		{"zero", 0, "27", "07", true, 0, 0, 0},
	}
	for _, tc := range tests {
		g := SplitGST(tc.tax, tc.seller, tc.buyer)
		if g.Inter != tc.inter || g.CGST != tc.cgst || g.SGST != tc.sgst || g.IGST != tc.igst {
			t.Errorf("%s: %+v", tc.name, g)
		}
		if !g.Inter && RoundTo2Decimals(g.CGST+g.SGST) != RoundTo2Decimals(tc.tax) {
			t.Errorf("%s: CGST+SGST %v != %v", tc.name, g.CGST+g.SGST, tc.tax)
		}
	}
}

func TestIndiaProfile(t *testing.T) {
	p := CountryProfileFor("IN")
	if p == nil {
		t.Fatal("India missing")
	}
	if p.GCC || p.CRRequired || p.TaxSplit != "gst" || p.RoundingStep != 1 || p.TaxNameEn != "GST" || p.CurrencyCode != "INR" {
		t.Errorf("profile: %+v", p)
	}
	for _, r := range []float64{0, 5, 18, 40, 3, 0.25} {
		if !p.ValidVatRate(r) {
			t.Errorf("GST rate %v rejected", r)
		}
	}
	for _, r := range []float64{12, 28, 15} {
		if p.ValidVatRate(r) {
			t.Errorf("old/foreign rate %v accepted", r)
		}
	}
	// GCC countries take any rate the store sets
	if !CountryProfileFor("SA").ValidVatRate(15) {
		t.Error("SA 15%")
	}
	if !p.ValidTaxID("27AAPFU0939F1ZV") || p.ValidTaxID("27AAPFU0939F1ZA") {
		t.Error("ValidTaxID must apply the GSTIN checksum")
	}
	if !p.ValidCR("abcde1234f") || p.ValidCR("1010101010") {
		t.Error("India CR is the PAN")
	}
	for _, tc := range []struct {
		v  string
		ok bool
	}{{"9876543210", true}, {"+919876543210", true}, {"09876543210", true}, {"5876543210", false}, {"98765", false}} {
		if p.ValidMobile(tc.v) != tc.ok {
			t.Errorf("mobile %q", tc.v)
		}
	}
	if !p.ValidPostal("682016") || p.ValidPostal("082016") || p.ValidPostal("68201") {
		t.Error("PIN code rule")
	}
	if p.StateName("32") != "Kerala" || p.StateByName("kerala") == nil || p.StateName("99") != "" {
		t.Error("state lookup")
	}
	if len(p.States) != 37 {
		t.Errorf("%d states", len(p.States))
	}
}

func TestStoreCountryLegacyChecks(t *testing.T) {
	sa, in, ae := &Store{CountryCode: "SA"}, &Store{CountryCode: "IN"}, &Store{CountryCode: "AE"}
	phones := []struct {
		store *Store
		phone string
		ok    bool
	}{
		{sa, "0512345678", true},
		{sa, "9876543210", false},
		{nil, "0512345678", true}, // no store: Saudi rules
		{&Store{}, "0512345678", true},
		{in, "9876543210", true},
		{in, "+912226543210", true},
		{in, "0512345678", false},
		{ae, "0501234567", true},
	}
	for _, tc := range phones {
		if ValidStorePhone(tc.store, tc.phone) != tc.ok {
			t.Errorf("ValidStorePhone(%v, %q) != %v", tc.store, tc.phone, tc.ok)
		}
	}
	vats := []struct {
		store *Store
		vat   string
		want  string
	}{
		{sa, "310122393500003", ""},
		{sa, "31012239350000", "VAT No. should be 15 digits"},
		{sa, "100123456700003", "VAT No. should start and end with 3"},
		{sa, "", ""},
		{in, "27AAPFU0939F1ZV", ""},
		{in, " 27AAPFU0939F1ZV ", ""},
		{in, "27AAPFU0939F1ZA", "GSTIN: " + CountryProfileFor("IN").TaxIDHint},
		{ae, "100123456700003", ""},
	}
	for _, tc := range vats {
		if got := StoreVATNoError(tc.store, tc.vat); got != tc.want {
			t.Errorf("StoreVATNoError(%s, %q) = %q, want %q", tc.store.CountryCode, tc.vat, got, tc.want)
		}
	}
}
