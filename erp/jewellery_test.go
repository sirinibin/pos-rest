package erp

import (
	"math"
	"testing"
)

// The Jewellery business category: its terminal, the product `jewel` details
// and the starter catalog's weight-priced pieces, converted making charges and
// country tax rates (India 3% GST on jewellery and 5% on job work; investment
// gold zero-rated in the Gulf).

func TestJewellery_Category(t *testing.T) {
	if got := CategoryTerminal("Jewellery"); got != "jewellery" {
		t.Fatalf("CategoryTerminal(Jewellery) = %q", got)
	}
	if got := CategoryTerminal("jewellery"); got != "jewellery" {
		t.Errorf("case-insensitive: %q", got)
	}
	if !ValidZatcaCategory("Jewellery") {
		t.Error("Jewellery must be ZATCA-safe")
	}
	if !posTerminals["jewellery"] {
		t.Error("jewellery is not a known POS terminal")
	}
	for _, cc := range []string{"SA", "AE", "OM", "QA", "BH", "KW", "IN", ""} {
		if !CategoryAllowedIn("Jewellery", cc) {
			t.Errorf("Jewellery should be open in %q", cc)
		}
	}
}

func TestValidateJewel(t *testing.T) {
	ok := func() M {
		return M{"metal": "gold", "purity": "22K", "pricing": "weight", "gross": 12.35, "stone": 0.4, "net": 11.95,
			"makingType": "gram", "making": 25.0, "wastage": 8.0, "stoneValue": 350.0, "huid": "AB12C3", "hallmark": true}
	}
	with := func(k string, v interface{}) M {
		m := ok()
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name  string
		jewel interface{}
		field string // "" = valid
	}{
		{"valid 22K piece", ok(), ""},
		{"no jewel object", nil, ""},
		{"not an object", "gold", "jewel"},
		{"unknown metal", with("metal", "copper"), "jewel.metal"},
		{"21K gold", with("purity", "21K"), ""},
		{"gold with silver purity", with("purity", "925"), "jewel.purity"},
		{"silver 925", M{"metal": "silver", "purity": "925", "gross": 30.0}, ""},
		{"platinum 950", M{"metal": "platinum", "purity": "950", "gross": 5.0}, ""},
		{"bad pricing", with("pricing", "auction"), "jewel.pricing"},
		{"weight pricing needs gross", with("gross", nil), "jewel.gross"},
		{"zero gross", with("gross", 0.0), "jewel.gross"},
		{"negative gross", with("gross", -1.0), "jewel.gross"},
		{"text weight", with("gross", "12"), "jewel.gross"},
		{"huge weight", with("gross", 200000.0), "jewel.gross"},
		{"fixed pricing without weight", M{"metal": "gold", "purity": "24K", "pricing": "fixed"}, ""},
		{"stones heavier than piece", with("stone", 20.0), "jewel.stone"},
		{"net over gross", with("net", 13.0), "jewel.net"},
		{"bad making type", with("makingType", "hour"), "jewel.makingType"},
		{"negative making", with("making", -5.0), "jewel.making"},
		{"percent making over 100", M{"metal": "gold", "purity": "18K", "gross": 3.0, "makingType": "percent", "making": 120.0}, "jewel.making"},
		{"percent making 12", M{"metal": "gold", "purity": "18K", "gross": 3.0, "makingType": "percent", "making": 12.0}, ""},
		{"wastage over 50", with("wastage", 55.0), "jewel.wastage"},
		{"negative stone value", with("stoneValue", -1.0), "jewel.stoneValue"},
		{"lowercase HUID", with("huid", "ab12c3"), "jewel.huid"},
		{"short HUID", with("huid", "AB12"), "jewel.huid"},
		{"empty HUID", with("huid", ""), ""},
		{"hallmark not bool", with("hallmark", "yes"), "jewel.hallmark"},
		{"long stone note", with("stoneNote", string(make([]rune, 81))), "jewel.stoneNote"},
		{"int weight (decoded)", with("gross", 12), ""},
	}
	for _, tc := range tests {
		e := map[string]string{}
		rec := M{}
		if tc.jewel != nil {
			rec["jewel"] = tc.jewel
		}
		validateJewel(rec, e)
		if tc.field == "" && len(e) > 0 {
			t.Errorf("%s: unexpected errors %v", tc.name, e)
		}
		if tc.field != "" && e[tc.field] == "" {
			t.Errorf("%s: want error on %s, got %v", tc.name, tc.field, e)
		}
	}
	// products run it through validatePosFields
	e := map[string]string{}
	validatePosFields(M{"posTerminal": "jewellery", "jewel": with("purity", "9K")}, e)
	if e["jewel.purity"] == "" {
		t.Errorf("validatePosFields should check jewel: %v", e)
	}
}

