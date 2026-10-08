package erp

import (
	"math"
	"regexp"
	"testing"
)

// Starter catalogs (seeddata/starter_catalogs.json): one per business
// category, bilingual, consistent references, valid product records.

var reStarterArabic = regexp.MustCompile(`[\x{0600}-\x{06FF}]`)

func TestStarterCatalogs_EveryBusinessCategory(t *testing.T) {
	d, err := loadStarterCatalogs()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(d.Terminals) != len(BusinessCategories) {
		t.Errorf("terminals %d, business categories %d", len(d.Terminals), len(BusinessCategories))
	}
	for _, bc := range BusinessCategories {
		tc := d.Terminals[bc.Terminal]
		if tc == nil {
			t.Errorf("%s (%s): no starter catalog", bc.Value, bc.Terminal)
			continue
		}
		if tc.Category != bc.Value {
			t.Errorf("%s: category %q, want %q", bc.Terminal, tc.Category, bc.Value)
		}
		if len(tc.Categories) < 2 || len(tc.Items) < 20 {
			t.Errorf("%s: %d categories, %d items (too few)", bc.Terminal, len(tc.Categories), len(tc.Items))
		}
	}
}

func TestStarterCatalogs_DataIsConsistent(t *testing.T) {
	d, _ := loadStarterCatalogs()
	for term, tc := range d.Terminals {
		cats := map[string]bool{}
		for _, c := range tc.Categories {
			if c.Key == "" || c.NameEn == "" || !reStarterArabic.MatchString(c.NameAr) {
				t.Errorf("%s: category %+v needs key, English and Arabic names", term, c)
			}
			if cats[c.Key] {
				t.Errorf("%s: duplicate category %s", term, c.Key)
			}
			cats[c.Key] = true
		}
		brands := map[string]bool{}
		for _, b := range tc.Brands {
			if b.Name == "" || brands[b.Name] {
				t.Errorf("%s: empty or duplicate brand %q", term, b.Name)
			}
			if b.NameAr != "" && !reStarterArabic.MatchString(b.NameAr) {
				t.Errorf("%s: brand %s Arabic name %q", term, b.Name, b.NameAr)
			}
			brands[b.Name] = true
		}
		specs := map[string]bool{}
		for _, s := range tc.Specs {
			if !validSpecKind(s.Kind) || s.Name == "" || len([]rune(s.Name)) > maxSpecName || s.NameAr == "" {
				t.Errorf("%s: bad spec option %+v", term, s)
			}
			// the option is valid for the productSpecs resource
			if e := productSpecValidate(nil, M{"kind": s.Kind, "name": s.Name, "nameAr": s.NameAr}, nil); len(e) > 0 {
				t.Errorf("%s: spec %+v rejected: %v", term, s, e)
			}
			specs[s.Kind+"|"+s.Name] = true
		}
		keys := map[string]bool{}
		for _, it := range tc.Items {
			if keys[it.Key] {
				t.Errorf("%s: duplicate item key %s", term, it.Key)
			}
			keys[it.Key] = true
			if !rePosToken.MatchString(it.Key) || !rePosToken.MatchString(it.Section) {
				t.Errorf("%s: %s posKey/posSection not valid tokens", term, it.Key)
			}
			if !cats[it.Section] {
				t.Errorf("%s: %s in unknown category %s", term, it.Key, it.Section)
			}
			if len([]rune(it.NameEn)) < 3 || !reStarterArabic.MatchString(it.NameAr) {
				t.Errorf("%s: %s needs an English (3+) and Arabic name: %q / %q", term, it.Key, it.NameEn, it.NameAr)
			}
			if it.Price <= 0 {
				t.Errorf("%s: %s price %v", term, it.Key, it.Price)
			}
			if it.Brand != "" && !brands[it.Brand] {
				t.Errorf("%s: %s brand %s not in the brand list", term, it.Key, it.Brand)
			}
			for _, b := range it.BrandBy {
				if !brands[b] {
					t.Errorf("%s: %s country brand %s not in the brand list", term, it.Key, b)
				}
			}
			for k, v := range it.Specs {
				if !specs[k+"|"+v] {
					t.Errorf("%s: %s spec %s=%s not in the options", term, it.Key, k, v)
				}
			}
			for _, cc := range it.Countries {
				if _, ok := starterPriceFactor[cc]; !ok {
					t.Errorf("%s: %s unknown country %s", term, it.Key, cc)
				}
			}
		}
	}
}

