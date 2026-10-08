package erp

import (
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

const testGSTIN = "27AAPFU0939F1ZV" // Maharashtra (27)

func indiaStore() M { return M{"country_code": "IN"} }

func indiaSignup() M {
	b := validSignup()
	c := b["company"].(M)
	c["countryCode"] = "IN"
	c["nameAr"] = ""
	c["vatNo"] = testGSTIN
	c["crNo"] = "AAPFU0939F"
	c["mobile"] = "02226543210"
	b["owner"].(M)["mobile"] = "9876543210"
	c["address"] = M{"streetEn": "MG Road", "cityEn": "Mumbai", "stateCode": "27", "postalCode": "400001"}
	return b
}

func TestValidateSignup_India(t *testing.T) {
	if e := ValidateSignup(indiaSignup()); len(e) != 0 {
		t.Fatalf("valid India sign-up rejected: %v", e)
	}
	tests := []struct {
		name  string
		mut   func(M)
		field string
	}{
		{"GSTIN optional", func(c M) { c["vatNo"] = "" }, ""},
		{"PAN optional", func(c M) { c["crNo"] = "" }, ""},
		{"Arabic name optional", func(c M) { delete(c, "nameAr") }, ""},
		{"bad GSTIN checksum", func(c M) { c["vatNo"] = "27AAPFU0939F1ZA" }, "company.vatNo"},
		{"GSTIN from another state", func(c M) { c["address"].(M)["stateCode"] = "32" }, "company.vatNo"},
		{"bad PAN", func(c M) { c["crNo"] = "1010101010" }, "company.crNo"},
		{"state required", func(c M) { c["address"].(M)["stateCode"] = "" }, "company.address.stateCode"},
		{"PIN required", func(c M) { c["address"].(M)["postalCode"] = "" }, "company.address.postalCode"},
		{"PIN is 6 digits", func(c M) { c["address"].(M)["postalCode"] = "40001" }, "company.address.postalCode"},
		{"unknown state", func(c M) { c["vatNo"] = ""; c["address"].(M)["stateCode"] = "99" }, "company.address.stateCode"},
		{"Indian landline with +91", func(c M) { c["mobile"] = "+912226543210" }, ""},
		{"Saudi phone", func(c M) { c["mobile"] = "0512345678" }, "company.mobile"},
	}
	for _, tc := range tests {
		b := indiaSignup()
		tc.mut(b["company"].(M))
		e := ValidateSignup(b)
		if tc.field == "" && len(e) != 0 {
			t.Errorf("%s: unexpected %v", tc.name, e)
		}
		if tc.field != "" && e[tc.field] == "" {
			t.Errorf("%s: expected error on %s, got %v", tc.name, tc.field, e)
		}
	}
	// an Indian owner mobile is accepted; a Saudi one is not
	b := indiaSignup()
	b["owner"].(M)["mobile"] = "0512345678"
	if e := ValidateSignup(b); e["owner.mobile"] == "" {
		t.Error("Saudi owner mobile accepted for India")
	}
}

func TestSignupAddressExtras_India(t *testing.T) {
	cp := models.CountryProfileFor("IN")
	x := signupAddressExtras(cp, M{"stateCode": "32", "cityEn": "Kochi"})
	if x["stateCode"] != "32" || x["stateEn"] != "Kerala" {
		t.Errorf("India address extras: %v", x)
	}
	if x := signupAddressExtras(models.CountryProfileFor("AE"), M{"stateCode": "32"}); x["stateCode"] != nil && x["stateCode"] != "" {
		t.Errorf("state saved for the UAE: %v", x)
	}
}

func TestStoreCreateErrors_India(t *testing.T) {
	ok := M{"nameEn": "Navya Grocery", "countryCode": "IN", "email": "b@x.example", "phone": "02226543210",
		"category": "Grocery", "address": M{"streetEn": "MG Road", "cityEn": "Mumbai", "stateCode": "27", "postalCode": "400001"}}
	if e := storeCreateErrors(ok); len(e) != 0 {
		t.Fatalf("valid India store rejected (no Arabic name, GSTIN or PAN needed): %v", e)
	}
	for _, f := range []string{"stateCode", "postalCode"} {
		b := cloneM(ok)
		b["address"].(M)[f] = ""
		if e := storeCreateErrors(b); e["address."+f] == "" {
			t.Errorf("address.%s must be required in India: %v", f, e)
		}
	}
}

func TestStoreValidate_India(t *testing.T) {
	withNoDocs(t, false)
	x := newMapCtx(nil, "")
	base := func(extra M) M {
		r := M{"nameEn": "A", "category": "Grocery", "countryCode": "IN"}
		for k, v := range extra {
			r[k] = v
		}
		return r
	}
	tests := []struct {
		name  string
		rec   M
		field string
	}{
		{"GSTIN", base(M{"vatNo": testGSTIN, "address": M{"stateCode": "27"}}), ""},
		{"GSTIN without state", base(M{"vatNo": testGSTIN}), ""},
		{"GSTIN state mismatch", base(M{"vatNo": testGSTIN, "address": M{"stateCode": "29"}}), "vatNo"},
		{"bad GSTIN", base(M{"vatNo": "27AAPFU0939F1ZA"}), "vatNo"},
		{"PAN", base(M{"crNo": "AAPFU0939F"}), ""},
		{"bad PAN", base(M{"crNo": "12345"}), "crNo"},
		{"GST 18%", base(M{"vatPercent": 18.0}), ""},
		{"GST 12% (pre-2025 slab)", base(M{"vatPercent": 12.0}), "vatPercent"},
		{"PIN", base(M{"address": M{"postalCode": "682016"}}), ""},
		{"bad PIN", base(M{"address": M{"postalCode": "68201"}}), "address.postalCode"},
		{"unknown state", base(M{"address": M{"stateCode": "99"}}), "address.stateCode"},
	}
	for _, tc := range tests {
		e := storeValidate(x, tc.rec, nil)
		if tc.field == "" && len(e) > 0 {
			t.Errorf("%s: unexpected %v", tc.name, e)
		}
		if tc.field != "" && e[tc.field] == "" {
			t.Errorf("%s: expected error on %s, got %v", tc.name, tc.field, e)
		}
	}
}

func TestPartyValidate_India(t *testing.T) {
	x := newMapCtxForStore(indiaStore())
	tests := []struct {
		name  string
		rec   M
		field string
	}{
		{"GSTIN matching state", M{"nameEn": "Kumar Traders", "vatNo": testGSTIN, "address": M{"stateCode": "27"}}, ""},
		{"GSTIN, state not given", M{"nameEn": "Kumar Traders", "vatNo": testGSTIN}, ""},
		{"GSTIN other state", M{"nameEn": "Kumar Traders", "vatNo": testGSTIN, "address": M{"stateCode": "32"}}, "vatNo"},
		{"bad checksum", M{"nameEn": "Kumar Traders", "vatNo": "27AAPFU0939F1ZA"}, "vatNo"},
		{"Indian mobile", M{"nameEn": "Kumar Traders", "phone": "9876543210"}, ""},
		{"Saudi mobile", M{"nameEn": "Kumar Traders", "phone": "0512345678"}, "phone"},
		{"bad PIN", M{"nameEn": "Kumar Traders", "address": M{"postalCode": "1234"}}, "address.postalCode"},
	}
	for _, tc := range tests {
		e := partyValidate(x, tc.rec, nil)
		if tc.field == "" && len(e) > 0 {
			t.Errorf("%s: unexpected %v", tc.name, e)
		}
		if tc.field != "" && e[tc.field] == "" {
			t.Errorf("%s: expected error on %s, got %v", tc.name, tc.field, e)
		}
	}
	// a GCC store ignores state codes and takes its own tax numbers
	if e := partyValidate(newMapCtxForStore(M{"country_code": "AE"}), M{"nameEn": "AB", "vatNo": "100123456700003"}, nil); len(e) != 0 {
		t.Errorf("UAE party: %v", e)
	}
}

func TestProductValidate_IndiaHSN(t *testing.T) {
	x := newMapCtxForStore(indiaStore())
	tests := []struct {
		hsn string
		ok  bool
	}{{"0401", true}, {"040110", true}, {"04011000", true}, {"998313", true}, {"04011", false}, {"HSN1", false}, {"040110001", false}, {"", true}}
	for _, tc := range tests {
		e := productValidate(x, M{"nameEn": "Milk", "hsn": tc.hsn}, nil)
		if (e["hsn"] == "") != tc.ok {
			t.Errorf("hsn %q: %v", tc.hsn, e)
		}
	}
	if e := productValidate(x, M{"nameEn": "Milk", "vatPercent": 5.0}, nil); e["vatPercent"] != "" {
		t.Errorf("GST 5%%: %v", e)
	}
	if e := productValidate(x, M{"nameEn": "Milk", "vatPercent": 12.0}, nil); e["vatPercent"] == "" {
		t.Error("GST 12% must be refused after GST 2.0")
	}
	// Saudi products keep any rate
	if e := productValidate(newMapCtx(nil, ""), M{"nameEn": "Milk", "vatPercent": 12.0}, nil); e["vatPercent"] != "" {
		t.Errorf("SA product rate: %v", e)
	}
}

func TestAutoRounding_India(t *testing.T) {
	in, sa := models.CountryProfileFor("IN"), models.CountryProfileFor("SA")
	tests := []struct {
		p      *models.CountryProfile
		before float64
		want   float64
	}{
		{in, 472.85, 0.15},
		{in, 472.49, -0.49},
		{in, 472.5, 0.5},
		{in, 473, 0},
		{sa, 10.02, -0.02},
		{sa, 10.03, 0.02},
		{nil, 10.03, 0.02},
	}
	for _, tc := range tests {
		if got := autoRounding(tc.before, tc.p); got != tc.want {
			t.Errorf("autoRounding(%v, %v) = %v, want %v", tc.before, tc.p != nil && tc.p.Code == "IN", got, tc.want)
		}
	}
}

func TestStarterPrice_India(t *testing.T) {
	tests := []struct {
		sar, want float64
	}{{2, 30}, {6.5, 98}, {21, 315}, {52, 780}, {0.05, 1}, {100, 1500}, {123.45, 1850}}
	for _, tc := range tests {
		if got := starterPrice(tc.sar, "IN", 2); got != tc.want {
			t.Errorf("starterPrice(%v, IN) = %v, want %v", tc.sar, got, tc.want)
		}
	}
	if got := starterPrice(2, "SA", 2); got != 2 {
		t.Errorf("SA price changed: %v", got)
	}
}

func TestGSTINStateError(t *testing.T) {
	in := models.CountryProfileFor("IN")
	if gstinStateError(in, testGSTIN, M{"stateCode": "27"}) != "" || gstinStateError(in, testGSTIN, M{}) != "" {
		t.Error("matching or missing state must pass")
	}
	if msg := gstinStateError(in, testGSTIN, M{"stateCode": "32"}); msg == "" {
		t.Error("mismatch must fail")
	} else if want := "GSTIN state code 27 (Maharashtra) does not match the address state 32 (Kerala)"; msg != want {
		t.Errorf("message %q", msg)
	}
	if gstinStateError(models.CountryProfileFor("SA"), testGSTIN, M{"stateCode": "32"}) != "" {
		t.Error("only India checks states")
	}
	if partyStateCode(testGSTIN, M{"stateCode": "32"}) != "27" || partyStateCode("", M{"stateCode": "32"}) != "32" {
		t.Error("partyStateCode prefers the GSTIN")
	}
}
