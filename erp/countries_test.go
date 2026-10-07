package erp

import (
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

// GCC store countries: profiles, store contract, validation, sign-up, ZATCA.

func TestCountryProfiles_GCC(t *testing.T) {
	want := []struct {
		code, cur string
		dec       int
		vat       float64
		hasVAT    bool
		zatca     bool
		tz        string
	}{
		{"SA", "SAR", 2, 15, true, true, "Asia/Riyadh"},
		{"AE", "AED", 2, 5, true, false, "Asia/Dubai"},
		{"OM", "OMR", 3, 5, true, false, "Asia/Muscat"},
		{"QA", "QAR", 2, 0, false, false, "Asia/Qatar"},
		{"BH", "BHD", 3, 10, true, false, "Asia/Bahrain"},
		{"KW", "KWD", 3, 0, false, false, "Asia/Kuwait"},
	}
	if len(models.GCCCountries) != len(want) {
		t.Fatalf("%d countries, want %d", len(models.GCCCountries), len(want))
	}
	for i, w := range want {
		p := models.CountryProfileFor(w.code)
		if p == nil || models.GCCCountries[i].Code != w.code {
			t.Fatalf("%s: missing or out of order", w.code)
		}
		if p.CurrencyCode != w.cur || p.Decimals != w.dec || p.VatPercent != w.vat || p.HasVAT != w.hasVAT ||
			models.ZatcaApplies(w.code) != w.zatca || p.TimeZone != w.tz {
			t.Errorf("%s profile: %+v", w.code, p)
		}
		// the zone must be the one the rest of the backend uses
		if models.TimezoneMap[w.code] != w.tz {
			t.Errorf("%s: TimezoneMap %q, profile %q", w.code, models.TimezoneMap[w.code], w.tz)
		}
		if _, err := time.LoadLocation(p.TimeZone); err != nil {
			t.Errorf("%s: %v", w.code, err)
		}
		if p.NameAr == "" || p.CurrencySymbolAr == "" || p.FractionAr == "" || p.InvoiceTitleAr == "" {
			t.Errorf("%s: Arabic labels missing", w.code)
		}
	}
	if models.CountryProfileFor("ae") == nil || models.CountryProfileFor(" kw ") == nil {
		t.Error("lookup must ignore case and spaces")
	}
	for _, c := range []string{"", "IN", "GB", "ZZ"} {
		if models.CountryProfileFor(c) != nil {
			t.Errorf("%q is not GCC", c)
		}
		if models.CountryProfileOrSaudi(c).Code != "SA" || !models.ZatcaApplies(c) {
			t.Errorf("%q must keep the Saudi rules", c)
		}
	}
}

func TestCountryProfiles_Formats(t *testing.T) {
	tests := []struct {
		code, kind, v string
		ok            bool
	}{
		{"SA", "tax", "310122393500003", true},
		{"SA", "tax", "210122393500003", false},
		{"AE", "tax", "100123456700003", true},
		{"AE", "tax", "10012345670000", false},
		{"AE", "tax", "10012345670000A", false},
		{"OM", "tax", "OM1100012345", true},
		{"OM", "tax", "om1100012345", true},
		{"OM", "tax", "1100012345", false},
		{"BH", "tax", "200000898300002", true},
		{"BH", "tax", "2000008983", false},
		{"QA", "tax", "QA-12345", true},
		{"KW", "tax", "12345", true},
		{"SA", "mobile", "0512345678", true},
		{"SA", "mobile", "+966512345678", true},
		{"SA", "mobile", "0412345678", false},
		{"AE", "mobile", "0501234567", true},
		{"AE", "mobile", "+971 50 123 4567", true},
		{"AE", "mobile", "0412345678", false},
		{"OM", "mobile", "92123456", true},
		{"OM", "mobile", "+96872123456", true},
		{"OM", "mobile", "22123456", false},
		{"QA", "mobile", "55123456", true},
		{"QA", "mobile", "+974 3312 3456", true},
		{"QA", "mobile", "44123456", false},
		{"BH", "mobile", "36123456", true},
		{"BH", "mobile", "+97339123456", true},
		{"BH", "mobile", "17123456", false},
		{"KW", "mobile", "99123456", true},
		{"KW", "mobile", "+965 6512 3456", true},
		{"KW", "mobile", "٩٩١٢٣٤٥٦", true},
		{"KW", "mobile", "79123456", false},
		{"AE", "phone", "042345678", true},
		{"QA", "phone", "44123456", true},
		{"BH", "phone", "17123456", true},
		{"SA", "cr", "1010101010", true},
		{"BH", "cr", "12345-1", true},
		{"AE", "cr", "CN-1234567", true},
		{"AE", "cr", "", false},
		{"AE", "cr", "has space", false},
		{"AE", "postal", "", true},
		{"OM", "postal", "112", true},
		{"SA", "postal", "1234", false},
	}
	for _, tc := range tests {
		p := models.CountryProfileFor(tc.code)
		var got bool
		switch tc.kind {
		case "tax":
			got = validTaxIDFor(p, tc.v)
		case "mobile":
			got = validMobileFor(p, tc.v)
		case "phone":
			got = validPhoneFor(p, tc.v)
		case "cr":
			got = p.ValidCR(tc.v)
		case "postal":
			got = p.ValidPostal(tc.v)
		}
		if got != tc.ok {
			t.Errorf("%s %s %q = %v, want %v", tc.code, tc.kind, tc.v, got, tc.ok)
		}
	}
	if !validAnyGCCPhone("0512345678") || !validAnyGCCPhone("+96599123456") || validAnyGCCPhone("123") {
		t.Error("validAnyGCCPhone")
	}
}

func TestStoreVatPercent_ByCountry(t *testing.T) {
	tests := []struct {
		store M
		want  float64
	}{
		{M{}, 15},
		{M{"country_code": "SA", "vat_percent": 15.0}, 15},
		{M{"country_code": "AE"}, 5},
		{M{"country_code": "AE", "vat_percent": 5.0}, 5},
		{M{"country_code": "BH"}, 10},
		{M{"country_code": "OM"}, 5},
		// no VAT in Qatar and Kuwait, whatever an older record holds
		{M{"country_code": "QA", "vat_percent": 15.0}, 0},
		{M{"country_code": "KW"}, 0},
		// legacy non-GCC store keeps its rate
		{M{"country_code": "IN", "vat_percent": 18.0}, 18},
	}
	for _, tc := range tests {
		if got := storeVatPercent(tc.store); got != tc.want {
			t.Errorf("%v: %v, want %v", tc.store, got, tc.want)
		}
		if got := newMapCtxForStore(tc.store).vatPercent(); got != tc.want {
			t.Errorf("mapCtx %v: %v, want %v", tc.store, got, tc.want)
		}
	}
	if (&mapCtx{}).vatPercent() != 15 {
		t.Error("no store: 15")
	}
}

func newMapCtxForStore(st M) *mapCtx { return &mapCtx{store: st, cache: map[string]M{}} }

func withNoDocs(t *testing.T, has bool) {
	old := storeHasZatcaDocs
	storeHasZatcaDocs = func(M) bool { return has }
	t.Cleanup(func() { storeHasZatcaDocs = old })
}

func TestStoreToContract_Country(t *testing.T) {
	withNoDocs(t, false)
	tests := []struct {
		code, cur, sym string
		dec            int
		vat            float64
		zatca, gcc     bool
		countryEn      string
		countryAr      string
	}{
		{"", "SAR", "ر.س", 2, 15, true, false, "Saudi Arabia", "المملكة العربية السعودية"},
		{"SA", "SAR", "ر.س", 2, 15, true, true, "Saudi Arabia", "المملكة العربية السعودية"},
		{"AE", "AED", "د.إ", 2, 5, false, true, "United Arab Emirates", "الإمارات العربية المتحدة"},
		{"OM", "OMR", "ر.ع.", 3, 5, false, true, "Oman", "سلطنة عُمان"},
		{"QA", "QAR", "ر.ق", 2, 0, false, true, "Qatar", "قطر"},
		{"BH", "BHD", "د.ب", 3, 10, false, true, "Bahrain", "البحرين"},
		{"KW", "KWD", "د.ك", 3, 0, false, true, "Kuwait", "الكويت"},
	}
	for _, tc := range tests {
		d := M{"_id": "x", "name": "S", "country_code": tc.code, "country_name": "whatever"}
		if tc.code == "" {
			d["country_name"] = ""
		}
		rec := storeToContract(newMapCtx(nil, ""), d)
		cur := sub(rec, "currency")
		c := sub(rec, "country")
		if cur["code"] != tc.cur || cur["symbolAr"] != tc.sym || cur["decimals"] != tc.dec {
			t.Errorf("%s currency %v", tc.code, cur)
		}
		if rec["vatPercent"] != tc.vat {
			t.Errorf("%s vatPercent %v", tc.code, rec["vatPercent"])
		}
		if c["zatca"] != tc.zatca || c["gcc"] != tc.gcc || c["locked"] != false || c["hasVat"] != (tc.vat > 0) {
			t.Errorf("%s country %v", tc.code, c)
		}
		if get(rec, "address.countryEn") != tc.countryEn || get(rec, "address.countryAr") != tc.countryAr {
			t.Errorf("%s address country %v / %v", tc.code, get(rec, "address.countryEn"), get(rec, "address.countryAr"))
		}
	}
	withNoDocs(t, true)
	rec := storeToContract(newMapCtx(nil, ""), M{"_id": "x", "country_code": "AE"})
	if get(rec, "country.locked") != true || get(rec, "zatca.envLocked") != true {
		t.Errorf("locked: %v / %v", get(rec, "country.locked"), get(rec, "zatca.envLocked"))
	}
	if get(rec, "country.taxIdLabelEn") != "TRN" || get(rec, "country.invoiceTitleEn") != "Tax Invoice" {
		t.Errorf("UAE labels %v", rec["country"])
	}
	rec = storeToContract(newMapCtx(nil, ""), M{"_id": "x", "country_code": "KW"})
	if get(rec, "country.invoiceTitleEn") != "Invoice" || get(rec, "country.taxIdRequired") != false {
		t.Errorf("Kuwait labels %v", rec["country"])
	}
}

func TestStoreValidate_CountryRules(t *testing.T) {
	withNoDocs(t, false)
	x := newMapCtx(nil, "")
	base := func(code string, extra M) M {
		r := M{"nameEn": "A", "category": "Grocery", "countryCode": code}
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
		{"SA VAT 3…3", base("SA", M{"vatNo": "310122393500003"}), ""},
		{"SA bad VAT", base("SA", M{"vatNo": "100123456700003"}), "vatNo"},
		{"UAE TRN", base("AE", M{"vatNo": "100123456700003"}), ""},
		{"UAE short TRN", base("AE", M{"vatNo": "1001234"}), "vatNo"},
		{"Oman VATIN", base("OM", M{"vatNo": "OM1100012345"}), ""},
		{"Oman digits only", base("OM", M{"vatNo": "1100012345"}), "vatNo"},
		{"Bahrain 15 digits", base("BH", M{"vatNo": "200000898300002"}), ""},
		{"SA CR 10 digits", base("SA", M{"crNo": "123"}), "crNo"},
		{"Bahrain CR with dash", base("BH", M{"crNo": "12345-1"}), ""},
		{"UAE licence", base("AE", M{"crNo": "CN-1234567"}), ""},
		{"SA building 4 digits", base("SA", M{"address": M{"buildingNo": "12"}}), "address.buildingNo"},
		{"UAE building free text", base("AE", M{"address": M{"buildingNo": "Tower 2"}}), ""},
		{"Oman 3-digit postal", base("OM", M{"address": M{"postalCode": "112"}}), ""},
		{"SA 3-digit postal", base("SA", M{"address": M{"postalCode": "112"}}), "address.postalCode"},
		{"Kuwait 0% VAT", base("KW", M{"vatPercent": 0.0}), ""},
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
	// the store's saved country applies when the PATCH does not send one
	if e := storeValidate(x, M{"nameEn": "A", "category": "Grocery", "vatNo": "100123456700003"}, M{"country_code": "AE", "business_category": "Grocery", "category": "Grocery"}); e["vatNo"] != "" {
		t.Errorf("saved UAE country ignored: %v", e)
	}
}

func TestStoreValidate_CountryLockedOnceDocumentsExist(t *testing.T) {
	x := newMapCtx(nil, "")
	prev := M{"_id": "x", "country_code": "SA", "business_category": "Grocery"}
	withNoDocs(t, true)
	if e := storeValidate(x, M{"nameEn": "A", "category": "Grocery", "countryCode": "AE"}, prev); e["countryCode"] == "" {
		t.Fatal("country change with invoices must be refused")
	}
	// same country is not a change
	if e := storeValidate(x, M{"nameEn": "A", "category": "Grocery", "countryCode": "sa"}, prev); e["countryCode"] != "" {
		t.Fatalf("unchanged country refused: %v", e)
	}
	// an empty legacy country becoming SA is not a change either
	if e := storeValidate(x, M{"nameEn": "A", "category": "Grocery", "countryCode": "SA"}, M{"_id": "x", "business_category": "Grocery"}); e["countryCode"] != "" {
		t.Fatalf("SA on legacy store refused: %v", e)
	}
	withNoDocs(t, false)
	if e := storeValidate(x, M{"nameEn": "A", "category": "Grocery", "countryCode": "AE"}, prev); e["countryCode"] != "" {
		t.Fatalf("country change without invoices refused: %v", e)
	}
}

func TestStoreToLegacy_CountryBringsVAT(t *testing.T) {
	x := newMapCtx(nil, "")
	tests := []struct {
		rec  M
		ch   map[string]bool
		name string
		vat  float64
	}{
		{M{"countryCode": "AE"}, map[string]bool{"countryCode": true}, "United Arab Emirates", 5},
		{M{"countryCode": "BH"}, map[string]bool{"countryCode": true}, "Bahrain", 10},
		{M{"countryCode": "KW"}, map[string]bool{"countryCode": true}, "Kuwait", 0},
		// a rate edited in the same save wins where the country has VAT
		{M{"countryCode": "AE", "vatPercent": 7.0}, map[string]bool{"countryCode": true, "vatPercent": true}, "United Arab Emirates", 7},
		// …but never in a country without VAT
		{M{"countryCode": "QA", "vatPercent": 7.0}, map[string]bool{"countryCode": true, "vatPercent": true}, "Qatar", 0},
	}
	for _, tc := range tests {
		p, err := storeToLegacy(x, tc.rec, M{}, tc.ch, false)
		if err != nil {
			t.Fatal(err)
		}
		if p["country_name"] != tc.name || p["vat_percent"] != tc.vat {
			t.Errorf("%v: %v / %v", tc.rec, p["country_name"], p["vat_percent"])
		}
	}
}

func gccSignup(code string) M {
	b := validSignup()
	c := b["company"].(M)
	c["countryCode"] = code
	o := b["owner"].(M)
	addr := M{"streetEn": "Sheikh Zayed Rd", "cityEn": "Dubai"}
	switch code {
	case "AE":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "0501234567", "042345678", "100123456700003", "CN-1234567"
	case "OM":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "92123456", "24123456", "OM1100012345", "1234567"
		addr["postalCode"] = "112"
	case "QA":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "55123456", "44123456", "", "123456"
	case "BH":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "36123456", "17123456", "200000898300002", "12345-1"
	case "KW":
		o["mobile"], c["mobile"], c["vatNo"], c["crNo"] = "99123456", "22123456", "", "123456"
	}
	c["address"] = addr
	return b
}

func TestValidateSignup_GCCCountries(t *testing.T) {
	for _, code := range []string{"AE", "OM", "QA", "BH", "KW"} {
		if e := ValidateSignup(gccSignup(code)); len(e) != 0 {
			t.Errorf("%s valid sign-up rejected: %v", code, e)
		}
	}
	// Saudi rules still apply to Saudi (and an omitted country)
	sa := validSignup()
	sa["company"].(M)["countryCode"] = "SA"
	if e := ValidateSignup(sa); len(e) != 0 {
		t.Errorf("SA sign-up rejected: %v", e)
	}
	tests := []struct {
		name  string
		code  string
		mut   func(M)
		field string
	}{
		{"unknown country", "AE", func(b M) { b["company"].(M)["countryCode"] = "GB" }, "company.countryCode"},
		{"Saudi mobile in UAE", "AE", func(b M) { b["owner"].(M)["mobile"] = "0412345678" }, "owner.mobile"},
		{"bad TRN", "AE", func(b M) { b["company"].(M)["vatNo"] = "310" }, "company.vatNo"},
		{"Oman VATIN without OM", "OM", func(b M) { b["company"].(M)["vatNo"] = "1100012345" }, "company.vatNo"},
		{"street required", "BH", func(b M) { b["company"].(M)["address"].(M)["streetEn"] = "" }, "company.address.streetEn"},
		{"city required", "KW", func(b M) { delete(b["company"].(M)["address"].(M), "cityEn") }, "company.address.cityEn"},
		{"CR required", "QA", func(b M) { b["company"].(M)["crNo"] = "" }, "company.crNo"},
		{"bad store phone", "QA", func(b M) { b["company"].(M)["mobile"] = "123" }, "company.mobile"},
		{"plan still checked", "KW", func(b M) { b["company"].(M)["plan"] = "gold" }, "company.plan"},
	}
	for _, tc := range tests {
		b := gccSignup(tc.code)
		tc.mut(b)
		if e := ValidateSignup(b); e[tc.field] == "" {
			t.Errorf("%s: expected error on %s, got %v", tc.name, tc.field, e)
		}
	}
	// VAT number is required in Saudi Arabia only
	b := validSignup()
	b["company"].(M)["vatNo"] = ""
	if e := ValidateSignup(b); e["company.vatNo"] == "" {
		t.Error("SA VAT number must be required")
	}
	b = gccSignup("AE")
	b["company"].(M)["vatNo"] = ""
	if e := ValidateSignup(b); e["company.vatNo"] != "" {
		t.Errorf("UAE VAT number must be optional: %v", e)
	}
}

func TestCountriesEndpoint(t *testing.T) {
	r := call(t, "GET", "/countries", "", nil)
	if r.Code != 200 {
		t.Fatalf("GET /countries: %d %s", r.Code, r.Raw)
	}
	d := r.data()
	if len(d) != 6 {
		t.Fatalf("%d countries", len(d))
	}
	first, _ := d[0].(M)
	if first["code"] != "SA" || get(first, "currency.code") != "SAR" || first["zatca"] != true {
		t.Errorf("first: %v", first)
	}
	last, _ := d[5].(M)
	if last["code"] != "KW" || get(last, "currency.decimals") != 3.0 || last["hasVat"] != false {
		t.Errorf("last: %v", last)
	}
}