func TestPlanStarterCatalog_Countries(t *testing.T) {
	cases := []struct {
		terminal, cc string
		check        func(t *testing.T, p *StarterPlan)
	}{
		{"grocery", "SA", func(t *testing.T, p *StarterPlan) {
			it := planItem(p, "g13") // water
			if it == nil || it.Brand != "Nova" || it.Price != 1.5 {
				t.Errorf("SA water: %+v", it)
			}
			if !planHasBrand(p, "Nova") || planHasBrand(p, "Al Ain") {
				t.Errorf("SA brands should list Nova only: %+v", p.Brands)
			}
		}},
		{"grocery", "AE", func(t *testing.T, p *StarterPlan) {
			if it := planItem(p, "g13"); it == nil || it.Brand != "Al Ain" {
				t.Errorf("UAE water brand: %+v", it)
			}
			if it := planItem(p, "g6"); it == nil || it.Brand != "Al Rawabi" {
				t.Errorf("UAE milk brand: %+v", it)
			}
		}},
		{"grocery", "KW", func(t *testing.T, p *StarterPlan) {
			it := planItem(p, "g23") // basmati 52 SAR → KWD × 0.08
			if it == nil || math.Abs(it.Price-4.16) > 0.0001 {
				t.Errorf("KWD price: %+v", it)
			}
		}},
		{"grocery", "OM", func(t *testing.T, p *StarterPlan) {
			it := planItem(p, "g27") // 2.75 SAR → 0.275 OMR (nearest 0.005)
			if it == nil || math.Abs(it.Price-0.275) > 0.0001 {
				t.Errorf("OMR price: %+v", it)
			}
		}},
		{"mobile", "SA", func(t *testing.T, p *StarterPlan) {
			if planItem(p, "sim-stc") == nil || planItem(p, "sim-du-ae") != nil {
				t.Errorf("SA SIMs: stc=%v du=%v", planItem(p, "sim-stc"), planItem(p, "sim-du-ae"))
			}
			it := planItem(p, "i17pm-256GB")
			if it == nil || it.Specs["size"] != "256 GB" || it.Specs["class"] != "Flagship" || it.Brand != "Apple" {
				t.Errorf("iPhone specs: %+v", it)
			}
		}},
		{"mobile", "AE", func(t *testing.T, p *StarterPlan) {
			if planItem(p, "sim-stc") != nil {
				t.Errorf("UAE store got a Saudi SIM")
			}
			it := planItem(p, "sim-du-ae")
			if it == nil || it.Price != 55 || it.Brand != "du" {
				t.Errorf("UAE du SIM (local price): %+v", it)
			}
		}},
		{"construction", "BH", func(t *testing.T, p *StarterPlan) {
			if it := planItem(p, "rb-8"); it == nil || it.Brand != "SULB" || it.NameEn != "Rebar 8 mm" {
				t.Errorf("Bahrain rebar: %+v", it)
			}
			if it := planItem(p, "cem-opc"); it == nil || it.Brand != "" {
				t.Errorf("Bahrain cement has no local brand: %+v", it)
			}
		}},
		{"softwaresa", "SA", func(t *testing.T, p *StarterPlan) {
			if planItem(p, "zatca") == nil || planItem(p, "dae") != nil {
				t.Errorf("SA software: ZATCA item kept, .ae domain absent")
			}
			if it := planItem(p, "hr"); it == nil || it.NameEn != "HR & Payroll (GOSI / Mudad)" {
				t.Errorf("SA HR name: %+v", it)
			}
		}},
		{"softwaresa", "QA", func(t *testing.T, p *StarterPlan) {
			if planItem(p, "zatca") != nil || planItem(p, "dqa") == nil {
				t.Errorf("QA software: no ZATCA, .qa domain")
			}
			if it := planItem(p, "hr"); it == nil || it.NameEn != "HR & Payroll (WPS)" {
				t.Errorf("QA HR name: %+v", it)
			}
		}},
		{"salon", "", func(t *testing.T, p *StarterPlan) {
			c := p.counts()
			if c.Services != len(p.Items) || c.Products != 0 {
				t.Errorf("salon is services only: %+v", c)
			}
		}},
		{"industrial", "SA", func(t *testing.T, p *StarterPlan) {
			it := planItem(p, "in-gv-2")
			if it == nil || it.Specs["size"] != `2"` || it.Specs["class"] != "PN16" || it.Specs["material"] != "Cast iron" {
				t.Errorf("industrial specs: %+v", it)
			}
		}},
	}
	for _, c := range cases {
		p, ok := planStarterCatalog(c.terminal, c.cc)
		if !ok {
			t.Fatalf("%s: no plan", c.terminal)
		}
		c.check(t, p)
	}
	if _, ok := planStarterCatalog("nope", "SA"); ok {
		t.Errorf("unknown terminal should have no plan")
	}
}