func TestJewellery_StarterPlan(t *testing.T) {
	sa, ok := planStarterCatalog("jewellery", "SA")
	if !ok {
		t.Fatal("no jewellery starter catalog")
	}
	if sa.Category != "Jewellery" || sa.VatInclusive {
		t.Errorf("category %q vatInclusive %v (jewellery is priced before tax)", sa.Category, sa.VatInclusive)
	}
	ring := planItem(sa, "jw-ring-21")
	if ring == nil || ring.Jewel == nil || str(ring.Jewel["purity"]) != "21K" || num(ring.Jewel["making"]) != 18 {
		t.Fatalf("SA 21K ring: %+v", ring)
	}
	if ring.Tax != nil {
		t.Errorf("SA jewellery uses the store's VAT, got %v", *ring.Tax)
	}
	if bar := planItem(sa, "cn-5g"); bar == nil || bar.Tax == nil || *bar.Tax != 0 {
		t.Errorf("SA investment gold bar must be zero-rated: %+v", bar)
	}
	if sov := planItem(sa, "cn-8g"); sov == nil || sov.Tax != nil {
		t.Errorf("22K coin is not investment gold: %+v", sov)
	}
	if planItem(sa, "in-ring-22") != nil {
		t.Error("India-only pieces must not reach a Saudi store")
	}
	if planItem(sa, "og-21k") == nil || planItem(sa, "og-22k") == nil {
		t.Error("Saudi store needs old gold 21K and 22K")
	}

	kw, _ := planStarterCatalog("jewellery", "KW")
	if r := planItem(kw, "jw-ring-21"); r == nil || math.Abs(num(r.Jewel["making"])-1.44) > 1e-9 {
		t.Errorf("KWD making 18 × 0.08: %+v", r)
	}
	if d := planItem(kw, "dm-ring"); d == nil || math.Abs(num(d.Jewel["stoneValue"])-256) > 1e-9 {
		t.Errorf("KWD stone value 3200 × 0.08: %+v", d)
	}
	// the source data is untouched by a country's conversion
	again, _ := planStarterCatalog("jewellery", "SA")
	if r := planItem(again, "jw-ring-21"); num(r.Jewel["making"]) != 18 {
		t.Errorf("conversion leaked into the catalog: %v", r.Jewel["making"])
	}

	in, _ := planStarterCatalog("jewellery", "IN")
	if planItem(in, "jw-ring-21") != nil || planItem(in, "og-21k") != nil {
		t.Error("21K Gulf pieces must not reach an Indian store")
	}
	r := planItem(in, "in-ring-22")
	if r == nil || r.Tax == nil || *r.Tax != 3 || num(r.Jewel["making"]) != 350 {
		t.Fatalf("India 22K ring: 3%% GST, ₹350/g making: %+v", r)
	}
	if bar := planItem(in, "cn-5g"); bar == nil || bar.Tax == nil || *bar.Tax != 3 {
		t.Errorf("India gold bar: 3%% GST: %+v", bar)
	}
	if rp := planItem(in, "rp-resize"); rp == nil || rp.Tax == nil || *rp.Tax != 5 || !rp.Service {
		t.Errorf("India repair job work: 5%% GST: %+v", rp)
	}
	if d := planItem(in, "dm-ring"); d == nil || num(d.Jewel["stoneValue"]) != 48000 {
		t.Errorf("India stone value 3200 × 15: %+v", d)
	}
}

func TestJewellery_StarterProductRecord(t *testing.T) {
	in, _ := planStarterCatalog("jewellery", "IN")
	r := starterProductRecord("jewellery", in.VatInclusive, 18, *planItem(in, "in-ring-22"), "c1", "", nil)
	if r["vatPercent"] != 3.0 {
		t.Errorf("product tax 3%%, got %v", r["vatPercent"])
	}
	j, _ := r["jewel"].(M)
	if j == nil || str(j["purity"]) != "22K" || num(j["net"]) != 3.85 {
		t.Errorf("jewel details on the product: %v", r["jewel"])
	}
	if num(get(r, "pricing.retail")) != 0 {
		t.Errorf("weight-priced piece has no list price: %v", r["pricing"])
	}
	e := map[string]string{}
	validatePosFields(r, e)
	if len(e) > 0 {
		t.Errorf("seeded product must validate: %v", e)
	}
	sa, _ := planStarterCatalog("jewellery", "SA")
	if rr := starterProductRecord("jewellery", false, 15, *planItem(sa, "rp-polish"), "", "", nil); rr["vatPercent"] != 15.0 || rr["jewel"] != nil {
		t.Errorf("Saudi service: store VAT, no jewel: %v %v", rr["vatPercent"], rr["jewel"])
	}
}
