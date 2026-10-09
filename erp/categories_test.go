package erp

import (
	"strings"
	"testing"
)

// Pure-function tests for business categories (ZATCA-safe values that pick
// the store's POS terminal) and the product POS catalog fields.

func TestBusinessCategories_ZatcaSafeAndUnique(t *testing.T) {
	if len(BusinessCategories) != 29 {
		t.Fatalf("expected 29 POS categories (26 + 3 India-only), got %d", len(BusinessCategories))
	}
	values, terminals := map[string]bool{}, map[string]bool{}
	for _, c := range BusinessCategories {
		if !ValidZatcaCategory(c.Value) {
			t.Errorf("%q is not ZATCA-safe (letters and single spaces, 2-64 chars)", c.Value)
		}
		lv := strings.ToLower(c.Value)
		if values[lv] {
			t.Errorf("duplicate category %q", c.Value)
		}
		if terminals[c.Terminal] {
			t.Errorf("terminal %q used by two categories", c.Terminal)
		}
		values[lv], terminals[c.Terminal] = true, true
		if !posTerminals[c.Terminal] {
			t.Errorf("terminal %q not a known POS terminal", c.Terminal)
		}
	}
}

func TestBusinessCategories_TerminalMapping(t *testing.T) {
	cases := map[string]string{
		"Grocery": "grocery", "Supermarket": "supermarket", "Restaurant": "restaurant", "Coffee Shop": "coffee",
		"Auto Spare Parts": "parts", "Auto Repair Workshop": "workshop", "Trading": "trading",
		"Ladies Beauty Salon": "salon", "Barber Shop": "barber", "Thobe Tailoring": "thobe",
		"Construction and Contracting": "construction", "Software and IT Services": "softwaresa",
		"Gaming and Entertainment": "gamingsa", "Mobile Phones and Accessories": "mobile",
		"Business Visa and Travels": "travel", "business visa and travels": "travel",
		// the display spelling is not ZATCA-safe and is not a stored value
		"Business, Visa & Travels": "",
		// case-insensitive and trimmed (legacy values such as "restaurant", "TRADING")
		"restaurant": "restaurant", "  TRADING ": "trading",
		// free text legacy values open no terminal
		"retail": "", "Retail": "", "Auto parts & industrial supplies": "", "": "",
	}
	for in, want := range cases {
		if got := CategoryTerminal(in); got != want {
			t.Errorf("CategoryTerminal(%q) = %q, want %q", in, got, want)
		}
	}
	if v, ok := CanonicalCategory("coffee shop"); !ok || v != "Coffee Shop" {
		t.Fatalf("canonical: %q %v", v, ok)
	}
	if _, ok := CanonicalCategory("Coffee"); ok {
		t.Fatal("partial name must not match")
	}
}