func planItem(p *StarterPlan, key string) *starterPlanItem {
	for i := range p.Items {
		if p.Items[i].Key == key {
			return &p.Items[i]
		}
	}
	return nil
}

func planHasBrand(p *StarterPlan, name string) bool {
	for _, b := range p.Brands {
		if b.Name == name {
			return true
		}
	}
	return false
}

func TestStarterPrice(t *testing.T) {
	cases := []struct {
		sar  float64
		cc   string
		dec  int
		want float64
	}{
		{10, "SA", 2, 10}, {9.95, "AE", 2, 9.95}, {10, "QA", 2, 10}, {10, "OM", 3, 1}, {2.75, "BH", 3, 0.275},
		{1, "KW", 3, 0.08}, {13.3, "KW", 3, 1.065}, {5, "XX", 2, 5},
	}
	for _, c := range cases {
		if got := starterPrice(c.sar, c.cc, c.dec); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("starterPrice(%v,%s) = %v, want %v", c.sar, c.cc, got, c.want)
		}
	}
}

func TestStarterProductRecord(t *testing.T) {
	it := starterPlanItem{Key: "g13", Section: "drinks", NameEn: "Water 1.5 L", NameAr: "مياه 1.5 لتر", Unit: "Btl", Price: 1.15}
	rec := starterProductRecord("grocery", true, 15, it, "cat1", "br1", map[string]string{"size": "psp_1"})
	if r := num(sub(rec, "pricing")["retail"]); math.Abs(r-1) > 1e-9 {
		t.Errorf("VAT-inclusive 1.15 at 15%% → retail %v, want 1", r)
	}
	if rec["posTerminal"] != "grocery" || rec["posSection"] != "drinks" || rec["posKey"] != "g13" || rec["brandId"] != "br1" ||
		len(arr(rec["categoryIds"])) != 1 || str(sub(rec, "specs")["size"]) != "psp_1" || rec["isService"] != false {
		t.Errorf("record: %v", rec)
	}
	e := map[string]string{}
	validatePosFields(rec, e)
	if len(e) > 0 {
		t.Errorf("record fails product POS validation: %v", e)
	}
	// ex-VAT terminals and no-VAT countries keep the price; services default their unit
	svc := starterPlanItem{Key: "x", Section: "s", NameEn: "Design visit", NameAr: "زيارة", Price: 500, Service: true}
	r2 := starterProductRecord("interior", false, 15, svc, "", "", nil)
	if num(sub(r2, "pricing")["retail"]) != 500 || r2["unit"] != "Service" || r2["isService"] != true || r2["brandId"] != nil || r2["specs"] != nil {
		t.Errorf("service record: %v", r2)
	}
	r3 := starterProductRecord("grocery", true, 0, it, "", "", nil)
	if num(sub(r3, "pricing")["retail"]) != 1.15 {
		t.Errorf("no-VAT country retail: %v", sub(r3, "pricing")["retail"])
	}
}

func TestCtxScopedTo(t *testing.T) {
	a, b := "64b000000000000000000001", "64b000000000000000000002"
	oa, _ := oidOf(a)
	ob, _ := oidOf(b)
	c := &Ctx{User: M{"store_id": oa, "store_ids": []interface{}{oa, ob}}}
	c.Stores = []M{{"_id": oa}, {"_id": ob}}
	c.storeIdx = map[string]M{a: c.Stores[0], b: c.Stores[1]}
	s := c.scopedTo(b)
	if s.primaryStore() != b || len(s.storeHexes()) != 1 || s.store(a) != nil {
		t.Errorf("scoped ctx: primary=%s stores=%v", s.primaryStore(), s.storeHexes())
	}
	if c.primaryStore() != a || len(c.Stores) != 2 {
		t.Errorf("original ctx changed")
	}
}

func TestUniqueBrandCode(t *testing.T) {
	used := map[string]bool{"ARABIA": true}
	cases := []struct{ name, want string }{
		{"Arabian Pipes", "ARABIA2"}, {"Arabian Oud", "ARABIA3"}, {"3M", "3M"}, {"e&", "E"}, {"!!", "BRAND"},
		{"Al Ain", "ALAIN"}, {"Al-Ain", "ALAIN2"},
	}
	for _, c := range cases {
		if got := uniqueBrandCode(c.name, used); got != c.want {
			t.Errorf("uniqueBrandCode(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