func TestValidZatcaCategory(t *testing.T) {
	good := []string{"Trading", "Supply activities", "Coffee Shop"}
	bad := []string{"", "A", "Café", "Food & drink", "Retail/Wholesale", " Trading", "Trading ", "Two  spaces",
		"مطعم", strings.Repeat("a", 65), "Auto-parts", "Shop1"}
	for _, s := range good {
		if !ValidZatcaCategory(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range bad {
		if ValidZatcaCategory(s) {
			t.Errorf("%q should be rejected", s)
		}
	}
}

func TestStoreValidate_Category(t *testing.T) {
	x := newMapCtx(nil, "")
	tests := []struct {
		name string
		rec  M
		prev M
		err  string
	}{
		{"create needs a category", M{"nameEn": "A"}, nil, "required"},
		{"create rejects free text", M{"nameEn": "A", "category": "Food & drink"}, nil, "choose a business category from the list"},
		{"create accepts a category", M{"nameEn": "A", "category": "Grocery"}, nil, ""},
		{"create accepts any case", M{"nameEn": "A", "category": "grocery"}, nil, ""},
		{"unchanged legacy value is kept", M{"nameEn": "A", "category": "Auto parts & industrial supplies"}, M{"business_category": "Auto parts & industrial supplies"}, ""},
		{"changed value must be in the list", M{"nameEn": "A", "category": "Bakery things"}, M{"business_category": "Retail"}, "choose a business category from the list"},
		{"changed to a category", M{"nameEn": "A", "category": "Barber Shop"}, M{"business_category": "Retail"}, ""},
		{"cleared value", M{"nameEn": "A", "category": ""}, M{"business_category": "Retail"}, "required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := storeValidate(x, tc.rec, tc.prev)
			if e["category"] != tc.err {
				t.Fatalf("category error %q, want %q (all: %v)", e["category"], tc.err, e)
			}
		})
	}
}

func TestStoreToLegacy_CanonicalCategory(t *testing.T) {
	p, err := storeToLegacy(newMapCtx(nil, ""), M{"category": "  coffee shop "}, M{}, map[string]bool{"category": true}, false)
	if err != nil || p["business_category"] != "Coffee Shop" {
		t.Fatalf("business_category=%v err=%v", p["business_category"], err)
	}
	p, _ = storeToLegacy(newMapCtx(nil, ""), M{"category": "Retail"}, M{}, map[string]bool{"category": true}, false)
	if p["business_category"] != "Retail" {
		t.Fatalf("unchanged legacy text: %v", p["business_category"])
	}
	p, _ = storeToLegacy(newMapCtx(nil, ""), M{"category": "Grocery"}, M{}, map[string]bool{"nameEn": true}, false)
	if _, set := p["business_category"]; set {
		t.Fatal("category written although unchanged")
	}
}

func TestStoreToContract_PosTerminal(t *testing.T) {
	r := storeToContract(newMapCtx(nil, ""), M{"name": "S", "business_category": "Yemeni Restaurant"})
	if r["category"] != "Yemeni Restaurant" || r["posTerminal"] != "yemeni" {
		t.Fatalf("category=%v posTerminal=%v", r["category"], r["posTerminal"])
	}
	r = storeToContract(newMapCtx(nil, ""), M{"name": "S", "business_category": "Retail"})
	if r["posTerminal"] != "" {
		t.Fatalf("legacy text posTerminal=%v", r["posTerminal"])
	}
	if !storeKnown["posTerminal"] {
		t.Fatal("posTerminal must be a known (derived) field, never saved to erp.x")
	}
}

func TestStoreCreateErrors(t *testing.T) {
	ok := M{"nameEn": "Branch", "nameAr": "فرع", "vatNo": "310122393500003", "crNo": "1010101010", "email": "b@x.example",
		"phone": "0112345678", "category": "Grocery",
		"address": M{"buildingNo": "1234", "streetEn": "Olaya", "streetAr": "العليا", "districtEn": "Olaya", "districtAr": "العليا",
			"cityEn": "Riyadh", "postalCode": "12345"}}
	if e := storeCreateErrors(ok); len(e) != 0 {
		t.Fatalf("valid store rejected: %v", e)
	}
	cases := []struct{ path, value, field string }{
		{"nameAr", "", "nameAr"}, {"nameAr", "Latin", "nameAr"}, {"vatNo", "", "vatNo"}, {"crNo", "", "crNo"},
		{"email", "", "email"}, {"phone", "", "phone"}, {"phone", "12", "phone"},
		{"address.buildingNo", "12", "address.buildingNo"}, {"address.streetEn", "", "address.streetEn"},
		{"address.streetAr", "Olaya", "address.streetAr"}, {"address.districtEn", "", "address.districtEn"},
		{"address.districtAr", "", "address.districtAr"}, {"address.cityEn", "", "address.cityEn"},
		{"address.postalCode", "1234", "address.postalCode"},
	}
	for _, c := range cases {
		b := cloneM(ok)
		parts := splitPath(c.path)
		cur := b
		for _, p := range parts[:len(parts)-1] {
			cur = cur[p].(M)
		}
		cur[parts[len(parts)-1]] = c.value
		if e := storeCreateErrors(b); e[c.field] == "" {
			t.Errorf("%s=%q: expected error on %s, got %v", c.path, c.value, c.field, e)
		}
	}
}

func TestSignup_BusinessCategory(t *testing.T) {
	b := validSignup()
	b["company"].(M)["type"] = "Coffee Shop"
	if e := ValidateSignup(b); len(e) != 0 {
		t.Fatalf("category type rejected: %v", e)
	}
	b["company"].(M)["type"] = "coffee shop"
	if e := ValidateSignup(b); len(e) != 0 {
		t.Fatalf("lower-case category rejected: %v", e)
	}
	b["company"].(M)["type"] = "Café"
	if e := ValidateSignup(b); e["company.type"] == "" {
		t.Fatal("unknown category accepted")
	}
	// legacy types still accepted (older clients)
	b["company"].(M)["type"] = "retail"
	if e := ValidateSignup(b); len(e) != 0 {
		t.Fatalf("legacy type rejected: %v", e)
	}
	if signupCategory("coffee shop") != "Coffee Shop" || signupCategory("retail") != "retail" {
		t.Fatal("signupCategory")
	}
}

func TestValidatePosFields(t *testing.T) {
	cases := []struct {
		rec   M
		field string
	}{
		{M{"posTerminal": "grocery", "posSection": "dairy", "posKey": "g6"}, ""},
		{M{"posTerminal": "tailorin"}, ""},
		{M{"posTerminal": ""}, ""},
		{M{"posTerminal": nil}, ""},
		{M{"posTerminal": "nope"}, "posTerminal"},
		{M{"posSection": "a b"}, "posSection"},
		{M{"posSection": strings.Repeat("a", 41)}, "posSection"},
		{M{"posKey": "<script>"}, "posKey"},
	}
	for _, c := range cases {
		e := map[string]string{}
		validatePosFields(c.rec, e)
		if c.field == "" && len(e) != 0 {
			t.Errorf("%v: unexpected %v", c.rec, e)
		}
		if c.field != "" && e[c.field] == "" {
			t.Errorf("%v: expected error on %s", c.rec, c.field)
		}
	}
	if e := productValidate(newMapCtx(nil, ""), M{"nameEn": "Milk", "posTerminal": "zzz"}, nil); e["posTerminal"] == "" {
		t.Fatal("productValidate must check POS fields")
	}
}

const gccErr = "choose a supported country: Saudi Arabia, UAE, Oman, Qatar, Bahrain, Kuwait or India"

func TestStoreValidate_Country(t *testing.T) {
	x := newMapCtx(nil, "")
	tests := []struct {
		name string
		code interface{}
		err  string
	}{
		{"Saudi Arabia", "SA", ""},
		{"lower case", "ae", ""},
		{"Oman", "OM", ""},
		{"Qatar", "QA", ""},
		{"Bahrain", "BH", ""},
		{"Kuwait", "KW", ""},
		{"United Kingdom (not GCC)", "GB", gccErr},
		{"India", "IN", ""},
		{"India lower case", "in", ""},
		{"not sent", nil, ""},
		{"empty", "", ""},
		{"unknown code", "ZZ", gccErr},
		{"a name, not a code", "Saudi Arabia", gccErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := M{"nameEn": "A", "category": "Grocery"}
			if tc.code != nil {
				rec["countryCode"] = tc.code
			}
			if e := storeValidate(x, rec, nil); e["countryCode"] != tc.err {
				t.Fatalf("countryCode error %q, want %q", e["countryCode"], tc.err)
			}
		})
	}
}

func TestStoreToLegacy_Country(t *testing.T) {
	x := newMapCtx(nil, "")
	p, err := storeToLegacy(x, M{"countryCode": " ae ", "address": M{"countryEn": "United Arab Emirates"}}, M{},
		map[string]bool{"countryCode": true, "address": true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if p["country_code"] != "AE" || p["country_name"] != "United Arab Emirates" {
		t.Errorf("country: %v / %v", p["country_code"], p["country_name"])
	}
	// the zone the store then reports follows the new country
	if got := storeTimezoneName(M{"country_code": p["country_code"]}); got != "Asia/Dubai" {
		t.Errorf("timezone %q, want Asia/Dubai", got)
	}
	// unchanged country: nothing written
	p, _ = storeToLegacy(x, M{"countryCode": "AE"}, M{}, map[string]bool{}, false)
	if _, ok := p["country_code"]; ok {
		t.Errorf("unchanged country written: %v", p)
	}
}
